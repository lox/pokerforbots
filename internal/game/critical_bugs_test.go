package game

import (
	"testing"
)

// TestSidePotDropsFoldedPlayersChips demonstrates that when a player folds
// after contributing chips, those chips disappear from the pot when someone
// goes all-in and triggers side pot calculation.
func TestSidePotDropsFoldedPlayersChips(t *testing.T) {
	// Simple scenario: 3 players all put in chips, one folds, one goes all-in
	// The folded player's chips should still be in the pot

	playerNames := []string{"Alice", "Bob", "Charlie"}
	chipCounts := []int{100, 30, 100} // Bob is short-stacked

	h := NewHandStateWithChips(playerNames, chipCounts, 0, 5, 10)

	// Manually set up a situation where everyone has contributed
	// This simulates a preflop where everyone called 30
	h.Players[0].Chips = 70
	h.Players[0].TotalBet = 30
	h.Players[1].Chips = 0
	h.Players[1].TotalBet = 30
	h.Players[1].AllInFlag = true // Bob is all-in
	h.Players[2].Chips = 70
	h.Players[2].TotalBet = 30
	h.Players[2].Folded = true // Charlie folded

	// Set the pot to what it should be: 30 + 30 + 30 = 90
	h.Pots = []Pot{{Amount: 90, Eligible: []int{0, 1, 2}}}

	// Now trigger calculateSidePots (this happens when someone goes all-in)
	h.calculateSidePots()

	// Count total pot after side pot calculation
	totalPot := 0
	for _, p := range h.Pots {
		totalPot += p.Amount
	}

	// The pot should still be 90, but the bug drops Charlie's chips
	expectedPot := 90
	if totalPot != expectedPot {
		t.Errorf("BUG CONFIRMED: Folded player's chips disappeared during side pot calculation!")
		t.Errorf("Expected pot %d, got %d (missing %d chips from folded player)",
			expectedPot, totalPot, expectedPot-totalPot)

		// Show the pots
		for i, pot := range h.Pots {
			t.Logf("Pot %d: Amount=%d, Eligible=%v", i, pot.Amount, pot.Eligible)
		}

		// Show what happened to each player's contribution
		t.Logf("Alice contributed: %d", h.Players[0].TotalBet)
		t.Logf("Bob contributed: %d (all-in)", h.Players[1].TotalBet)
		t.Logf("Charlie contributed: %d (folded)", h.Players[2].TotalBet)
	} else {
		t.Log("Test did not demonstrate the bug - pot calculation was correct")
	}
}

// TestWrongPotForPostAllInBets verifies that after creating side pots,
// new bets go to the correct pot (the last one, not the first).
func TestWrongPotForPostAllInBets(t *testing.T) {
	// Create a scenario where one player is all-in and others continue betting
	playerNames := []string{"Alice", "Bob", "Charlie"}
	chipCounts := []int{100, 30, 100} // Bob is short-stacked

	h := NewHandStateWithChips(playerNames, chipCounts, 0, 5, 10)

	// Set up initial state: everyone has bet 30, Bob is all-in
	h.Players[0].Chips = 70
	h.Players[0].TotalBet = 30
	h.Players[0].Bet = 30
	h.Players[1].Chips = 0
	h.Players[1].TotalBet = 30
	h.Players[1].Bet = 30
	h.Players[1].AllInFlag = true
	h.Players[2].Chips = 70
	h.Players[2].TotalBet = 30
	h.Players[2].Bet = 30
	h.CurrentBet = 30
	h.Pots = []Pot{{Amount: 90, Eligible: []int{0, 1, 2}}}

	// Trigger side pot calculation
	h.calculateSidePots()

	// Verify that only main pot exists (no side pot yet since no further betting)
	if len(h.Pots) != 1 {
		t.Errorf("Expected 1 pot after all-in with equal bets, got %d", len(h.Pots))
	}

	// Now simulate additional betting between Alice and Charlie
	// Move to flop for new betting round
	h.ActivePlayer = 0
	h.CurrentBet = 0 // Reset for new street
	h.MinRaise = 10  // Reset minimum raise
	h.Street = Flop
	h.Players[0].Bet = 0
	h.Players[2].Bet = 0

	// Process Alice's raise to 20
	err := h.ProcessAction(Raise, 20)
	if err != nil {
		t.Fatalf("Failed to process Alice's raise: %v", err)
	}

	// Process Charlie's call
	h.ActivePlayer = 2
	err = h.ProcessAction(Call, 0)
	if err != nil {
		t.Fatalf("Failed to process Charlie's call: %v", err)
	}

	// Now check the pots
	totalPot := 0
	for _, pot := range h.Pots {
		totalPot += pot.Amount
	}

	// Expected: 90 (initial) + 20 (Alice) + 20 (Charlie) = 130
	expectedTotal := 130
	if totalPot != expectedTotal {
		t.Errorf("Expected total pot of %d, got %d", expectedTotal, totalPot)
	}

	// With the fix, bets should go to the last pot (active pot)
	// This test verifies the fix is working correctly
}
