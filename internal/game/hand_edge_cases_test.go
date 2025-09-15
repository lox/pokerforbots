package game

import (
	"testing"
)

// TestMinimumRaiseValidation tests that raises must meet minimum requirements
func TestMinimumRaiseValidation(t *testing.T) {
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(players, 0, 5, 10, 1000)

	// Alice raises to 30 (raise of 20 over BB)
	err := h.ProcessAction(Raise, 30)
	if err != nil {
		t.Errorf("Valid raise rejected: %v", err)
	}

	// Bob tries to raise to 35 (only 5 more, but min raise is 20)
	err = h.ProcessAction(Raise, 35)
	if err == nil {
		t.Error("Should reject raise below minimum")
	}

	// Bob makes valid min-raise to 50
	err = h.ProcessAction(Raise, 50)
	if err != nil {
		t.Errorf("Valid min-raise rejected: %v", err)
	}
}

// TestAllPlayersFoldExceptOne tests winner when everyone folds
func TestAllPlayersFoldExceptOne(t *testing.T) {
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(players, 0, 5, 10, 1000)

	// Alice folds
	if err := h.ProcessAction(Fold, 0); err != nil {
		t.Fatalf("Failed to process Alice's fold: %v", err)
	}

	// Bob folds
	if err := h.ProcessAction(Fold, 0); err != nil {
		t.Fatalf("Failed to process Bob's fold: %v", err)
	}

	// Charlie should win without showdown
	if !h.IsComplete() {
		t.Error("Hand should be complete when only one player remains")
	}

	winners := h.GetWinners()
	if len(winners[0]) != 1 || winners[0][0] != 2 {
		t.Errorf("Charlie (seat 2) should win, got: %v", winners)
	}
}

// TestSplitPot tests when multiple players have identical hands
func TestSplitPot(t *testing.T) {
	players := []string{"Alice", "Bob"}
	h := NewHandState(players, 0, 5, 10, 1000)

	// Give both players same hand (pocket aces)
	h.Players[0].HoleCards = parseCards("As", "Ah")
	h.Players[1].HoleCards = parseCards("Ac", "Ad")

	// Board with no flush/straight possibility
	h.Board = parseCards("Ks", "Qd", "Jc", "5h", "2s")
	h.Street = Showdown

	winners := h.GetWinners()
	if len(winners[0]) != 2 {
		t.Errorf("Should be split pot between 2 players, got %d winners", len(winners[0]))
	}
}

// TestHeadsUpBlinds tests blind posting in heads-up
func TestHeadsUpBlinds(t *testing.T) {
	players := []string{"Alice", "Bob"}
	h := NewHandState(players, 0, 5, 10, 1000)

	// In heads-up with button=0:
	// Alice (button) should post SB
	// Bob should post BB
	// Alice should act first preflop

	// Check blinds were posted correctly
	if h.Players[0].TotalBet != 5 {
		t.Errorf("Button (Alice) should post small blind (5), got %d", h.Players[0].TotalBet)
	}
	if h.Players[1].TotalBet != 10 {
		t.Errorf("Non-button (Bob) should post big blind (10), got %d", h.Players[1].TotalBet)
	}

	// Check chips were deducted
	if h.Players[0].Chips != 995 {
		t.Errorf("Button should have 995 chips after posting SB, got %d", h.Players[0].Chips)
	}
	if h.Players[1].Chips != 990 {
		t.Errorf("Non-button should have 990 chips after posting BB, got %d", h.Players[1].Chips)
	}

	// Check button acts first preflop
	if h.ActivePlayer != 0 {
		t.Errorf("Button should act first preflop in heads-up, got player %d", h.ActivePlayer)
	}

	// Button raises to 20
	if err := h.ProcessAction(Raise, 20); err != nil {
		t.Fatalf("Failed to process button raise: %v", err)
	}

	// BB should be next to act
	if h.ActivePlayer != 1 {
		t.Errorf("BB should act after button preflop, got player %d", h.ActivePlayer)
	}

	// BB calls
	if err := h.ProcessAction(Call, 0); err != nil {
		t.Fatalf("Failed to process BB call: %v", err)
	}

	// Should move to flop
	if h.Street != Flop {
		t.Errorf("Should be on flop after both players act, got %v", h.Street)
	}

	// On flop, BB should act first (opposite of preflop)
	if h.ActivePlayer != 1 {
		t.Errorf("BB should act first on flop in heads-up, got player %d", h.ActivePlayer)
	}
}

// TestShortStackCantCoverBlinds tests when player can't afford blinds
func TestShortStackCantCoverBlinds(t *testing.T) {
	players := []string{"Alice", "Bob", "Charlie"}

	// Create hand state
	h := NewHandState(players, 0, 5, 10, 100)

	// Manually set a player to have less than blind
	h.Players[1].Chips = 3
	h.Players[1].TotalBet = 0
	h.Players[1].Bet = 0

	// Post blinds again
	h.postBlinds(5, 10)

	// Player should be all-in for 3 chips
	if h.Players[1].Chips != 0 {
		t.Errorf("Short stack should be all-in, has %d chips", h.Players[1].Chips)
	}

	if h.Players[1].TotalBet != 3 {
		t.Errorf("Short stack should have bet 3 chips, bet %d", h.Players[1].TotalBet)
	}
}

