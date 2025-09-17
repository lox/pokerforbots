package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
)

const (
	colorReset   = "\033[0m"
	colorBold    = "\033[1m"
	colorDim     = "\033[2m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
)

type handState struct {
	id               string
	holeCards        []string
	yourSeat         int
	button           int
	smallBlind       int
	bigBlind         int
	board            []string
	currentStreet    string
	pot              int
	players          []protocol.Player
	initialStacks    []int
	roles            map[int][]string
	foldStreet       map[int]string
	printedHoleCards bool
	printedStreets   map[string]bool
}

type client struct {
	conn              *websocket.Conn
	name              string
	input             *bufio.Reader
	mu                sync.Mutex
	state             *handState
	pendingAction     *protocol.ActionRequest
	closed            bool
	actionPromptLines int
}

func newClient(name string) *client {
	return &client{
		name:  name,
		input: bufio.NewReader(os.Stdin),
	}
}

func (c *client) connect(server string) error {
	u, err := url.Parse(server)
	if err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to dial server: %w", err)
	}

	c.conn = conn

	connectMsg := &protocol.Connect{Type: protocol.TypeConnect, Name: c.name}
	payload, err := protocol.Marshal(connectMsg)
	if err != nil {
		return fmt.Errorf("failed to encode connect message: %w", err)
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		return fmt.Errorf("failed to send connect message: %w", err)
	}

	stdoutf("Connected to %s as %s\n", u.String(), c.name)
	stdoutln("Waiting to be seated...")
	return nil
}

func (c *client) run() error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.readLoop()
	}()

	go c.inputLoop()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	defer signal.Stop(interrupt)

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, websocket.ErrCloseSent) {
			return err
		}
		return nil
	case <-interrupt:
		stdoutln("\nInterrupt received, closing connection...")
		c.close()
		return <-errCh
	}
}

func (c *client) readLoop() error {
	for {
		msgType, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				stdoutln("Connection closed by server")
				return nil
			}
			return err
		}

		if msgType != websocket.BinaryMessage {
			continue
		}

		if err := c.handleMessage(data); err != nil {
			fmt.Fprintf(os.Stderr, "error handling message: %v\n", err)
		}
	}
}

func (c *client) inputLoop() {
	for {
		line, err := c.input.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			c.mu.Lock()
			closed := c.closed
			c.mu.Unlock()
			if closed {
				return
			}
			fmt.Fprintf(os.Stderr, "%s\n", colorize(fmt.Sprintf("input error: %v", err), colorRed))
			return
		}

		c.handleInputLine(strings.TrimRight(line, "\r\n"))
	}
}

func (c *client) handleMessage(data []byte) error {
	var actionReq protocol.ActionRequest
	if err := protocol.Unmarshal(data, &actionReq); err == nil && actionReq.Type == protocol.TypeActionRequest {
		return c.handleActionRequest(&actionReq)
	}

	var handStart protocol.HandStart
	if err := protocol.Unmarshal(data, &handStart); err == nil && handStart.Type == protocol.TypeHandStart {
		return c.handleHandStart(&handStart)
	}

	var gameUpdate protocol.GameUpdate
	if err := protocol.Unmarshal(data, &gameUpdate); err == nil && gameUpdate.Type == protocol.TypeGameUpdate {
		return c.handleGameUpdate(&gameUpdate)
	}

	var playerAction protocol.PlayerAction
	if err := protocol.Unmarshal(data, &playerAction); err == nil && playerAction.Type == protocol.TypePlayerAction {
		return c.handlePlayerAction(&playerAction)
	}

	var streetChange protocol.StreetChange
	if err := protocol.Unmarshal(data, &streetChange); err == nil && streetChange.Type == protocol.TypeStreetChange {
		return c.handleStreetChange(&streetChange)
	}

	var handResult protocol.HandResult
	if err := protocol.Unmarshal(data, &handResult); err == nil && handResult.Type == protocol.TypeHandResult {
		return c.handleHandResult(&handResult)
	}

	var msgErr protocol.Error
	if err := protocol.Unmarshal(data, &msgErr); err == nil && msgErr.Type == protocol.TypeError {
		c.handleServerError(&msgErr)
		return nil
	}

	stdoutln("Received unrecognized message from server (ignored)")
	return nil
}

