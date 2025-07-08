package game

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/lox/pokerforbots/internal/evaluator"
	"github.com/lox/pokerforbots/sdk/deck"
)

func TestNewTable(t *testing.T) {
	table := NewTestTable(
		WithSeed(0),
		WithBlinds(1, 2),
	)

	if table.maxSeats != 6 {
		t.Errorf("Expected 6 seats, got %d", table.maxSeats)
	}

	if table.smallBlind != 1 {
		t.Errorf("Expected small blind 1, got %d", table.smallBlind)
	}

	if table.bigBlind != 2 {
		t.Errorf("Expected big blind 2, got %d", table.bigBlind)
	}

	if table.state != WaitingToStart {
		t.Errorf("Expected WaitingToStart state, got %s", table.state)
	}
}

func TestAddPlayer(t *testing.T) {
	eventBus := NewEventBus()
	table := NewTable(rand.New(rand.NewSource(0)), eventBus, TableConfig{
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

	if len(table.players) != 2 {
		t.Errorf("Expected 2 players, got %d", len(table.players))
	}

	// Check seat assignments
	if player1.SeatNumber != 1 {
		t.Errorf("Expected player1 in seat 1, got %d", player1.SeatNumber)
	}

	if player2.SeatNumber != 2 {
		t.Errorf("Expected player2 in seat 2, got %d", player2.SeatNumber)
	}
}

func TestAddPlayerDuplicateID(t *testing.T) {
	eventBus := NewEventBus()
	table := NewTable(rand.New(rand.NewSource(0)), eventBus, TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	player1 := NewPlayer(1, "Alice", Human, 200)
	player2 := NewPlayer(1, "Bob", AI, 200) // Same ID as player1

	if !table.AddPlayer(player1) {
		t.Error("Should be able to add first player")
	}

	if table.AddPlayer(player2) {
		t.Error("Should not be able to add player with duplicate ID")
	}

	if len(table.players) != 1 {
		t.Errorf("Expected 1 player, got %d", len(table.players))
	}

	// Verify the first player is still there
	if table.players[0].Name != "Alice" {
		t.Errorf("Expected first player to be Alice, got %s", table.players[0].Name)
	}
}

func TestTableFull(t *testing.T) {
	eventBus := NewEventBus()
	table := NewTable(rand.New(rand.NewSource(0)), eventBus, TableConfig{
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
	eventBus := NewEventBus()
	table := NewTable(rand.New(rand.NewSource(0)), eventBus, TableConfig{
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

	if table.state != InProgress {
		t.Errorf("Expected InProgress state, got %s", table.state)
	}

	if table.handID == "" {
		t.Errorf("Expected hand ID to be generated, got empty string")
	}

	if table.currentRound != PreFlop {
		t.Errorf("Expected PreFlop round, got %s", table.currentRound)
	}

	// Check that players have hole cards
	for _, player := range table.activePlayers {
		if len(player.HoleCards) != 2 {
			t.Errorf("Player %s should have 2 hole cards, got %d", player.Name, len(player.HoleCards))
		}
	}

	// Check blinds were posted
	if table.pot != 3 { // 1 + 2
		t.Errorf("Expected pot of 3 after blinds, got %d", table.pot)
	}
}

func TestPositionsHeadsUp(t *testing.T) {
	eventBus := NewEventBus()
	table := NewTable(rand.New(rand.NewSource(0)), eventBus, TableConfig{
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
	for _, player := range table.activePlayers {
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
	eventBus := NewEventBus()
	table := NewTable(rand.New(rand.NewSource(0)), eventBus, TableConfig{
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
	if table.currentRound != PreFlop {
		t.Errorf("Expected PreFlop, got %s", table.currentRound)
	}

	// Deal flop
	table.DealFlop()
	if table.currentRound != Flop {
		t.Errorf("Expected Flop, got %s", table.currentRound)
	}

	if len(table.communityCards) != 3 {
		t.Errorf("Expected 3 community cards after flop, got %d", len(table.communityCards))
	}

	// Deal turn
	table.DealTurn()
	if table.currentRound != Turn {
		t.Errorf("Expected Turn, got %s", table.currentRound)
	}

	if len(table.communityCards) != 4 {
		t.Errorf("Expected 4 community cards after turn, got %d", len(table.communityCards))
	}

	// Deal river
	table.DealRiver()
	if table.currentRound != River {
		t.Errorf("Expected River, got %s", table.currentRound)
	}

	if len(table.communityCards) != 5 {
		t.Errorf("Expected 5 community cards after river, got %d", len(table.communityCards))
	}
}

// Position and button rotation tests

func TestButtonRotation(t *testing.T) {
	eventBus := NewEventBus()
	table := NewTable(rand.New(rand.NewSource(0)), eventBus, TableConfig{
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
	if table.dealerPosition != 1 {
		t.Errorf("Expected first dealer to be seat 1, got %d", table.dealerPosition)
		return
	}

	// Test subsequent rotations - should cycle through seats 1->2->3->1
	expectedSequence := []int{2, 3, 1}

	for i, expected := range expectedSequence {
		table.StartNewHand()
		if table.dealerPosition != expected {
			t.Errorf("Hand %d: expected dealer %d, got %d", i+2, expected, table.dealerPosition)
		}
	}
}

// Test pot distribution functionality
func TestPotDistribution(t *testing.T) {
	eventBus := NewEventBus()
	table := NewTable(rand.New(rand.NewSource(0)), eventBus, TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	// Add players
	player1 := NewPlayer(1, "Alice", Human, 200)
	player2 := NewPlayer(2, "Bob", AI, 200)
	table.AddPlayer(player1)
	table.AddPlayer(player2)

	// Start hand and simulate realistic betting
	table.StartNewHand()
	// Pot should be 3 after blinds (1+2)

	// Simulate realistic betting: current player raises
	currentPlayer := table.GetCurrentPlayer()
	if currentPlayer != nil {
		decision := Decision{Action: Raise, Amount: 10, Reasoning: "test raise"}
		_, err := table.ApplyDecision(decision)
		if err != nil {
			t.Fatalf("Failed to apply raise: %v", err)
		}
		table.AdvanceAction()

		// Other player calls
		currentPlayer = table.GetCurrentPlayer()
		if currentPlayer != nil {
			decision = Decision{Action: Call, Amount: 10, Reasoning: "call"}
			_, err = table.ApplyDecision(decision)
			if err != nil {
				t.Fatalf("Failed to apply call: %v", err)
			}
		}
	}

	finalPot := table.pot

	// Find winner and award pot
	winners := table.FindWinners()
	if len(winners) != 1 {
		t.Fatalf("Expected exactly 1 winner, got %d", len(winners))
	}
	winner := winners[0]
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
	if table.pot != 0 {
		t.Errorf("Pot should be 0 after awarding, got %d", table.pot)
	}
}

// TestFindWinnerEvaluatesHandStrength tests that FindWinners correctly evaluates hand strength
func TestFindWinnerEvaluatesHandStrength(t *testing.T) {
	eventBus := NewEventBus()
	table := NewTable(rand.New(rand.NewSource(0)), eventBus, TableConfig{
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
	table.communityCards = deck.MustParseCards("3dAh6h9cQd")

	// Now with proper hand evaluation: Player2 should win with pair of Aces
	winners := table.FindWinners()

	// Debug: check hand evaluations
	cards1 := append(player1.HoleCards, table.communityCards...)
	cards2 := append(player2.HoleCards, table.communityCards...)
	hand1 := evaluator.Evaluate7(cards1)
	hand2 := evaluator.Evaluate7(cards2)
	e := evaluator.NewEvaluator()

	t.Logf("Player1 hand: %s, rank: %d, class: %s", cards1, hand1, e.GetHandClass(int(hand1)))
	t.Logf("Player2 hand: %s, rank: %d, class: %s", cards2, hand2, e.GetHandClass(int(hand2)))
	t.Logf("Compare result: %d (positive means hand2 is better)", hand1.Compare(hand2))

	if len(winners) != 1 {
		t.Fatalf("Expected exactly 1 winner, got %d", len(winners))
	}
	winner := winners[0]

	// Player2 should win because they have pair of Aces vs player1's Jack high
	if winner != player2 {
		t.Error("Player2 should win with pair of Aces vs Jack high")
	}

	t.Logf("Player1 cards: %s %s (Jack high)",
		player1.HoleCards[0], player1.HoleCards[1])
	t.Logf("Player2 cards: %s %s (pair of Aces)",
		player2.HoleCards[0], player2.HoleCards[1])
	t.Logf("Community: %v", table.communityCards)
	t.Logf("Winner: %s (correct hand evaluation)", winner.Name)
}

// TestPotAmountPreservedForSummary tests that pot amount is available for summary display
func TestPotAmountPreservedForSummary(t *testing.T) {
	eventBus := NewEventBus()
	table := NewTable(rand.New(rand.NewSource(0)), eventBus, TableConfig{
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

	// Simulate realistic betting to create a larger pot
	currentPlayer := table.GetCurrentPlayer()
	if currentPlayer != nil {
		// First player raises to 25
		decision := Decision{Action: Raise, Amount: 25, Reasoning: "big raise"}
		_, err := table.ApplyDecision(decision)
		if err != nil {
			t.Fatalf("Failed to apply raise: %v", err)
		}
		table.AdvanceAction()

		// Second player calls
		currentPlayer = table.GetCurrentPlayer()
		if currentPlayer != nil {
			decision = Decision{Action: Call, Amount: 25, Reasoning: "call"}
			_, err = table.ApplyDecision(decision)
			if err != nil {
				t.Fatalf("Failed to apply call: %v", err)
			}
		}
	}

	// Verify pot is preserved before awarding
	potBeforeAward := table.pot
	if potBeforeAward < 3 { // Should be at least blinds + some betting
		t.Errorf("Expected pot to be larger, got %d", potBeforeAward)
	}

	// Find winner
	winners := table.FindWinners()
	if len(winners) != 1 {
		t.Fatalf("Expected exactly 1 winner, got %d", len(winners))
	}
	winner := winners[0]
	if winner == nil {
		t.Fatal("Should have a winner")
	}

	// Pot should still be intact for summary display
	potForSummary := table.pot
	if potForSummary != potBeforeAward {
		t.Errorf("Pot should still be %d for summary display, got %d", potBeforeAward, potForSummary)
	}

	// Award pot (this will reset it to 0)
	initialChips := winner.Chips
	table.AwardPot()

	// Verify pot was awarded correctly
	if winner.Chips != initialChips+potBeforeAward {
		t.Errorf("Winner should have %d chips, got %d", initialChips+potBeforeAward, winner.Chips)
	}

	// Verify pot is now empty
	if table.pot != 0 {
		t.Errorf("Pot should be 0 after awarding, got %d", table.pot)
	}
}

func TestBettingRoundCompleteWhenAllCheck(t *testing.T) {
	// This test reproduces the infinite checking loop bug
	eventBus := NewEventBus()
	table := NewTable(rand.New(rand.NewSource(42)), eventBus, TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
		Seed:       42,
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
		if currentPlayer.BetThisRound < table.currentBet {
			callAmount := table.currentBet
			decision := Decision{Action: Call, Amount: callAmount, Reasoning: "call to see flop"}
			_, err := table.ApplyDecision(decision)
			if err != nil {
				t.Fatalf("Failed to apply call: %v", err)
			}
		} else {
			decision := Decision{Action: Check, Amount: 0, Reasoning: "check"}
			_, err := table.ApplyDecision(decision)
			if err != nil {
				t.Fatalf("Failed to apply check: %v", err)
			}
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
		t.Logf("CurrentBet: %d", table.currentBet)
		for i, player := range table.activePlayers {
			t.Logf("Player %d (%s): BetThisRound=%d, PlayersActed[%d]=%v",
				i, player.Name, player.BetThisRound, player.ID, table.playersActed[player.ID])
		}
	}

	// Verify we didn't hit the infinite loop protection
	if actionCount >= maxActions {
		t.Errorf("Infinite loop detected: took %d actions, expected ~3", actionCount)
	}
}

func TestPostFlopCheckingRoundBug(t *testing.T) {
	// Test the post-flop scenario where CurrentBet = 0 and everyone checks
	eventBus := NewEventBus()
	table := NewTable(rand.New(rand.NewSource(42)), eventBus, TableConfig{
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
	table.currentRound = Flop

	t.Logf("Post-flop state:")
	t.Logf("CurrentBet: %d", table.currentBet)
	t.Logf("CurrentRound: %s", table.currentRound)
	for _, player := range table.activePlayers {
		t.Logf("Player %s: BetThisRound=%d", player.Name, player.BetThisRound)
	}

	// Now simulate everyone checking
	for i, player := range table.activePlayers {
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
		for _, p := range table.activePlayers {
			if p.IsInHand() {
				playersInHand++
				if table.playersActed[p.ID] && (p.IsAllIn || p.BetThisRound == table.currentBet) {
					playersActed++
					t.Logf("  Player %s counted as acted", p.Name)
				} else {
					t.Logf("  Player %s NOT counted as acted (acted=%v, BetThisRound=%d, CurrentBet=%d)",
						p.Name, table.playersActed[p.ID], p.BetThisRound, table.currentBet)
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
	eventBus := NewEventBus()
	table := NewTable(rand.New(rand.NewSource(42)), eventBus, TableConfig{
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
		if table.currentBet > currentPlayer.BetThisRound {
			decision = Decision{Action: Call, Amount: table.currentBet, Reasoning: "Call raise"}
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
	eventBus := NewEventBus()
	table := NewTable(rand.New(rand.NewSource(123)), eventBus, TableConfig{
		MaxSeats:   3,
		SmallBlind: 1,
		BigBlind:   2,
	})

	// Add 3 players for simpler scenario
	player1 := NewPlayer(1, "SB", AI, 200)  // Small blind
	player2 := NewPlayer(2, "BB", AI, 200)  // Big blind
	player3 := NewPlayer(3, "BTN", AI, 200) // Button/Dealer

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
	_, _ = table.ApplyDecision(decision)
	table.AdvanceAction()

	// Small blind raises
	currentPlayer = table.GetCurrentPlayer()
	if currentPlayer.Position != SmallBlind {
		t.Errorf("Expected small blind to act second, got %s", currentPlayer.Position)
	}

	decision = Decision{Action: Raise, Amount: 6, Reasoning: "Raise to $6"}
	_, _ = table.ApplyDecision(decision)
	table.AdvanceAction()

	// Big blind should act next
	currentPlayer = table.GetCurrentPlayer()
	if currentPlayer.Position != BigBlind {
		t.Errorf("Expected big blind to act after SB raise, got %s", currentPlayer.Position)
	}

	// Big blind calls
	decision = Decision{Action: Call, Amount: 6, Reasoning: "Call raise"}
	_, _ = table.ApplyDecision(decision)
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

// TestPotAwardingBug reproduces the bug where winner doesn't receive full pot
func TestPotAwardingBug(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	eventBus := NewEventBus()
	table := NewTable(rng, eventBus, TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	// Add 6 players with 200 chips each
	players := []*Player{
		NewPlayer(1, "You", Human, 200),
		NewPlayer(2, "AI-2", AI, 200),
		NewPlayer(3, "AI-3", AI, 200),
		NewPlayer(4, "AI-4", AI, 200),
		NewPlayer(5, "AI-5", AI, 200),
		NewPlayer(6, "AI-6", AI, 200),
	}

	for _, player := range players {
		table.AddPlayer(player)
	}

	// Start hand and deal hole cards
	table.StartNewHand()

	// Simulate the exact betting scenario from the bug:
	// Pre-flop: AI-6 calls $2, You raise to $7, AI-2 calls $7, AI-3 folds, AI-4 raises to $12, AI-5 folds, AI-6 folds, You calls $5, AI-2 calls $5
	// After pre-flop: You=$12, AI-2=$12, AI-4=$12, others folded with smaller amounts

	// Manually set up the betting state after pre-flop
	you := players[0] // You
	ai2 := players[1] // AI-2
	ai3 := players[2] // AI-3
	ai4 := players[3] // AI-4
	ai5 := players[4] // AI-5
	ai6 := players[5] // AI-6

	// Set up final state manually to match the bug scenario
	// Each player's chips should be: starting_chips - total_bet
	you.Chips = 0 // All-in for $200, so 0 chips left
	you.TotalBet = 200
	you.IsAllIn = true

	ai2.Chips = 200 - 52 // Bet $52, folded
	ai2.TotalBet = 52
	ai2.IsFolded = true

	ai3.Chips = 200 - 0 // Folded pre-flop, no contribution
	ai3.TotalBet = 0
	ai3.IsFolded = true

	ai4.Chips = 200 - 52 // Bet $52, still in hand but didn't call all-in
	ai4.TotalBet = 52
	ai4.IsFolded = false // Still in hand but didn't call all-in

	ai5.Chips = 200 - 2 // Posted big blind $2, then folded
	ai5.TotalBet = 2
	ai5.IsFolded = true

	ai6.Chips = 200 - 2 // Called big blind $2 initially, then folded
	ai6.TotalBet = 2
	ai6.IsFolded = true

	// Set the pot to match the total contributions
	expectedPot := 200 + 52 + 0 + 52 + 2 + 2 // = 308
	table.pot = expectedPot

	// Track total chips before awarding (should be starting chips minus pot)
	totalChipsBefore := 0
	for _, p := range players {
		totalChipsBefore += p.Chips
	}
	// Total should be: 6*200 - 308 = 1200 - 308 = 892
	expectedTotalBefore := 6*200 - expectedPot
	if totalChipsBefore != expectedTotalBefore {
		t.Errorf("Total chips before awarding should be %d, got %d", expectedTotalBefore, totalChipsBefore)
	}

	// Record initial chip counts
	initialChips := make([]int, len(players))
	for i, p := range players {
		initialChips[i] = p.Chips
	}

	// Award pot (You should be the only winner since AI-4 didn't call the all-in)
	table.AwardPot()

	// Track total chips after awarding
	totalChipsAfter := 0
	for _, p := range players {
		totalChipsAfter += p.Chips
	}

	// Check that total chips equals starting amount (chips conserved)
	expectedTotalAfter := 6 * 200 // Should be back to starting total
	if totalChipsAfter != expectedTotalAfter {
		t.Errorf("Total chips should be %d, got %d (difference: %d)",
			expectedTotalAfter, totalChipsAfter, expectedTotalAfter-totalChipsAfter)
	}

	// Use the new chip conservation validation method
	if err := table.ValidateChipConservation(expectedTotalAfter); err != nil {
		t.Errorf("Chip conservation validation failed: %v", err)
	}

	// You should have won the entire pot since you're the only player still in hand
	expectedYourChips := initialChips[0] + expectedPot
	if you.Chips != expectedYourChips {
		t.Errorf("You should have %d chips (initial %d + pot %d), but got %d",
			expectedYourChips, initialChips[0], expectedPot, you.Chips)
	}

	// Verify pot was fully distributed
	if table.pot != 0 {
		t.Errorf("Pot should be 0 after awarding, but got %d", table.pot)
	}
}

// TestChipConservation demonstrates the chip conservation validation
// This assertion can be used in any test to ensure chips aren't created or destroyed
// TestFlopBettingRoundCompleteness reproduces the bug where only one player acts on flop
// before the betting round completes, when all players should get a chance to act
func TestFlopBettingRoundCompleteness(t *testing.T) {
	eventBus := NewEventBus()
	table := NewTable(rand.New(rand.NewSource(12345)), eventBus, TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	// Add 6 players to match the scenario from the logs
	players := []*Player{
		NewPlayer(1, "Bot_4", AI, 200),
		NewPlayer(2, "Bot_5", AI, 200),
		NewPlayer(3, "Lox", Human, 200),
		NewPlayer(4, "Bot_1", AI, 200),
		NewPlayer(5, "Bot_2", AI, 200),
		NewPlayer(6, "Bot_3", AI, 200),
	}

	for _, player := range players {
		table.AddPlayer(player)
	}

	// Set positions to match the original scenario
	players[0].Position = SmallBlind     // Bot_4
	players[1].Position = BigBlind       // Bot_5
	players[2].Position = UnderTheGun    // Lox
	players[3].Position = MiddlePosition // Bot_1
	players[4].Position = Cutoff         // Bot_2
	players[5].Position = Button         // Bot_3

	// Start hand
	table.StartNewHand()

	// Simulate pre-flop: Bot_3 folds, others call
	actionCount := 0
	maxPreFlopActions := 10
	for !table.IsBettingRoundComplete() && actionCount < maxPreFlopActions {
		currentPlayer := table.GetCurrentPlayer()
		if currentPlayer == nil {
			t.Fatal("No current player during pre-flop")
		}

		var decision Decision
		if currentPlayer.Name == "Bot_3" {
			decision = Decision{Action: Fold, Amount: 0, Reasoning: "Folding on button"}
		} else {
			// Check what action is needed - if player hasn't bet enough, call; otherwise check
			if currentPlayer.BetThisRound < table.currentBet {
				callAmount := table.currentBet - currentPlayer.BetThisRound
				decision = Decision{Action: Call, Amount: callAmount, Reasoning: "Calling to see flop"}
			} else {
				decision = Decision{Action: Check, Amount: 0, Reasoning: "Checking"}
			}
		}

		_, err := table.ApplyDecision(decision)
		if err != nil {
			t.Fatalf("Failed to apply decision for %s: %v", currentPlayer.Name, err)
		}

		table.AdvanceAction()
		actionCount++
	}

	if !table.IsBettingRoundComplete() {
		t.Fatal("Pre-flop should be complete")
	}

	// Count players still in hand after pre-flop
	playersInHandPreFlop := 0
	for _, player := range table.activePlayers {
		if player.IsInHand() {
			playersInHandPreFlop++
		}
	}

	// Should have 5 players in hand (all except Bot_3 who folded)
	if playersInHandPreFlop != 5 {
		t.Fatalf("Expected 5 players in hand after pre-flop, got %d", playersInHandPreFlop)
	}

	// Move to flop
	table.DealFlop()

	// Verify flop state
	if table.currentRound != Flop {
		t.Errorf("Expected Flop round, got %s", table.currentRound)
	}

	if table.currentBet != 0 {
		t.Errorf("Expected current bet to be 0 on flop, got %d", table.currentBet)
	}

	// Count players who can act on flop
	playersCanActFlop := 0
	for _, player := range table.activePlayers {
		if player.IsInHand() && player.CanAct() {
			playersCanActFlop++
		}
	}

	t.Logf("Players who can act on flop: %d", playersCanActFlop)

	// Track flop actions - this is where the bug manifests
	flopActionCount := 0
	playersWhoActedOnFlop := make(map[string]bool)
	maxFlopActions := 10 // Safety limit

	// Before we start, verify the betting round is not already complete
	if table.IsBettingRoundComplete() {
		t.Fatal("BUG REPRODUCED: Flop betting round is already complete before any player acts!")
	}

	for !table.IsBettingRoundComplete() && flopActionCount < maxFlopActions {
		currentPlayer := table.GetCurrentPlayer()
		if currentPlayer == nil {
			t.Fatal("No current player on flop")
		}

		// Track which players act
		playersWhoActedOnFlop[currentPlayer.Name] = true

		t.Logf("Flop action %d: %s acting", flopActionCount+1, currentPlayer.Name)

		// Everyone checks on flop
		decision := Decision{Action: Check, Amount: 0, Reasoning: "Checking flop"}
		_, err := table.ApplyDecision(decision)
		if err != nil {
			t.Fatalf("Failed to apply check for %s: %v", currentPlayer.Name, err)
		}

		table.AdvanceAction()
		flopActionCount++

		// Log state after each action for debugging
		t.Logf("After %s acted: IsBettingRoundComplete=%v", currentPlayer.Name, table.IsBettingRoundComplete())
	}

	t.Logf("Flop completed after %d actions", flopActionCount)
	t.Logf("Players who acted on flop: %v", playersWhoActedOnFlop)

	// THE BUG: Only Bot_4 acts on flop before round completes
	// This should be fixed by our changes to IsBettingRoundComplete()
	expectedFlopActions := playersInHandPreFlop // All 5 players should act
	if flopActionCount < expectedFlopActions {
		t.Errorf("BUG REPRODUCED: Only %d players acted on flop, expected %d", flopActionCount, expectedFlopActions)

		// Debug information
		t.Logf("Debug info:")
		t.Logf("  Current round: %v", table.GetCurrentRound())
		t.Logf("  Current bet: %d", table.GetCurrentBet())
		t.Logf("  Action on: %d", table.GetActionOn())
		t.Logf("  Active players: %d", len(table.GetActivePlayers()))

		playersInHand := 0
		playersCanAct := 0
		for i, player := range table.GetActivePlayers() {
			inHand := player.IsInHand()
			canAct := player.CanAct()
			if inHand {
				playersInHand++
			}
			if canAct {
				playersCanAct++
			}
			t.Logf("  [%d] %s: InHand=%v, CanAct=%v, Active=%v, Folded=%v",
				i, player.Name, inHand, canAct, player.IsActive, player.IsFolded)
		}
		t.Logf("  Players in hand: %d, can act: %d", playersInHand, playersCanAct)
	}

	// Verify that ALL players who were in hand got to act
	if len(playersWhoActedOnFlop) != expectedFlopActions {
		t.Errorf("Expected %d different players to act on flop, but %d acted", expectedFlopActions, len(playersWhoActedOnFlop))
	}

	// Verify specific players acted (all except Bot_3 who folded pre-flop)
	expectedPlayers := []string{"Bot_4", "Bot_5", "Lox", "Bot_1", "Bot_2"}
	for _, expectedPlayer := range expectedPlayers {
		if !playersWhoActedOnFlop[expectedPlayer] {
			t.Errorf("Player %s should have acted on flop but didn't", expectedPlayer)
		}
	}

	// Bot_3 should NOT have acted (folded pre-flop)
	if playersWhoActedOnFlop["Bot_3"] {
		t.Errorf("Bot_3 should not have acted on flop (folded pre-flop)")
	}
}

// TestIsBettingRoundComplete tests the edge cases of betting round completion logic
func TestIsBettingRoundComplete(t *testing.T) {
	eventBus := NewEventBus()

	t.Run("should not complete early with multiple players who can act", func(t *testing.T) {
		table := NewTable(rand.New(rand.NewSource(42)), eventBus, TableConfig{
			MaxSeats:   6,
			SmallBlind: 1,
			BigBlind:   2,
		})

		// Add multiple active players
		for i := 1; i <= 5; i++ {
			player := NewPlayer(i, fmt.Sprintf("Player%d", i), AI, 200)
			table.AddPlayer(player)
		}

		table.StartNewHand()

		// Move to flop (currentBet = 0, everyone can check)
		table.DealFlop()

		// Initially, no one has acted - round should NOT be complete
		if table.IsBettingRoundComplete() {
			t.Error("Betting round should not be complete when no one has acted")
		}

		// After first player acts, round should still NOT be complete
		firstPlayer := table.GetCurrentPlayer()
		if firstPlayer != nil {
			table.playersActed[firstPlayer.ID] = true
			firstPlayer.BetThisRound = 0 // Check
		}

		if table.IsBettingRoundComplete() {
			t.Error("Betting round should not be complete after only one player acts")
		}
	})

	t.Run("should complete when all players have acted", func(t *testing.T) {
		table := NewTable(rand.New(rand.NewSource(43)), eventBus, TableConfig{
			MaxSeats:   3,
			SmallBlind: 1,
			BigBlind:   2,
		})

		// Add 3 players
		for i := 1; i <= 3; i++ {
			player := NewPlayer(i, fmt.Sprintf("Player%d", i), AI, 200)
			table.AddPlayer(player)
		}

		table.StartNewHand()
		table.DealFlop()

		// Mark all players as having acted (checked)
		for _, player := range table.activePlayers {
			if player.IsInHand() {
				table.playersActed[player.ID] = true
				player.BetThisRound = 0 // Everyone checked
			}
		}

		// Now round should be complete
		if !table.IsBettingRoundComplete() {
			t.Error("Betting round should be complete when all players have acted")
		}
	})

	t.Run("should complete when only one player can act", func(t *testing.T) {
		table := NewTable(rand.New(rand.NewSource(44)), eventBus, TableConfig{
			MaxSeats:   3,
			SmallBlind: 1,
			BigBlind:   2,
		})

		// Add 3 players
		players := []*Player{
			NewPlayer(1, "Player1", AI, 200),
			NewPlayer(2, "Player2", AI, 200),
			NewPlayer(3, "Player3", AI, 200),
		}

		for _, player := range players {
			table.AddPlayer(player)
		}

		table.StartNewHand()
		table.DealFlop()

		// Make two players all-in (can't act) - they should be marked as having acted
		players[0].IsAllIn = true
		players[1].IsAllIn = true
		table.playersActed[players[0].ID] = true
		table.playersActed[players[1].ID] = true
		// Player 3 can still act

		// Player 3 acts
		table.playersActed[players[2].ID] = true
		players[2].BetThisRound = 0

		// Round should complete since only one player could act and they acted
		if !table.IsBettingRoundComplete() {
			t.Error("Betting round should be complete when only one player can act and they have acted")
		}
	})

	t.Run("should not complete when only one player can act but hasn't acted yet", func(t *testing.T) {
		table := NewTable(rand.New(rand.NewSource(45)), eventBus, TableConfig{
			MaxSeats:   3,
			SmallBlind: 1,
			BigBlind:   2,
		})

		// Add 3 players
		players := []*Player{
			NewPlayer(1, "Player1", AI, 200),
			NewPlayer(2, "Player2", AI, 200),
			NewPlayer(3, "Player3", AI, 200),
		}

		for _, player := range players {
			table.AddPlayer(player)
		}

		table.StartNewHand()
		table.DealFlop()

		// Make two players all-in (can't act) - they should be marked as having acted
		players[0].IsAllIn = true
		players[1].IsAllIn = true
		table.playersActed[players[0].ID] = true
		table.playersActed[players[1].ID] = true
		// Player 3 can still act but hasn't yet

		// Round should NOT complete since the one player who can act hasn't acted
		if table.IsBettingRoundComplete() {
			t.Error("Betting round should not be complete when one player can act but hasn't acted yet")
		}
	})
}

func TestChipConservation(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	eventBus := NewEventBus()
	table := NewTable(rng, eventBus, TableConfig{
		MaxSeats:   3,
		SmallBlind: 1,
		BigBlind:   2,
	})

	// Add 3 players with 100 chips each
	players := []*Player{
		NewPlayer(1, "Alice", AI, 100),
		NewPlayer(2, "Bob", AI, 100),
		NewPlayer(3, "Charlie", AI, 100),
	}

	for _, player := range players {
		table.AddPlayer(player)
	}

	expectedTotal := 300 // 3 players × 100 chips

	// Should pass initially
	if err := table.ValidateChipConservation(expectedTotal); err != nil {
		t.Errorf("Initial chip conservation should pass: %v", err)
	}

	// Simulate some betting
	table.StartNewHand()
	table.pot = 50        // Simulate 50 chips in pot
	players[0].Chips = 80 // Alice lost 20 to pot
	players[1].Chips = 75 // Bob lost 25 to pot
	players[2].Chips = 95 // Charlie lost 5 to pot

	// Should still pass (pot + player chips = 300)
	if err := table.ValidateChipConservation(expectedTotal); err != nil {
		t.Errorf("Conservation should pass with pot: %v", err)
	}

	// Award pot to Alice
	table.pot = 0
	players[0].Chips += 50 // Alice wins the 50 chip pot

	// Should still pass
	if err := table.ValidateChipConservation(expectedTotal); err != nil {
		t.Errorf("Conservation should pass after pot award: %v", err)
	}

	// Simulate chip creation bug (should fail)
	players[0].Chips += 10 // Alice magically gets 10 extra chips

	// Should fail
	if err := table.ValidateChipConservation(expectedTotal); err == nil {
		t.Error("Conservation should fail when chips are created")
	} else {
		t.Logf("Correctly detected chip creation: %v", err)
	}

	// Test the helper method too
	if table.GetTotalChips() != expectedTotal+10 {
		t.Errorf("GetTotalChips should return %d, got %d", expectedTotal+10, table.GetTotalChips())
	}
}
