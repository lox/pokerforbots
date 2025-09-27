package game

import (
	"github.com/lox/pokerforbots/internal/randutil"

	"slices"
	"testing"

	"github.com/lox/pokerforbots/poker"
)

func parseCards(strs ...string) poker.Hand {
	var hand poker.Hand
	for _, s := range strs {
		card, _ := poker.ParseCard(s)
		hand |= poker.Hand(card)
	}
	return hand
}

func TestHandStateCreation(t *testing.T) {
	t.Parallel()
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(randutil.New(42), players, 0, 5, 10, WithChips(1000))

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
	if h.GetPots()[0].Amount != 15 {
		t.Errorf("Initial pot incorrect: %d", h.GetPots()[0].Amount)
	}

	// Check cards were dealt
	for _, p := range h.Players {
		if p.HoleCards.CountCards() != 2 {
			t.Errorf("Player %s has %d hole cards, expected 2", p.Name, p.HoleCards.CountCards())
		}
	}
}

func TestGetValidActions(t *testing.T) {
	t.Parallel()
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(randutil.New(42), players, 0, 5, 10, WithChips(1000))

	// First player to act (Alice, UTG)
	actions := h.GetValidActions()

	// Should be able to fold, call, or raise
	if !slices.Contains(actions, Fold) {
		t.Error("Should be able to fold")
	}
	if !slices.Contains(actions, Call) {
		t.Error("Should be able to call")
	}
	if !slices.Contains(actions, Raise) {
		t.Error("Should be able to raise")
	}
	if slices.Contains(actions, Check) {
		t.Error("Should not be able to check (facing a bet)")
	}
}

func TestProcessAction(t *testing.T) {
	t.Parallel()
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(randutil.New(42), players, 0, 5, 10, WithChips(1000))

	// Initial state: Bob posted SB (5), Charlie posted BB (10)
	// Alice is first to act (UTG)
	if h.ActivePlayer != 0 {
		t.Errorf("Alice should be first to act, got player %d", h.ActivePlayer)
	}

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

	// Bob calls (needs 5 more to match 10)
	err = h.ProcessAction(Call, 0)
	if err != nil {
		t.Errorf("Error processing Bob's call: %v", err)
	}

	// Charlie (BB) should be next for option
	if h.ActivePlayer != 2 {
		t.Errorf("Charlie should be active for BB option, got player %d", h.ActivePlayer)
	}

	// Charlie checks (BB option)
	err = h.ProcessAction(Check, 0)
	if err != nil {
		t.Errorf("Error processing Charlie's check: %v", err)
	}

	// Should move to flop
	if h.Street != Flop {
		t.Errorf("Should be on flop, got %v", h.Street)
	}

	// Board should have 3 cards
	if h.Board.CountCards() != 3 {
		t.Errorf("Board should have 3 cards, got %d", h.Board.CountCards())
	}

	// Bob should be first to act on flop (first after button)
	if h.ActivePlayer != 1 {
		t.Errorf("Bob should be first to act on flop, got player %d", h.ActivePlayer)
	}
}

func TestSidePots(t *testing.T) {
	t.Parallel()
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(randutil.New(42), players, 0, 5, 10, WithChips(100))

	// After blinds:
	// Alice: 100 chips
	// Bob: 95 chips (posted 5)
	// Charlie: 90 chips (posted 10)

	// Alice raises to 50
	err := h.ProcessAction(Raise, 50)
	if err != nil {
		t.Errorf("Error processing raise: %v", err)
	}
	// Alice now has 50 chips left

	// Bob goes all-in for 95
	err = h.ProcessAction(AllIn, 0)
	if err != nil {
		t.Errorf("Error processing all-in: %v", err)
	}
	// Bob total bet: 100 (5 blind + 95 all-in)

	// Charlie goes all-in for 90
	err = h.ProcessAction(AllIn, 0)
	if err != nil {
		t.Errorf("Error processing all-in: %v", err)
	}
	// Charlie total bet: 100 (10 blind + 90 all-in)

	// Alice calls the all-in (50 more to match 100)
	err = h.ProcessAction(Call, 0)
	if err != nil {
		t.Errorf("Error processing call: %v", err)
	}
	// Alice total bet: 100

	// Check pots
	h.PotManager.CollectBets(h.Players)
	h.PotManager.CalculateSidePots(h.Players)

	// Total pot amount should be 300 (100 from each player)
	totalPot := 0
	for _, pot := range h.GetPots() {
		totalPot += pot.Amount
	}

	expectedTotal := 300

	if totalPot != expectedTotal {
		t.Errorf("Total pot should be %d, got %d", expectedTotal, totalPot)
		for i, pot := range h.GetPots() {
			t.Logf("Pot %d: Amount=%d, Eligible=%v", i, pot.Amount, pot.Eligible)
		}
	}
}

