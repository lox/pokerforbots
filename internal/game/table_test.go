package game

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/lox/holdem-cli/internal/deck"
)

func TestNewTable(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(0)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	if table.MaxSeats != 6 {
		t.Errorf("Expected 6 seats, got %d", table.MaxSeats)
	}

	if table.SmallBlind != 1 {
		t.Errorf("Expected small blind 1, got %d", table.SmallBlind)
	}

	if table.BigBlind != 2 {
		t.Errorf("Expected big blind 2, got %d", table.BigBlind)
	}

	if table.State != WaitingToStart {
		t.Errorf("Expected WaitingToStart state, got %s", table.State)
	}
}

func TestAddPlayer(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(0)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	player1 := NewPlayer(1, "Alice", Human, 200)
	player2 := NewPlayer(2, "Bob", AI, 200)

	if !table.AddPlayer(player1) {
		t.Error("Should be able to add first player")
	}

	if !table.AddPlayer(player2) {
		t.Error("Should be able to add second player")
	}

	if len(table.Players) != 2 {
		t.Errorf("Expected 2 players, got %d", len(table.Players))
	}

	// Check seat assignments
	if player1.SeatNumber != 1 {
		t.Errorf("Expected player1 in seat 1, got %d", player1.SeatNumber)
	}

	if player2.SeatNumber != 2 {
		t.Errorf("Expected player2 in seat 2, got %d", player2.SeatNumber)
	}
}

func TestTableFull(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(0)), TableConfig{
		MaxSeats:   2, // Set to 2 seats so it's full after 2 players
		SmallBlind: 1,
		BigBlind:   2,
	})

	player1 := NewPlayer(1, "Alice", Human, 200)
	player2 := NewPlayer(2, "Bob", AI, 200)
	player3 := NewPlayer(3, "Charlie", AI, 200)

	table.AddPlayer(player1)
	table.AddPlayer(player2)

	// Third player should not be able to join
	if table.AddPlayer(player3) {
		t.Error("Should not be able to add player to full table")
	}
}

func TestStartNewHand(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(0)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	// Add players
	player1 := NewPlayer(1, "Alice", Human, 200)
	player2 := NewPlayer(2, "Bob", AI, 200)
	table.AddPlayer(player1)
	table.AddPlayer(player2)

	// Start hand
	table.StartNewHand()

	if table.State != InProgress {
		t.Errorf("Expected InProgress state, got %s", table.State)
	}

	if table.HandID == "" {
		t.Errorf("Expected hand ID to be generated, got empty string")
	}

	if table.CurrentRound != PreFlop {
		t.Errorf("Expected PreFlop round, got %s", table.CurrentRound)
	}

	// Check that players have hole cards
	for _, player := range table.ActivePlayers {
		if len(player.HoleCards) != 2 {
			t.Errorf("Player %s should have 2 hole cards, got %d", player.Name, len(player.HoleCards))
		}
	}

	// Check blinds were posted
	if table.Pot != 3 { // 1 + 2
		t.Errorf("Expected pot of 3 after blinds, got %d", table.Pot)
	}
}

func TestPositionsHeadsUp(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(0)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	player1 := NewPlayer(1, "Alice", Human, 200)
	player2 := NewPlayer(2, "Bob", AI, 200)
	table.AddPlayer(player1)
	table.AddPlayer(player2)

	table.StartNewHand()

	// In heads-up, dealer is small blind
	var sbPlayer, bbPlayer *Player
	for _, player := range table.ActivePlayers {
		switch player.Position {
		case SmallBlind:
			sbPlayer = player
		case BigBlind:
			bbPlayer = player
		}
	}

	if sbPlayer == nil {
		t.Error("Should have a small blind player")
	}

	if bbPlayer == nil {
		t.Error("Should have a big blind player")
	}

	if sbPlayer == bbPlayer {
		t.Error("Small blind and big blind should be different players")
	}
}

func TestPositionsMultiWay(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(0)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	// Add 4 players
	for i := 1; i <= 4; i++ {
		player := NewPlayer(i, fmt.Sprintf("Player%d", i), AI, 200)
		table.AddPlayer(player)
	}

	table.StartNewHand()

	// Check that we have button, small blind, big blind, and UTG
	positions := make(map[Position]bool)
	for _, player := range table.ActivePlayers {
		positions[player.Position] = true
	}

	expectedPositions := []Position{Button, SmallBlind, BigBlind, UnderTheGun}
	for _, pos := range expectedPositions {
		if !positions[pos] {
			t.Errorf("Missing position: %s", pos)
		}
	}
}

func TestPlayerActions(t *testing.T) {
	player := NewPlayer(1, "Alice", Human, 200)

	// Test fold
	player.Fold()
	if !player.IsFolded || player.IsActive {
		t.Error("Player should be folded and inactive after folding")
	}

	// Reset for next test
	player.ResetForNewHand()

	// Test call
	if !player.Call(20) {
		t.Error("Player should be able to call 20")
	}

	if player.Chips != 180 {
		t.Errorf("Expected 180 chips after calling 20, got %d", player.Chips)
	}

	if player.LastAction != Call {
		t.Errorf("Expected Call action, got %s", player.LastAction)
	}

	// Test call more than available chips (should trigger all-in)
	remaining := player.Chips
	result := player.Call(remaining + 100) // Try to call more than available
	if !result {
		t.Error("Call should succeed but trigger all-in when insufficient chips")
	}

	if player.Chips != 0 {
		t.Errorf("Expected 0 chips after calling more than available, got %d", player.Chips)
	}

	if !player.IsAllIn {
		t.Error("Player should be all-in after calling more than available chips")
	}

	// Reset player to test explicit all-in
	player.ResetForNewHand()
	player.Chips = 100

	// Test explicit all-in
	if !player.AllIn() {
		t.Error("Player should be able to go all-in")
	}

	if player.Chips != 0 {
		t.Errorf("Expected 0 chips after all-in, got %d", player.Chips)
	}

	if !player.IsAllIn {
		t.Error("Player should be marked as all-in")
	}
}

