package game

import (
	"fmt"

	"github.com/lox/pokerforbots/internal/deck"
)

// GetCurrentPlayer returns the current player whose turn it is to act
func (t *Table) GetCurrentPlayer() *Player {
	if t.actionOn >= 0 && t.actionOn < len(t.activePlayers) {
		return t.activePlayers[t.actionOn]
	}
	return nil
}

// AdvanceAction moves to the next player
func (t *Table) AdvanceAction() {
	if t.actionOn == -1 {
		return
	}

	currentPlayer := t.activePlayers[t.actionOn]
	t.playersActed[currentPlayer.ID] = true

	t.actionOn = t.findNextActivePlayer(t.actionOn)
}

// CreateTableState creates a snapshot of the table state for a specific acting player
func (t *Table) CreateTableState(actingPlayer *Player) TableState {
	players := make([]PlayerState, len(t.activePlayers))
	actingIdx := -1

	for i, p := range t.activePlayers {
		players[i] = PlayerState{
			Name:         p.Name,
			Chips:        p.Chips,
			Position:     p.Position,
			BetThisRound: p.BetThisRound,
			TotalBet:     p.TotalBet,
			IsActive:     p.IsActive,
			IsFolded:     p.IsFolded,
			IsAllIn:      p.IsAllIn,
			LastAction:   p.LastAction,
			// HoleCards only visible to the acting player
			HoleCards: func() []deck.Card {
				if p == actingPlayer {
					return p.HoleCards
				}
				return nil // Hidden from other players
			}(),
		}

		if p == actingPlayer {
			actingIdx = i
		}
	}

	return TableState{
		CurrentBet:      t.currentBet,
		Pot:             t.pot,
		CurrentRound:    t.currentRound,
		CommunityCards:  t.communityCards,
		SmallBlind:      t.smallBlind,
		BigBlind:        t.bigBlind,
		Players:         players,
		ActingPlayerIdx: actingIdx,
		HandHistory:     t.handHistory,
	}
}

// RemovePlayer removes a player from the table and handles any necessary cleanup
func (t *Table) RemovePlayer(playerName string) error {
	playerIndex := -1
	activeIndex := -1

	// Find player in main players list
	for i, player := range t.players {
		if player.Name == playerName {
			playerIndex = i
			break
		}
	}

	if playerIndex == -1 {
		return fmt.Errorf("player not found: %s", playerName)
	}

	player := t.players[playerIndex]
	removedChips := player.Chips

	// Find player in active players list
	for i, activePlayer := range t.activePlayers {
		if activePlayer.Name == playerName {
			activeIndex = i
			break
		}
	}

	// If player is in active hand, fold them
	if activeIndex != -1 && player.IsInHand() {
		player.Fold()

		// If this was the current acting player, advance action
		if t.actionOn == activeIndex {
			t.AdvanceAction()
		} else if t.actionOn > activeIndex {
			// Adjust action index since we're removing a player before current action
			t.actionOn--
		}

		// Remove from active players list
		t.activePlayers = append(t.activePlayers[:activeIndex], t.activePlayers[activeIndex+1:]...)
	}

	// Remove from main players list
	t.players = append(t.players[:playerIndex], t.players[playerIndex+1:]...)

	// Track removed chips for conservation validation if we have tracking capability
	// This is a simple workaround - chips are removed from the system when players disconnect
	_ = removedChips // Acknowledge we're intentionally removing chips

	return nil
}

// GetValidActions calculates the valid actions for the current acting player
func (t *Table) GetValidActions() []ValidAction {
	currentPlayer := t.GetCurrentPlayer()
	if currentPlayer == nil || !currentPlayer.CanAct() {
		return []ValidAction{}
	}

	var actions []ValidAction

	// Fold is always available (except when checking is possible)
	callAmount := t.currentBet - currentPlayer.BetThisRound
	if callAmount > 0 {
		actions = append(actions, ValidAction{
			Action:    Fold,
			MinAmount: 0,
			MaxAmount: 0,
		})
	}

	// Check is available when no bet to call
	if callAmount == 0 {
		actions = append(actions, ValidAction{
			Action:    Check,
			MinAmount: 0,
			MaxAmount: 0,
		})
	}

	// Call is available when there's a bet to call and player has chips
	if callAmount > 0 && callAmount <= currentPlayer.Chips {
		actions = append(actions, ValidAction{
			Action:    Call,
			MinAmount: callAmount,
			MaxAmount: callAmount,
		})
	}

	// Raise is available if player has enough chips for minimum raise
	minRaise := t.currentBet + t.minRaise
	totalNeeded := minRaise - currentPlayer.BetThisRound
	if totalNeeded <= currentPlayer.Chips {
		actions = append(actions, ValidAction{
			Action:    Raise,
			MinAmount: minRaise,
			MaxAmount: currentPlayer.BetThisRound + currentPlayer.Chips, // All-in amount
		})
	}

	// All-in is available if player has chips and isn't already all-in
	if currentPlayer.Chips > 0 && !currentPlayer.IsAllIn {
		allInAmount := currentPlayer.BetThisRound + currentPlayer.Chips
		actions = append(actions, ValidAction{
			Action:    AllIn,
			MinAmount: allInAmount,
			MaxAmount: allInAmount,
		})
	}

	return actions
}

