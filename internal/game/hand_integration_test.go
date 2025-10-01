package game

import (
	"github.com/lox/pokerforbots/v2/internal/randutil"

	"slices"
	"testing"
)

// TestCompleteHandFlow tests a complete hand from preflop to showdown
func TestCompleteHandFlow(t *testing.T) {
	t.Parallel()
	// Create a 3-player game
	h := NewHandState(randutil.New(42), []string{"Alice", "Bob", "Charlie"}, 0, 5, 10, WithChips(100))

	// Verify initial state
	if h.Street != Preflop {
		t.Errorf("Expected street to be Preflop, got %v", h.Street)
	}

	// Verify blinds were posted
	if h.Players[1].Bet != 5 { // Small blind
		t.Errorf("Small blind not posted correctly: got %d, want 5", h.Players[1].Bet)
	}
	if h.Players[2].Bet != 10 { // Big blind
		t.Errorf("Big blind not posted correctly: got %d, want 10", h.Players[2].Bet)
	}

	// Verify first active player is UTG (position 0 in 3-player game)
	if h.ActivePlayer != 0 {
		t.Errorf("First active player should be UTG (0), got %d", h.ActivePlayer)
	}

	// Verify pot has blinds
	if h.GetPots()[0].Amount != 15 {
		t.Errorf("Initial pot should be 15 (5+10), got %d", h.GetPots()[0].Amount)
	}

	// Test preflop action sequence
	t.Run("Preflop", func(t *testing.T) {
		// UTG calls
		if err := h.ProcessAction(Call, 0); err != nil {
			t.Fatalf("UTG call failed: %v", err)
		}
		if h.ActivePlayer != 1 {
			t.Errorf("After UTG call, active should be SB (1), got %d", h.ActivePlayer)
		}

		// SB calls (completes to 10)
		if err := h.ProcessAction(Call, 0); err != nil {
			t.Fatalf("SB call failed: %v", err)
		}
		if h.ActivePlayer != 2 {
			t.Errorf("After SB call, active should be BB (2), got %d", h.ActivePlayer)
		}

		// BB checks
		if err := h.ProcessAction(Check, 0); err != nil {
			t.Fatalf("BB check failed: %v", err)
		}

		// Should move to flop
		if h.Street != Flop {
			t.Errorf("After preflop betting complete, street should be Flop, got %v", h.Street)
		}

		// Pot should be 30 (3 players * 10)
		if h.GetPots()[0].Amount != 30 {
			t.Errorf("After preflop, pot should be 30, got %d", h.GetPots()[0].Amount)
		}
	})

	// Test flop action
	t.Run("Flop", func(t *testing.T) {
		// First to act should be SB (position 1)
		if h.ActivePlayer != 1 {
			t.Errorf("First to act on flop should be SB (1), got %d", h.ActivePlayer)
		}

		// Verify board has 3 cards
		boardCards := 0
		for i := range uint(52) {
			if h.Board&(1<<i) != 0 {
				boardCards++
			}
		}
		if boardCards != 3 {
			t.Errorf("Flop should have 3 cards, got %d", boardCards)
		}

		// SB bets 20
		if err := h.ProcessAction(Raise, 20); err != nil {
			t.Fatalf("SB bet failed: %v", err)
		}

		// BB calls
		if err := h.ProcessAction(Call, 0); err != nil {
			t.Fatalf("BB call failed: %v", err)
		}

		// UTG folds
		if err := h.ProcessAction(Fold, 0); err != nil {
			t.Fatalf("UTG fold failed: %v", err)
		}

		// Should move to turn with 2 active players
		if h.Street != Turn {
			t.Errorf("After flop betting, street should be Turn, got %v", h.Street)
		}

		// Pot should be 70 (30 + 20 + 20)
		if h.GetPots()[0].Amount != 70 {
			t.Errorf("After flop, pot should be 70, got %d", h.GetPots()[0].Amount)
		}
	})

	// Test turn action
	t.Run("Turn", func(t *testing.T) {
		// Verify board has 4 cards
		boardCards := 0
		for i := range uint(52) {
			if h.Board&(1<<i) != 0 {
				boardCards++
			}
		}
		if boardCards != 4 {
			t.Errorf("Turn should have 4 cards, got %d", boardCards)
		}

		// Count active players (player 0 folded on flop)
		activePlayers := 0
		for _, p := range h.Players {
			if !p.Folded {
				activePlayers++
			}
		}
		t.Logf("Active players on turn: %d", activePlayers)
		t.Logf("Current active player: %d", h.ActivePlayer)

		// Both remaining players check
		if err := h.ProcessAction(Check, 0); err != nil {
			t.Fatalf("Turn check 1 failed: %v", err)
		}
		if err := h.ProcessAction(Check, 0); err != nil {
			t.Fatalf("Turn check 2 failed: %v", err)
		}

		// Should move to river
		if h.Street != River {
			t.Errorf("After turn checks, street should be River, got %v", h.Street)
		}
	})

	// Test river action
	t.Run("River", func(t *testing.T) {
		// Verify board has 5 cards
		boardCards := 0
		for i := range uint(52) {
			if h.Board&(1<<i) != 0 {
				boardCards++
			}
		}
		if boardCards != 5 {
			t.Errorf("River should have 5 cards, got %d", boardCards)
		}

		// Both check to showdown
		if err := h.ProcessAction(Check, 0); err != nil {
			t.Fatalf("River check 1 failed: %v", err)
		}
		if err := h.ProcessAction(Check, 0); err != nil {
			t.Fatalf("River check 2 failed: %v", err)
		}

		// Should move to showdown
		if h.Street != Showdown {
			t.Errorf("After river checks, street should be Showdown, got %v", h.Street)
		}

		// Hand should be complete
		if !h.IsComplete() {
			t.Error("Hand should be complete at showdown")
		}
	})

	// Verify we have winners
	winners := h.GetWinners()
	if len(winners) == 0 {
		t.Error("Should have at least one winner")
	}
	if len(winners[0]) == 0 {
		t.Error("Main pot should have at least one winner")
	}

	// Verify player 0 is folded
	if !h.Players[0].Folded {
		t.Error("Player 0 should be folded")
	}

	// Verify players 1 and 2 are not folded
	if h.Players[1].Folded || h.Players[2].Folded {
		t.Error("Players 1 and 2 should not be folded")
	}
}

