package game

import (
	"testing"
)

func TestHandStateCreation(t *testing.T) {
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(players, 0, 5, 10, 1000)

	if len(h.Players) != 3 {
		t.Errorf("Expected 3 players, got %d", len(h.Players))
	}

	// Check blinds were posted
	if h.Players[1].TotalBet != 5 {
		t.Errorf("Small blind not posted correctly: %d", h.Players[1].TotalBet)
	}
	if h.Players[2].TotalBet != 10 {
		t.Errorf("Big blind not posted correctly: %d", h.Players[2].TotalBet)
	}

	// Check chips were deducted
	if h.Players[1].Chips != 995 {
		t.Errorf("Small blind chips not deducted: %d", h.Players[1].Chips)
	}
	if h.Players[2].Chips != 990 {
		t.Errorf("Big blind chips not deducted: %d", h.Players[2].Chips)
	}

	// Check pot
	if h.Pots[0].Amount != 15 {
		t.Errorf("Initial pot incorrect: %d", h.Pots[0].Amount)
	}

	// Check cards were dealt
	for _, p := range h.Players {
		if p.HoleCards.CountCards() != 2 {
			t.Errorf("Player %s has %d hole cards, expected 2", p.Name, p.HoleCards.CountCards())
		}
	}
}

func TestGetValidActions(t *testing.T) {
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(players, 0, 5, 10, 1000)

	// First player to act (Alice, UTG)
	actions := h.GetValidActions()

	hasAction := func(actions []Action, target Action) bool {
		for _, a := range actions {
			if a == target {
				return true
			}
		}
		return false
	}

	// Should be able to fold, call, or raise
	if !hasAction(actions, Fold) {
		t.Error("Should be able to fold")
	}
	if !hasAction(actions, Call) {
		t.Error("Should be able to call")
	}
	if !hasAction(actions, Raise) {
		t.Error("Should be able to raise")
	}
	if hasAction(actions, Check) {
		t.Error("Should not be able to check (facing a bet)")
	}
}

func TestProcessAction(t *testing.T) {
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(players, 0, 5, 10, 1000)

	// Alice calls
	err := h.ProcessAction(Call, 0)
	if err != nil {
		t.Errorf("Error processing call: %v", err)
	}

	if h.Players[0].Bet != 10 {
		t.Errorf("Alice's bet should be 10, got %d", h.Players[0].Bet)
	}
	if h.Players[0].Chips != 990 {
		t.Errorf("Alice's chips should be 990, got %d", h.Players[0].Chips)
	}

	// Bob (SB) should be next
	if h.ActivePlayer != 1 {
		t.Errorf("Bob should be active, got player %d", h.ActivePlayer)
	}

	// Bob calls
	err = h.ProcessAction(Call, 0)
	if err != nil {
		t.Errorf("Error processing call: %v", err)
	}

	// Charlie (BB) should be next
	if h.ActivePlayer != 2 {
		t.Errorf("Charlie should be active, got player %d", h.ActivePlayer)
	}

	// Charlie checks (BB option)
	err = h.ProcessAction(Check, 0)
	if err != nil {
		t.Errorf("Error processing check: %v", err)
	}

	// Should move to flop
	if h.Street != Flop {
		t.Errorf("Should be on flop, got %v", h.Street)
	}

	// Board should have 3 cards
	if h.Board.CountCards() != 3 {
		t.Errorf("Board should have 3 cards, got %d", h.Board.CountCards())
	}
}