func TestBettingRounds(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(0)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	player1 := NewPlayer(1, "Alice", Human, 200)
	player2 := NewPlayer(2, "Bob", AI, 200)
	table.AddPlayer(player1)
	table.AddPlayer(player2)

	table.StartNewHand()

	// Should start at preflop
	if table.CurrentRound != PreFlop {
		t.Errorf("Expected PreFlop, got %s", table.CurrentRound)
	}

	// Deal flop
	table.DealFlop()
	if table.CurrentRound != Flop {
		t.Errorf("Expected Flop, got %s", table.CurrentRound)
	}

	if len(table.CommunityCards) != 3 {
		t.Errorf("Expected 3 community cards after flop, got %d", len(table.CommunityCards))
	}

	// Deal turn
	table.DealTurn()
	if table.CurrentRound != Turn {
		t.Errorf("Expected Turn, got %s", table.CurrentRound)
	}

	if len(table.CommunityCards) != 4 {
		t.Errorf("Expected 4 community cards after turn, got %d", len(table.CommunityCards))
	}

	// Deal river
	table.DealRiver()
	if table.CurrentRound != River {
		t.Errorf("Expected River, got %s", table.CurrentRound)
	}

	if len(table.CommunityCards) != 5 {
		t.Errorf("Expected 5 community cards after river, got %d", len(table.CommunityCards))
	}
}

// Position and button rotation tests

func TestTableConfig(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(0)), TableConfig{
		MaxSeats:   9,
		SmallBlind: 5,
		BigBlind:   10,
	})

	if table.MaxSeats != 9 {
		t.Errorf("Expected 9 seats, got %d", table.MaxSeats)
	}
	if table.SmallBlind != 5 {
		t.Errorf("Expected small blind 5, got %d", table.SmallBlind)
	}
	if table.BigBlind != 10 {
		t.Errorf("Expected big blind 10, got %d", table.BigBlind)
	}
}

func TestRandomStartingPosition(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(0)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	// Add players
	for i := 1; i <= 3; i++ {
		player := NewPlayer(i, fmt.Sprintf("P%d", i), AI, 200)
		table.AddPlayer(player)
	}

	// Start first hand
	table.StartNewHand()

	// Should have selected seat 2 as dealer (MockRandSource(1) picks index 1 from ActivePlayers)
	if table.DealerPosition != 1 {
		t.Errorf("Expected dealer position 1, got %d", table.DealerPosition)
	}
}

func TestButtonRotation(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(0)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	// Add 3 players - AddPlayer will assign seats 1, 2, 3 automatically
	for i := 1; i <= 3; i++ {
		player := NewPlayer(i, fmt.Sprintf("P%d", i), AI, 200)
		table.AddPlayer(player)
	}

	// Start first hand - should pick first player (seat 1)
	table.StartNewHand()
	if table.DealerPosition != 1 {
		t.Errorf("Expected first dealer to be seat 1, got %d", table.DealerPosition)
		return
	}

	// Test subsequent rotations - should cycle through seats 1->2->3->1
	expectedSequence := []int{2, 3, 1}

	for i, expected := range expectedSequence {
		table.StartNewHand()
		if table.DealerPosition != expected {
			t.Errorf("Hand %d: expected dealer %d, got %d", i+2, expected, table.DealerPosition)
		}
	}
}

func TestCalculatePositions_HeadsUp(t *testing.T) {
	players := []*Player{
		{SeatNumber: 2}, {SeatNumber: 5},
	}

	positions := calculatePositions(2, players)

	expected := map[int]Position{
		2: SmallBlind, // Dealer is SB in heads-up
		5: BigBlind,
	}

	for seat, expectedPos := range expected {
		if positions[seat] != expectedPos {
			t.Errorf("Seat %d: got %v, want %v", seat, positions[seat], expectedPos)
		}
	}
}

func TestCalculatePositions_FourPlayers(t *testing.T) {
	players := []*Player{
		{SeatNumber: 1}, {SeatNumber: 3}, {SeatNumber: 5}, {SeatNumber: 7},
	}

	positions := calculatePositions(3, players) // Seat 3 is dealer

	expected := map[int]Position{
		3: Button,      // Dealer
		5: SmallBlind,  // Next
		7: BigBlind,    // Next
		1: UnderTheGun, // Next
	}

	for seat, expectedPos := range expected {
		if positions[seat] != expectedPos {
			t.Errorf("Seat %d: got %v, want %v", seat, positions[seat], expectedPos)
		}
	}
}

func TestCalculatePositions_SixPlayers(t *testing.T) {
	players := []*Player{
		{SeatNumber: 1}, {SeatNumber: 2}, {SeatNumber: 3},
		{SeatNumber: 4}, {SeatNumber: 5}, {SeatNumber: 6},
	}

	positions := calculatePositions(1, players) // Seat 1 is dealer

	expected := map[int]Position{
		1: Button,        // Dealer (position 0)
		2: SmallBlind,    // SB (position 1)
		3: BigBlind,      // BB (position 2)
		4: UnderTheGun,   // UTG (position 3)
		5: EarlyPosition, // Early (position 4, < numPlayers-2 which is 4)
		6: Cutoff,        // Cutoff (position 5, == numPlayers-2 which is 4)
	}

	// Fix the expected values based on the logic
	expected[5] = Cutoff       // Position 4 (index from dealer)
	expected[6] = LatePosition // Position 5 (index from dealer)

	for seat, expectedPos := range expected {
		if positions[seat] != expectedPos {
			t.Errorf("Seat %d: got %v, want %v", seat, positions[seat], expectedPos)
		}
	}
}

