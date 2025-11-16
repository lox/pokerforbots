package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/lox/pokerforbots/v2/internal/phh"
	"github.com/lox/pokerforbots/v2/internal/server"
)

// HandHistoryCmd is the root command for PHH utilities.
type HandHistoryCmd struct {
	Render HandHistoryRenderCmd `cmd:"render" help:"Render a PHH session file using the pretty hand view"`
}

// HandHistoryRenderCmd replays a PHH file through the pretty-print monitor.
type HandHistoryRenderCmd struct {
	File  string `arg:"" name:"file" help:"Path to session.phhs file"`
	Limit int    `help:"Maximum number of hands to render (0 = all)"`
}

func (cmd HandHistoryRenderCmd) Run() error {
	if cmd.File == "" {
		return errors.New("hand-history render requires a file path")
	}

	hands, err := loadPHHFile(cmd.File)
	if err != nil {
		return err
	}
	if len(hands) == 0 {
		return fmt.Errorf("no hands found in %s", cmd.File)
	}

	limit := cmd.Limit
	if limit <= 0 || limit > len(hands) {
		limit = len(hands)
	}

	monitor := server.NewPrettyPrintMonitor(os.Stdout)
	monitor.OnGameStart(uint64(limit))

	playback := newPHHPlayback(monitor)
	for i := 0; i < limit; i++ {
		if err := playback.RenderHand(i, hands[i]); err != nil {
			return fmt.Errorf("rendering hand %d: %w", i+1, err)
		}
	}

	monitor.OnGameComplete(uint64(limit), "PHH playback")
	return nil
}

// loadPHHFile decodes a PHH session file into structured hands.
func loadPHHFile(path string) ([]phh.HandHistory, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := io.ReadAll(bufio.NewReader(f))
	if err != nil {
		return nil, err
	}

	if hands, ok, err := decodeSectionedPHH(string(data)); err == nil && ok {
		return hands, nil
	} else if err != nil {
		return nil, err
	}

	chunks := splitPHHChunks(string(data))
	hands := make([]phh.HandHistory, 0, len(chunks))
	for idx, chunk := range chunks {
		var hand phh.HandHistory
		if _, err := toml.NewDecoder(strings.NewReader(chunk)).Decode(&hand); err != nil {
			return nil, fmt.Errorf("decode chunk %d: %w", idx+1, err)
		}
		normalizeHandID(&hand, len(hands)+1)
		hands = append(hands, hand)
	}
	return hands, nil
}

func decodeSectionedPHH(raw string) ([]phh.HandHistory, bool, error) {
	sections := make(map[string]phh.HandHistory)
	if _, err := toml.Decode(raw, &sections); err != nil {
		return nil, false, nil
	}
	if len(sections) == 0 {
		return nil, false, nil
	}
	keys := make([]string, 0, len(sections))
	for k := range sections {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return compareSectionKeys(keys[i], keys[j])
	})
	hands := make([]phh.HandHistory, 0, len(keys))
	for _, key := range keys {
		hand := sections[key]
		normalizeHandID(&hand, len(hands)+1)
		if hand.HandID == "" {
			hand.HandID = key
		}
		hands = append(hands, hand)
	}
	return hands, true, nil
}

func compareSectionKeys(a, b string) bool {
	ai, errA := strconv.Atoi(strings.TrimLeft(a, "hand_"))
	bi, errB := strconv.Atoi(strings.TrimLeft(b, "hand_"))
	if errA == nil && errB == nil {
		return ai < bi
	}
	return a < b
}

func splitPHHChunks(data string) []string {
	lines := strings.Split(data, "\n")
	var chunks []string
	cur := make([]string, 0, 64)
	flush := func() {
		if len(cur) == 0 {
			return
		}
		chunk := strings.TrimSpace(strings.Join(cur, "\n"))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		cur = cur[:0]
	}
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "# ─") {
			// explicit separator between hands
			flush()
			continue
		}
		if strings.HasPrefix(trim, "version") && strings.Contains(trim, "=") && len(cur) > 0 {
			flush()
		}
		if trim == "" && len(cur) == 0 {
			continue
		}
		cur = append(cur, line)
	}
	flush()
	return chunks
}

