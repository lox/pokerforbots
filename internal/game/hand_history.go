package game

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lox/holdem-cli/internal/deck"
)

// HandAction represents a single action taken during a hand
type HandAction struct {
	PlayerName string       // Name of the player who acted
	Action     Action       // The action taken (fold, call, raise, etc)
	Amount     int          // Amount involved (for bets/raises)
	PotAfter   int          // Pot size after this action
	Round      BettingRound // Which betting round this occurred in
	Thinking   string       // AI reasoning (empty for human players)
	Timestamp  time.Time    // When the action occurred
}

// HandHistory tracks all actions and metadata for a single hand
type HandHistory struct {
	HandID         string
	StartTime      time.Time
	SmallBlind     int
	BigBlind       int
	DealerPosition int
	Players        []PlayerSnapshot // Player state at hand start
	Actions        []HandAction     // All actions taken during the hand
	CommunityCards []deck.Card      // Final community cards
	FinalPot       int
	Winners        []WinnerInfo
}

// PlayerSnapshot captures player state at the start of a hand
type PlayerSnapshot struct {
	Name      string
	Chips     int
	Position  Position
	HoleCards []deck.Card // Only filled if hand goes to showdown or player folds
}

// WinnerInfo captures winner information
type WinnerInfo struct {
	PlayerName string
	Amount     int
	HoleCards  []deck.Card
	HandRank   string
}

// NewHandHistory creates a new hand history for a hand
func NewHandHistory(table *Table) *HandHistory {
	players := make([]PlayerSnapshot, len(table.Players))
	for i, player := range table.Players {
		players[i] = PlayerSnapshot{
			Name:     player.Name,
			Chips:    player.Chips,
			Position: player.Position,
			// HoleCards filled later when appropriate
		}
	}

	return &HandHistory{
		HandID:         table.HandID,
		StartTime:      time.Now(),
		SmallBlind:     table.SmallBlind,
		BigBlind:       table.BigBlind,
		DealerPosition: table.DealerPosition,
		Players:        players,
		Actions:        make([]HandAction, 0),
		FinalPot:       0,
		Winners:        make([]WinnerInfo, 0),
	}
}

// AddAction records a new action in the hand history
func (hh *HandHistory) AddAction(playerName string, action Action, amount int, potAfter int, round BettingRound, thinking string) {
	hh.Actions = append(hh.Actions, HandAction{
		PlayerName: playerName,
		Action:     action,
		Amount:     amount,
		PotAfter:   potAfter,
		Round:      round,
		Thinking:   thinking,
		Timestamp:  time.Now(),
	})
}

// AddPlayerHoleCards adds hole cards for a player (for showdown or when folding)
func (hh *HandHistory) AddPlayerHoleCards(playerName string, holeCards []deck.Card) {
	for i := range hh.Players {
		if hh.Players[i].Name == playerName {
			hh.Players[i].HoleCards = make([]deck.Card, len(holeCards))
			copy(hh.Players[i].HoleCards, holeCards)
			break
		}
	}
}

// SetFinalResults sets the final pot and winner information
func (hh *HandHistory) SetFinalResults(pot int, winners []WinnerInfo) {
	hh.FinalPot = pot
	hh.Winners = winners
}

// SetCommunityCards sets the final community cards
func (hh *HandHistory) SetCommunityCards(cards []deck.Card) {
	hh.CommunityCards = make([]deck.Card, len(cards))
	copy(hh.CommunityCards, cards)
}

