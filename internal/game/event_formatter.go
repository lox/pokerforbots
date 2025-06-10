package game

import (
	"fmt"
	"strings"

	"github.com/lox/pokerforbots/internal/deck"
)

// FormattingOptions controls how events are formatted for different contexts
type FormattingOptions struct {
	ShowReasonings bool   // Include AI reasoning (for hand history analysis)
	ShowTimeouts   bool   // Include timeout information (for TUI)
	ShowHoleCards  bool   // Include hole cards in summaries (for hand history)
	Perspective    string // Player name for personalized formatting
}

// EventFormatter provides centralized formatting for all game events
type EventFormatter struct {
	opts FormattingOptions
}

// NewEventFormatter creates a new event formatter with the given options
func NewEventFormatter(opts FormattingOptions) *EventFormatter {
	return &EventFormatter{opts: opts}
}

// FormatPlayerAction formats a player action event into a human-readable string
func (ef *EventFormatter) FormatPlayerAction(event PlayerActionEvent) string {
	playerName := event.Player.Name
	action := event.Action
	amount := event.Amount
	potAfter := event.PotAfter
	reasoning := event.Reasoning

	// Check for timeout in reasoning
	isTimeout := ef.opts.ShowTimeouts && (strings.Contains(reasoning, "timeout") || strings.Contains(reasoning, "Decision timeout"))

	var actionText string
	switch action {
	case Fold:
		if isTimeout {
			actionText = fmt.Sprintf("%s: times out and folds", playerName)
		} else {
			actionText = fmt.Sprintf("%s: folds", playerName)
		}
	case Call:
		// Check if this is a blind posting based on reasoning or position
		if ef.isBlindPosting(event) {
			switch event.Player.Position {
			case SmallBlind:
				actionText = fmt.Sprintf("%s: posts small blind $%d", playerName, amount)
			case BigBlind:
				actionText = fmt.Sprintf("%s: posts big blind $%d", playerName, amount)
			default:
				actionText = fmt.Sprintf("%s: calls $%d (pot now: $%d)", playerName, amount, potAfter)
			}
		} else {
			actionText = fmt.Sprintf("%s: calls $%d (pot now: $%d)", playerName, amount, potAfter)
		}
	case Check:
		if isTimeout {
			actionText = fmt.Sprintf("%s: times out and checks", playerName)
		} else {
			actionText = fmt.Sprintf("%s: checks", playerName)
		}
	case Raise:
		actionText = fmt.Sprintf("%s: raises $%d (pot now: $%d)", playerName, amount, potAfter)
	case AllIn:
		actionText = fmt.Sprintf("%s: goes all-in for $%d (pot now: $%d)", playerName, amount, potAfter)
	case SitOut:
		actionText = fmt.Sprintf("%s: sits out", playerName)
	case SitIn:
		actionText = fmt.Sprintf("%s: sits back in", playerName)
	default:
		actionText = fmt.Sprintf("%s: %s $%d", playerName, action.String(), amount)
	}

	// Add reasoning if requested and available
	if ef.opts.ShowReasonings && reasoning != "" && !isTimeout {
		actionText += fmt.Sprintf(" (%s)", reasoning)
	}

	return actionText
}

// FormatStreetChange formats a street change event into a human-readable string
func (ef *EventFormatter) FormatStreetChange(event StreetChangeEvent) string {
	switch event.Round {
	case Flop:
		if len(event.CommunityCards) >= 3 {
			flop := event.CommunityCards[:3]
			return fmt.Sprintf("*** FLOP *** [%s]", ef.formatCards(flop))
		}
		return "*** FLOP ***"
	case Turn:
		if len(event.CommunityCards) >= 4 {
			board := ef.formatCards(event.CommunityCards[:3])
			turn := event.CommunityCards[3].String()
			return fmt.Sprintf("*** TURN *** [%s] [%s]", board, turn)
		}
		return "*** TURN ***"
	case River:
		if len(event.CommunityCards) >= 5 {
			board := ef.formatCards(event.CommunityCards[:4])
			river := event.CommunityCards[4].String()
			return fmt.Sprintf("*** RIVER *** [%s] [%s]", board, river)
		}
		return "*** RIVER ***"
	case Showdown:
		if len(event.CommunityCards) > 0 {
			return fmt.Sprintf("*** SHOWDOWN *** [%s]", ef.formatCards(event.CommunityCards))
		}
		return "*** SHOWDOWN ***"
	default:
		return fmt.Sprintf("*** %s ***", event.Round.String())
	}
}

// FormatHandStart formats a hand start event into a human-readable string
func (ef *EventFormatter) FormatHandStart(event HandStartEvent) string {
	return fmt.Sprintf("Hand %s • %d players • $%d/$%d",
		event.HandID, len(event.Players), event.SmallBlind, event.BigBlind)
}

// FormatHandEnd formats a hand end event into a human-readable string
func (ef *EventFormatter) FormatHandEnd(event HandEndEvent) string {
	var result strings.Builder

	result.WriteString(fmt.Sprintf("=== Hand %s Complete ===\n", event.HandID))
	result.WriteString(fmt.Sprintf("Pot: $%d\n", event.PotSize))

	for _, winner := range event.Winners {
		winnerText := fmt.Sprintf("Winner: %s ($%d)", winner.PlayerName, winner.Amount)
		if winner.HandRank != "" {
			winnerText += fmt.Sprintf(" - %s", winner.HandRank)
		}

		// Add hole cards if showing cards and available
		if ef.opts.ShowHoleCards && len(winner.HoleCards) > 0 {
			winnerText += fmt.Sprintf(" [%s]", ef.formatCards(winner.HoleCards))
		}

		result.WriteString(winnerText + "\n")
	}

	return result.String()
}

// FormatHoleCards formats hole cards for a specific player
func (ef *EventFormatter) FormatHoleCards(playerName string, cards []deck.Card) string {
	if len(cards) == 0 {
		return ""
	}

	// Only show hole cards if it's the perspective player or if showing all cards
	if ef.opts.Perspective != "" && playerName != ef.opts.Perspective && !ef.opts.ShowHoleCards {
		return "" // Hidden
	}

	return fmt.Sprintf("Dealt to %s: [%s]", playerName, ef.formatCards(cards))
}

// formatCards formats a slice of cards with appropriate styling
func (ef *EventFormatter) formatCards(cards []deck.Card) string {
	if len(cards) == 0 {
		return ""
	}

	var formatted []string
	for _, card := range cards {
		formatted = append(formatted, card.String())
	}

	return strings.Join(formatted, " ")
}

// isBlindPosting determines if a call action is actually a blind posting
func (ef *EventFormatter) isBlindPosting(event PlayerActionEvent) bool {
	// Check if it's preflop and the player is in a blind position
	if event.Round != PreFlop || event.Action != Call {
		return false
	}

	// Check if the reasoning indicates it's a blind posting
	if strings.Contains(event.Reasoning, "blind") || strings.Contains(event.Reasoning, "Blind") {
		return true
	}

	// For now, don't assume position alone determines blind posting
	// since we need more context about timing and game state
	return false
}