func (c *client) handleHandStart(msg *protocol.HandStart) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	players := make([]protocol.Player, len(msg.Players))
	copy(players, msg.Players)
	if msg.YourSeat >= 0 && msg.YourSeat < len(players) && strings.TrimSpace(c.name) != "" {
		players[msg.YourSeat].Name = c.name
	}

	initial := make([]int, len(players))
	for i, p := range players {
		initial[i] = p.Chips
	}

	roles := make(map[int][]string)
	roles[msg.Button] = appendRole(roles[msg.Button], "button")
	if msg.YourSeat >= 0 {
		roles[msg.YourSeat] = appendRole(roles[msg.YourSeat], "hero")
	}

	c.state = &handState{
		id:               msg.HandID,
		holeCards:        append([]string(nil), msg.HoleCards...),
		yourSeat:         msg.YourSeat,
		button:           msg.Button,
		smallBlind:       msg.SmallBlind,
		bigBlind:         msg.BigBlind,
		board:            nil,
		currentStreet:    "preflop",
		players:          players,
		initialStacks:    initial,
		roles:            roles,
		foldStreet:       make(map[int]string),
		printedHoleCards: false,
		printedStreets:   make(map[string]bool),
	}
	c.pendingAction = nil

	stdoutf("\n%s\n", colorize(fmt.Sprintf("Table '%s' %d-max Seat #%d is the button", msg.HandID, len(players), msg.Button+1), colorBold+colorMagenta))
	stdoutln(colorize(fmt.Sprintf("Blinds %d/%d", msg.SmallBlind, msg.BigBlind), colorDim))
	for _, p := range players {
		seatNum := p.Seat + 1
		name := formatPlayerName(c.state, p.Seat, p.Name, p.Folded, p.AllIn)
		line := fmt.Sprintf("Seat %d: %s", seatNum, name)
		line += fmt.Sprintf(" (%s in chips)", formatAmountPlain(p.Chips))
		stdoutln(line)
	}
	return nil
}

func (c *client) handleGameUpdate(msg *protocol.GameUpdate) error {
	c.mu.Lock()
	if c.state == nil || c.state.id != msg.HandID {
		c.mu.Unlock()
		return nil
	}

	c.state.pot = msg.Pot

	for i := range msg.Players {
		if i >= len(c.state.players) {
			continue
		}
		update := msg.Players[i]
		player := &c.state.players[i]
		if update.Name != "" {
			player.Name = update.Name
		}
		if i == c.state.yourSeat && strings.TrimSpace(c.name) != "" {
			player.Name = c.name
		}
		player.Chips = update.Chips
		player.Bet = update.Bet
		player.Folded = update.Folded
		player.AllIn = update.AllIn
	}
	if c.state != nil && c.pendingAction != nil && c.state.yourSeat < len(c.state.players) {
		your := c.state.players[c.state.yourSeat]
		if your.Folded {
			c.pendingAction = nil
			c.mu.Unlock()
			c.clearActionPrompt()
			stdoutln(colorize("\nServer folded you due to timeout or invalid action.", colorDim+colorRed))
			return nil
		}
	}

	c.mu.Unlock()
	return nil
}

func (c *client) handleStreetChange(msg *protocol.StreetChange) error {
	c.mu.Lock()
	if c.state == nil || c.state.id != msg.HandID {
		c.mu.Unlock()
		return nil
	}
	prevBoard := append([]string(nil), c.state.board...)
	ordered := mergeBoard(prevBoard, msg.Board)
	c.state.board = ordered
	if strings.TrimSpace(msg.Street) != "" {
		c.state.currentStreet = strings.ToLower(msg.Street)
	}
	var header string
	if msg.Street != "preflop" && !c.state.printedStreets[msg.Street] {
		header = formatStreetHeader(msg.Street, c.state.board)
		c.state.printedStreets[msg.Street] = true
	}
	c.mu.Unlock()

	if header != "" {
		stdoutln()
		stdoutln(colorize(header, colorBold+colorBlue))
	}
	c.printPrompt()
	return nil
}