func TestSidePots(t *testing.T) {
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(players, 0, 5, 10, 100)

	// Set up different stack sizes
	h.Players[0].Chips = 90  // Alice has 90 after posting nothing
	h.Players[1].Chips = 45  // Bob has 45 after posting SB
	h.Players[2].Chips = 190 // Charlie has 190 after posting BB

	// Alice raises to 50
	err := h.ProcessAction(Raise, 50)
	if err != nil {
		t.Errorf("Error processing raise: %v", err)
	}

	// Bob goes all-in for 45 more (total 50)
	err = h.ProcessAction(AllIn, 0)
	if err != nil {
		t.Errorf("Error processing all-in: %v", err)
	}

	// Charlie raises to 100
	err = h.ProcessAction(Raise, 100)
	if err != nil {
		t.Errorf("Error processing raise: %v", err)
	}

	// Alice goes all-in for 40 more (total 90)
	err = h.ProcessAction(AllIn, 0)
	if err != nil {
		t.Errorf("Error processing all-in: %v", err)
	}

	// Charlie calls
	err = h.ProcessAction(Call, 0)
	if err != nil {
		t.Errorf("Error processing call: %v", err)
	}

	// Check pots
	h.calculateSidePots()

	// Should have multiple pots
	if len(h.Pots) < 2 {
		t.Errorf("Should have side pots, got %d pots", len(h.Pots))
	}

	// Total pot amount
	totalPot := 0
	for _, pot := range h.Pots {
		totalPot += pot.Amount
	}

	expectedTotal := 50 + 50 + 100 // Bob's 50 + Alice's 90 + Charlie's 100 = 240
	// But Alice only had 90 total and Bob only had 50 total
	expectedTotal = 50 + 90 + 90 // 230

	if totalPot != expectedTotal {
		t.Errorf("Total pot should be %d, got %d", expectedTotal, totalPot)
		for i, pot := range h.Pots {
			t.Logf("Pot %d: Amount=%d, Eligible=%v", i, pot.Amount, pot.Eligible)
		}
	}
}

func TestHandCompletion(t *testing.T) {
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(players, 0, 5, 10, 1000)

	// Alice folds
	err := h.ProcessAction(Fold, 0)
	if err != nil {
		t.Errorf("Error processing fold: %v", err)
	}

	// Bob folds
	err = h.ProcessAction(Fold, 0)
	if err != nil {
		t.Errorf("Error processing fold: %v", err)
	}

	// Hand should be complete (only Charlie left)
	if !h.IsComplete() {
		t.Error("Hand should be complete with only one player left")
	}
}

func TestGetWinners(t *testing.T) {
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(players, 0, 5, 10, 1000)

	// Give specific cards for testing
	h.Players[0].HoleCards = parseCards("As", "Ah") // Alice has pocket aces
	h.Players[1].HoleCards = parseCards("Ks", "Kh") // Bob has pocket kings
	h.Players[2].HoleCards = parseCards("7s", "2h") // Charlie has 7-2

	// Set board
	h.Board = parseCards("Qd", "Jc", "Ts", "9h", "3d")

	// Everyone checks to showdown (simplified)
	h.Street = Showdown

	winners := h.GetWinners()

	// Alice should win with pair of aces
	if len(winners[0]) != 1 || winners[0][0] != 0 {
		t.Errorf("Alice should win, got winners: %v", winners)
	}
}

func TestAllInWithSidePots(t *testing.T) {
	players := []string{"Alice", "Bob", "Charlie", "Dave"}
	h := NewHandState(players, 0, 5, 10, 1000)

	// Set specific stacks
	h.Players[0].Chips = 100
	h.Players[1].Chips = 200
	h.Players[2].Chips = 300
	h.Players[3].Chips = 400

	// Alice goes all-in for 100
	h.ProcessAction(AllIn, 0)

	// Bob goes all-in for 200
	h.ProcessAction(AllIn, 0)

	// Charlie goes all-in for 300
	h.ProcessAction(AllIn, 0)

	// Dave calls 300
	h.ProcessAction(Call, 0)

	// Calculate side pots
	h.calculateSidePots()

	// Verify pot structure
	if len(h.Pots) < 3 {
		t.Errorf("Expected at least 3 pots, got %d", len(h.Pots))
	}

	// First pot: everyone contributes 100 (plus blinds)
	// Should have all 4 players eligible
	if len(h.Pots[0].Eligible) != 4 {
		t.Errorf("First pot should have 4 eligible players, got %d", len(h.Pots[0].Eligible))
	}
}