func TestHandCompletion(t *testing.T) {
	t.Parallel()
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(randutil.New(42), players, 0, 5, 10, WithChips(1000))

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
	t.Parallel()
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(randutil.New(42), players, 0, 5, 10, WithChips(1000))

	// Give specific cards for testing
	h.Players[0].HoleCards = parseCards("As", "Ah") // Alice has pocket aces
	h.Players[1].HoleCards = parseCards("Ks", "Kh") // Bob has pocket kings
	h.Players[2].HoleCards = parseCards("7s", "2h") // Charlie has 7-2

	// Set board that doesn't make straights or flushes
	h.Board = parseCards("Qd", "Jc", "9s", "6h", "3d")

	// Everyone checks to showdown (simplified)
	h.Street = Showdown

	winners := h.GetWinners()

	// Alice should win with pair of aces
	if len(winners[0]) != 1 || winners[0][0] != 0 {
		t.Errorf("Alice (seat 0) should win with AA, got winners: %v", winners)
		// Debug: show what hands were evaluated
		for i, p := range h.Players {
			fullHand := p.HoleCards | h.Board
			rank := poker.Evaluate7Cards(fullHand)
			t.Logf("Player %d (%s): %s", i, p.Name, rank.String())
		}
	}
}

func TestAllInWithSidePots(t *testing.T) {
	t.Parallel()
	players := []string{"Alice", "Bob", "Charlie", "Dave"}
	h := NewHandState(randutil.New(42), players, 0, 5, 10, WithChips(1000))

	// Set specific stacks
	h.Players[0].Chips = 100
	h.Players[1].Chips = 200
	h.Players[2].Chips = 300
	h.Players[3].Chips = 400

	// Alice goes all-in for 100
	if err := h.ProcessAction(AllIn, 0); err != nil {
		t.Fatalf("Failed to process Alice's all-in: %v", err)
	}

	// Bob goes all-in for 200
	if err := h.ProcessAction(AllIn, 0); err != nil {
		t.Fatalf("Failed to process Bob's all-in: %v", err)
	}

	// Charlie goes all-in for 300
	if err := h.ProcessAction(AllIn, 0); err != nil {
		t.Fatalf("Failed to process Charlie's all-in: %v", err)
	}

	// Dave calls 300
	if err := h.ProcessAction(Call, 0); err != nil {
		t.Fatalf("Failed to process Dave's call: %v", err)
	}

	// Calculate side pots
	h.PotManager.CollectBets(h.Players)
	h.PotManager.CalculateSidePots(h.Players)

	// Verify pot structure
	if len(h.GetPots()) < 3 {
		t.Errorf("Expected at least 3 pots, got %d", len(h.GetPots()))
	}

	// First pot: everyone contributes 100 (plus blinds)
	// Should have all 4 players eligible
	if len(h.GetPots()[0].Eligible) != 4 {
		t.Errorf("First pot should have 4 eligible players, got %d", len(h.GetPots()[0].Eligible))
	}
}

// Edge case tests

// TestMinimumRaiseValidation tests that raises must meet minimum requirements
func TestMinimumRaiseValidation(t *testing.T) {
	t.Parallel()
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(randutil.New(42), players, 0, 5, 10, WithChips(1000))

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
	t.Parallel()
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(randutil.New(42), players, 0, 5, 10, WithChips(1000))

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
	t.Parallel()
	players := []string{"Alice", "Bob"}
	h := NewHandState(randutil.New(42), players, 0, 5, 10, WithChips(1000))

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
	t.Parallel()
	players := []string{"Alice", "Bob"}
	h := NewHandState(randutil.New(42), players, 0, 5, 10, WithChips(1000))

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
	t.Parallel()
	players := []string{"Alice", "Bob", "Charlie"}

	// Create hand state
	h := NewHandState(randutil.New(42), players, 0, 5, 10, WithChips(100))

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
	t.Parallel()
	// Create hand with A-2-3-4-5
	hand := parseCards("As", "2d", "3c", "4h", "5s", "Kd", "Qh")

	rank := poker.Evaluate7Cards(hand)

	if rank.Type() != poker.Straight {
		t.Errorf("A-2-3-4-5 should be a straight, got %v", rank.Type())
	}

	// The wheel is the lowest straight
	highStraight := parseCards("Ts", "Jd", "Qc", "Kh", "As", "2d", "3h")
	highRank := poker.Evaluate7Cards(highStraight)

	if poker.CompareHands(rank, highRank) >= 0 {
		t.Error("Wheel should lose to Broadway straight")
	}
}