func (c *client) handlePlayerAction(msg *protocol.PlayerAction) error {
	c.mu.Lock()
	state := c.state
	var (
		nameLabel        string
		holeHeaderNeeded bool
		heroName         string
		heroCards        string
	)
	if state != nil && state.id == msg.HandID && msg.Seat >= 0 && msg.Seat < len(state.players) {
		player := &state.players[msg.Seat]
		if msg.PlayerName != "" {
			player.Name = msg.PlayerName
		}
		if msg.Seat == state.yourSeat && strings.TrimSpace(c.name) != "" {
			player.Name = c.name
		}
		player.Chips = msg.PlayerChips
		player.Bet = msg.PlayerBet
		if msg.Action == "fold" || msg.Action == "timeout_fold" {
			player.Folded = true
			foldStreet := state.currentStreet
			if foldStreet == "" {
				foldStreet = strings.ToLower(msg.Street)
			}
			if foldStreet == "" {
				foldStreet = "preflop"
			}
			state.foldStreet[msg.Seat] = titleCase(foldStreet)
		}
		if msg.Action == "allin" || (msg.PlayerChips == 0 && msg.Action != "fold" && msg.Action != "timeout_fold") {
			player.AllIn = true
		}
		switch msg.Action {
		case "post_small_blind":
			state.roles[msg.Seat] = appendRole(state.roles[msg.Seat], "small blind")
		case "post_big_blind":
			state.roles[msg.Seat] = appendRole(state.roles[msg.Seat], "big blind")
		}
		state.pot = msg.Pot
		nameLabel = formatPlayerName(state, msg.Seat, player.Name, player.Folded, player.AllIn)
		if !state.printedHoleCards && msg.Street == "preflop" && msg.Action != "post_small_blind" && msg.Action != "post_big_blind" {
			state.printedHoleCards = true
			holeHeaderNeeded = true
		}
		if state.yourSeat >= 0 && state.yourSeat < len(state.players) {
			heroName = formatPlayerName(state, state.yourSeat, state.players[state.yourSeat].Name, state.players[state.yourSeat].Folded, state.players[state.yourSeat].AllIn)
			heroCards = formatCards(state.holeCards)
		}
	} else {
		nameLabel = formatPlayerName(nil, msg.Seat, msg.PlayerName, false, false)
	}
	c.mu.Unlock()

	if holeHeaderNeeded {
		stdoutln()
		stdoutln(colorize("*** HOLE CARDS ***", colorBold+colorBlue))
		if heroName != "" && heroCards != "" {
			stdoutf("%s: %s\n", heroName, heroCards)
		}
	}

	actionDesc := describePlayerAction(msg.Action, msg.AmountPaid, msg.PlayerBet, msg.PlayerChips)
	if actionDesc == "" {
		actionDesc = msg.Action
	}
	stdoutf("%s: %s\n", nameLabel, actionDesc)
	c.printPrompt()
	return nil
}

