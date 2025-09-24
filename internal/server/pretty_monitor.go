package server

import (
	"fmt"
	"io"
	"os"
	"strings"
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

// handState tracks the current hand for pretty printing
type prettyHandState struct {
	handID         string
	players        []HandPlayer
	button         int
	smallBlind     int
	bigBlind       int
	board          []string
	currentStreet  string
	playerStacks   map[int]int
	playerFolded   map[int]bool
	playerAllIn    map[int]bool
	roles          map[int][]string
	printedStreets map[string]bool
	printedHole    bool
}

// PrettyPrintMonitor implements HandMonitor for formatted hand display
type PrettyPrintMonitor struct {
	writer        io.Writer
	handsStarted  uint64
	handsComplete uint64
	handLimit     uint64
	currentHand   *prettyHandState
}

// NewPrettyPrintMonitor creates a new pretty print monitor
func NewPrettyPrintMonitor(writer io.Writer) *PrettyPrintMonitor {
	if writer == nil {
		writer = os.Stdout
	}
	return &PrettyPrintMonitor{
		writer: writer,
	}
}

// OnGameStart is called when the game starts
func (p *PrettyPrintMonitor) OnGameStart(handLimit uint64) {
	p.handLimit = handLimit
	p.handsStarted = 0
	p.handsComplete = 0
}

// OnGameComplete is called when the game completes
func (p *PrettyPrintMonitor) OnGameComplete(handsCompleted uint64, reason string) {
	fmt.Fprintln(p.writer)
	fmt.Fprintln(p.writer, colorize("=== GAME COMPLETED ===", colorBold+colorCyan))
	fmt.Fprintf(p.writer, "Hands completed: %d", handsCompleted)
	if p.handLimit > 0 {
		fmt.Fprintf(p.writer, " / %d", p.handLimit)
	}
	fmt.Fprintln(p.writer)
	if reason != "" {
		fmt.Fprintf(p.writer, "Reason: %s\n", reason)
	}
}

// OnHandStart is called when a new hand begins
func (p *PrettyPrintMonitor) OnHandStart(handID string, players []HandPlayer, button int, blinds Blinds) {
	p.handsStarted++

	// Initialize hand state
	p.currentHand = &prettyHandState{
		handID:         handID,
		players:        players,
		button:         button,
		smallBlind:     blinds.Small,
		bigBlind:       blinds.Big,
		board:          []string{},
		currentStreet:  "preflop",
		playerStacks:   make(map[int]int),
		playerFolded:   make(map[int]bool),
		playerAllIn:    make(map[int]bool),
		roles:          make(map[int][]string),
		printedStreets: make(map[string]bool),
		printedHole:    false,
	}

	// Initialize player stacks
	for _, player := range players {
		p.currentHand.playerStacks[player.Seat] = player.Chips
	}

	// Assign roles
	p.currentHand.roles[button] = append(p.currentHand.roles[button], "button")

	// Print hand header
	fmt.Fprintf(p.writer, "\n%s\n", colorize(fmt.Sprintf("Table '%s' %d-max Seat #%d is the button", handID, len(players), button+1), colorBold+colorMagenta))
	fmt.Fprintln(p.writer, colorize(fmt.Sprintf("Blinds %d/%d", blinds.Small, blinds.Big), colorDim))

	// Print players
	for _, player := range players {
		seatNum := player.Seat + 1
		name := p.formatPlayerName(player.Seat, player.Name, false, false)
		line := fmt.Sprintf("Seat %d: %s", seatNum, name)
		line += fmt.Sprintf(" (%s in chips)", formatAmountPlain(player.Chips))
		fmt.Fprintln(p.writer, line)
	}
}

// OnPlayerAction is called when a player takes an action
func (p *PrettyPrintMonitor) OnPlayerAction(handID string, seat int, action string, amount int, stack int) {
	if p.currentHand == nil || p.currentHand.handID != handID {
		return
	}

	// Update player state
	p.currentHand.playerStacks[seat] = stack

	// Track folds and all-ins
	if action == "fold" || action == "timeout_fold" {
		p.currentHand.playerFolded[seat] = true
	}
	if action == "allin" || (stack == 0 && action != "fold" && action != "timeout_fold") {
		p.currentHand.playerAllIn[seat] = true
	}

	// Track blinds
	switch action {
	case "post_small_blind":
		p.currentHand.roles[seat] = append(p.currentHand.roles[seat], "small blind")
	case "post_big_blind":
		p.currentHand.roles[seat] = append(p.currentHand.roles[seat], "big blind")
	}

	// Print hole cards header if this is first non-blind action
	if !p.currentHand.printedHole && p.currentHand.currentStreet == "preflop" &&
		action != "post_small_blind" && action != "post_big_blind" {
		p.currentHand.printedHole = true
		fmt.Fprintln(p.writer)
		fmt.Fprintln(p.writer, colorize("*** HOLE CARDS ***", colorBold+colorBlue))
	}

	// Get player name
	playerName := "Unknown"
	for _, player := range p.currentHand.players {
		if player.Seat == seat {
			playerName = player.Name
			break
		}
	}

	// Format and print action
	nameLabel := p.formatActionName(seat, playerName, p.currentHand.playerFolded[seat], p.currentHand.playerAllIn[seat])
	actionDesc := p.describePlayerAction(action, amount, stack)
	fmt.Fprintf(p.writer, "%s: %s\n", nameLabel, actionDesc)
}

// OnStreetChange is called when the street changes
func (p *PrettyPrintMonitor) OnStreetChange(handID string, street string, cards []string) {
	if p.currentHand == nil || p.currentHand.handID != handID {
		return
	}

	// Update board and street
	p.currentHand.board = cards
	p.currentHand.currentStreet = strings.ToLower(street)

	// Print street header
	if street != "preflop" && !p.currentHand.printedStreets[street] {
		p.currentHand.printedStreets[street] = true
		header := formatStreetHeader(street, cards)
		fmt.Fprintln(p.writer)
		fmt.Fprintln(p.writer, colorize(header, colorBold+colorBlue))
	}
}

// OnHandComplete is called after each hand completes
func (p *PrettyPrintMonitor) OnHandComplete(outcome HandOutcome) {
	p.handsComplete++

	if outcome.Detail == nil {
		// No detail available, just print basic result
		fmt.Fprintln(p.writer, colorize("\n*** HAND COMPLETE ***", colorBold+colorBlue))
		fmt.Fprintln(p.writer, colorize("────────────────────────────────────────", colorDim))
		return
	}

	detail := outcome.Detail

	// Determine winners and showdown
	winners := make([]BotHandOutcome, 0)
	showdown := make([]BotHandOutcome, 0)

	for _, bot := range detail.BotOutcomes {
		switch {
		case bot.NetChips > 0:
			winners = append(winners, bot)
		case bot.WentToShowdown:
			showdown = append(showdown, bot)
		}
	}

	// Print showdown if applicable
	hasShowdown := len(showdown) > 0 || (len(winners) > 0 && len(winners[0].HoleCards) > 0)
	if hasShowdown {
		fmt.Fprintln(p.writer)
		fmt.Fprintln(p.writer, colorize("*** SHOWDOWN ***", colorBold+colorBlue))

		// Show winners
		for _, winner := range winners {
			if len(winner.HoleCards) > 0 {
				name := p.formatSummaryName(winner.Bot.DisplayName())
				showLine := fmt.Sprintf("%s: shows %s", name, formatCards(winner.HoleCards))
				// Note: HandRank would need to be added to BotHandOutcome for full detail
				fmt.Fprintln(p.writer, showLine)
			}
		}

		// Show losers
		for _, bot := range showdown {
			name := p.formatSummaryName(bot.Bot.DisplayName())
			line := fmt.Sprintf("%s: shows %s", name, formatCards(bot.HoleCards))
			fmt.Fprintln(p.writer, line)
		}
	}

	// Print winners collecting pot
	for _, winner := range winners {
		name := p.formatSummaryName(winner.Bot.DisplayName())
		fmt.Fprintf(p.writer, "%s collected %s from pot\n", name, formatAmount(winner.NetChips))
	}

	// Print summary
	fmt.Fprintln(p.writer)
	fmt.Fprintln(p.writer, colorize("*** SUMMARY ***", colorBold+colorBlue))
	fmt.Fprintf(p.writer, "Total pot %s | Rake 0\n", formatAmount(detail.TotalPot))
	fmt.Fprintf(p.writer, "Board %s\n", formatBoardAll(detail.Board))

	// Player summaries
	for _, bot := range detail.BotOutcomes {
		seat := bot.Position
		name := bot.Bot.DisplayName()
		nameFmt := p.formatSummaryName(name)

		// Get roles for this seat
		rolesSuffix := ""
		if p.currentHand != nil && len(p.currentHand.roles[seat]) > 0 {
			rolesSuffix = colorize(" ("+strings.Join(p.currentHand.roles[seat], ", ")+")", colorDim)
		}

		line := fmt.Sprintf("Seat %d: %s%s", seat+1, nameFmt, rolesSuffix)

		// Add outcome description
		switch {
		case bot.NetChips > 0:
			line += fmt.Sprintf(" showed %s and won (%s)", formatCards(bot.HoleCards), formatAmountPlain(bot.NetChips))
		case bot.WentToShowdown:
			line += fmt.Sprintf(" showed %s and lost", formatCards(bot.HoleCards))
		default:
			// Get street from actions map
			streetFolded := ""
			for street, action := range bot.Actions {
				if action == "fold" {
					streetFolded = street
					break
				}
			}
			if streetFolded != "" {
				line += fmt.Sprintf(" folded %s", foldDescription(streetFolded))
			} else {
				line += " folded"
			}
		}

		// Add chip delta
		if bot.NetChips != 0 {
			line += fmt.Sprintf(" (%s)", formatDelta(bot.NetChips))
		}

		fmt.Fprintln(p.writer, line)
	}

	fmt.Fprintln(p.writer, colorize("────────────────────────────────────────", colorDim))
}

// Helper methods

func (p *PrettyPrintMonitor) formatPlayerName(seat int, name string, folded, allIn bool) string {
	display := fallbackName(name, seat)
	var suffix string
	if p.currentHand != nil && p.currentHand.roles != nil {
		suffix = formatRolesSuffix(p.currentHand.roles[seat])
	}
	var base string
	switch {
	case folded:
		base = colorize(display, colorDim)
	case allIn:
		base = colorize(display, colorRed+colorBold)
	default:
		base = colorize(display, colorBold)
	}
	if suffix != "" {
		return base + colorize(suffix, colorDim)
	}
	return base
}

func (p *PrettyPrintMonitor) formatActionName(seat int, name string, folded, allIn bool) string {
	display := fallbackName(name, seat)
	var base string
	switch {
	case folded:
		base = colorize(display, colorDim)
	case allIn:
		base = colorize(display, colorRed+colorBold)
	default:
		base = colorize(display, colorBold)
	}

	// Add position abbreviation
	if p.currentHand != nil {
		if pos := getPositionName(seat-p.currentHand.button, len(p.currentHand.players)); pos != "" {
			return base + colorize(fmt.Sprintf(" (%s)", pos), colorDim)
		}
	}
	return base
}

func (p *PrettyPrintMonitor) formatSummaryName(name string) string {
	return colorize(name, colorBold)
}

func (p *PrettyPrintMonitor) describePlayerAction(action string, amount, stack int) string {
	switch action {
	case "fold":
		return colorize("folds", colorDim)
	case "check":
		return colorize("checks", colorBlue)
	case "call":
		if amount > 0 {
			return fmt.Sprintf("calls %s", formatAmount(amount))
		}
		return colorize("calls", colorGreen)
	case "raise":
		if amount > 0 {
			return fmt.Sprintf("raises %s", formatAmount(amount))
		}
		return colorize("raises", colorBold)
	case "allin":
		if amount > 0 {
			return fmt.Sprintf("bets %s %s", formatAmount(amount), colorize("and is all-in", colorRed+colorBold))
		}
		return colorize("moves all-in", colorRed+colorBold)
	case "post_small_blind":
		return fmt.Sprintf("posts small blind %s", formatAmount(amount))
	case "post_big_blind":
		return fmt.Sprintf("posts big blind %s", formatAmount(amount))
	case "timeout_fold":
		return colorize("times out and folds", colorRed)
	case "bet":
		return fmt.Sprintf("bets %s", formatAmount(amount))
	default:
		return colorize(action, colorBold)
	}
}

// Utility functions

func getPositionName(buttonDistance, totalPlayers int) string {
	if totalPlayers == 2 {
		if buttonDistance == 0 {
			return "BTN/SB"
		}
		return "BB"
	}

	// Normalize button distance
	if buttonDistance < 0 {
		buttonDistance += totalPlayers
	}
	buttonDistance %= totalPlayers

	positions := []string{"BTN", "SB", "BB", "UTG", "MP", "MP+1", "MP+2", "HJ", "CO"}
	if buttonDistance < len(positions) {
		return positions[buttonDistance]
	}
	return fmt.Sprintf("UTG+%d", buttonDistance-3)
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

func formatCard(card string) string {
	if len(card) < 2 {
		return card
	}

	rank := strings.ToUpper(card[:1])
	suit := card[len(card)-1]

	var emoji, color string
	switch suit {
	case 's', 'S':
		emoji = "♠"
		color = colorBlue
	case 'h', 'H':
		emoji = "♥"
		color = colorRed
	case 'd', 'D':
		emoji = "♦"
		color = colorYellow
	case 'c', 'C':
		emoji = "♣"
		color = colorGreen
	default:
		emoji = string(suit)
		color = ""
	}

	return colorize(rank+emoji, colorBold+color)
}

func formatAmount(amount int) string {
	return colorize(fmt.Sprintf("%d", amount), colorBold+colorYellow)
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

func formatRolesSuffix(roles []string) string {
	if len(roles) == 0 {
		return ""
	}
	return " (" + strings.Join(roles, ", ") + ")"
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

func colorize(text string, color string) string {
	if color == "" {
		return text
	}
	return color + text + colorReset
}
