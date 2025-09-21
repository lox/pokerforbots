package game

import (
	"math/rand"
	"testing"

	"github.com/lox/pokerforbots/poker"
)

// TestBasicHandFlow tests a simple hand with minimal actions
func TestBasicHandFlow(t *testing.T) {
	t.Parallel()
	h := NewHandState(rand.New(rand.NewSource(42)), []string{"Alice", "Bob"}, 0, 5, 10, WithChips(100))

	// Verify preflop state
	t.Logf("Initial state: Street=%v, ActivePlayer=%d, CurrentBet=%d", h.Street, h.ActivePlayer, h.Betting.CurrentBet)

	if h.Street != Preflop {
		t.Fatalf("Should start at Preflop, got %v", h.Street)
	}

	// In heads-up, button (0) acts first preflop
	if h.ActivePlayer != 0 {
		t.Fatalf("Button should act first in heads-up preflop, got %d", h.ActivePlayer)
	}

	// Alice (button/SB) calls the BB
	t.Log("Alice calls")
	if err := h.ProcessAction(Call, 0); err != nil {
		t.Fatalf("Alice call failed: %v", err)
	}

	// Bob (BB) checks
	t.Log("Bob checks")
	if err := h.ProcessAction(Check, 0); err != nil {
		t.Fatalf("Bob check failed: %v", err)
	}

	// Should be on flop now
	if h.Street != Flop {
		t.Fatalf("Should be on Flop after preflop action, got %v", h.Street)
	}

	// Count flop cards
	flopCards := countBoardCards(h.Board)
	if flopCards != 3 {
		t.Fatalf("Flop should have 3 cards, got %d", flopCards)
	}
	t.Logf("Flop dealt: %d cards on board", flopCards)

	// On flop, BB (1) acts first in heads-up
	if h.ActivePlayer != 1 {
		t.Fatalf("BB should act first postflop in heads-up, got %d", h.ActivePlayer)
	}

	// Bob checks
	t.Log("Bob checks on flop")
	if err := h.ProcessAction(Check, 0); err != nil {
		t.Fatalf("Bob flop check failed: %v", err)
	}

	// Alice checks
	t.Log("Alice checks on flop")
	t.Logf("Before Alice check: Street=%v, ActivePlayer=%d", h.Street, h.ActivePlayer)
	if err := h.ProcessAction(Check, 0); err != nil {
		t.Fatalf("Alice flop check failed: %v", err)
	}
	t.Logf("After Alice check: Street=%v, ActivePlayer=%d", h.Street, h.ActivePlayer)

	// Should be on turn
	if h.Street != Turn {
		// Debug: check if betting is complete
		t.Logf("isBettingComplete: %v", h.Betting.IsBettingComplete(h.Players, h.Street, h.Button))
		t.Logf("ActivePlayer: %d", h.ActivePlayer)
		t.Logf("CurrentBet: %d", h.Betting.CurrentBet)
		for i, p := range h.Players {
			t.Logf("Player %d: Bet=%d, Folded=%v, AllIn=%v", i, p.Bet, p.Folded, p.AllInFlag)
		}
		t.Fatalf("Should be on Turn after flop checks, got %v", h.Street)
	}

	// Count turn cards
	turnCards := countBoardCards(h.Board)
	if turnCards != 4 {
		t.Fatalf("Turn should have 4 cards, got %d", turnCards)
	}
	t.Logf("Turn dealt: %d cards on board", turnCards)

	// Bob checks on turn
	t.Log("Bob checks on turn")
	if err := h.ProcessAction(Check, 0); err != nil {
		t.Fatalf("Bob turn check failed: %v", err)
	}

	// Alice checks on turn
	t.Log("Alice checks on turn")
	if err := h.ProcessAction(Check, 0); err != nil {
		t.Fatalf("Alice turn check failed: %v", err)
	}

	// Should be on river
	if h.Street != River {
		t.Fatalf("Should be on River after turn checks, got %v", h.Street)
	}

	// Count river cards
	riverCards := countBoardCards(h.Board)
	if riverCards != 5 {
		t.Fatalf("River should have 5 cards, got %d", riverCards)
	}
	t.Logf("River dealt: %d cards on board", riverCards)

	// Bob checks on river
	t.Log("Bob checks on river")
	if err := h.ProcessAction(Check, 0); err != nil {
		t.Fatalf("Bob river check failed: %v", err)
	}

	// Alice checks on river
	t.Log("Alice checks on river")
	if err := h.ProcessAction(Check, 0); err != nil {
		t.Fatalf("Alice river check failed: %v", err)
	}

	// Should be at showdown
	if h.Street != Showdown {
		t.Fatalf("Should be at Showdown after river checks, got %v", h.Street)
	}

	// Hand should be complete
	if !h.IsComplete() {
		t.Fatal("Hand should be complete at showdown")
	}

	// Should have a winner
	winners := h.GetWinners()
	if len(winners) == 0 || len(winners[0]) == 0 {
		t.Fatal("Should have at least one winner")
	}

	t.Logf("Hand complete! Winner(s): %v", winners[0])
}