// GenerateHistoryText creates a formatted text representation of the hand
func (hh *HandHistory) GenerateHistoryText() string {
	var history string

	// Header
	history += fmt.Sprintf("=== HAND %s ===\n", hh.HandID)
	history += fmt.Sprintf("Date: %s\n", hh.StartTime.Format("2006-01-02 15:04:05"))
	history += fmt.Sprintf("Blinds: %d/%d\n", hh.SmallBlind, hh.BigBlind)
	history += fmt.Sprintf("Players: %d\n", len(hh.Players))
	history += fmt.Sprintf("Dealer Position: %d\n\n", hh.DealerPosition)

	// Starting positions and chip counts
	history += "STARTING POSITIONS:\n"
	for i, player := range hh.Players {
		positionStr := hh.getPositionString(player.Position, i)
		history += fmt.Sprintf("Seat %d: %s (%d chips)%s\n", i+1, player.Name, player.Chips, positionStr)
	}
	history += "\n"

	// Hole cards (only show if known)
	history += "HOLE CARDS:\n"
	for _, player := range hh.Players {
		if len(player.HoleCards) > 0 {
			history += fmt.Sprintf("%s: %s %s\n", player.Name,
				player.HoleCards[0].String(), player.HoleCards[1].String())
		}
	}
	history += "\n"

	// Hand actions grouped by round
	if len(hh.Actions) > 0 {
		history += "HAND ACTION:\n"
		currentRound := PreFlop
		roundShown := make(map[BettingRound]bool)

		for _, action := range hh.Actions {
			// Show round header if it's a new round
			if action.Round != currentRound || !roundShown[action.Round] {
				if !roundShown[action.Round] {
					switch action.Round {
					case PreFlop:
						history += "*** PRE-FLOP ***\n"
					case Flop:
						history += "\n*** FLOP ***\n"
						if len(hh.CommunityCards) >= 3 {
							history += fmt.Sprintf("Board: [%s %s %s]\n",
								hh.CommunityCards[0].String(),
								hh.CommunityCards[1].String(),
								hh.CommunityCards[2].String())
						}
					case Turn:
						history += "\n*** TURN ***\n"
						if len(hh.CommunityCards) >= 4 {
							history += fmt.Sprintf("Board: [%s %s %s %s]\n",
								hh.CommunityCards[0].String(),
								hh.CommunityCards[1].String(),
								hh.CommunityCards[2].String(),
								hh.CommunityCards[3].String())
						}
					case River:
						history += "\n*** RIVER ***\n"
						if len(hh.CommunityCards) >= 5 {
							history += fmt.Sprintf("Board: [%s %s %s %s %s]\n",
								hh.CommunityCards[0].String(),
								hh.CommunityCards[1].String(),
								hh.CommunityCards[2].String(),
								hh.CommunityCards[3].String(),
								hh.CommunityCards[4].String())
						}
					case Showdown:
						history += "\n*** SHOWDOWN ***\n"
					}
					roundShown[action.Round] = true
					currentRound = action.Round
				}
			}

			// Add AI thinking before the action if present
			if action.Thinking != "" {
				history += fmt.Sprintf("%s: thinks \"%s\"\n", action.PlayerName, action.Thinking)
			}

			// Format the action
			actionText := hh.formatAction(action)
			history += fmt.Sprintf("%s\n", actionText)
		}
		history += "\n"
	}

	// Final results and summary
	if hh.FinalPot > 0 || len(hh.Winners) > 0 {
		history += hh.GenerateSummary(SummaryOpts{
			ShowHoleCards: true,
		})
	}

	history += "=== END HAND ===\n"
	return history
}

// formatAction creates a human-readable string for an action
func (hh *HandHistory) formatAction(action HandAction) string {
	switch action.Action {
	case Fold:
		return fmt.Sprintf("%s: folds", action.PlayerName)
	case Call:
		// Check if this is a blind posting (first actions in preflop with specific amounts)
		if action.Round == PreFlop && hh.isBlindPosting(action) {
			switch action.Amount {
			case hh.SmallBlind:
				return fmt.Sprintf("%s: posts small blind $%d", action.PlayerName, action.Amount)
			case hh.BigBlind:
				return fmt.Sprintf("%s: posts big blind $%d", action.PlayerName, action.Amount)
			}
		}
		return fmt.Sprintf("%s: calls $%d (pot now: $%d)", action.PlayerName, action.Amount, action.PotAfter)
	case Check:
		return fmt.Sprintf("%s: checks", action.PlayerName)
	case Raise:
		return fmt.Sprintf("%s: raises $%d (pot now: $%d)", action.PlayerName, action.Amount, action.PotAfter)
	case AllIn:
		return fmt.Sprintf("%s: goes all-in for $%d (pot now: $%d)", action.PlayerName, action.Amount, action.PotAfter)
	default:
		return fmt.Sprintf("%s: %s $%d", action.PlayerName, action.Action.String(), action.Amount)
	}
}

// isBlindPosting determines if a call action is actually a blind posting
func (hh *HandHistory) isBlindPosting(action HandAction) bool {
	// Count how many actions have occurred before this one in preflop
	actionCount := 0
	for _, a := range hh.Actions {
		if a.Round == PreFlop && a.Timestamp.Before(action.Timestamp) {
			actionCount++
		}
	}

	// First two actions in preflop with amounts matching blinds are blind posts
	return actionCount < 2 && (action.Amount == hh.SmallBlind || action.Amount == hh.BigBlind)
}

// getPositionString returns the position indicator for display
func (hh *HandHistory) getPositionString(position Position, seatIndex int) string {
	// Button position is always [D] and takes precedence
	if position == Button {
		return " [D]"
	}

	// Other special positions
	switch position {
	case SmallBlind:
		return " [SB]"
	case BigBlind:
		return " [BB]"
	case UnderTheGun:
		return " [UTG]"
	case EarlyPosition:
		return " [EP]"
	case MiddlePosition:
		return " [MP]"
	case LatePosition:
		return " [LP]"
	case Cutoff:
		return " [CO]"
	default:
		// For other positions, check if this seat is the dealer (shouldn't happen if positions are set correctly)
		if seatIndex == hh.DealerPosition {
			return " [D]"
		}
		return ""
	}
}

// playerFolded checks if a player folded during the hand
func (hh *HandHistory) playerFolded(playerName string) bool {
	for _, action := range hh.Actions {
		if action.PlayerName == playerName && action.Action == Fold {
			return true
		}
	}
	return false
}

