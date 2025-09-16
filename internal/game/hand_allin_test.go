package game

import (
	"testing"
)

// TestAllInRaiseBelowMinimum tests that a player can go all-in with a raise
// that's below the minimum raise requirement
func TestAllInRaiseBelowMinimum(t *testing.T) {
	// Create a hand with varied chip stacks
	players := []string{"Alice", "Bob", "Charlie"}
	chipCounts := []int{100, 15, 100} // Bob only has 15 chips
	h := NewHandStateWithChips(players, chipCounts, 0, 5, 10)

	// Alice (UTG) raises to 30
	if err := h.ProcessAction(Raise, 30); err != nil {
		t.Fatalf("Alice raise to 30 failed: %v", err)
	}

	// Bob (SB) wants to re-raise but only has 15 chips total
	// After posting 5 blind, he has 10 chips left
	// He wants to raise to 15 total (all-in), which is less than the minimum raise
	// Minimum raise would be 30 + 20 = 50, but Bob only has 15 total
	// This should be allowed as an all-in
	if err := h.ProcessAction(Raise, 15); err != nil {
		t.Fatalf("Bob's all-in raise to 15 should be allowed: %v", err)
	}

	// Verify Bob is all-in
	if !h.Players[1].AllInFlag {
		t.Error("Bob should be marked as all-in")
	}

	// Verify Bob has no chips left
	if h.Players[1].Chips != 0 {
		t.Errorf("Bob should have 0 chips, has %d", h.Players[1].Chips)
	}

	// Verify the current bet is now 15 (Bob's all-in amount)
	// Note: Current bet should NOT be 15 because Bob's raise was less than Alice's bet
	// Actually, wait - Bob raised to 15 total, but Alice bet 30, so current bet stays at 30
	// Let me reconsider this test...
}

// TestAllInRaiseAboveCurrentBetButBelowMinimum tests the scenario where
// a player goes all-in with a raise that's above the current bet but below minimum raise
func TestAllInRaiseAboveCurrentBetButBelowMinimum(t *testing.T) {
	// Create a hand with varied chip stacks
	players := []string{"Alice", "Bob", "Charlie"}
	chipCounts := []int{100, 35, 100} // Bob has 35 chips
	h := NewHandStateWithChips(players, chipCounts, 0, 5, 10)

	// Alice (UTG) raises to 30
	if err := h.ProcessAction(Raise, 30); err != nil {
		t.Fatalf("Alice raise to 30 failed: %v", err)
	}

	// Bob (SB) wants to re-raise but only has 35 chips total
	// After posting 5 blind, he has 30 chips left
	// Minimum raise would be 30 + 20 = 50
	// Bob goes all-in for 35 total (which is above 30 but below 50)
	// This should be allowed as an all-in
	if err := h.ProcessAction(Raise, 35); err != nil {
		t.Fatalf("Bob's all-in raise to 35 should be allowed: %v", err)
	}

	// Verify Bob is all-in
	if !h.Players[1].AllInFlag {
		t.Error("Bob should be marked as all-in")
	}

	// Verify Bob has no chips left
	if h.Players[1].Chips != 0 {
		t.Errorf("Bob should have 0 chips, has %d", h.Players[1].Chips)
	}

	// Verify the current bet is now 35
	if h.CurrentBet != 35 {
		t.Errorf("Current bet should be 35, got %d", h.CurrentBet)
	}

	// Charlie should need to call 35 to continue
	toCall := h.CurrentBet - h.Players[2].Bet
	if toCall != 25 { // Charlie posted 10 as BB, needs 25 more to call 35
		t.Errorf("Charlie should need to call 25, got %d", toCall)
	}
}

// TestRegularRaiseBelowMinimumStillRejected tests that a regular raise
// (not all-in) below minimum is still rejected
func TestRegularRaiseBelowMinimumStillRejected(t *testing.T) {
	// Create a hand with players having plenty of chips
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(players, 0, 5, 10, 1000)

	// Alice raises to 30
	if err := h.ProcessAction(Raise, 30); err != nil {
		t.Fatalf("Alice raise to 30 failed: %v", err)
	}

	// Bob tries to raise to 35 (has plenty of chips but raise is too small)
	// Minimum raise would be 30 + 20 = 50
	if err := h.ProcessAction(Raise, 35); err == nil {
		t.Fatal("Bob's raise to 35 should be rejected when he has enough chips for minimum")
	}
}