// TestBettingAndFolding tests betting and folding scenarios
func TestBettingAndFolding(t *testing.T) {
	t.Parallel()
	h := NewHandState(rand.New(rand.NewSource(42)), []string{"Alice", "Bob", "Charlie"}, 0, 5, 10, WithChips(100))

	// Verify initial state
	if h.ActivePlayer != 0 {
		t.Fatalf("UTG should act first, got %d", h.ActivePlayer)
	}

	// Alice (UTG) raises to 30
	t.Log("Alice raises to 30")
	if err := h.ProcessAction(Raise, 30); err != nil {
		t.Fatalf("Alice raise failed: %v", err)
	}

	// Bob (SB) folds
	t.Log("Bob folds")
	if err := h.ProcessAction(Fold, 0); err != nil {
		t.Fatalf("Bob fold failed: %v", err)
	}

	if !h.Players[1].Folded {
		t.Fatal("Bob should be folded")
	}

	// Charlie (BB) calls
	t.Log("Charlie calls")
	if err := h.ProcessAction(Call, 0); err != nil {
		t.Fatalf("Charlie call failed: %v", err)
	}

	// Should be on flop
	if h.Street != Flop {
		t.Fatalf("Should be on Flop, got %v", h.Street)
	}

	// Pot should be 65 (30 + 30 + 5 from Bob's SB)
	if h.GetPots()[0].Amount != 65 {
		t.Errorf("Pot should be 65, got %d", h.GetPots()[0].Amount)
	}

	// Charlie should act first on flop (first active player after button)
	if h.ActivePlayer != 2 {
		t.Fatalf("Charlie should act first on flop, got %d", h.ActivePlayer)
	}

	// Charlie bets 40
	t.Log("Charlie bets 40")
	if err := h.ProcessAction(Raise, 40); err != nil {
		t.Fatalf("Charlie bet failed: %v", err)
	}

	// Alice folds
	t.Log("Alice folds")
	if err := h.ProcessAction(Fold, 0); err != nil {
		t.Fatalf("Alice fold failed: %v", err)
	}

	// Hand should be complete (only one player left)
	if !h.IsComplete() {
		t.Fatal("Hand should be complete with only one active player")
	}

	// Charlie should be the winner
	winners := h.GetWinners()
	if len(winners) == 0 || len(winners[0]) != 1 || winners[0][0] != 2 {
		t.Fatalf("Charlie should be the sole winner, got %v", winners)
	}

	t.Log("Charlie wins by everyone folding!")
}

// TestAllInWithSidePotsValidation tests all-in scenarios with side pots
func TestAllInWithSidePotsValidation(t *testing.T) {
	t.Parallel()
	// Create players with different stacks
	h := NewHandState(rand.New(rand.NewSource(42)),
		[]string{"ShortStack", "MidStack", "BigStack"},
		0, 5, 10,
		WithChipsByPlayer([]int{25, 60, 100}),
	)

	// Verify chip counts
	if h.Players[0].Chips != 25 {
		t.Fatalf("ShortStack should have 25 chips, got %d", h.Players[0].Chips)
	}

	// ShortStack (UTG) goes all-in for 25
	t.Log("ShortStack goes all-in for 25")
	if err := h.ProcessAction(AllIn, 0); err != nil {
		t.Fatalf("ShortStack all-in failed: %v", err)
	}

	if !h.Players[0].AllInFlag {
		t.Fatal("ShortStack should be marked all-in")
	}

	// MidStack (SB) calls (will put in 25, has 5 already)
	t.Log("MidStack calls all-in")
	if err := h.ProcessAction(Call, 0); err != nil {
		t.Fatalf("MidStack call failed: %v", err)
	}

	// BigStack (BB) raises to 50
	t.Log("BigStack raises to 50")
	if err := h.ProcessAction(Raise, 50); err != nil {
		t.Fatalf("BigStack raise failed: %v", err)
	}

	// MidStack calls the raise (will go all-in)
	t.Log("MidStack calls the raise (goes all-in)")
	if err := h.ProcessAction(Call, 0); err != nil {
		t.Fatalf("MidStack call failed: %v", err)
	}

	// Should move to flop (no more betting possible)
	if h.Street != Flop {
		t.Fatalf("Should be on Flop, got %v", h.Street)
	}

	// Force to showdown
	for h.Street != Showdown && !h.IsComplete() {
		if h.ActivePlayer >= 0 {
			// BigStack can still act
			t.Logf("BigStack checks on %v", h.Street)
			h.ProcessAction(Check, 0)
		} else {
			// No active players, advance street
			h.NextStreet()
		}
	}

	// Check side pots
	if len(h.GetPots()) < 2 {
		t.Fatalf("Should have at least 2 pots, got %d", len(h.GetPots()))
	}

	// Main pot should have all 3 players eligible
	if len(h.GetPots()[0].Eligible) != 3 {
		t.Errorf("Main pot should have 3 eligible players, got %d", len(h.GetPots()[0].Eligible))
	}

	// Side pot should have only 2 players eligible (not ShortStack)
	if len(h.GetPots()[1].Eligible) != 2 {
		t.Errorf("Side pot should have 2 eligible players, got %d", len(h.GetPots()[1].Eligible))
	}

	// Verify ShortStack is not in side pot
	for _, seat := range h.GetPots()[1].Eligible {
		if seat == 0 {
			t.Error("ShortStack should not be eligible for side pot")
		}
	}

	t.Logf("Main pot: %d chips, eligible: %v", h.GetPots()[0].Amount, h.GetPots()[0].Eligible)
	t.Logf("Side pot: %d chips, eligible: %v", h.GetPots()[1].Amount, h.GetPots()[1].Eligible)
}

// Helper function to count board cards
func countBoardCards(board poker.Hand) int {
	count := 0
	for i := uint(0); i < 52; i++ {
		if board&(1<<i) != 0 {
			count++
		}
	}
	return count
}