func normalizeHandID(hand *phh.HandHistory, nextIndex int) {
	if hand.HandID == "" {
		hand.HandID = hand.LegacyHandID
	}
	if hand.HandID == "" {
		hand.HandID = fmt.Sprintf("hand-%d", nextIndex)
	}
}

// phhPlayback replays PHH hands through a server.HandMonitor.
type phhPlayback struct {
	monitor server.HandMonitor
}

func newPHHPlayback(monitor server.HandMonitor) *phhPlayback {
	return &phhPlayback{monitor: monitor}
}

func (p *phhPlayback) RenderHand(idx int, hand phh.HandHistory) error {
	playerCount := len(hand.Players)
	if playerCount == 0 {
		return fmt.Errorf("hand %s has no players", hand.HandID)
	}

	rawHoleCards := extractHoleCards(hand.Actions, playerCount)
	revealed := make([]bool, playerCount)
	positionToSeat := make([]int, playerCount)
	for i := 0; i < playerCount; i++ {
		seat := i
		if i < len(hand.Seats) && hand.Seats[i] > 0 {
			seat = hand.Seats[i] - 1
		}
		if seat < 0 {
			seat = 0
		}
		if seat >= playerCount {
			seat %= playerCount
		}
		positionToSeat[i] = seat
	}
	holeCards := make([][]string, playerCount)
	for pos := 0; pos < len(rawHoleCards); pos++ {
		seat := seatFromPosition(positionToSeat, pos)
		if seat < 0 || seat >= playerCount {
			continue
		}
		holeCards[seat] = rawHoleCards[pos]
	}
	players := make([]server.HandPlayer, playerCount)
	stacks := make([]int, playerCount)
	investments := make([]int, playerCount)
	for pos := 0; pos < playerCount; pos++ {
		seat := seatFromPosition(positionToSeat, pos)
		if seat < 0 || seat >= playerCount {
			continue
		}
		chips := 0
		if pos < len(hand.StartingStacks) {
			chips = hand.StartingStacks[pos]
		}
		stacks[seat] = chips
		name := hand.Players[pos]
		players[seat] = server.HandPlayer{
			Seat:        seat,
			Name:        name,
			DisplayName: name,
			Chips:       chips,
			HoleCards:   holeCards[seat],
		}
	}

	button := deriveButtonSeat(hand, positionToSeat)
	blinds := server.Blinds{}
	if len(hand.BlindsOrStraddles) >= 1 {
		blinds.Small = hand.BlindsOrStraddles[0]
	}
	if len(hand.BlindsOrStraddles) >= 2 {
		blinds.Big = hand.BlindsOrStraddles[1]
	}

	// Apply antes if present before the first betting round.
	for pos := 0; pos < len(hand.Antes); pos++ {
		seat := seatFromPosition(positionToSeat, pos)
		if seat < 0 || seat >= playerCount {
			continue
		}
		ante := hand.Antes[pos]
		if ante <= 0 {
			continue
		}
		investments[seat] += ante
		stacks[seat] -= ante
		if stacks[seat] < 0 {
			stacks[seat] = 0
		}
	}

	p.monitor.OnHandStart(hand.HandID, players, button, blinds)

	// Playback state
	contributions := make([]int, playerCount)
	currentBet := 0
	board := make([]string, 0, 5)

	if blinds.Small > 0 && blinds.Big > 0 && playerCount >= 2 {
		sb, bb := blindSeats(button, playerCount)
		contributions[sb] = blinds.Small
		stacks[sb] -= blinds.Small
		if stacks[sb] < 0 {
			stacks[sb] = 0
		}
		contributions[bb] = blinds.Big
		stacks[bb] -= blinds.Big
		if stacks[bb] < 0 {
			stacks[bb] = 0
		}
		investments[sb] += blinds.Small
		investments[bb] += blinds.Big
		currentBet = blinds.Big
		p.monitor.OnPlayerAction(hand.HandID, sb, "post_small_blind", blinds.Small, stacks[sb])
		p.monitor.OnPlayerAction(hand.HandID, bb, "post_big_blind", blinds.Big, stacks[bb])
	}

	actionIdx := skipHoleDeals(hand.Actions)

	for ; actionIdx < len(hand.Actions); actionIdx++ {
		raw := strings.TrimSpace(hand.Actions[actionIdx])
		if raw == "" {
			continue
		}
		switch {
		case strings.HasPrefix(raw, "d db"):
			cards := parseCardRun(strings.TrimSpace(strings.TrimPrefix(raw, "d db")))
			board, currentBet, contributions = p.advanceStreet(hand.HandID, board, cards, contributions)
		case strings.HasPrefix(raw, "p"):
			if err := p.playAction(hand.HandID, raw, positionToSeat, stacks, contributions, investments, players, revealed, &currentBet); err != nil {
				return err
			}
		default:
			// Ignore other annotations
		}
	}

	p.monitor.OnHandComplete(server.HandOutcome{
		HandID:         hand.HandID,
		HandsCompleted: uint64(idx + 1),
		Detail:         playbackOutcome(hand, players, board, investments, revealed),
	})
	return nil
}

