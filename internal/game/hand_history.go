package game

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lox/pokerforbots/internal/deck"
)

// HandHistoryWriter interface for writing hand history
type HandHistoryWriter interface {
	WriteHandHistory(handID string, content string) error
}

// FileHandHistoryWriter writes hand history to files
type FileHandHistoryWriter struct {
	directory string
}

// NewFileHandHistoryWriter creates a new file-based hand history writer
func NewFileHandHistoryWriter(directory string) *FileHandHistoryWriter {
	return &FileHandHistoryWriter{directory: directory}
}

// WriteHandHistory writes hand history to a file
func (w *FileHandHistoryWriter) WriteHandHistory(handID string, content string) error {
	// Ensure directory exists
	if err := os.MkdirAll(w.directory, 0755); err != nil {
		return fmt.Errorf("failed to create hand history directory: %w", err)
	}

	// Generate filename
	filename := filepath.Join(w.directory, fmt.Sprintf("hand_%s.txt", handID))

	// Write to file
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write hand history file: %w", err)
	}

	return nil
}

// NoOpHandHistoryWriter is a no-op writer for tests
type NoOpHandHistoryWriter struct{}

// WriteHandHistory does nothing (for tests)
func (w *NoOpHandHistoryWriter) WriteHandHistory(handID string, content string) error {
	return nil
}

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
	Seed           int64 // Random seed for exact reproduction
	SmallBlind     int
	BigBlind       int
	DealerPosition int
	Players        []PlayerSnapshot // Player state at hand start
	Actions        []HandAction     // All actions taken during the hand
	CommunityCards []deck.Card      // Final community cards
	FinalPot       int
	Winners        []WinnerInfo

	// Configuration
	writer HandHistoryWriter // Writer for saving hand history
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
func NewHandHistory(table *Table, seed int64, writer HandHistoryWriter) *HandHistory {
	players := make([]PlayerSnapshot, len(table.players))
	for i, player := range table.players {
		players[i] = PlayerSnapshot{
			Name:     player.Name,
			Chips:    player.Chips,
			Position: player.Position,
			// HoleCards filled later when appropriate
		}
	}

	return &HandHistory{
		HandID:         table.handID,
		StartTime:      time.Now(),
		Seed:           seed,
		SmallBlind:     table.smallBlind,
		BigBlind:       table.bigBlind,
		DealerPosition: table.dealerPosition,
		Players:        players,
		Actions:        make([]HandAction, 0),
		FinalPot:       0,
		Winners:        make([]WinnerInfo, 0),
		writer:         writer,
	}
}

// OnEvent implements EventSubscriber interface
func (hh *HandHistory) OnEvent(event GameEvent) {
	switch e := event.(type) {
	case PlayerActionEvent:
		hh.addAction(e.Player.Name, e.Action, e.Amount, e.PotAfter, e.Round, e.Reasoning)
	case StreetChangeEvent:
		hh.SetCommunityCards(e.CommunityCards)
	case HandStartEvent:
		// Hand history is already created when this event fires, but we could reset if needed
	case HandEndEvent:
		// Could record final results here if needed
	}
}

// AddAction records a new action in the hand history (used for automatic actions like blinds)
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

