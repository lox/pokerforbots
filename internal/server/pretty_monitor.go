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

// PrettyPrintMonitor implements HandMonitor for formatted hand display
type PrettyPrintMonitor struct {
	writer        io.Writer
	handsStarted  uint64
	handsComplete uint64
	handLimit     uint64
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

// OnHandComplete is called after each hand completes
func (p *PrettyPrintMonitor) OnHandComplete(outcome HandOutcome) {
	if outcome.Detail == nil {
		return // No detail to print
	}

	p.handsComplete++
	detail := outcome.Detail

	// Print hand header
	fmt.Fprintln(p.writer)
	fmt.Fprintln(p.writer, colorize(fmt.Sprintf("=== Hand #%d (ID: %s) ===", p.handsComplete, detail.HandID), colorBold+colorMagenta))

	// Group players by outcome
	winners := make([]BotHandOutcome, 0)
	showdown := make([]BotHandOutcome, 0)
	folded := make([]BotHandOutcome, 0)

	for _, bot := range detail.BotOutcomes {
		switch {
		case bot.NetChips > 0:
			winners = append(winners, bot)
		case bot.WentToShowdown:
			showdown = append(showdown, bot)
		default:
			folded = append(folded, bot)
		}
	}

	// Print board
	if len(detail.Board) > 0 {
		fmt.Fprintf(p.writer, "Board: %s\n", formatBoard(detail.Board))
	}

	// Print winners
	if len(winners) > 0 {
		fmt.Fprintln(p.writer, colorize("\n*** WINNERS ***", colorBold+colorGreen))
		for _, winner := range winners {
			p.printBotOutcome(winner, true)
		}
	}

	// Print showdown losers
	if len(showdown) > 0 {
		fmt.Fprintln(p.writer, colorize("\n*** SHOWDOWN ***", colorBold+colorYellow))
		for _, bot := range showdown {
			p.printBotOutcome(bot, false)
		}
	}

	// Print summary of folded players
	if len(folded) > 0 {
		fmt.Fprintln(p.writer, colorize("\n*** FOLDED ***", colorDim))
		for _, bot := range folded {
			name := bot.Bot.DisplayName()
			if name == "" {
				name = bot.Bot.ID[:8]
			}

			// Show position
			pos := getPositionName(bot.ButtonDistance, len(detail.BotOutcomes))

			// Show error status if any
			status := ""
			if bot.TimedOut {
				status = colorize(" (timed out)", colorRed)
			} else if bot.Disconnected {
				status = colorize(" (disconnected)", colorRed)
			}

			fmt.Fprintf(p.writer, "  %s (%s)%s\n", name, pos, status)
		}
	}

	// Print pot summary
	totalPot := 0
	for _, bot := range detail.BotOutcomes {
		if bot.NetChips > 0 {
			totalPot += bot.NetChips
		}
	}

	fmt.Fprintln(p.writer)
	fmt.Fprintf(p.writer, "Total pot: %s | Street: %s\n",
		colorize(fmt.Sprintf("%d", totalPot), colorBold+colorYellow),
		colorize(detail.StreetReached, colorBlue))

	// Show error summary if any
	errors := 0
	timeouts := 0
	disconnects := 0
	for _, bot := range detail.BotOutcomes {
		if bot.TimedOut {
			timeouts++
		}
		if bot.Disconnected {
			disconnects++
		}
		errors += bot.InvalidActions
	}

	if errors > 0 || timeouts > 0 || disconnects > 0 {
		fmt.Fprint(p.writer, colorize("Errors: ", colorRed))
		parts := []string{}
		if timeouts > 0 {
			parts = append(parts, fmt.Sprintf("%d timeout(s)", timeouts))
		}
		if disconnects > 0 {
			parts = append(parts, fmt.Sprintf("%d disconnect(s)", disconnects))
		}
		if errors > 0 {
			parts = append(parts, fmt.Sprintf("%d invalid action(s)", errors))
		}
		fmt.Fprintf(p.writer, "%s\n", strings.Join(parts, ", "))
	}

	fmt.Fprintln(p.writer, colorize("────────────────────────────────────────", colorDim))
}

func (p *PrettyPrintMonitor) printBotOutcome(bot BotHandOutcome, isWinner bool) {
	name := bot.Bot.DisplayName()
	if name == "" {
		name = bot.Bot.ID[:8]
	}

	// Show position
	pos := getPositionName(bot.ButtonDistance, bot.Position+1)

	// Format hole cards
	cards := formatCards(bot.HoleCards)

	// Format result
	result := ""
	if isWinner {
		result = colorize(fmt.Sprintf("+%d", bot.NetChips), colorGreen+colorBold)
	} else {
		result = colorize(fmt.Sprintf("%d", bot.NetChips), colorRed)
	}

	// Show actions summary
	actions := []string{}
	if preflop, ok := bot.Actions["preflop"]; ok && preflop != "fold" {
		actions = append(actions, fmt.Sprintf("pf:%s", shortAction(preflop)))
	}
	if flop, ok := bot.Actions["flop"]; ok {
		actions = append(actions, fmt.Sprintf("f:%s", shortAction(flop)))
	}
	if turn, ok := bot.Actions["turn"]; ok {
		actions = append(actions, fmt.Sprintf("t:%s", shortAction(turn)))
	}
	if river, ok := bot.Actions["river"]; ok {
		actions = append(actions, fmt.Sprintf("r:%s", shortAction(river)))
	}

	actionStr := ""
	if len(actions) > 0 {
		actionStr = " [" + strings.Join(actions, " ") + "]"
	}

	fmt.Fprintf(p.writer, "  %s (%s): %s %s%s\n", name, pos, cards, result, actionStr)
}

func getPositionName(buttonDistance, totalPlayers int) string {
	if totalPlayers == 2 {
		if buttonDistance == 0 {
			return "BTN/SB"
		}
		return "BB"
	}

	positions := []string{"BTN", "SB", "BB", "UTG", "MP", "MP+1", "MP+2", "HJ", "CO"}
	if buttonDistance < len(positions) {
		return positions[buttonDistance]
	}
	return fmt.Sprintf("UTG+%d", buttonDistance-3)
}

func formatBoard(cards []string) string {
	if len(cards) == 0 {
		return colorize("[]", colorDim)
	}

	formatted := make([]string, len(cards))
	for i, card := range cards {
		formatted[i] = formatCard(card)
	}

	result := "["
	if len(formatted) >= 3 {
		result += strings.Join(formatted[:3], " ")
		if len(formatted) >= 4 {
			result += " | " + formatted[3]
			if len(formatted) >= 5 {
				result += " | " + formatted[4]
			}
		}
	} else {
		result += strings.Join(formatted, " ")
	}
	result += "]"

	return result
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

func shortAction(action string) string {
	switch action {
	case "fold":
		return "f"
	case "check":
		return "x"
	case "call":
		return "c"
	case "raise", "bet":
		return "r"
	case "allin":
		return "a"
	default:
		return action[:1]
	}
}

func colorize(text string, color string) string {
	if color == "" {
		return text
	}
	return color + text + colorReset
}