func (p *phhPlayback) advanceStreet(handID string, currentBoard, newCards []string, contributions []int) ([]string, int, []int) {
	board := append([]string(nil), currentBoard...)
	switch {
	case len(currentBoard) == 0:
		board = append([]string{}, newCards...)
	case len(newCards) == 1:
		board = append(board, newCards...)
	case len(newCards) >= len(currentBoard):
		// Legacy entries that repeat the entire board
		board = append([]string{}, newCards...)
	default:
		board = append(board, newCards...)
	}

	street := streetName(len(board))
	if street != "" {
		p.monitor.OnStreetChange(handID, street, append([]string(nil), board...))
	}

	// Reset betting state for new street
	contributions = make([]int, len(contributions))
	return board, 0, contributions
}

func (p *phhPlayback) playAction(handID, raw string, positionToSeat []int, stacks, contributions, investments []int, players []server.HandPlayer, revealed []bool, currentBet *int) error {
	parts := strings.Fields(raw)
	if len(parts) < 2 {
		return fmt.Errorf("invalid action %q", raw)
	}
	pos := parseSeat(parts[0])
	if pos < 0 || pos >= len(positionToSeat) {
		return fmt.Errorf("invalid seat in %q", raw)
	}
	seat := seatFromPosition(positionToSeat, pos)
	if seat < 0 || seat >= len(contributions) {
		return fmt.Errorf("invalid seat mapping in %q", raw)
	}

	code := parts[1]
	switch code {
	case "f":
		p.monitor.OnPlayerAction(handID, seat, "fold", 0, stacks[seat])
	case "cc":
		toCall := *currentBet - contributions[seat]
		action := "check"
		amount := 0
		if toCall > 0 {
			action = "call"
			amount = toCall
			contributions[seat] += amount
			investments[seat] += amount
			stacks[seat] -= amount
			if stacks[seat] < 0 {
				stacks[seat] = 0
			}
		}
		p.monitor.OnPlayerAction(handID, seat, action, amount, stacks[seat])
	case "cbr":
		if len(parts) < 3 {
			return fmt.Errorf("missing amount in raise action %q", raw)
		}
		total, err := strconv.Atoi(parts[2])
		if err != nil {
			return fmt.Errorf("invalid raise amount in %q", raw)
		}
		amount := total - contributions[seat]
		if amount < 0 {
			amount = 0
		}
		contributions[seat] = total
		investments[seat] += amount
		stacks[seat] -= amount
		if stacks[seat] < 0 {
			stacks[seat] = 0
		}
		if total > *currentBet {
			*currentBet = total
		}
		action := "raise"
		switch {
		case stacks[seat] == 0:
			action = "allin"
		case *currentBet == total && total == 0:
			action = "bet"
		case *currentBet == total && amount == total:
			action = "bet"
		}
		p.monitor.OnPlayerAction(handID, seat, action, amount, stacks[seat])
	case "sm":
		if len(parts) < 3 {
			return fmt.Errorf("missing showdown cards in %q", raw)
		}
		cards := parseCardRun(parts[2])
		if seat >= 0 && seat < len(players) {
			players[seat].HoleCards = append([]string(nil), cards...)
		}
		if seat >= 0 && seat < len(revealed) {
			revealed[seat] = true
		}
	default:
		return fmt.Errorf("unsupported action code %q", raw)
	}
	return nil
}

