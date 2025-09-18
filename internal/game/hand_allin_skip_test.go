package game

import (
	"testing"
)

// TestAllInPlayerSkippedForActions verifies that all-in players are not prompted for actions
func TestAllInPlayerSkippedForActions(t *testing.T) {
	t.Parallel()
	// Create a 3-player game with different chip counts
	// Bob has more chips than Alice so he won't go all-in when calling
	players := []string{"Alice", "Bob", "Charlie"}
	chipCounts := []int{100, 500, 500}
	h := NewHandStateWithChips(players, chipCounts, 0, 5, 10)

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
	h := NewHandStateWithChips(players, chipCounts, 0, 5, 10)

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
	h := NewHandStateWithChips(players, chipCounts, 0, 5, 10)

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
	h := NewHandStateWithChips(players, chipCounts, 0, 5, 10)

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
	if len(h.Pots) < 2 {
		t.Errorf("Should have multiple pots with different stack sizes, got %d", len(h.Pots))
	}

	// Hand should be complete
	if !h.IsComplete() {
		t.Error("Hand should be complete at showdown")
	}
}