func (c *client) handleHandResult(msg *protocol.HandResult) error {
	c.mu.Lock()
	state := c.state
	var (
		players       []protocol.Player
		initialStacks []int
		rolesCopy     map[int][]string
		foldCopy      map[int]string
		yourSeat      = -1
	)
	if state != nil && state.id == msg.HandID {
		players = make([]protocol.Player, len(state.players))
		copy(players, state.players)
		initialStacks = append([]int(nil), state.initialStacks...)
		rolesCopy = make(map[int][]string, len(state.roles))
		for seat, list := range state.roles {
			rolesCopy[seat] = append([]string(nil), list...)
		}
		foldCopy = make(map[int]string, len(state.foldStreet))
		for seat, street := range state.foldStreet {
			foldCopy[seat] = street
		}
		yourSeat = state.yourSeat
	}
	c.state = nil
	c.pendingAction = nil
	c.mu.Unlock()

	c.clearActionPrompt()

	if len(players) == 0 {
		for _, winner := range msg.Winners {
			players = append(players, protocol.Player{Name: winner.Name})
		}
	}

	totalPot := 0
	for _, winner := range msg.Winners {
		totalPot += winner.Amount
	}

	hasShowdown := len(msg.Showdown) > 0
	if !hasShowdown {
		for _, winner := range msg.Winners {
			if len(winner.HoleCards) > 0 || strings.TrimSpace(winner.HandRank) != "" {
				hasShowdown = true
				break
			}
		}
	}
	printed := make(map[string]bool)
	if hasShowdown {
		if state == nil || state.printedStreets == nil || !state.printedStreets["showdown"] {
			stdoutln()
			stdoutln(colorize("*** SHOWDOWN ***", colorBold+colorBlue))
		}
		for _, winner := range msg.Winners {
			seat, _ := findSeatByName(players, winner.Name)
			name := formatSummaryName(players, yourSeat, seat, winner.Name)
			showLine := fmt.Sprintf("%s: shows %s", name, formatCards(winner.HoleCards))
			if strings.TrimSpace(winner.HandRank) != "" {
				showLine += fmt.Sprintf(" (%s)", colorize(winner.HandRank, colorYellow+colorBold))
			}
			stdoutln(showLine)
			printed[winner.Name] = true
		}
		for _, sd := range msg.Showdown {
			if printed[sd.Name] {
				continue
			}
			seat, _ := findSeatByName(players, sd.Name)
			name := formatSummaryName(players, yourSeat, seat, sd.Name)
			line := fmt.Sprintf("%s: shows %s", name, formatCards(sd.HoleCards))
			if strings.TrimSpace(sd.HandRank) != "" {
				line += fmt.Sprintf(" (%s)", colorize(sd.HandRank, colorYellow))
			}
			stdoutln(line)
			printed[sd.Name] = true
		}
	}
	for _, winner := range msg.Winners {
		seat, _ := findSeatByName(players, winner.Name)
		name := formatSummaryName(players, yourSeat, seat, winner.Name)
		stdoutf("%s collected %s from pot\n", name, formatAmount(winner.Amount))
	}

	stdoutln()
	stdoutln(colorize("*** SUMMARY ***", colorBold+colorBlue))
	stdoutf("Total pot %s | Rake 0\n", formatAmount(totalPot))
	boardForSummary := mergeBoard(nil, msg.Board)
	if state != nil && len(state.board) > 0 {
		boardForSummary = mergeBoard(state.board, msg.Board)
	}
	stdoutf("Board %s\n", formatBoardAll(boardForSummary))

	winnersBySeat := make(map[int]protocol.Winner)
	for _, winner := range msg.Winners {
		seat, ok := findSeatByName(players, winner.Name)
		if ok {
			winnersBySeat[seat] = winner
		}
	}
	showdownBySeat := make(map[int]protocol.ShowdownHand)
	for _, sd := range msg.Showdown {
		seat, ok := findSeatByName(players, sd.Name)
		if ok {
			showdownBySeat[seat] = sd
		}
	}

	for seat, player := range players {
		name := fallbackName(player.Name, seat)
		nameFmt := formatSummaryName(players, yourSeat, seat, name)
		rolesSuffix := formatRolesSuffix(rolesCopy[seat])
		chips := player.Chips
		initial := 0
		if seat < len(initialStacks) {
			initial = initialStacks[seat]
		}
		finalChips := chips
		if winner, ok := winnersBySeat[seat]; ok && winner.Amount > 0 && finalChips < initial {
			finalChips += winner.Amount
		}
		line := fmt.Sprintf("Seat %d: %s", seat+1, nameFmt)
		if rolesSuffix != "" {
			line += colorize(rolesSuffix, colorDim)
		}
		if winner, ok := winnersBySeat[seat]; ok {
			line += fmt.Sprintf(" showed %s and won (%s)", formatCards(winner.HoleCards), formatAmountPlain(winner.Amount))
			if strings.TrimSpace(winner.HandRank) != "" {
				line += fmt.Sprintf(" with %s", winner.HandRank)
			}
		} else if sd, ok := showdownBySeat[seat]; ok {
			line += fmt.Sprintf(" showed %s and lost with %s", formatCards(sd.HoleCards), sd.HandRank)
		} else if street, ok := foldCopy[seat]; ok {
			line += fmt.Sprintf(" folded %s", foldDescription(street))
		} else {
			line += fmt.Sprintf(" finished with %s", formatAmount(finalChips))
		}
		if initial > 0 && initial != finalChips {
			delta := finalChips - initial
			line += fmt.Sprintf(" (%s)", formatDelta(delta))
		}
		stdoutln(line)
	}

	stdoutln(colorize("Waiting for the next hand...", colorDim))
	return nil
}

func (c *client) handleActionRequest(req *protocol.ActionRequest) error {
	c.mu.Lock()
	if c.state != nil && c.state.id == req.HandID {
		c.state.pot = req.Pot
	}
	reqCopy := *req
	c.pendingAction = &reqCopy
	state := c.state
	c.mu.Unlock()

	c.clearActionPrompt()

	heroStack := -1
	lines := []string{"", colorize(fmt.Sprintf("=== Your action (hand %s) ===", req.HandID), colorBold+colorCyan)}
	if state != nil {
		lines = append(lines, fmt.Sprintf("Board: %s | Your cards: %s", formatCards(state.board), formatCards(state.holeCards)))
		if state.yourSeat >= 0 && state.yourSeat < len(state.players) {
			heroStack = state.players[state.yourSeat].Chips
		}
	}
	if heroStack >= 0 {
		lines = append(lines, fmt.Sprintf("Your stack: %s", formatAmount(heroStack)))
	}
	lines = append(lines, fmt.Sprintf("To call: %s | Min bet: %s | Pot: %s | Time remaining: %s", formatAmount(req.ToCall), formatAmount(req.MinBet), formatAmount(req.Pot), colorize(fmt.Sprintf("%dms", req.TimeRemaining), colorDim)))
	lines = append(lines, fmt.Sprintf("Valid actions: %s", formatActions(req.ValidActions)))
	if examples := formatActionExamples(req.ValidActions); examples != "" {
		lines = append(lines, colorize(fmt.Sprintf("Enter action (e.g. %s). Type 'info' to reprint table.", examples), colorDim))
	} else {
		lines = append(lines, colorize("Enter action. Type 'info' to reprint table.", colorDim))
	}
	lines = append(lines, colorize("Tip: type 'allin' any time to shove your stack.", colorDim))
	c.renderActionPrompt(lines)
	return nil
}