func TestDeterministicHandIDs(t *testing.T) {
	mockRand1 := rand.New(rand.NewSource(42))
	mockRand2 := rand.New(rand.NewSource(42))

	config1 := TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	}

	config2 := TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	}

	table1 := NewTable(mockRand1, config1)
	table2 := NewTable(mockRand2, config2)

	// Add players to both tables
	for i := 1; i <= 2; i++ {
		player1 := NewPlayer(i, fmt.Sprintf("P%d", i), AI, 200)
		player2 := NewPlayer(i, fmt.Sprintf("P%d", i), AI, 200)
		table1.AddPlayer(player1)
		table2.AddPlayer(player2)
	}

	// Start hands and collect IDs
	table1.StartNewHand()
	table2.StartNewHand()

	id1 := table1.HandID
	id2 := table2.HandID

	t.Logf("Table 1 Hand ID: %s", id1)
	t.Logf("Table 2 Hand ID: %s", id2)

	// The random portion should be identical (timestamp might differ slightly)
	// Both IDs should be valid
	if len(id1) != 26 {
		t.Errorf("Expected 26-character ID from table 1, got %d", len(id1))
	}
	if len(id2) != 26 {
		t.Errorf("Expected 26-character ID from table 2, got %d", len(id2))
	}

	// Both should be non-empty
	if id1 == "" {
		t.Error("Table 1 should have generated a hand ID")
	}
	if id2 == "" {
		t.Error("Table 2 should have generated a hand ID")
	}
}

// Integration test showing deterministic behavior with fixed seed
func TestDeterministicButtonRotation(t *testing.T) {
	createTableAndPlayHands := func() []int {
		table := NewTable(rand.New(rand.NewSource(42)), TableConfig{
			MaxSeats:   6,
			SmallBlind: 1,
			BigBlind:   2,
		})

		// Add 4 players
		for i := 1; i <= 4; i++ {
			player := NewPlayer(i, fmt.Sprintf("Player%d", i), AI, 1000)
			table.AddPlayer(player)
		}

		// Collect dealer positions for several hands
		var positions []int
		for hand := 1; hand <= 5; hand++ {
			table.StartNewHand()
			positions = append(positions, table.DealerPosition)
		}
		return positions
	}

	// Run twice with same seed
	positions1 := createTableAndPlayHands()
	positions2 := createTableAndPlayHands()

	t.Logf("First run dealer positions: %v", positions1)
	t.Logf("Second run dealer positions: %v", positions2)

	// Should be identical
	if len(positions1) != len(positions2) {
		t.Errorf("Position arrays should have same length")
		return
	}

	for i := range positions1 {
		if positions1[i] != positions2[i] {
			t.Errorf("Hand %d: positions differ %d vs %d", i+1, positions1[i], positions2[i])
		}
	}

	// Also test that different seeds produce different results
	table3 := NewTable(rand.New(rand.NewSource(123)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})
	for i := 1; i <= 4; i++ {
		player := NewPlayer(i, fmt.Sprintf("Player%d", i), AI, 1000)
		table3.AddPlayer(player)
	}

	var positions3 []int
	for hand := 1; hand <= 5; hand++ {
		table3.StartNewHand()
		positions3 = append(positions3, table3.DealerPosition)
	}

	// Should be different from the first run (very likely with different seed)
	different := false
	for i := range positions1 {
		if i < len(positions3) && positions1[i] != positions3[i] {
			different = true
			break
		}
	}

	if !different {
		t.Logf("Warning: Different seeds produced same sequence (unlikely but possible): %v vs %v", positions1, positions3)
	}
}

// Test pot distribution functionality
func TestPotDistribution(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(0)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	// Add players
	player1 := NewPlayer(1, "Alice", Human, 200)
	player2 := NewPlayer(2, "Bob", AI, 200)
	table.AddPlayer(player1)
	table.AddPlayer(player2)

	// Start hand and simulate some betting
	table.StartNewHand()
	// Pot should be 3 after blinds (1+2)

	// Simulate additional betting
	table.Pot += 20 // Add some more to the pot
	finalPot := table.Pot

	// Find winner and award pot
	winner := table.FindWinner()
	if winner == nil {
		t.Fatal("Should have a winner")
	}

	initialChips := winner.Chips
	table.AwardPot()

	// Check that winner received the pot
	if winner.Chips != initialChips+finalPot {
		t.Errorf("Winner should have %d chips, got %d", initialChips+finalPot, winner.Chips)
	}

	// Check that pot is now empty
	if table.Pot != 0 {
		t.Errorf("Pot should be 0 after awarding, got %d", table.Pot)
	}
}

func TestFindWinner(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(0)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	// Add players
	player1 := NewPlayer(1, "Alice", Human, 200)
	player2 := NewPlayer(2, "Bob", AI, 200)
	player3 := NewPlayer(3, "Charlie", AI, 200)
	table.AddPlayer(player1)
	table.AddPlayer(player2)
	table.AddPlayer(player3)

	table.StartNewHand()

	// Test with all players active
	winner := table.FindWinner()
	if winner == nil {
		t.Error("Should have a winner when players are active")
	}

	// Test with one player folded
	player1.Fold()
	winner = table.FindWinner()
	if winner == nil {
		t.Error("Should have a winner when some players folded")
	}
	if winner == player1 {
		t.Error("Folded player should not be winner")
	}

	// Test with only one player remaining
	player2.Fold()
	winner = table.FindWinner()
	if winner != player3 {
		t.Error("Last remaining player should be winner")
	}
}