// addAction is a private wrapper for event handling
func (hh *HandHistory) addAction(playerName string, action Action, amount int, potAfter int, round BettingRound, thinking string) {
	hh.AddAction(playerName, action, amount, potAfter, round, thinking)
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
	history += fmt.Sprintf("Seed: %d\n", hh.Seed)
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

// formatAction creates a human-readable string for an action using EventFormatter
func (hh *HandHistory) formatAction(action HandAction) string {
	// Create a temporary player for the action (needed for position-based blind detection)
	player := &Player{
		Name: action.PlayerName,
		// Determine position from the action context if it's a blind posting
		Position: hh.getPlayerPositionForAction(action),
	}

	// Convert HandAction to PlayerActionEvent for the formatter
	event := PlayerActionEvent{
		Player:    player,
		Action:    action.Action,
		Amount:    action.Amount,
		Round:     action.Round,
		Reasoning: action.Thinking,
		PotAfter:  action.PotAfter,
		timestamp: action.Timestamp,
	}

	// Use EventFormatter to format the action
	formatter := NewEventFormatter(FormattingOptions{
		ShowReasonings: false, // Don't show AI thinking in hand history by default
		ShowTimeouts:   false, // Hand history tracks timeouts differently
	})

	return formatter.FormatPlayerAction(event)
}

// getPlayerPositionForAction determines a player's position for action formatting
func (hh *HandHistory) getPlayerPositionForAction(action HandAction) Position {
	// Find the player in the snapshot to get their position
	for _, player := range hh.Players {
		if player.Name == action.PlayerName {
			return player.Position
		}
	}
	return UnknownPosition
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

// SaveToFile saves the hand history using the configured writer
func (hh *HandHistory) SaveToFile() error {
	// Generate content
	content := hh.GenerateHistoryText()

	// Write using the configured writer
	return hh.writer.WriteHandHistory(hh.HandID, content)
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

// BettingRoundSummary provides analysis of betting action for a round
type BettingRoundSummary struct {
	Round         BettingRound
	Actions       []HandAction
	NumRaises     int
	NumCallers    int
	LastAggressor string
	InitialBet    int
	CurrentBet    int
}

// BetSizingInfo provides analysis of individual bets
type BetSizingInfo struct {
	PlayerName string
	Amount     int
	PotBefore  int
	Ratio      float64 // bet size / pot size
}

// GetCurrentRoundActions returns all actions for the specified betting round
func (hh *HandHistory) GetCurrentRoundActions(currentRound BettingRound) []HandAction {
	var actions []HandAction
	for _, action := range hh.Actions {
		if action.Round == currentRound {
			actions = append(actions, action)
		}
	}
	return actions
}

// GetBettingRoundSummary analyzes the betting action for a specific round
func (hh *HandHistory) GetBettingRoundSummary(currentRound BettingRound) BettingRoundSummary {
	actions := hh.GetCurrentRoundActions(currentRound)

	summary := BettingRoundSummary{
		Round:   currentRound,
		Actions: actions,
	}

	// Count raises and find aggressor
	for _, action := range actions {
		switch action.Action {
		case Raise:
			summary.NumRaises++
			summary.LastAggressor = action.PlayerName
			summary.CurrentBet = action.PotAfter - (action.PotAfter - action.Amount) // This is the new bet level
		case Call:
			// Skip blind posts for counting callers
			if currentRound != PreFlop || !hh.isBlindPosting(action) {
				summary.NumCallers++
			}
		}
	}

	// Set initial bet (big blind for preflop, 0 for other rounds)
	if currentRound == PreFlop {
		summary.InitialBet = hh.BigBlind
	}

	return summary
}

// GetBetSizingInfo analyzes bet sizing patterns for a round
func (hh *HandHistory) GetBetSizingInfo(currentRound BettingRound) []BetSizingInfo {
	actions := hh.GetCurrentRoundActions(currentRound)
	var sizing []BetSizingInfo

	for _, action := range actions {
		if action.Action == Raise {
			// Calculate pot size before this bet
			potBefore := action.PotAfter - action.Amount

			sizing = append(sizing, BetSizingInfo{
				PlayerName: action.PlayerName,
				Amount:     action.Amount,
				PotBefore:  potBefore,
				Ratio:      float64(action.Amount) / float64(potBefore),
			})
		}
	}

	return sizing
}

// GetPlayerActions returns all actions by a specific player in this hand
func (hh *HandHistory) GetPlayerActions(playerName string) []HandAction {
	var actions []HandAction
	for _, action := range hh.Actions {
		if action.PlayerName == playerName {
			actions = append(actions, action)
		}
	}
	return actions
}

// HasPlayerActed checks if a player has taken any action in the specified round
func (hh *HandHistory) HasPlayerActed(playerName string, round BettingRound) bool {
	for _, action := range hh.Actions {
		if action.PlayerName == playerName && action.Round == round {
			return true
		}
	}
	return false
}

// GetNewActions returns actions that occurred after the given index
func (hh *HandHistory) GetNewActions(lastActionIndex int) []HandAction {
	if lastActionIndex < 0 || lastActionIndex >= len(hh.Actions) {
		return hh.Actions
	}
	return hh.Actions[lastActionIndex+1:]
}

// GetActionCount returns the total number of actions recorded
func (hh *HandHistory) GetActionCount() int {
	return len(hh.Actions)
}

// GetDisplayActionsSince returns formatted action strings for TUI display starting from the given index
func (hh *HandHistory) GetDisplayActionsSince(lastActionIndex int) []string {
	newActions := hh.GetNewActions(lastActionIndex)
	if len(newActions) == 0 {
		return []string{}
	}

	var displayActions []string
	currentRound := PreFlop
	if lastActionIndex >= 0 && lastActionIndex < len(hh.Actions) {
		currentRound = hh.Actions[lastActionIndex].Round
	}

	for _, action := range newActions {
		// Add round header if we've moved to a new round
		if action.Round != currentRound {
			switch action.Round {
			case Flop:
				displayActions = append(displayActions, "")
				displayActions = append(displayActions, "*** FLOP ***")
				if len(hh.CommunityCards) >= 3 {
					flop := hh.CommunityCards[:3]
					boardDisplay := fmt.Sprintf("Board: [%s %s %s]",
						flop[0].String(), flop[1].String(), flop[2].String())
					displayActions = append(displayActions, boardDisplay)
				}
			case Turn:
				displayActions = append(displayActions, "")
				displayActions = append(displayActions, "*** TURN ***")
				if len(hh.CommunityCards) >= 4 {
					boardDisplay := fmt.Sprintf("Board: [%s %s %s %s]",
						hh.CommunityCards[0].String(), hh.CommunityCards[1].String(),
						hh.CommunityCards[2].String(), hh.CommunityCards[3].String())
					displayActions = append(displayActions, boardDisplay)
				}
			case River:
				displayActions = append(displayActions, "")
				displayActions = append(displayActions, "*** RIVER ***")
				if len(hh.CommunityCards) >= 5 {
					boardDisplay := fmt.Sprintf("Board: [%s %s %s %s %s]",
						hh.CommunityCards[0].String(), hh.CommunityCards[1].String(),
						hh.CommunityCards[2].String(), hh.CommunityCards[3].String(),
						hh.CommunityCards[4].String())
					displayActions = append(displayActions, boardDisplay)
				}
			case Showdown:
				displayActions = append(displayActions, "")
				displayActions = append(displayActions, "*** SHOWDOWN ***")
			}
			currentRound = action.Round
		}

		// Add the action (no thinking for TUI to preserve poker experience)
		displayActions = append(displayActions, hh.formatAction(action))
	}

	return displayActions
}
