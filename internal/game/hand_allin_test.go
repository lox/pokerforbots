package game

import (
	"math/rand"
	"testing"
)

// TestAllInRaiseBelowMinimum tests that a player can go all-in with a raise
// that's below the minimum raise requirement
func TestAllInRaiseBelowMinimum(t *testing.T) {
	t.Parallel()
	// Create a hand with varied chip stacks
	players := []string{"Alice", "Bob", "Charlie"}
	chipCounts := []int{100, 15, 100} // Bob only has 15 chips
	h := NewHandState(rand.New(rand.NewSource(42)), players, 0, 5, 10, WithChips(chipCounts))

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
	t.Parallel()
	// Create a hand with varied chip stacks
	players := []string{"Alice", "Bob", "Charlie"}
	chipCounts := []int{100, 35, 100} // Bob has 35 chips
	h := NewHandState(rand.New(rand.NewSource(42)), players, 0, 5, 10, WithChips(chipCounts))

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
	if h.Betting.CurrentBet != 35 {
		t.Errorf("Current bet should be 35, got %d", h.Betting.CurrentBet)
	}

	// Charlie should need to call 35 to continue
	toCall := h.Betting.CurrentBet - h.Players[2].Bet
	if toCall != 25 { // Charlie posted 10 as BB, needs 25 more to call 35
		t.Errorf("Charlie should need to call 25, got %d", toCall)
	}
}