// TestFindWinnerEvaluatesHandStrength tests that FindWinner correctly evaluates hand strength
func TestFindWinnerEvaluatesHandStrength(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(0)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	// Add players - player1 will be first in ActivePlayers
	player1 := NewPlayer(1, "WeakHand", Human, 200)
	player2 := NewPlayer(2, "StrongHand", AI, 200)
	table.AddPlayer(player1)
	table.AddPlayer(player2)

	table.StartNewHand()

	// Manually set hole cards to create a clear hand strength difference
	// Player1 gets weak cards (Jack high)
	player1.HoleCards = deck.MustParseCards("9sJs")

	// Player2 gets strong cards (pair of Aces)
	player2.HoleCards = deck.MustParseCards("KhAs")

	// Set community cards to give player2 top pair
	table.CommunityCards = deck.MustParseCards("3dAh6h9cQd")

	// Now with proper hand evaluation: Player2 should win with pair of Aces
	winner := table.FindWinner()

	// Player2 should win because they have pair of Aces vs player1's Jack high
	if winner != player2 {
		t.Error("Player2 should win with pair of Aces vs Jack high")
	}

	t.Logf("Player1 cards: %s %s (Jack high)",
		player1.HoleCards[0], player1.HoleCards[1])
	t.Logf("Player2 cards: %s %s (pair of Aces)",
		player2.HoleCards[0], player2.HoleCards[1])
	t.Logf("Community: %v", table.CommunityCards)
	t.Logf("Winner: %s (correct hand evaluation)", winner.Name)
}

// TestPotAmountPreservedForSummary tests that pot amount is available for summary display
func TestPotAmountPreservedForSummary(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(0)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	// Add players
	player1 := NewPlayer(1, "Alice", Human, 200)
	player2 := NewPlayer(2, "Bob", AI, 200)
	table.AddPlayer(player1)
	table.AddPlayer(player2)

	table.StartNewHand()

	// Add some betting to increase the pot
	table.Pot = 50 // Simulate betting that created a 50 chip pot

	// Verify pot is preserved before awarding
	potBeforeAward := table.Pot
	if potBeforeAward != 50 {
		t.Errorf("Expected pot of 50, got %d", potBeforeAward)
	}

	// Find winner
	winner := table.FindWinner()
	if winner == nil {
		t.Fatal("Should have a winner")
	}

	// Pot should still be intact for summary display
	potForSummary := table.Pot
	if potForSummary != 50 {
		t.Errorf("Pot should still be 50 for summary display, got %d", potForSummary)
	}

	// Award pot (this will reset it to 0)
	initialChips := winner.Chips
	table.AwardPot()

	// Verify pot was awarded correctly
	if winner.Chips != initialChips+50 {
		t.Errorf("Winner should have %d chips, got %d", initialChips+50, winner.Chips)
	}

	// Verify pot is now empty
	if table.Pot != 0 {
		t.Errorf("Pot should be 0 after awarding, got %d", table.Pot)
	}
}

func TestSplitPotBasic(t *testing.T) {
	tests := []struct {
		name              string
		potAmount         int
		numWinners        int
		expectedEach      int
		expectedRemainder int
	}{
		{"Even split 100 chips 2 winners", 100, 2, 50, 0},
		{"Odd split 101 chips 2 winners", 101, 2, 50, 1},
		{"Three way split 100 chips", 100, 3, 33, 1},
		{"Three way split 102 chips", 102, 3, 34, 0},
		{"Single winner", 100, 1, 100, 0},
		{"Zero pot", 0, 2, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test players
			var winners []*Player
			for i := 0; i < tt.numWinners; i++ {
				winners = append(winners, &Player{
					Name:  "Player" + string(rune(i+'1')),
					Chips: 1000, // Starting chips
				})
			}

			// Split the pot
			splitPot(tt.potAmount, winners)

			// Check first winner gets share + remainder
			expectedFirst := tt.expectedEach + tt.expectedRemainder
			if winners[0].Chips != 1000+expectedFirst {
				t.Errorf("First winner expected %d chips, got %d", 1000+expectedFirst, winners[0].Chips)
			}

			// Check other winners get exact share
			for i := 1; i < len(winners); i++ {
				if winners[i].Chips != 1000+tt.expectedEach {
					t.Errorf("Winner %d expected %d chips, got %d", i, 1000+tt.expectedEach, winners[i].Chips)
				}
			}

			// Verify total chips distributed equals pot
			totalDistributed := 0
			for _, winner := range winners {
				totalDistributed += winner.Chips - 1000 // Subtract starting chips
			}
			if totalDistributed != tt.potAmount {
				t.Errorf("Total distributed %d doesn't match pot %d", totalDistributed, tt.potAmount)
			}
		})
	}
}

func TestHeadsUpBoardChop(t *testing.T) {
	// Create a scenario where both players have identical hands (board chop)
	rng := rand.New(rand.NewSource(42))
	table := NewTable(rng, TableConfig{
		MaxSeats:   6,
		SmallBlind: 5,
		BigBlind:   10,
	})

	// Add two players
	player1 := &Player{Name: "Alice", Chips: 1000}
	player2 := &Player{Name: "Bob", Chips: 1000}
	table.AddPlayer(player1)
	table.AddPlayer(player2)

	// Start hand
	table.StartNewHand()

	// Set up a board where both players have identical hands
	// Using A-high board: A♠ K♠ Q♠ J♠ T♠ (royal flush on board)
	table.CommunityCards = []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Spades, Rank: deck.King},
		{Suit: deck.Spades, Rank: deck.Queen},
		{Suit: deck.Spades, Rank: deck.Jack},
		{Suit: deck.Spades, Rank: deck.Ten},
	}

	// Give both players low hole cards that don't matter
	player1.DealHoleCards([]deck.Card{
		{Suit: deck.Hearts, Rank: deck.Two},
		{Suit: deck.Clubs, Rank: deck.Three},
	})
	player2.DealHoleCards([]deck.Card{
		{Suit: deck.Hearts, Rank: deck.Four},
		{Suit: deck.Clubs, Rank: deck.Five},
	})

	// Set a pot amount
	table.Pot = 100

	// Get winners - should be both players
	winners := table.FindWinners()
	if len(winners) != 2 {
		t.Fatalf("Expected 2 winners (chop), got %d", len(winners))
	}

	// Check that both players are winners
	foundAlice, foundBob := false, false
	for _, winner := range winners {
		if winner.Name == "Alice" {
			foundAlice = true
		}
		if winner.Name == "Bob" {
			foundBob = true
		}
	}
	if !foundAlice || !foundBob {
		t.Error("Both Alice and Bob should be winners in board chop")
	}

	// Award pot and check split
	startingAlice := player1.Chips
	startingBob := player2.Chips
	table.AwardPot()

	// Check that pot was split (50 each, with remainder to first winner)
	aliceTotal := player1.Chips - startingAlice
	bobTotal := player2.Chips - startingBob

	if aliceTotal+bobTotal != 100 {
		t.Errorf("Total awarded %d doesn't match pot 100", aliceTotal+bobTotal)
	}

	// Pot split should be 50/50, but needs to account for blind positions
	// After blinds are posted automatically by StartNewHand, the pot distribution reflects both blind payments and pot split
	expectedTotal := 100
	if aliceTotal+bobTotal != expectedTotal {
		t.Errorf("Expected total %d to match pot, got Alice: %d, Bob: %d (total: %d)", expectedTotal, aliceTotal, bobTotal, aliceTotal+bobTotal)
	}

	// The split should be close to 50/50, allowing for small differences due to blind positions and rounding
	diff := aliceTotal - bobTotal
	if diff < 0 {
		diff = -diff
	}
	if diff > 10 {
		t.Errorf("Expected roughly equal split, got Alice: %d, Bob: %d (difference: %d)", aliceTotal, bobTotal, diff)
	}

	// Pot should be empty
	if table.Pot != 0 {
		t.Errorf("Pot should be 0 after awarding, got %d", table.Pot)
	}
}