// TestAceLowStraight tests the wheel straight (A-2-3-4-5)
func TestAceLowStraight(t *testing.T) {
	// Create hand with A-2-3-4-5
	hand := parseCards("As", "2d", "3c", "4h", "5s", "Kd", "Qh")

	rank := Evaluate7Cards(hand)

	if rank.Type() != Straight {
		t.Errorf("A-2-3-4-5 should be a straight, got %v", rank.Type())
	}

	// The wheel is the lowest straight
	highStraight := parseCards("Ts", "Jd", "Qc", "Kh", "As", "2d", "3h")
	highRank := Evaluate7Cards(highStraight)

	if CompareHands(rank, highRank) >= 0 {
		t.Error("Wheel should lose to Broadway straight")
	}
}

// TestComplexSidePots tests multiple all-ins with different amounts
func TestComplexSidePots(t *testing.T) {
	players := []string{"Alice", "Bob", "Charlie", "Dave"}
	h := NewHandState(players, 0, 5, 10, 1000)

	// Set different chip stacks
	h.Players[0].Chips = 50   // Alice: 50
	h.Players[1].Chips = 995  // Bob: 995 (after SB)
	h.Players[2].Chips = 150  // Charlie: 150 (after BB)
	h.Players[3].Chips = 300  // Dave: 300

	// Everyone goes all-in
	if err := h.ProcessAction(AllIn, 0); err != nil { // Alice: 50
		t.Fatalf("Failed to process Alice's all-in: %v", err)
	}
	if err := h.ProcessAction(AllIn, 0); err != nil { // Bob: 995
		t.Fatalf("Failed to process Bob's all-in: %v", err)
	}
	if err := h.ProcessAction(AllIn, 0); err != nil { // Charlie: 150
		t.Fatalf("Failed to process Charlie's all-in: %v", err)
	}
	if err := h.ProcessAction(AllIn, 0); err != nil { // Dave: 300
		t.Fatalf("Failed to process Dave's all-in: %v", err)
	}

	h.calculateSidePots()

	// Should have multiple pots:
	// Main pot: 50*4 = 200 (all can contest)
	// Side pot 1: (150-50)*3 = 300 (Bob, Charlie, Dave)
	// Side pot 2: (300-150)*2 = 300 (Bob, Dave)
	// Side pot 3: (995-300)*1 = 695 (Bob only)

	totalInPots := 0
	for _, pot := range h.Pots {
		totalInPots += pot.Amount
	}

	// Total should be sum of all bets plus blinds
	expectedTotal := 50 + 1000 + 160 + 300 // 1510 (includes blinds)
	if totalInPots != expectedTotal {
		t.Errorf("Total in pots should be %d, got %d", expectedTotal, totalInPots)
		for i, pot := range h.Pots {
			t.Logf("Pot %d: Amount=%d, Eligible=%v", i, pot.Amount, pot.Eligible)
		}
	}
}

// TestReraiseLimits tests betting cap scenarios
func TestReraiseLimits(t *testing.T) {
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(players, 0, 5, 10, 1000)

	// Track number of raises
	raiseCount := 0

	// Alice raises to 30
	err := h.ProcessAction(Raise, 30)
	if err != nil {
		t.Fatal(err)
	}
	raiseCount++

	// Bob re-raises to 70
	err = h.ProcessAction(Raise, 70)
	if err != nil {
		t.Fatal(err)
	}
	raiseCount++

	// Charlie re-raises to 150
	err = h.ProcessAction(Raise, 150)
	if err != nil {
		t.Fatal(err)
	}
	raiseCount++

	// In no-limit, there's no cap on number of raises
	// But we should track that MinRaise is updated correctly
	if h.MinRaise != 80 { // Last raise was 80 (150-70)
		t.Errorf("MinRaise should be 80, got %d", h.MinRaise)
	}
}

// TestKickerComparison tests that kickers are properly compared
func TestKickerComparison(t *testing.T) {
	// Both players have a pair of aces, different kickers
	// Player 1: AA with KQJ kickers
	// Player 2: AA with KQ10 kickers
	hand1 := parseCards("As", "Ah", "Kd", "Qc", "Jh", "5s", "2d")
	hand2 := parseCards("Ac", "Ad", "Kh", "Qs", "Td", "5c", "2h")

	rank1 := Evaluate7Cards(hand1)
	rank2 := Evaluate7Cards(hand2)

	// First hand should win due to jack vs ten kicker
	if CompareHands(rank1, rank2) <= 0 {
		t.Error("AA with KQJ kickers should beat AA with KQT kickers")
	}
}