func (c *client) renderActionPrompt(lines []string) {
	for _, line := range lines {
		stdoutln(line)
	}
	total := len(lines)
	c.printPrompt()
	c.mu.Lock()
	c.actionPromptLines = total + 1
	c.mu.Unlock()
}

type invalidActionError struct {
	action string
	valid  []string
}

func (e *invalidActionError) Error() string {
	return fmt.Sprintf("action '%s' is not allowed", e.action)
}

func (c *client) parseAction(input string, req *protocol.ActionRequest, heroStack int) (string, int, string, error) {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return "", 0, "", errors.New("please enter an action")
	}

	action := normalizeAction(fields[0])
	if !actionAllowed(action, req.ValidActions) {
		if action == "call" && actionAllowed("allin", req.ValidActions) && heroStack >= 0 && req.ToCall >= heroStack {
			return "allin", 0, "Call matches your remaining stack; treating as all-in.", nil
		}
		return "", 0, "", &invalidActionError{action: action, valid: append([]string(nil), req.ValidActions...)}
	}

	switch action {
	case "fold", "call", "check", "allin":
		if len(fields) > 1 {
			return "", 0, "", fmt.Errorf("action '%s' does not take an amount", action)
		}
		return action, 0, "", nil
	case "raise", "bet":
		if len(fields) < 2 {
			return "", 0, "", fmt.Errorf("%s requires an amount (total bet)", action)
		}
		amount, err := strconv.Atoi(fields[1])
		if err != nil || amount <= 0 {
			return "", 0, "", fmt.Errorf("invalid %s amount", action)
		}
		if amount < req.MinBet {
			return "", 0, "", fmt.Errorf("%s must be at least %d", action, req.MinBet)
		}
		return action, amount, "", nil
	default:
		return "", 0, "", fmt.Errorf("unsupported action '%s'", action)
	}
}

func (c *client) printPlayers(label string) {
	if c.state == nil {
		return
	}

	stdoutf("%s\n", colorize(label+":", colorBold))
	for i, p := range c.state.players {
		seatLabel := formatSeatLabel(c.state, i)
		name := formatPlayerName(c.state, i, p.Name, p.Folded, p.AllIn)

		status := []string{colorize(fmt.Sprintf("chips=%d", p.Chips), colorGreen)}
		if p.Bet > 0 {
			status = append(status, colorize(fmt.Sprintf("bet=%d", p.Bet), colorYellow))
		}
		if p.Folded {
			status = append(status, colorize("folded", colorDim))
		}
		if p.AllIn {
			status = append(status, colorize("all-in", colorRed))
		}

		stdoutf("  %s: %s [%s]\n", seatLabel, name, strings.Join(status, ", "))
	}
}