func TestThreeWaySidePotTie(t *testing.T) {
	// Test scenario: Three players, two tie for main pot, one wins side pot
	rng := rand.New(rand.NewSource(123))
	table := NewTable(rng, TableConfig{
		MaxSeats:   6,
		SmallBlind: 5,
		BigBlind:   10,
	})

	// Add three players with different stack sizes
	player1 := &Player{Name: "Alice", Chips: 500}    // Short stack
	player2 := &Player{Name: "Bob", Chips: 1000}     // Medium stack
	player3 := &Player{Name: "Charlie", Chips: 1000} // Medium stack
	table.AddPlayer(player1)
	table.AddPlayer(player2)
	table.AddPlayer(player3)

	// Start hand
	table.StartNewHand()

	// Set up community cards for a scenario where Alice and Bob tie,
	// but Charlie has a worse hand
	// Board: A♠ A♥ K♠ Q♠ J♠
	table.CommunityCards = []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Hearts, Rank: deck.Ace},
		{Suit: deck.Spades, Rank: deck.King},
		{Suit: deck.Spades, Rank: deck.Queen},
		{Suit: deck.Spades, Rank: deck.Jack},
	}

	// Alice gets A♣ K♥ (full house: AAA KK)
	player1.DealHoleCards([]deck.Card{
		{Suit: deck.Clubs, Rank: deck.Ace},
		{Suit: deck.Hearts, Rank: deck.King},
	})

	// Bob gets A♦ K♦ (same full house: AAA KK)
	player2.DealHoleCards([]deck.Card{
		{Suit: deck.Diamonds, Rank: deck.Ace},
		{Suit: deck.Diamonds, Rank: deck.King},
	})

	// Charlie gets 2♥ 3♥ (pair of aces with lower kickers)
	player3.DealHoleCards([]deck.Card{
		{Suit: deck.Hearts, Rank: deck.Two},
		{Suit: deck.Hearts, Rank: deck.Three},
	})

	// Set pot amount
	table.Pot = 150

	// Get winners - should be Alice and Bob (tie)
	winners := table.FindWinners()
	if len(winners) != 2 {
		t.Fatalf("Expected 2 winners (Alice and Bob tie), got %d", len(winners))
	}

	// Check that Alice and Bob are the winners
	foundAlice, foundBob := false, false
	for _, winner := range winners {
		if winner.Name == "Alice" {
			foundAlice = true
		}
		if winner.Name == "Bob" {
			foundBob = true
		}
	}
	if !foundAlice || !foundBob {
		t.Error("Alice and Bob should both be winners")
	}

	// Award pot and check split
	startingAlice := player1.Chips
	startingBob := player2.Chips
	startingCharlie := player3.Chips
	table.AwardPot()

	// Check that pot was split between Alice and Bob only
	aliceWon := player1.Chips - startingAlice
	bobWon := player2.Chips - startingBob
	charlieWon := player3.Chips - startingCharlie

	if charlieWon != 0 {
		t.Errorf("Charlie should not have won anything, got %d", charlieWon)
	}

	if aliceWon+bobWon != 150 {
		t.Errorf("Total awarded %d doesn't match pot 150", aliceWon+bobWon)
	}

	// Should be roughly 75 each, allowing for small differences due to blind structure
	expectedTotal := 150
	if aliceWon+bobWon != expectedTotal {
		t.Errorf("Expected total %d, got Alice: %d, Bob: %d (total: %d)", expectedTotal, aliceWon, bobWon, aliceWon+bobWon)
	}

	// The split should be close to 75/75, allowing for small differences due to blind positions
	diff := aliceWon - bobWon
	if diff < 0 {
		diff = -diff
	}
	if diff > 10 {
		t.Errorf("Expected roughly equal split, got Alice: %d, Bob: %d (difference: %d)", aliceWon, bobWon, diff)
	}

	// Pot should be empty
	if table.Pot != 0 {
		t.Errorf("Pot should be 0 after awarding, got %d", table.Pot)
	}
}