func extractHoleCards(actions []string, playerCount int) [][]string {
	holes := make([][]string, playerCount)
	for _, raw := range actions {
		raw = strings.TrimSpace(raw)
		if !strings.HasPrefix(raw, "d dh") {
			break
		}
		parts := strings.Fields(raw)
		if len(parts) < 4 {
			continue
		}
		seat := parseSeat(parts[2])
		if seat < 0 || seat >= playerCount {
			continue
		}
		holes[seat] = parseCardRun(parts[3])
	}
	return holes
}

func skipHoleDeals(actions []string) int {
	idx := 0
	for ; idx < len(actions); idx++ {
		raw := strings.TrimSpace(actions[idx])
		if !strings.HasPrefix(raw, "d dh") {
			break
		}
	}
	return idx
}

func parseSeat(token string) int {
	if strings.HasPrefix(token, "p") {
		if v, err := strconv.Atoi(token[1:]); err == nil {
			return v - 1
		}
	}
	return -1
}

func parseCardRun(run string) []string {
	run = strings.TrimSpace(run)
	if run == "" {
		return nil
	}
	cards := []string{}
	for i := 0; i < len(run); i += 2 {
		end := i + 2
		if end > len(run) {
			end = len(run)
		}
		cards = append(cards, run[i:end])
	}
	return cards
}

func blindSeats(button, playerCount int) (int, int) {
	if playerCount <= 1 {
		return 0, 0
	}
	if playerCount == 2 {
		return button % playerCount, (button + 1) % playerCount
	}
	sb := (button + 1) % playerCount
	bb := (button + 2) % playerCount
	return sb, bb
}

func deriveButtonSeat(hand phh.HandHistory, positionToSeat []int) int {
	count := len(positionToSeat)
	if count == 0 {
		return 0
	}
	if count > 2 {
		if seat := seatFromPosition(positionToSeat, count-1); seat >= 0 {
			return seat
		}
	}
	if count == 2 {
		if seat := seatFromPosition(positionToSeat, 0); seat >= 0 {
			return seat
		}
		idx := smallBlindPosition(positionToSeat, hand.BlindsOrStraddles)
		if seat := seatFromPosition(positionToSeat, idx); seat >= 0 {
			return seat
		}
	}
	if seat := metadataInt(hand.Metadata, "button_seat", -1); seat > 0 {
		return seat - 1
	}
	if seat := metadataInt(hand.Metadata, "button", -1); seat >= 0 {
		return seat
	}
	fallback := count - 1
	if fallback < 0 {
		fallback = 0
	}
	if seat := seatFromPosition(positionToSeat, fallback); seat >= 0 {
		return seat
	}
	return 0
}

func seatFromPosition(mapping []int, pos int) int {
	if pos < 0 || pos >= len(mapping) {
		return -1
	}
	return mapping[pos]
}

func smallBlindPosition(positionToSeat []int, blinds []int) int {
	limit := len(positionToSeat)
	if len(blinds) < limit {
		limit = len(blinds)
	}
	idx := 0
	best := -1
	for pos := 0; pos < limit; pos++ {
		amount := blinds[pos]
		if amount <= 0 {
			continue
		}
		if best == -1 || amount < best {
			best = amount
			idx = pos
		}
	}
	return idx
}

func streetName(boardLen int) string {
	switch boardLen {
	case 3:
		return "flop"
	case 4:
		return "turn"
	case 5:
		return "river"
	default:
		return ""
	}
}