// TestAllInScenarios tests various all-in situations
func TestAllInScenarios(t *testing.T) {
	t.Parallel()
	t.Run("PreFlopAllIn", func(t *testing.T) {
		h := NewHandState(randutil.New(42), []string{"Alice", "Bob", "Charlie"}, 0, 5, 10, WithChips(100))

		// UTG goes all-in
		if err := h.ProcessAction(AllIn, 0); err != nil {
			t.Fatalf("UTG all-in failed: %v", err)
		}

		// Verify player is marked as all-in
		if !h.Players[0].AllInFlag {
			t.Error("Player 0 should be marked as all-in")
		}

		// Verify bet amount is correct (100 chips)
		if h.Players[0].Bet != 100 {
			t.Errorf("All-in bet should be 100, got %d", h.Players[0].Bet)
		}

		// SB folds
		if err := h.ProcessAction(Fold, 0); err != nil {
			t.Fatalf("SB fold failed: %v", err)
		}

		// BB folds
		if err := h.ProcessAction(Fold, 0); err != nil {
			t.Fatalf("BB fold failed: %v", err)
		}

		// Hand should be complete
		if !h.IsComplete() {
			t.Error("Hand should be complete after all opponents fold")
		}

		// UTG should win
		winners := h.GetWinners()
		if len(winners) == 0 || len(winners[0]) != 1 || winners[0][0] != 0 {
			t.Error("UTG should be the sole winner")
		}
	})

	t.Run("MultipleAllIns", func(t *testing.T) {
		// Create players with different stack sizes
		h := NewHandState(randutil.New(42),
			[]string{"Alice", "Bob", "Charlie"},
			0, 5, 10,
			WithChipsByPlayer([]int{50, 100, 150}), // Different stack sizes
		)

		// Alice (50 chips) goes all-in
		if err := h.ProcessAction(AllIn, 0); err != nil {
			t.Fatalf("Alice all-in failed: %v", err)
		}

		// Bob (100 chips) raises to 100 (all-in)
		if err := h.ProcessAction(AllIn, 0); err != nil {
			t.Fatalf("Bob all-in failed: %v", err)
		}

		// Charlie (150 chips) calls 100
		if err := h.ProcessAction(Call, 0); err != nil {
			t.Fatalf("Charlie call failed: %v", err)
		}

		// Force to showdown (Charlie can still act on later streets)
		for h.Street != Showdown && !h.IsComplete() {
			if h.ActivePlayer >= 0 {
				h.ProcessAction(Check, 0)
			} else {
				h.NextStreet()
			}
		}

		// Should have created side pots
		t.Logf("Number of pots: %d", len(h.GetPots()))
		for i, pot := range h.GetPots() {
			t.Logf("Pot %d: Amount=%d, Eligible=%v", i, pot.Amount, pot.Eligible)
		}

		// With Alice all-in for 50, Bob all-in for 100, Charlie calls 100:
		// Main pot: 50 * 3 = 150 (all eligible)
		// Side pot: 50 * 2 = 100 (Bob and Charlie eligible)
		if len(h.GetPots()) < 2 {
			t.Errorf("Should have at least 2 pots, got %d", len(h.GetPots()))
		}

		if len(h.GetPots()) >= 1 {
			// Check main pot - should be 150 (50 from each of 3 players)
			// Blinds are already included in the players' bets
			if h.GetPots()[0].Amount != 150 {
				t.Errorf("Main pot should be 150, got %d", h.GetPots()[0].Amount)
			}
		}

		if len(h.GetPots()) >= 2 {
			// Check side pot has only 2 eligible players
			if len(h.GetPots()[1].Eligible) != 2 {
				t.Errorf("Side pot should have 2 eligible players, got %d", len(h.GetPots()[1].Eligible))
			}
		}
	})
}