// ApplyDecision applies a decision to the table state and returns the reasoning
func (t *Table) ApplyDecision(decision Decision) (string, error) {
	currentPlayer := t.GetCurrentPlayer()
	if currentPlayer == nil {
		return "", fmt.Errorf("no current player")
	}

	if !currentPlayer.CanAct() {
		return "Player cannot act", nil
	}

	// Validate decision against valid actions (Quit and SitOut/SitIn are always valid)
	validActions := t.GetValidActions()
	valid := decision.Action == Quit || decision.Action == SitOut || decision.Action == SitIn

	if !valid {
		for _, validAction := range validActions {
			if validAction.Action == decision.Action {
				if decision.Action == Raise &&
					(decision.Amount < validAction.MinAmount || decision.Amount > validAction.MaxAmount) {
					return "", fmt.Errorf("invalid raise amount: %d (valid range: %d-%d)",
						decision.Amount, validAction.MinAmount, validAction.MaxAmount)
				}
				valid = true
				break
			}
		}
	}

	if !valid {
		return "", fmt.Errorf("invalid action: %s", decision.Action)
	}

	// Apply the decision
	switch decision.Action {
	case Fold:
		currentPlayer.Fold()
	case Call:
		callAmount := t.currentBet - currentPlayer.BetThisRound
		if callAmount > 0 && callAmount <= currentPlayer.Chips {
			currentPlayer.Call(callAmount)
			t.pot += callAmount
		} else {
			currentPlayer.Check()
		}
	case Check:
		currentPlayer.Check()
	case Raise:
		totalNeeded := decision.Amount - currentPlayer.BetThisRound
		if totalNeeded > 0 && totalNeeded <= currentPlayer.Chips {
			// Calculate the size of this raise for future minimum raise calculations
			raiseSize := decision.Amount - t.currentBet

			currentPlayer.Raise(totalNeeded)
			t.pot += totalNeeded
			t.currentBet = decision.Amount

			// Update minimum raise to be the size of this raise
			// This is the correct Texas Hold'em rule
			if raiseSize > 0 {
				t.minRaise = raiseSize
			}
		} else {
			return "", fmt.Errorf("insufficient chips for raise")
		}
	case AllIn:
		allInAmount := currentPlayer.Chips
		if currentPlayer.AllIn() {
			t.pot += allInAmount

			// If this all-in raises the bet, update minimum raise
			if currentPlayer.TotalBet > t.currentBet {
				raiseSize := currentPlayer.TotalBet - t.currentBet
				t.currentBet = currentPlayer.TotalBet

				// Only update MinRaise if this all-in is a raise (not just a call)
				if raiseSize >= t.minRaise {
					t.minRaise = raiseSize
				}
			}
		}
	case Quit:
		// Player wants to quit - this will be handled at the engine level
		// For now, just set the action on the player
		currentPlayer.LastAction = Quit
		return decision.Reasoning, nil
	case SitOut:
		// Player wants to sit out - fold current hand and mark as sitting out
		currentPlayer.SitOut()

		// Publish sit-out event
		if t.eventBus != nil {
			event := NewPlayerActionEvent(currentPlayer, SitOut, 0, t.currentRound, decision.Reasoning, t.pot)
			t.eventBus.Publish(event)
		}

		return decision.Reasoning, nil
	case SitIn:
		// Player wants to return from sitting out
		currentPlayer.SitIn()

		// Publish sit-in event
		if t.eventBus != nil {
			event := NewPlayerActionEvent(currentPlayer, SitIn, 0, t.currentRound, decision.Reasoning, t.pot)
			t.eventBus.Publish(event)
		}

		return decision.Reasoning, nil
	}

	return decision.Reasoning, nil
}

// ValidateChipConservation ensures that the total chips in the game equals the expected amount
// This is a critical invariant - chips should never be created or destroyed
func (t *Table) ValidateChipConservation(expectedTotal int) error {
	actualTotal := 0

	// Count chips held by all players
	for _, player := range t.players {
		actualTotal += player.Chips
	}

	// Add chips currently in the pot (if any)
	actualTotal += t.pot

	if actualTotal != expectedTotal {
		return fmt.Errorf("chip conservation violation: expected %d total chips, but found %d (difference: %d)",
			expectedTotal, actualTotal, actualTotal-expectedTotal)
	}

	return nil
}

// GetTotalChips returns the current total chips in the game (player chips + pot)
func (t *Table) GetTotalChips() int {
	total := t.pot
	for _, player := range t.players {
		total += player.Chips
	}
	return total
}