func (c *client) handleInputLine(raw string) {
	line := strings.TrimSpace(raw)
	if line == "" {
		c.printPrompt()
		return
	}

	lower := strings.ToLower(line)
	if lower == "info" {
		c.mu.Lock()
		c.printPlayers("Current status")
		if c.state != nil {
			stdoutf("Board: %s | Your cards: %s | Pot: %d\n", formatCards(c.state.board), formatCards(c.state.holeCards), c.state.pot)
		}
		c.mu.Unlock()
		c.printPrompt()
		return
	}

	c.mu.Lock()
	heroStack := -1
	if c.state != nil && c.state.yourSeat >= 0 && c.state.yourSeat < len(c.state.players) {
		heroStack = c.state.players[c.state.yourSeat].Chips
	}
	if c.pendingAction == nil {
		c.mu.Unlock()
		stdoutln(colorize("No action pending right now. Type 'info' to view the table.", colorDim))
		return
	}
	currentReq := *c.pendingAction
	c.mu.Unlock()

	action, amount, infoMsg, err := c.parseAction(lower, &currentReq, heroStack)
	if err != nil {
		var iaErr *invalidActionError
		if errors.As(err, &iaErr) {
			stdoutln(colorize(iaErr.Error(), colorRed))
			stdoutf("%s %s\n", colorize("Valid actions:", colorDim), formatActions(iaErr.valid))
		} else {
			stdoutln(colorize(err.Error(), colorRed))
		}
		c.printPrompt()
		return
	}
	if infoMsg != "" {
		stdoutln(colorize(infoMsg, colorDim))
	}

	msg := &protocol.Action{Type: protocol.TypeAction, Action: action, Amount: amount}
	payload, err := protocol.Marshal(msg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", colorize(fmt.Sprintf("failed to encode action: %v", err), colorRed))
		c.printPrompt()
		return
	}

	if err := c.conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", colorize(fmt.Sprintf("failed to send action: %v", err), colorRed))
		c.printPrompt()
		return
	}

	c.clearActionPrompt()

	c.mu.Lock()
	c.pendingAction = nil
	c.mu.Unlock()

	stdoutf("Sent action: %s", formatActionLabel(action))
	if action == "raise" || action == "bet" {
		stdoutf(" %s", formatAmount(amount))
	}
	stdoutln()
}

func (c *client) close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	conn := c.conn
	c.mu.Unlock()

	if conn != nil {
		_ = conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		_ = conn.Close()
	}
}

func formatCards(cards []string) string {
	if len(cards) == 0 {
		return colorize("--", colorDim)
	}
	formatted := make([]string, len(cards))
	for i, card := range cards {
		formatted[i] = formatCard(card)
	}
	return strings.Join(formatted, " ")
}

func fallbackName(name string, seat int) string {
	trimmed := strings.TrimSpace(name)
	if trimmed != "" {
		return trimmed
	}
	if seat >= 0 {
		return fmt.Sprintf("Seat%d", seat+1)
	}
	return "Seat"
}

func formatSeatLabel(state *handState, seat int) string {
	base := fmt.Sprintf("Seat %d", seat+1)
	if state == nil {
		return colorize(base, colorBold)
	}
	if seat == state.yourSeat {
		return colorize(base, colorCyan+colorBold)
	}
	return colorize(base, colorBold)
}

func formatPlayerName(state *handState, seat int, name string, folded, allIn bool) string {
	display := fallbackName(name, seat)
	var suffix string
	if state != nil && state.roles != nil {
		suffix = formatRolesSuffix(state.roles[seat])
	}
	var base string
	switch {
	case folded:
		base = colorize(display, colorDim)
	case allIn:
		base = colorize(display, colorRed+colorBold)
	case state != nil && seat == state.yourSeat:
		base = colorize(display, colorCyan+colorBold)
	default:
		base = colorize(display, colorBold)
	}
	if suffix != "" {
		return base + colorize(suffix, colorDim)
	}
	return base
}

func describePlayerAction(action string, amountPaid, playerBet, playerChips int) string {
	switch action {
	case "fold":
		return colorize("folds", colorDim)
	case "check":
		return colorize("checks", colorBlue)
	case "call":
		if amountPaid > 0 {
			return fmt.Sprintf("calls %s", formatAmount(amountPaid))
		}
		return colorize("calls", colorGreen)
	case "raise":
		if amountPaid > 0 {
			return fmt.Sprintf("raises %s to %s", formatAmount(amountPaid), formatAmount(playerBet))
		}
		return fmt.Sprintf("raises to %s", formatAmount(playerBet))
	case "allin":
		var base string
		switch {
		case amountPaid > 0 && playerBet > amountPaid:
			base = fmt.Sprintf("raises %s to %s", formatAmount(amountPaid), formatAmount(playerBet))
		case playerBet > 0:
			base = fmt.Sprintf("bets %s", formatAmount(playerBet))
		default:
			base = colorize("moves all-in", colorRed+colorBold)
		}
		return fmt.Sprintf("%s %s", base, colorize("and is all-in", colorRed+colorBold))
	case "post_small_blind":
		return fmt.Sprintf("posts small blind %s", formatAmount(amountPaid))
	case "post_big_blind":
		return fmt.Sprintf("posts big blind %s", formatAmount(amountPaid))
	case "timeout_fold":
		return colorize("times out and folds", colorRed)
	case "bet":
		return fmt.Sprintf("bets %s", formatAmount(amountPaid))
	default:
		return colorize(action, colorBold)
	}
}