// TestBettingRules tests various betting rules and constraints
func TestBettingRules(t *testing.T) {
	t.Parallel()
	t.Run("MinimumRaise", func(t *testing.T) {
		h := NewHandState(randutil.New(42), []string{"Alice", "Bob"}, 0, 5, 10, WithChips(1000))

		// Alice raises to 25 (min raise is 20)
		if err := h.ProcessAction(Raise, 25); err != nil {
			t.Fatalf("Raise to 25 failed: %v", err)
		}

		// Bob tries to raise to 30 (less than min raise of 40)
		if err := h.ProcessAction(Raise, 30); err == nil {
			t.Error("Should not allow raise below minimum")
		}

		// Bob raises to 50 (valid)
		if err := h.ProcessAction(Raise, 50); err != nil {
			t.Fatalf("Raise to 50 failed: %v", err)
		}
	})

	t.Run("CannotCheckWhenFacingBet", func(t *testing.T) {
		h := NewHandState(randutil.New(42), []string{"Alice", "Bob"}, 0, 5, 10, WithChips(1000))

		// Alice raises
		if err := h.ProcessAction(Raise, 25); err != nil {
			t.Fatalf("Raise failed: %v", err)
		}

		// Bob tries to check (should fail)
		if err := h.ProcessAction(Check, 0); err == nil {
			t.Error("Should not allow check when facing a bet")
		}

		// Verify valid actions don't include check
		actions := h.GetValidActions()
		for _, action := range actions {
			if action == Check {
				t.Error("Check should not be in valid actions when facing bet")
			}
		}
	})

	t.Run("BigBlindOption", func(t *testing.T) {
		h := NewHandState(randutil.New(42), []string{"Alice", "Bob", "Charlie"}, 0, 5, 10, WithChips(1000))

		// UTG calls
		h.ProcessAction(Call, 0)
		// SB calls
		h.ProcessAction(Call, 0)

		// BB should have option to call (check) or raise
		actions := h.GetValidActions()
		hasCall := false
		hasRaise := false
		for _, action := range actions {
			if action == Call { // Protocol v2: Call not Check
				hasCall = true
			}
			if action == Raise {
				hasRaise = true
			}
		}

		if !hasCall {
			t.Error("BB should have call (check) option when everyone limps")
		}
		if !hasRaise {
			t.Error("BB should have raise option when everyone limps")
		}

		// BB raises
		if err := h.ProcessAction(Raise, 30); err != nil {
			t.Fatalf("BB raise failed: %v", err)
		}

		// Should still be preflop
		if h.Street != Preflop {
			t.Error("Should still be in preflop after BB raises")
		}

		// UTG should be next to act
		if h.ActivePlayer != 0 {
			t.Errorf("After BB raise, UTG should act next, got player %d", h.ActivePlayer)
		}
	})
}

