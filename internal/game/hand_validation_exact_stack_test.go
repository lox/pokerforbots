package game

import (
	"testing"

	"github.com/lox/pokerforbots/v2/internal/randutil"
)

// TestExactStackMatchesCall tests the scenario where player's stack exactly equals the amount to call.
// This reproduces the Aragorn bot issue where stack=120, to_call=120, min_bet=1120.
//
// Expected behavior:
// - When player.Chips == toCall, valid_actions should be [Fold, AllIn]
// - Raise should NOT be in valid_actions
func TestExactStackMatchesCall(t *testing.T) {
	t.Parallel()

	// Setup: 3-player game
	// Alice has 1000 chips
	// Bob has 120 chips (short stack)
	// Charlie has 1000 chips
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(randutil.New(42), players, 0, 5, 10,
		WithChipsByPlayer([]int{1000, 120, 1000}))

	// Preflop: SB=5, BB=10
	// Bob (seat 1) posts SB = 5, now has 115 chips
	// Charlie (seat 2) posts BB = 10, now has 990 chips
	// Alice (seat 0) acts first (UTG)

	// Alice raises to 120 (all of Bob's remaining stack + 5 for SB)
	err := h.ProcessAction(Raise, 120)
	if err != nil {
		t.Fatalf("Alice raise failed: %v", err)
	}

	// Now Bob is next to act
	// Bob has 115 chips remaining (started with 120, posted 5 SB)
	// CurrentBet is 120
	// Bob's current bet is 5 (the SB)
	// toCall = 120 - 5 = 115
	// Bob has exactly 115 chips
	// So: player.Chips == toCall (115 == 115)

	if h.ActivePlayer != 1 {
		t.Fatalf("Expected Bob (seat 1) to act, got seat %d", h.ActivePlayer)
	}

	bob := h.Players[1]
	if bob.Chips != 115 {
		t.Errorf("Expected Bob to have 115 chips, got %d", bob.Chips)
	}

	toCall := h.Betting.CurrentBet - bob.Bet
	if toCall != 115 {
		t.Errorf("Expected toCall to be 115, got %d", toCall)
	}

	// Get valid actions for Bob
	validActions := h.GetValidActions()

	// CRITICAL CHECK: When chips == toCall, player should only be able to Fold or AllIn
	// Raise should NOT be included because player doesn't have enough chips to make minimum raise
	t.Logf("Bob's situation: chips=%d, toCall=%d, currentBet=%d, minRaise=%d",
		bob.Chips, toCall, h.Betting.CurrentBet, h.Betting.MinRaise)
	t.Logf("Valid actions: %v", validActions)
	t.Logf("MinBet (CurrentBet + MinRaise) = %d + %d = %d",
		h.Betting.CurrentBet, h.Betting.MinRaise, h.Betting.CurrentBet+h.Betting.MinRaise)

	// Verify expected actions
	hasRaise := false
	hasAllIn := false
	hasFold := false
	hasCall := false

	for _, action := range validActions {
		switch action {
		case Fold:
			hasFold = true
		case Call:
			hasCall = true
		case Raise:
			hasRaise = true
		case AllIn:
			hasAllIn = true
		}
	}

	if !hasFold {
		t.Error("Fold should always be available")
	}

	// When chips >= toCall (matching scenario), should have AllIn as option
	if !hasAllIn {
		t.Error("AllIn should be available when chips == toCall")
	}

	// Call should NOT be available when chips == toCall (would leave 0 chips)
	// Based on code: toCall >= player.Chips triggers AllIn path
	if hasCall {
		t.Error("Call should NOT be available when chips == toCall (should use AllIn instead)")
	}

	// Raise should NOT be available - player can't make minimum raise
	if hasRaise {
		t.Errorf("Raise should NOT be available when player can only call all-in (chips=%d, toCall=%d, minRaiseTotal=%d)",
			bob.Chips, toCall, h.Betting.CurrentBet+h.Betting.MinRaise)
	}
}

// TestStackOneChipOverCall tests when stack is exactly 1 chip more than toCall.
// This would trigger the condition: chips > toCall but chips <= toCall + minRaise
func TestStackOneChipOverCall(t *testing.T) {
	t.Parallel()

	// Setup: 3-player game with Bob having 121 chips (1 more than needed to call)
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(randutil.New(42), players, 0, 5, 10,
		WithChipsByPlayer([]int{1000, 121, 1000}))

	// Alice raises to 120
	err := h.ProcessAction(Raise, 120)
	if err != nil {
		t.Fatalf("Alice raise failed: %v", err)
	}

	// Now Bob acts
	// Bob has 116 chips (121 - 5 SB)
	// toCall = 120 - 5 = 115
	// Bob has 1 chip more than toCall

	if h.ActivePlayer != 1 {
		t.Fatalf("Expected Bob (seat 1) to act, got seat %d", h.ActivePlayer)
	}

	bob := h.Players[1]
	toCall := h.Betting.CurrentBet - bob.Bet

	t.Logf("Bob's situation: chips=%d, toCall=%d, currentBet=%d, minRaise=%d",
		bob.Chips, toCall, h.Betting.CurrentBet, h.Betting.MinRaise)

	validActions := h.GetValidActions()
	t.Logf("Valid actions: %v", validActions)

	hasRaise := false
	hasAllIn := false
	hasCall := false

	for _, action := range validActions {
		switch action {
		case Call:
			hasCall = true
		case Raise:
			hasRaise = true
		case AllIn:
			hasAllIn = true
		}
	}

	// With 116 chips and needing to call 115:
	// - Can call (leaves 1 chip)
	// - Cannot raise to minimum (would need CurrentBet + MinRaise = much more)
	// - Can go all-in with the extra chip

	if !hasCall {
		t.Error("Call should be available when chips > toCall")
	}

	if !hasAllIn {
		t.Error("AllIn should be available when chips > toCall but not enough for min raise")
	}

	// Key question: Can Bob raise?
	// chips = 116, toCall = 115, minRaise = 110 (typical)
	// To raise, need: chips > toCall + minRaise → 116 > 115 + 110 → 116 > 225 → FALSE
	// So Raise should NOT be available
	if hasRaise {
		t.Errorf("Raise should NOT be available (chips=%d, toCall=%d, minRaise=%d, need=%d)",
			bob.Chips, toCall, h.Betting.MinRaise, toCall+h.Betting.MinRaise)
	}
}