func formatCard(card string) string {
	if len(card) < 2 {
		return card
	}
	rank := strings.ToUpper(card[:1])
	suit := card[len(card)-1]
	var emoji, color string
	switch suit {
	case 's', 'S':
		emoji = "♠️"
		color = colorBlue
	case 'h', 'H':
		emoji = "♥️"
		color = colorRed
	case 'd', 'D':
		emoji = "♦️"
		color = colorYellow
	case 'c', 'C':
		emoji = "♣️"
		color = colorGreen
	default:
		emoji = string(suit)
		color = colorBold
	}
	return colorize(rank+emoji, colorBold+color)
}

func formatAmount(amount int) string {
	return colorize(fmt.Sprintf("%d", amount), colorBold+colorYellow)
}

func formatActions(actions []string) string {
	if len(actions) == 0 {
		return ""
	}
	formatted := make([]string, len(actions))
	for i, action := range actions {
		formatted[i] = formatActionLabel(action)
	}
	return strings.Join(formatted, colorize(", ", colorDim))
}

func formatActionExamples(actions []string) string {
	if len(actions) == 0 {
		return ""
	}
	examples := make([]string, 0, len(actions))
	for _, action := range actions {
		switch action {
		case "fold", "call", "check", "allin":
			examples = append(examples, fmt.Sprintf("'%s'", action))
		case "raise":
			examples = append(examples, "'raise <amount>'")
		case "bet":
			examples = append(examples, "'bet <amount>'")
		default:
			examples = append(examples, fmt.Sprintf("'%s'", action))
		}
	}
	return strings.Join(examples, ", ")
}

func formatActionLabel(action string) string {
	switch action {
	case "fold":
		return colorize(action, colorRed)
	case "check":
		return colorize(action, colorBlue)
	case "call":
		return colorize(action, colorGreen)
	case "raise":
		return colorize(action, colorMagenta+colorBold)
	case "allin":
		return colorize(action, colorMagenta+colorBold)
	case "bet":
		return colorize(action, colorMagenta+colorBold)
	default:
		return colorize(action, colorBold)
	}
}

func colorize(text string, color string) string {
	if color == "" {
		return text
	}
	return color + text + colorReset
}

func stdoutf(format string, args ...any) {
	fmt.Fprintf(os.Stdout, format, args...)
}

func stdoutln(args ...any) {
	fmt.Fprintln(os.Stdout, args...)
}

func stdout(text string) {
	fmt.Fprint(os.Stdout, text)
}

func normalizeAction(action string) string {
	switch action {
	case "f":
		return "fold"
	case "c":
		return "call"
	case "k":
		return "check"
	case "r":
		return "raise"
	case "b":
		return "bet"
	case "a":
		return "allin"
	default:
		return action
	}
}

func actionAllowed(action string, valid []string) bool {
	if action == "allin" {
		return true
	}
	for _, v := range valid {
		if v == action {
			return true
		}
	}
	return false
}

func (c *client) clearActionPrompt() {
	c.mu.Lock()
	lines := c.actionPromptLines
	c.actionPromptLines = 0
	c.mu.Unlock()

	if lines <= 0 {
		return
	}

	stdoutf("\033[%dF\033[J", lines)
}

func (c *client) printPromptLocked() {
	if c.pendingAction != nil {
		stdout(colorize("> ", colorYellow+colorBold))
	}
}

func (c *client) printPrompt() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.printPromptLocked()
}

func (c *client) handleServerError(msg *protocol.Error) {
	stdoutf("%s\n", colorize(fmt.Sprintf("Server error: %s (%s)", msg.Message, msg.Code), colorRed))
	if msg.Code == "action_timeout" {
		c.mu.Lock()
		c.pendingAction = nil
		c.mu.Unlock()
	}
}

func titleCase(value string) string {
	if value == "" {
		return ""
	}
	upper := strings.ToUpper(value[:1])
	if len(value) == 1 {
		return upper
	}
	return upper + value[1:]
}

func appendRole(list []string, role string) []string {
	for _, existing := range list {
		if existing == role {
			return list
		}
	}
	return append(list, role)
}

func formatRolesSuffix(list []string) string {
	ordered := orderRoles(list)
	if len(ordered) == 0 {
		return ""
	}
	return " (" + strings.Join(ordered, ", ") + ")"
}

func orderRoles(list []string) []string {
	if len(list) <= 1 {
		return append([]string(nil), list...)
	}
	ordered := append([]string(nil), list...)
	priority := map[string]int{
		"hero":        0,
		"button":      1,
		"small blind": 2,
		"big blind":   3,
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		pi := priorityValue(priority, ordered[i], i)
		pj := priorityValue(priority, ordered[j], j)
		if pi == pj {
			return ordered[i] < ordered[j]
		}
		return pi < pj
	})
	return ordered
}