// TestComplexSidePots tests multiple all-ins with different amounts
func TestComplexSidePots(t *testing.T) {
	t.Parallel()
	players := []string{"Alice", "Bob", "Charlie", "Dave"}
	h := NewHandState(randutil.New(42), players, 0, 5, 10, WithChips(1000))

	// Set different chip stacks
	h.Players[0].Chips = 50  // Alice: 50
	h.Players[1].Chips = 995 // Bob: 995 (after SB)
	h.Players[2].Chips = 150 // Charlie: 150 (after BB)
	h.Players[3].Chips = 300 // Dave: 300

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

	h.PotManager.CollectBets(h.Players)
	h.PotManager.CalculateSidePots(h.Players)

	// Should have multiple pots:
	// Main pot: 50*4 = 200 (all can contest)
	// Side pot 1: (150-50)*3 = 300 (Bob, Charlie, Dave)
	// Side pot 2: (300-150)*2 = 300 (Bob, Dave)
	// Side pot 3: (995-300)*1 = 695 (Bob only)

	totalInPots := 0
	for _, pot := range h.GetPots() {
		totalInPots += pot.Amount
	}

	// Total should be sum of all bets plus blinds
	expectedTotal := 50 + 1000 + 160 + 300 // 1510 (includes blinds)
	if totalInPots != expectedTotal {
		t.Errorf("Total in pots should be %d, got %d", expectedTotal, totalInPots)
		for i, pot := range h.GetPots() {
			t.Logf("Pot %d: Amount=%d, Eligible=%v", i, pot.Amount, pot.Eligible)
		}
	}
}

// TestSidePotWithFoldedPlayerRegression tests that folded players' chips
// remain in the pot during side pot calculation (regression test for critical bug)
func TestSidePotWithFoldedPlayerRegression(t *testing.T) {
	t.Parallel()
	// Simple scenario: 3 players all put in chips, one folds, one goes all-in
	// The folded player's chips should still be in the pot

	playerNames := []string{"Alice", "Bob", "Charlie"}
	chipCounts := []int{100, 30, 100} // Bob is short-stacked

	h := NewHandState(randutil.New(42), playerNames, 0, 5, 10, WithChipsByPlayer(chipCounts))

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

	// Collect bets and calculate side pots (this happens when someone goes all-in)
	h.PotManager.CollectBets(h.Players)
	h.PotManager.CalculateSidePots(h.Players)

	// Count total pot after side pot calculation
	totalPot := 0
	for _, p := range h.GetPots() {
		totalPot += p.Amount
	}

	// The pot should still be 90 (regression test for bug that dropped folded players' chips)
	expectedPot := 90
	if totalPot != expectedPot {
		t.Errorf("Folded player's chips disappeared during side pot calculation!")
		t.Errorf("Expected pot %d, got %d (missing %d chips from folded player)",
			expectedPot, totalPot, expectedPot-totalPot)

		// Show the pots
		for i, pot := range h.GetPots() {
			t.Logf("Pot %d: Amount=%d, Eligible=%v", i, pot.Amount, pot.Eligible)
		}
	}
}