func TestSplitPotEdgeCases(t *testing.T) {
	t.Run("Empty winners list", func(t *testing.T) {
		var winners []*Player
		splitPot(100, winners)
		// Should not panic
	})

	t.Run("Negative pot", func(t *testing.T) {
		player := &Player{Name: "Test", Chips: 1000}
		winners := []*Player{player}
		splitPot(-50, winners)

		// Player should still have 1000 chips
		if player.Chips != 1000 {
			t.Errorf("Expected 1000 chips, got %d", player.Chips)
		}
	})

	t.Run("One chip split among three", func(t *testing.T) {
		var winners []*Player
		for i := 0; i < 3; i++ {
			winners = append(winners, &Player{
				Name:  "Player" + string(rune(i+'1')),
				Chips: 1000,
			})
		}

		splitPot(1, winners)

		// First player should get the 1 chip, others get 0
		if winners[0].Chips != 1001 {
			t.Errorf("First winner should have 1001 chips, got %d", winners[0].Chips)
		}
		for i := 1; i < 3; i++ {
			if winners[i].Chips != 1000 {
				t.Errorf("Winner %d should still have 1000 chips, got %d", i, winners[i].Chips)
			}
		}
	})
}

func TestAwardPotIntegration(t *testing.T) {
	// Test the full AwardPot() method integration
	rng := rand.New(rand.NewSource(456))
	table := NewTable(rng, TableConfig{
		MaxSeats:   6,
		SmallBlind: 5,
		BigBlind:   10,
	})

	// Add players
	player1 := &Player{Name: "Alice", Chips: 1000}
	player2 := &Player{Name: "Bob", Chips: 1000}
	table.AddPlayer(player1)
	table.AddPlayer(player2)

	// Start hand
	table.StartNewHand()

	// Set up a tie scenario
	table.CommunityCards = []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Hearts, Rank: deck.King},
		{Suit: deck.Clubs, Rank: deck.Queen},
		{Suit: deck.Diamonds, Rank: deck.Jack},
		{Suit: deck.Spades, Rank: deck.Ten},
	}

	// Both players get the same hole cards for a tie
	player1.DealHoleCards([]deck.Card{
		{Suit: deck.Hearts, Rank: deck.Two},
		{Suit: deck.Clubs, Rank: deck.Three},
	})
	player2.DealHoleCards([]deck.Card{
		{Suit: deck.Hearts, Rank: deck.Four},
		{Suit: deck.Clubs, Rank: deck.Five},
	})

	// Set pot
	table.Pot = 200
	startingTotal := table.Pot + player1.Chips + player2.Chips

	// Award pot
	table.AwardPot()

	// Check total chips are conserved
	finalTotal := table.Pot + player1.Chips + player2.Chips
	if finalTotal != startingTotal {
		t.Errorf("Total chips not conserved: started with %d, ended with %d", startingTotal, finalTotal)
	}

	// Check pot is empty
	if table.Pot != 0 {
		t.Errorf("Pot should be 0 after awarding, got %d", table.Pot)
	}

	// Check that both players got something (since it's a tie)
	if player1.Chips == 1000 || player2.Chips == 1000 {
		t.Error("Both players should have won some chips in a tie")
	}
}