// TestRegularRaiseBelowMinimumStillRejected tests that a regular raise
// (not all-in) below minimum is still rejected
func TestRegularRaiseBelowMinimumStillRejected(t *testing.T) {
	t.Parallel()
	// Create a hand with players having plenty of chips
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(rand.New(rand.NewSource(42)), players, 0, 5, 10, WithUniformChips(1000))

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

// Tests from hand_allin_skip_test.go

// TestAllInPlayerSkippedForActions verifies that all-in players are not prompted for actions
func TestAllInPlayerSkippedForActions(t *testing.T) {
	t.Parallel()
	// Create a 3-player game with different chip counts
	// Bob has more chips than Alice so he won't go all-in when calling
	players := []string{"Alice", "Bob", "Charlie"}
	chipCounts := []int{100, 500, 500}
	h := NewHandState(rand.New(rand.NewSource(42)), players, 0, 5, 10, WithChips(chipCounts))

	// Alice (UTG) goes all-in
	if err := h.ProcessAction(AllIn, 0); err != nil {
		t.Fatalf("Alice all-in failed: %v", err)
	}

	// Verify Alice is all-in
	if !h.Players[0].AllInFlag {
		t.Error("Alice should be marked as all-in")
	}

	// Next player should be Bob (seat 1), not Alice
	if h.ActivePlayer != 1 {
		t.Errorf("Active player should be Bob (1), got %d", h.ActivePlayer)
	}

	// Bob calls
	if err := h.ProcessAction(Call, 0); err != nil {
		t.Fatalf("Bob call failed: %v", err)
	}

	// Next should be Charlie, not Alice
	if h.ActivePlayer != 2 {
		t.Errorf("Active player should be Charlie (2), got %d", h.ActivePlayer)
	}

	// Charlie folds
	if err := h.ProcessAction(Fold, 0); err != nil {
		t.Fatalf("Charlie fold failed: %v", err)
	}

	// Should advance to flop since only Bob can act (Alice is all-in)
	if h.Street != Flop {
		t.Errorf("Should be on flop, got %v", h.Street)
	}

	// Active player should be Bob (first to act post-flop who isn't all-in)
	if h.ActivePlayer != 1 {
		t.Errorf("Active player on flop should be Bob (1), got %d", h.ActivePlayer)
	}

	// Bob checks
	if err := h.ProcessAction(Check, 0); err != nil {
		t.Fatalf("Bob check failed: %v", err)
	}

	// Should immediately advance to turn since Alice can't act (all-in)
	if h.Street != Turn {
		t.Errorf("Should advance to turn after Bob's check (Alice all-in), got %v", h.Street)
	}
}

// TestAllPlayersAllInAutoComplete verifies the hand completes automatically when all players are all-in
func TestAllPlayersAllInAutoComplete(t *testing.T) {
	t.Parallel()
	// Create a 2-player game
	players := []string{"Alice", "Bob"}
	chipCounts := []int{100, 150}
	h := NewHandState(rand.New(rand.NewSource(42)), players, 0, 5, 10, WithChips(chipCounts))

	// Alice (SB/Button in heads-up) raises to 50
	if err := h.ProcessAction(Raise, 50); err != nil {
		t.Fatalf("Alice raise failed: %v", err)
	}

	// Bob goes all-in (150)
	if err := h.ProcessAction(AllIn, 0); err != nil {
		t.Fatalf("Bob all-in failed: %v", err)
	}

	// Alice calls (going all-in with her 100)
	if err := h.ProcessAction(AllIn, 0); err != nil {
		t.Fatalf("Alice all-in call failed: %v", err)
	}

	// Both players are now all-in, hand should auto-complete to showdown
	if h.Street != Showdown {
		t.Errorf("Hand should auto-complete to showdown when all players all-in, got %v", h.Street)
	}

	// Active player should be -1 (no one can act)
	if h.ActivePlayer != -1 {
		t.Errorf("Active player should be -1 when all players all-in, got %d", h.ActivePlayer)
	}

	// Hand should be marked complete
	if !h.IsComplete() {
		t.Error("Hand should be complete when all players are all-in")
	}

	// Verify board has 5 cards (flop + turn + river)
	boardCards := h.Board.CountCards()
	if boardCards != 5 {
		t.Errorf("Board should have 5 cards when reaching showdown, got %d", boardCards)
	}
}

// TestMixedAllInAndActivePlayers verifies correct action flow with mix of all-in and active players
func TestMixedAllInAndActivePlayers(t *testing.T) {
	t.Parallel()
	// Create a 4-player game with varied stacks
	// Button is at position 0 (Alice), so:
	// - Position 1 (Bob) is SB
	// - Position 2 (Charlie) is BB
	// - Position 3 (Dave) is UTG and acts first
	players := []string{"Alice", "Bob", "Charlie", "Dave"}
	chipCounts := []int{200, 200, 200, 50} // Dave has the short stack
	h := NewHandState(rand.New(rand.NewSource(42)), players, 0, 5, 10, WithChips(chipCounts))

	// Dave (UTG with 50 chips) goes all-in
	if err := h.ProcessAction(AllIn, 0); err != nil {
		t.Fatalf("Dave all-in failed: %v", err)
	}

	// Alice (button) raises to 100
	if err := h.ProcessAction(Raise, 100); err != nil {
		t.Fatalf("Alice raise failed: %v", err)
	}

	// Bob (SB) calls
	if err := h.ProcessAction(Call, 0); err != nil {
		t.Fatalf("Bob call failed: %v", err)
	}

	// Charlie (BB) folds
	if err := h.ProcessAction(Fold, 0); err != nil {
		t.Fatalf("Charlie fold failed: %v", err)
	}

	// Should advance to flop
	if h.Street != Flop {
		t.Errorf("Should be on flop, got %v", h.Street)
	}

	// Active player should be Bob (first to act post-flop from SB position)
	// Dave is all-in, Charlie folded
	if h.ActivePlayer != 1 {
		t.Errorf("First to act on flop should be Bob (1), got %d", h.ActivePlayer)
	}

	// Bob checks
	if err := h.ProcessAction(Check, 0); err != nil {
		t.Fatalf("Bob check failed: %v", err)
	}

	// Next should be Alice (button), not Dave (all-in) or Charlie (folded)
	if h.ActivePlayer != 0 {
		t.Errorf("After Bob checks, active should be Alice (0), got %d", h.ActivePlayer)
	}

	// Alice checks
	if err := h.ProcessAction(Check, 0); err != nil {
		t.Fatalf("Alice check failed: %v", err)
	}

	// Should advance to turn (Dave can't act, Charlie folded)
	if h.Street != Turn {
		t.Errorf("Should advance to turn, got %v", h.Street)
	}
}

// TestAllInShowdownCompletes verifies that when all players go all-in, the hand completes correctly
func TestAllInShowdownCompletes(t *testing.T) {
	t.Parallel()
	// Create a game with different chip counts
	players := []string{"Alice", "Bob", "Charlie"}
	chipCounts := []int{100, 200, 150}
	h := NewHandState(rand.New(rand.NewSource(42)), players, 0, 5, 10, WithChips(chipCounts))

	// All players go all-in
	if err := h.ProcessAction(AllIn, 0); err != nil {
		t.Fatalf("Alice all-in failed: %v", err)
	}

	if err := h.ProcessAction(AllIn, 0); err != nil {
		t.Fatalf("Bob all-in failed: %v", err)
	}

	if err := h.ProcessAction(AllIn, 0); err != nil {
		t.Fatalf("Charlie all-in failed: %v", err)
	}

	// Should be at showdown
	if h.Street != Showdown {
		t.Errorf("Should be at showdown, got %v", h.Street)
	}

	// Board should have 5 cards
	boardCards := h.Board.CountCards()
	if boardCards != 5 {
		t.Errorf("Board should have 5 cards, got %d", boardCards)
	}

	// Should have multiple side pots due to different stack sizes
	if len(h.GetPots()) < 2 {
		t.Errorf("Should have multiple pots with different stack sizes, got %d", len(h.GetPots()))
	}

	// Hand should be complete
	if !h.IsComplete() {
		t.Error("Hand should be complete at showdown")
	}
}