// getPlayerLoss calculates how much a player lost (total amount they bet)
func (hh *HandHistory) getPlayerLoss(playerName string) int {
	totalBet := 0
	for _, action := range hh.Actions {
		if action.PlayerName == playerName && action.Amount > 0 {
			totalBet += action.Amount
		}
	}
	return totalBet
}

// SummaryOpts configures how the hand summary is displayed
type SummaryOpts struct {
	ShowHoleCards     bool   // Show all hole cards (for hand history)
	PlayerPerspective string // Show only this player's cards (for TUI)
}

// GenerateSummary creates a formatted summary section with configurable options
func (hh *HandHistory) GenerateSummary(opts SummaryOpts) string {
	var summary string

	summary += "*** SUMMARY ***\n"
	summary += fmt.Sprintf("Total pot $%d\n", hh.FinalPot)

	// Show final board if available
	if len(hh.CommunityCards) > 0 {
		boardStr := ""
		for i, card := range hh.CommunityCards {
			if i > 0 {
				boardStr += " "
			}
			boardStr += card.String()
		}
		summary += fmt.Sprintf("Board [%s]\n", boardStr)
	}

	// Show each player's result
	for i, player := range hh.Players {
		seatInfo := fmt.Sprintf("Seat %d: %s", i+1, player.Name)

		// Add position info
		switch player.Position {
		case Button:
			seatInfo += " (button)"
		case SmallBlind:
			seatInfo += " (small blind)"
		case BigBlind:
			seatInfo += " (big blind)"
		}

		// Determine if this player won
		var winner *WinnerInfo
		for _, w := range hh.Winners {
			if w.PlayerName == player.Name {
				winner = &w
				break
			}
		}

		// Determine if we should show this player's hole cards
		playerFolded := hh.playerFolded(player.Name)
		showCards := opts.ShowHoleCards ||
			(opts.PlayerPerspective != "" && player.Name == opts.PlayerPerspective) ||
			!playerFolded // Show cards for players who reached showdown

		if winner != nil {
			// Player won
			if showCards && len(player.HoleCards) > 0 {
				seatInfo += fmt.Sprintf(" showed [%s %s] and won ($%d)",
					player.HoleCards[0].String(), player.HoleCards[1].String(), winner.Amount)
				if winner.HandRank != "" {
					seatInfo += fmt.Sprintf(" with %s", winner.HandRank)
				}
			} else {
				seatInfo += fmt.Sprintf(" won ($%d)", winner.Amount)
				if winner.HandRank != "" {
					seatInfo += fmt.Sprintf(" with %s", winner.HandRank)
				}
			}
		} else {
			// Player didn't win
			if playerFolded {
				seatInfo += " folded"
			} else if showCards && len(player.HoleCards) > 0 {
				seatInfo += fmt.Sprintf(" mucked [%s %s]",
					player.HoleCards[0].String(), player.HoleCards[1].String())
			}

			// Add loss amount if they contributed to pot
			lossAmount := hh.getPlayerLoss(player.Name)
			if lossAmount > 0 {
				seatInfo += fmt.Sprintf(" and lost $%d", lossAmount)
			}
		}

		summary += fmt.Sprintf("%s\n", seatInfo)
	}
	summary += "\n"

	return summary
}

// SaveToFile saves the hand history to a file
func (hh *HandHistory) SaveToFile() error {
	// Ensure handhistory directory exists
	if err := os.MkdirAll("handhistory", 0755); err != nil {
		return fmt.Errorf("failed to create handhistory directory: %w", err)
	}

	// Generate filename
	filename := filepath.Join("handhistory", fmt.Sprintf("hand_%s.txt", hh.HandID))

	// Generate content
	content := hh.GenerateHistoryText()

	// Write to file
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write hand history file: %w", err)
	}

	return nil
}

// GetDisplayActions returns formatted action strings for display in TUI
func (hh *HandHistory) GetDisplayActions() []string {
	actions := make([]string, 0, len(hh.Actions)*2) // *2 to account for thinking entries

	currentRound := PreFlop
	roundShown := make(map[BettingRound]bool)

	for _, action := range hh.Actions {
		// Add round header if needed
		if action.Round != currentRound || !roundShown[action.Round] {
			if !roundShown[action.Round] {
				switch action.Round {
				case PreFlop:
					actions = append(actions, "*** PRE-FLOP ***")
				case Flop:
					actions = append(actions, "*** FLOP ***")
				case Turn:
					actions = append(actions, "*** TURN ***")
				case River:
					actions = append(actions, "*** RIVER ***")
				case Showdown:
					actions = append(actions, "*** SHOWDOWN ***")
				}
				roundShown[action.Round] = true
				currentRound = action.Round
			}
		}

		// Add the action (no thinking for TUI display to preserve poker experience)
		actions = append(actions, hh.formatAction(action))
	}

	return actions
}