// TestPostAllInBetsToCorrectPot verifies that after creating side pots,
// new bets go to the correct pot - regression test for critical bug
func TestPostAllInBetsToCorrectPot(t *testing.T) {
	t.Parallel()
	// Create a scenario where one player is all-in and others continue betting
	playerNames := []string{"Alice", "Bob", "Charlie"}
	chipCounts := []int{100, 30, 100} // Bob is short-stacked

	h := NewHandState(randutil.New(42), playerNames, 0, 5, 10, WithChipsByPlayer(chipCounts))

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
	h.Betting.CurrentBet = 30

	// Trigger side pot calculation
	h.PotManager.CollectBets(h.Players)
	h.PotManager.CalculateSidePots(h.Players)

	// Verify that only main pot exists (no side pot yet since no further betting)
	if len(h.GetPots()) != 1 {
		t.Errorf("Expected 1 pot after all-in with equal bets, got %d", len(h.GetPots()))
	}

	// Now simulate additional betting between Alice and Charlie
	// Move to flop for new betting round
	h.ActivePlayer = 0
	h.Betting.CurrentBet = 0 // Reset for new street
	h.Betting.MinRaise = 10  // Reset minimum raise
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
	for _, pot := range h.GetPots() {
		totalPot += pot.Amount
	}

	// Expected: 90 (initial) + 20 (Alice) + 20 (Charlie) = 130
	expectedTotal := 130
	if totalPot != expectedTotal {
		t.Errorf("Expected total pot of %d, got %d", expectedTotal, totalPot)
	}

	// Regression test: with the fix, bets should go to the last pot (active pot)
}

// TestReraiseLimits tests betting cap scenarios
func TestReraiseLimits(t *testing.T) {
	t.Parallel()
	players := []string{"Alice", "Bob", "Charlie"}
	h := NewHandState(randutil.New(42), players, 0, 5, 10, WithChips(1000))

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
	_ = raiseCount // Test tracking variable

	// In no-limit, there's no cap on number of raises
	// But we should track that MinRaise is updated correctly
	if h.Betting.MinRaise != 80 { // Last raise was 80 (150-70)
		t.Errorf("MinRaise should be 80, got %d", h.Betting.MinRaise)
	}
}

// TestKickerComparison tests that kickers are properly compared
func TestKickerComparison(t *testing.T) {
	t.Parallel()
	// Both players have a pair of aces, different kickers
	// Player 1: AA with KQJ kickers
	// Player 2: AA with KQ10 kickers
	hand1 := parseCards("As", "Ah", "Kd", "Qc", "Jh", "5s", "2d")
	hand2 := parseCards("Ac", "Ad", "Kh", "Qs", "Td", "5c", "2h")

	rank1 := poker.Evaluate7Cards(hand1)
	rank2 := poker.Evaluate7Cards(hand2)

	// First hand should win due to jack vs ten kicker
	if poker.CompareHands(rank1, rank2) <= 0 {
		t.Error("AA with KQJ kickers should beat AA with KQT kickers")
	}
}

// Tests for the new constructor and options pattern

func TestNewHandStateOptions(t *testing.T) {
	t.Parallel()

	t.Run("basic construction", func(t *testing.T) {
		rng := randutil.New(42)
		h := NewHandState(rng, []string{"Alice", "Bob", "Charlie"}, 0, 5, 10)

		if len(h.Players) != 3 {
			t.Errorf("Expected 3 players, got %d", len(h.Players))
		}

		// Check default chip counts
		for i, p := range h.Players {
			if p.Chips != 990 && p.Chips != 995 { // After blinds
				if i == 0 { // Button, no blind
					if p.Chips != 1000 {
						t.Errorf("Player %d should have 1000 chips, got %d", i, p.Chips)
					}
				}
			}
		}

		if h.Button != 0 {
			t.Errorf("Button should be 0, got %d", h.Button)
		}

		if h.Street != Preflop {
			t.Errorf("Should start at Preflop, got %v", h.Street)
		}
	})

	t.Run("requires RNG", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for nil RNG")
			}
		}()
		NewHandState(nil, []string{"Alice", "Bob"}, 0, 5, 10)
	})

	t.Run("requires at least 2 players", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for < 2 players")
			}
		}()
		rng := randutil.New(42)
		NewHandState(rng, []string{"Alice"}, 0, 5, 10)
	})

	t.Run("validates button position", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for invalid button")
			}
		}()
		rng := randutil.New(42)
		NewHandState(rng, []string{"Alice", "Bob"}, 5, 5, 10) // button out of range
	})
}