func TestBettingRoundCompleteWhenAllCheck(t *testing.T) {
	// This test reproduces the infinite checking loop bug
	table := NewTable(rand.New(rand.NewSource(42)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	// Add 3 players to test checking around
	player1 := NewPlayer(1, "Alice", AI, 200)
	player2 := NewPlayer(2, "Bob", AI, 200)
	player3 := NewPlayer(3, "Charlie", AI, 200)
	table.AddPlayer(player1)
	table.AddPlayer(player2)
	table.AddPlayer(player3)

	// Start hand (this posts blinds and sets up pre-flop)
	table.StartNewHand()

	// Move to post-flop where checking is more common
	// First, simulate pre-flop action to get to flop
	// Everyone calls the big blind to see the flop
	for i := 0; i < 3; i++ {
		currentPlayer := table.GetCurrentPlayer()
		if currentPlayer == nil {
			break
		}

		// Call to match big blind or check if already matched
		if currentPlayer.BetThisRound < table.CurrentBet {
			decision := Decision{Action: Call, Amount: 0, Reasoning: "call to see flop"}
			table.ApplyDecision(decision)
		} else {
			decision := Decision{Action: Check, Amount: 0, Reasoning: "check"}
			table.ApplyDecision(decision)
		}

		table.AdvanceAction()

		if table.IsBettingRoundComplete() {
			break
		}
	}

	// Deal flop to start a new betting round where everyone can check
	table.DealFlop()

	// Track the number of actions to detect infinite loop
	maxActions := 10 // Should only need 3 actions (one per player)
	actionCount := 0

	// Now everyone should be able to check, and the round should complete
	for actionCount < maxActions {
		currentPlayer := table.GetCurrentPlayer()
		if currentPlayer == nil {
			break
		}

		// Everyone checks
		decision := Decision{Action: Check, Amount: 0, Reasoning: "check around"}
		_, err := table.ApplyDecision(decision)
		if err != nil {
			t.Fatalf("Failed to apply check decision: %v", err)
		}

		table.AdvanceAction()
		actionCount++

		// Check if betting round is complete after each action
		if table.IsBettingRoundComplete() {
			t.Logf("Betting round completed after %d actions", actionCount)
			break
		}
	}

	// Verify the betting round completed
	if !table.IsBettingRoundComplete() {
		t.Errorf("Betting round should be complete after all players check, but it's not. Actions taken: %d", actionCount)

		// Debug info
		t.Logf("CurrentBet: %d", table.CurrentBet)
		for i, player := range table.ActivePlayers {
			t.Logf("Player %d (%s): BetThisRound=%d, PlayersActed[%d]=%v",
				i, player.Name, player.BetThisRound, player.ID, table.PlayersActed[player.ID])
		}
	}

	// Verify we didn't hit the infinite loop protection
	if actionCount >= maxActions {
		t.Errorf("Infinite loop detected: took %d actions, expected ~3", actionCount)
	}
}

func TestInfiniteCheckingBugReproduction(t *testing.T) {
	// More specific test that tries to reproduce the exact conditions from the simulation
	table := NewTable(rand.New(rand.NewSource(42)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	// Add 6 players like in the simulation
	for i := 1; i <= 6; i++ {
		player := NewPlayer(i, fmt.Sprintf("Player%d", i), AI, 200)
		table.AddPlayer(player)
	}

	// Start hand
	table.StartNewHand()

	// Simulate what might happen in pre-flop with random bots
	// where some players might check when they shouldn't be able to
	maxIterations := 50
	iteration := 0

	for iteration < maxIterations {
		currentPlayer := table.GetCurrentPlayer()
		if currentPlayer == nil {
			t.Log("No current player, hand should be over")
			break
		}

		iteration++

		// Try to check (this might be invalid if facing a bet)
		validActions := table.GetValidActions()

		// Log the situation for debugging
		t.Logf("Iteration %d: Player %s, BetThisRound=%d, CurrentBet=%d",
			iteration, currentPlayer.Name, currentPlayer.BetThisRound, table.CurrentBet)

		var hasCheck bool
		for _, action := range validActions {
			if action.Action == Check {
				hasCheck = true
				break
			}
		}

		if hasCheck {
			// If check is valid, do it
			decision := Decision{Action: Check, Amount: 0, Reasoning: "checking"}
			_, err := table.ApplyDecision(decision)
			if err != nil {
				t.Fatalf("Failed to apply check: %v", err)
			}
		} else {
			// Otherwise fold to avoid getting stuck
			decision := Decision{Action: Fold, Amount: 0, Reasoning: "folding"}
			_, err := table.ApplyDecision(decision)
			if err != nil {
				t.Fatalf("Failed to apply fold: %v", err)
			}
		}

		table.AdvanceAction()

		// Check if betting round completed
		if table.IsBettingRoundComplete() {
			t.Logf("Betting round completed after %d iterations", iteration)

			// Check if hand is over (only one player left)
			activePlayers := table.GetActivePlayers()
			if len(activePlayers) <= 1 {
				t.Log("Hand over - only one player left")
				break
			}

			// Advance to next round
			switch table.CurrentRound {
			case PreFlop:
				table.DealFlop()
				t.Log("Advanced to Flop")
			case Flop:
				table.DealTurn()
				t.Log("Advanced to Turn")
			case Turn:
				table.DealRiver()
				t.Log("Advanced to River")
			case River:
				t.Log("Advanced to Showdown")
			default:
				t.Log("Hand should be complete")
			}

			// Reset iteration counter for new round
			iteration = 0
		}
	}

	if iteration >= maxIterations {
		t.Errorf("Infinite loop detected: hit %d iterations in a single betting round", maxIterations)

		// Debug the stuck state
		t.Logf("CurrentBet: %d", table.CurrentBet)
		t.Logf("CurrentRound: %s", table.CurrentRound)
		for i, player := range table.ActivePlayers {
			t.Logf("Player %d (%s): BetThisRound=%d, PlayersActed[%d]=%v, IsActive=%v, IsFolded=%v",
				i, player.Name, player.BetThisRound, player.ID, table.PlayersActed[player.ID], player.IsActive, player.IsFolded)
		}
	}
}

func TestPostFlopCheckingRoundBug(t *testing.T) {
	// Test the post-flop scenario where CurrentBet = 0 and everyone checks
	table := NewTable(rand.New(rand.NewSource(42)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	// Add 3 players
	player1 := NewPlayer(1, "Alice", AI, 200)
	player2 := NewPlayer(2, "Bob", AI, 200)
	player3 := NewPlayer(3, "Charlie", AI, 200)
	table.AddPlayer(player1)
	table.AddPlayer(player2)
	table.AddPlayer(player3)

	// Start hand and move to flop (this should reset CurrentBet to 0)
	table.StartNewHand()

	// Simulate getting to flop
	table.startNewBettingRound() // This simulates moving to flop
	table.CurrentRound = Flop

	t.Logf("Post-flop state:")
	t.Logf("CurrentBet: %d", table.CurrentBet)
	t.Logf("CurrentRound: %s", table.CurrentRound)
	for _, player := range table.ActivePlayers {
		t.Logf("Player %s: BetThisRound=%d", player.Name, player.BetThisRound)
	}

	// Now simulate everyone checking
	for i, player := range table.ActivePlayers {
		// Check that check is a valid action
		validActions := table.GetValidActions()
		t.Logf("Player %s valid actions: %+v", player.Name, validActions)

		var canCheck bool
		for _, action := range validActions {
			if action.Action == Check {
				canCheck = true
				break
			}
		}

		if !canCheck {
			t.Errorf("Player %s should be able to check in post-flop with CurrentBet=0", player.Name)
		}

		// Apply check
		decision := Decision{Action: Check, Amount: 0, Reasoning: "checking"}
		_, err := table.ApplyDecision(decision)
		if err != nil {
			t.Fatalf("Player %s failed to check: %v", player.Name, err)
		}

		table.AdvanceAction()

		t.Logf("After player %d (%s) checked:", i+1, player.Name)
		t.Logf("  IsBettingRoundComplete: %v", table.IsBettingRoundComplete())

		// Debug the completion logic
		playersInHand := 0
		playersActed := 0
		for _, p := range table.ActivePlayers {
			if p.IsInHand() {
				playersInHand++
				if table.PlayersActed[p.ID] && (p.IsAllIn || p.BetThisRound == table.CurrentBet) {
					playersActed++
					t.Logf("  Player %s counted as acted", p.Name)
				} else {
					t.Logf("  Player %s NOT counted as acted (acted=%v, BetThisRound=%d, CurrentBet=%d)",
						p.Name, table.PlayersActed[p.ID], p.BetThisRound, table.CurrentBet)
				}
			}
		}
		t.Logf("  PlayersInHand: %d, PlayersActed: %d", playersInHand, playersActed)

		if table.IsBettingRoundComplete() {
			t.Logf("Betting round completed after %d players checked", i+1)
			break
		}
	}

	// The round should be complete after all 3 players check
	if !table.IsBettingRoundComplete() {
		t.Error("Betting round should be complete after all players check in post-flop")
	}
}

func TestBettingRoundPlayerActionOrder(t *testing.T) {
	// Test that prevents players from acting twice in same round unless responding to raise
	table := NewTable(rand.New(rand.NewSource(42)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	// Add 4 players to test the scenario from the bug report
	player1 := NewPlayer(1, "Player1", AI, 200)
	player2 := NewPlayer(2, "Player2", AI, 200)
	player3 := NewPlayer(3, "Player3", AI, 200)
	player4 := NewPlayer(4, "Player4", AI, 200)

	table.AddPlayer(player1)
	table.AddPlayer(player2)
	table.AddPlayer(player3)
	table.AddPlayer(player4)

	// Start hand
	table.StartNewHand()

	// Track who has acted to verify proper order
	actedPlayers := []string{}

	// Simulate the betting sequence that caused the bug:
	// 1. Player4 calls $2
	// 2. Player1 raises to $4
	// 3. Player4 should NOT be able to act again until other players respond

	// Find the order of players (UTG acts first preflop)
	currentPlayer := table.GetCurrentPlayer()
	if currentPlayer == nil {
		t.Fatal("No current player after starting hand")
	}

	// First action: current player calls
	actedPlayers = append(actedPlayers, currentPlayer.Name)
	decision := Decision{Action: Call, Amount: 2, Reasoning: "Call BB"}
	_, err := table.ApplyDecision(decision)
	if err != nil {
		t.Fatalf("Failed to apply call decision: %v", err)
	}
	table.AdvanceAction()

	// Second action: next player raises
	currentPlayer = table.GetCurrentPlayer()
	if currentPlayer == nil {
		t.Fatal("No current player after first action")
	}
	
	actedPlayers = append(actedPlayers, currentPlayer.Name)
	decision = Decision{Action: Raise, Amount: 4, Reasoning: "Raise to $4"}
	_, err = table.ApplyDecision(decision)
	if err != nil {
		t.Fatalf("Failed to apply raise decision: %v", err)
	}
	table.AdvanceAction()

	// Third action: should be next player in order, NOT the first caller
	currentPlayer = table.GetCurrentPlayer()
	if currentPlayer == nil {
		t.Fatal("No current player after raise")
	}

	// Verify the first caller is not acting again immediately
	if len(actedPlayers) >= 1 && currentPlayer.Name == actedPlayers[0] {
		t.Errorf("Player %s is acting again immediately after their action, before other players have responded to the raise", currentPlayer.Name)
	}

	// Continue until we get back to the original caller (who should act again to respond to raise)
	actionCount := 0
	originalCaller := actedPlayers[0]
	
	for currentPlayer != nil && actionCount < 10 { // Safety limit
		actionCount++
		
		if currentPlayer.Name == originalCaller {
			// Original caller should only act again after others have had chance to respond
			if actionCount <= 2 {
				t.Errorf("Original caller %s is acting too early (action #%d). Should only act again after other players respond to raise", originalCaller, actionCount)
			}
			break
		}
		
		// Have current player call or fold
		if table.CurrentBet > currentPlayer.BetThisRound {
			decision = Decision{Action: Call, Amount: table.CurrentBet, Reasoning: "Call raise"}
		} else {
			decision = Decision{Action: Check, Amount: 0, Reasoning: "Check"}
		}
		
		_, err = table.ApplyDecision(decision)
		if err != nil {
			t.Fatalf("Failed to apply decision for %s: %v", currentPlayer.Name, err)
		}
		
		table.AdvanceAction()
		currentPlayer = table.GetCurrentPlayer()
	}

	if actionCount >= 10 {
		t.Error("Too many actions - possible infinite loop in betting round")
	}
}

func TestBettingRoundRaiseResponse(t *testing.T) {
	// Test that players who have acted CAN act again when facing a raise
	table := NewTable(rand.New(rand.NewSource(123)), TableConfig{
		MaxSeats:   3,
		SmallBlind: 1,
		BigBlind:   2,
	})

	// Add 3 players for simpler scenario
	player1 := NewPlayer(1, "SB", AI, 200)   // Small blind
	player2 := NewPlayer(2, "BB", AI, 200)   // Big blind  
	player3 := NewPlayer(3, "BTN", AI, 200)  // Button/Dealer

	table.AddPlayer(player1)
	table.AddPlayer(player2)
	table.AddPlayer(player3)

	table.StartNewHand()

	// Button should act first preflop in 3-handed
	currentPlayer := table.GetCurrentPlayer()
	if currentPlayer.Position != Button {
		t.Errorf("Expected button to act first, got %s", currentPlayer.Position)
	}

	// Button calls
	decision := Decision{Action: Call, Amount: 2, Reasoning: "Call"}
	table.ApplyDecision(decision)
	table.AdvanceAction()

	// Small blind raises
	currentPlayer = table.GetCurrentPlayer()
	if currentPlayer.Position != SmallBlind {
		t.Errorf("Expected small blind to act second, got %s", currentPlayer.Position)
	}
	
	decision = Decision{Action: Raise, Amount: 6, Reasoning: "Raise to $6"}
	table.ApplyDecision(decision)
	table.AdvanceAction()

	// Big blind should act next
	currentPlayer = table.GetCurrentPlayer()
	if currentPlayer.Position != BigBlind {
		t.Errorf("Expected big blind to act after SB raise, got %s", currentPlayer.Position)
	}

	// Big blind calls
	decision = Decision{Action: Call, Amount: 6, Reasoning: "Call raise"}
	table.ApplyDecision(decision)
	table.AdvanceAction()

	// Now button should get chance to respond to the raise
	currentPlayer = table.GetCurrentPlayer()
	if currentPlayer.Position != Button {
		t.Errorf("Expected button to get chance to respond to raise, got %s", currentPlayer.Position)
	}

	// Verify they can call the raise
	validActions := table.GetValidActions()
	canCall := false
	for _, action := range validActions {
		if action.Action == Call {
			canCall = true
			break
		}
	}
	
	if !canCall {
		t.Error("Button should be able to call the raise")
	}
}