func priorityValue(priority map[string]int, role string, fallback int) int {
	if v, ok := priority[role]; ok {
		return v
	}
	return 100 + fallback
}

func formatSummaryName(players []protocol.Player, yourSeat, seat int, name string) string {
	display := strings.TrimSpace(name)
	if seat >= 0 && seat < len(players) {
		display = fallbackName(players[seat].Name, seat)
	}
	if display == "" {
		if seat >= 0 {
			display = fallbackName(name, seat)
		} else {
			display = name
		}
	}
	if seat >= 0 && seat == yourSeat {
		return colorize(display, colorCyan+colorBold)
	}
	return colorize(display, colorBold)
}

func formatBoardSegment(cards []string) string {
	if len(cards) == 0 {
		return "[]"
	}
	formatted := make([]string, len(cards))
	for i, card := range cards {
		formatted[i] = formatCard(card)
	}
	return "[" + strings.Join(formatted, " ") + "]"
}

func formatBoardAll(board []string) string {
	if len(board) == 0 {
		return colorize("[]", colorDim)
	}
	return formatBoardSegment(board)
}

func formatStreetHeader(street string, board []string) string {
	upper := strings.ToUpper(street)
	switch street {
	case "flop":
		if len(board) >= 3 {
			return fmt.Sprintf("*** %s *** %s", upper, formatBoardSegment(board[:3]))
		}
	case "turn":
		if len(board) >= 4 {
			return fmt.Sprintf("*** %s *** %s %s", upper, formatBoardSegment(board[:3]), formatBoardSegment(board[3:4]))
		}
	case "river":
		if len(board) >= 5 {
			return fmt.Sprintf("*** %s *** %s %s", upper, formatBoardSegment(board[:4]), formatBoardSegment(board[4:5]))
		}
	default:
		return fmt.Sprintf("*** %s ***", upper)
	}
	return fmt.Sprintf("*** %s *** %s", upper, formatBoardSegment(board))
}

func findSeatByName(players []protocol.Player, name string) (int, bool) {
	for i, p := range players {
		if strings.EqualFold(p.Name, name) {
			return i, true
		}
	}
	return -1, false
}

func foldDescription(street string) string {
	switch strings.ToLower(street) {
	case "preflop", "pre-flop", "pre flop":
		return "before Flop"
	case "flop":
		return "on the Flop"
	case "turn":
		return "on the Turn"
	case "river":
		return "on the River"
	default:
		if street == "" {
			return ""
		}
		return "on " + street
	}
}

func mergeBoard(prev []string, new []string) []string {
	if len(prev) == 0 {
		return uniqueCards(new)
	}
	seen := make(map[string]struct{}, len(prev))
	result := append([]string(nil), prev...)
	for _, card := range result {
		seen[card] = struct{}{}
	}
	for _, card := range new {
		if _, ok := seen[card]; ok {
			continue
		}
		seen[card] = struct{}{}
		result = append(result, card)
	}
	if len(new) < len(prev) || len(result) < len(new) {
		return uniqueCards(new)
	}
	return result
}

func uniqueCards(cards []string) []string {
	if len(cards) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(cards))
	ordered := make([]string, 0, len(cards))
	for _, card := range cards {
		if _, ok := seen[card]; ok {
			continue
		}
		seen[card] = struct{}{}
		ordered = append(ordered, card)
	}
	return ordered
}

func formatAmountPlain(amount int) string {
	return fmt.Sprintf("%d", amount)
}

func formatDelta(delta int) string {
	switch {
	case delta > 0:
		return colorize(fmt.Sprintf("+%d", delta), colorGreen)
	case delta < 0:
		return colorize(fmt.Sprintf("%d", delta), colorRed)
	default:
		return colorize("0", colorDim)
	}
}

func main() {
	server := flag.String("server", "ws://localhost:8080/ws", "WebSocket server URL")
	name := flag.String("name", os.Getenv("USER"), "Display name")
	flag.Parse()

	clientName := strings.TrimSpace(*name)
	if clientName == "" {
		clientName = "Player"
	}

	c := newClient(clientName)
	if err := c.connect(*server); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect: %v\n", err)
		os.Exit(1)
	}

	if err := c.run(); err != nil {
		fmt.Fprintf(os.Stderr, "Client error: %v\n", err)
		os.Exit(1)
	}

	c.close()
	stdoutln("Goodbye!")
}