// TestHeadsUpBlindsIntegration tests heads-up blind posting
func TestHeadsUpBlindsIntegration(t *testing.T) {
	t.Parallel()
	h := NewHandState(randutil.New(42), []string{"Alice", "Bob"}, 0, 5, 10, WithChips(100))

	// In heads-up, button posts small blind
	if h.Players[0].Bet != 5 {
		t.Errorf("Button should post small blind (5), got %d", h.Players[0].Bet)
	}

	// Non-button posts big blind
	if h.Players[1].Bet != 10 {
		t.Errorf("Non-button should post big blind (10), got %d", h.Players[1].Bet)
	}

	// Button acts first preflop in heads-up
	if h.ActivePlayer != 0 {
		t.Errorf("Button should act first preflop in heads-up, got %d", h.ActivePlayer)
	}

	// Button calls
	h.ProcessAction(Call, 0)

	// BB should be next
	if h.ActivePlayer != 1 {
		t.Errorf("BB should act after button calls, got %d", h.ActivePlayer)
	}

	// BB checks
	h.ProcessAction(Check, 0)

	// Should move to flop
	if h.Street != Flop {
		t.Errorf("Should be on flop, got %v", h.Street)
	}

	// On flop, BB acts first (opposite of preflop)
	if h.ActivePlayer != 1 {
		t.Errorf("BB should act first postflop in heads-up, got %d", h.ActivePlayer)
	}
}

// TestSidePotCalculation tests side pot creation and distribution
func TestSidePotCalculation(t *testing.T) {
	t.Parallel()
	// Create players with specific chip amounts
	h := NewHandState(randutil.New(42),
		[]string{"ShortStack", "MidStack", "BigStack"},
		0, 5, 10,
		WithChipsByPlayer([]int{20, 50, 100}),
	)

	// ShortStack goes all-in for 20
	h.ProcessAction(AllIn, 0)
	// MidStack goes all-in for 50
	h.ProcessAction(AllIn, 0)
	// BigStack calls 50
	h.ProcessAction(Call, 0)

	// Force to showdown
	for h.Street != Showdown && !h.IsComplete() {
		if h.ActivePlayer >= 0 {
			h.ProcessAction(Check, 0)
		} else {
			h.NextStreet()
		}
	}

	// Should have main pot and side pot
	if len(h.GetPots()) != 2 {
		t.Fatalf("Should have 2 pots, got %d", len(h.GetPots()))
	}

	// Main pot: 20 * 3 = 60 (after accounting for blinds)
	// All three players eligible
	mainPot := h.GetPots()[0]
	if len(mainPot.Eligible) != 3 {
		t.Errorf("Main pot should have 3 eligible players, got %d", len(mainPot.Eligible))
	}

	// Side pot: (50-20) * 2 = 60
	// Only MidStack and BigStack eligible
	sidePot := h.GetPots()[1]
	if len(sidePot.Eligible) != 2 {
		t.Errorf("Side pot should have 2 eligible players, got %d", len(sidePot.Eligible))
	}

	// Verify ShortStack is not eligible for side pot
	shortStackEligible := slices.Contains(sidePot.Eligible, 0)
	if shortStackEligible {
		t.Error("ShortStack should not be eligible for side pot")
	}
}