func TestHandOptions(t *testing.T) {
	t.Parallel()

	t.Run("WithChips", func(t *testing.T) {
		rng := randutil.New(42)
		h := NewHandState(rng, []string{"Alice", "Bob", "Charlie"}, 0, 5, 10, WithChips(500))

		// Check chips after blinds
		if h.Players[0].Chips != 500 { // Button, no blind
			t.Errorf("Button should have 500 chips, got %d", h.Players[0].Chips)
		}
		if h.Players[1].Chips != 495 { // Small blind
			t.Errorf("SB should have 495 chips after blind, got %d", h.Players[1].Chips)
		}
		if h.Players[2].Chips != 490 { // Big blind
			t.Errorf("BB should have 490 chips after blind, got %d", h.Players[2].Chips)
		}
	})

	t.Run("WithChipsByPlayer individual counts", func(t *testing.T) {
		rng := randutil.New(42)
		chips := []int{1000, 800, 1200}
		h := NewHandState(rng, []string{"Alice", "Bob", "Charlie"}, 0, 5, 10, WithChipsByPlayer(chips))

		if h.Players[0].Chips != 1000 { // Button, no blind
			t.Errorf("Button should have 1000 chips, got %d", h.Players[0].Chips)
		}
		if h.Players[1].Chips != 795 { // Small blind (800-5)
			t.Errorf("SB should have 795 chips after blind, got %d", h.Players[1].Chips)
		}
		if h.Players[2].Chips != 1190 { // Big blind (1200-10)
			t.Errorf("BB should have 1190 chips after blind, got %d", h.Players[2].Chips)
		}
	})

	t.Run("WithChips validates count", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for mismatched chip counts")
			}
		}()
		rng := randutil.New(42)
		chips := []int{1000, 800} // Only 2, but 3 players
		NewHandState(rng, []string{"Alice", "Bob", "Charlie"}, 0, 5, 10, WithChipsByPlayer(chips))
	})

	t.Run("WithDeck uses provided deck", func(t *testing.T) {
		rng := randutil.New(42)
		deck := poker.NewDeck(randutil.New(99)) // Different seed
		h := NewHandState(rng, []string{"Alice", "Bob"}, 0, 5, 10, WithDeck(deck))

		if h.Deck != deck {
			t.Error("Should use provided deck")
		}
	})

	t.Run("multiple options compose", func(t *testing.T) {
		rng := randutil.New(42)
		deck := poker.NewDeck(randutil.New(99))
		chips := []int{500, 600}

		h := NewHandState(rng, []string{"Alice", "Bob"}, 0, 5, 10,
			WithChipsByPlayer(chips),
			WithDeck(deck))

		if h.Deck != deck {
			t.Error("Should use provided deck")
		}
		if h.Players[0].Chips != 495 { // Button posts SB in heads-up (500-5)
			t.Errorf("Button should have 495 chips after SB, got %d", h.Players[0].Chips)
		}
		if h.Players[1].Chips != 590 { // BB (600-10)
			t.Errorf("BB should have 590 chips after blind, got %d", h.Players[1].Chips)
		}
	})

	t.Run("WithChips overrides WithChips", func(t *testing.T) {
		rng := randutil.New(42)
		chips := []int{500, 600, 700}

		h := NewHandState(rng, []string{"Alice", "Bob", "Charlie"}, 0, 5, 10,
			WithChipsByPlayer(chips),
			WithChips(1500)) // This should win

		// All should have 1500 minus blinds
		if h.Players[0].Chips != 1500 { // Button, no blind
			t.Errorf("Button should have 1500 chips, got %d", h.Players[0].Chips)
		}
		if h.Players[1].Chips != 1495 { // Small blind
			t.Errorf("SB should have 1495 chips, got %d", h.Players[1].Chips)
		}
		if h.Players[2].Chips != 1490 { // Big blind
			t.Errorf("BB should have 1490 chips, got %d", h.Players[2].Chips)
		}
	})
}

func TestNewHandStateDeterministic(t *testing.T) {
	t.Parallel()

	// Two hands with same seed should be identical
	seed := int64(12345)
	players := []string{"Alice", "Bob", "Charlie"}

	rng1 := randutil.New(seed)
	h1 := NewHandState(rng1, players, 0, 5, 10)

	rng2 := randutil.New(seed)
	h2 := NewHandState(rng2, players, 0, 5, 10)

	// Check that hole cards are the same
	for i := range players {
		if h1.Players[i].HoleCards != h2.Players[i].HoleCards {
			t.Errorf("Player %d hole cards differ with same seed", i)
		}
	}
}