func playbackOutcome(hand phh.HandHistory, players []server.HandPlayer, board []string, investments []int, revealed []bool) *server.HandOutcomeDetail {
	if len(players) == 0 {
		return nil
	}
	nets, hasStackDiffs := stackDiffsBySeat(hand, len(players))
	if !hasStackDiffs {
		nets = mapWinningEntriesBySeat(hand, len(players))
	}
	winnerSeats := seatsWithNet(nets)
	if len(winnerSeats) == 0 {
		fallback := mapWinningEntriesBySeat(hand, len(players))
		winnerSeats = seatsWithNet(fallback)
		if len(winnerSeats) == 0 {
			legacy := metadataStringSlice(hand.Metadata, "winners")
			for _, name := range legacy {
				seat := findPlayerSeat(players, name)
				if seat >= 0 {
					winnerSeats = append(winnerSeats, seat)
				}
			}
		}
	}
	if len(winnerSeats) == 0 {
		return nil
	}

	totalPot := sumInts(investments)
	if totalPot == 0 {
		totalPot = metadataInt(hand.Metadata, "total_pot", 0)
	}

	botOutcomes := make([]server.BotHandOutcome, 0, len(players))
	for _, player := range players {
		seat := player.Seat
		net := nets[seat]
		wentToShowdown := seat >= 0 && seat < len(revealed) && revealed[seat]
		var holeCards []string
		if wentToShowdown && len(player.HoleCards) > 0 {
			holeCards = append([]string(nil), player.HoleCards...)
		}
		botOutcomes = append(botOutcomes, server.BotHandOutcome{
			Bot:            &server.Bot{ID: player.Name},
			Position:       seat,
			HoleCards:      holeCards,
			NetChips:       net,
			WentToShowdown: wentToShowdown,
			WonAtShowdown:  net > 0,
		})
	}

	return &server.HandOutcomeDetail{
		Board:       append([]string(nil), board...),
		TotalPot:    totalPot,
		BotOutcomes: botOutcomes,
	}
}

func stackDiffsBySeat(hand phh.HandHistory, playerCount int) ([]int, bool) {
	nets := make([]int, playerCount)
	limit := min(len(hand.StartingStacks), len(hand.FinishingStacks))
	if limit == 0 {
		return nets, false
	}
	for pos := 0; pos < limit; pos++ {
		seat := seatForPosition(hand, pos, playerCount)
		if seat < 0 || seat >= len(nets) {
			continue
		}
		start := hand.StartingStacks[pos]
		finish := hand.FinishingStacks[pos]
		nets[seat] = finish - start
	}
	return nets, true
}

func mapWinningEntriesBySeat(hand phh.HandHistory, playerCount int) []int {
	values := make([]int, playerCount)
	for pos, amount := range hand.Winnings {
		seat := seatForPosition(hand, pos, playerCount)
		if seat >= 0 && seat < len(values) {
			values[seat] = amount
		}
	}
	return values
}

func seatForPosition(hand phh.HandHistory, pos, playerCount int) int {
	if pos < len(hand.Seats) {
		seat := hand.Seats[pos] - 1
		if seat >= 0 && seat < playerCount {
			return seat
		}
	}
	if pos >= 0 && pos < playerCount {
		return pos
	}
	if playerCount == 0 {
		return -1
	}
	return pos % playerCount
}

func seatsWithNet(nets []int) []int {
	seats := make([]int, 0, len(nets))
	for seat, amount := range nets {
		if amount > 0 {
			seats = append(seats, seat)
		}
	}
	return seats
}

func metadataStringSlice(meta map[string]any, key string) []string {
	if meta == nil {
		return nil
	}
	raw, ok := meta[key]
	if !ok {
		return nil
	}
	switch val := raw.(type) {
	case []string:
		out := make([]string, len(val))
		copy(out, val)
		return out
	case []any:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func sumInts(values []int) int {
	sum := 0
	for _, v := range values {
		sum += v
	}
	return sum
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func metadataInt(meta map[string]any, key string, fallback int) int {
	if meta == nil {
		return fallback
	}
	if v, ok := meta[key]; ok {
		switch val := v.(type) {
		case int64:
			return int(val)
		case int:
			return val
		case float64:
			return int(val)
		case string:
			if parsed, err := strconv.Atoi(val); err == nil {
				return parsed
			}
		}
	}
	return fallback
}

func findPlayerSeat(players []server.HandPlayer, name string) int {
	for _, p := range players {
		if p.Name == name {
			return p.Seat
		}
	}
	return -1
}
