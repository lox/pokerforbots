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
	}, nil)

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
	}, nil)

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
	}, nil)

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
	}, nil)

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
	}, nil)

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
	}, nil)

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

func TestButtonRotation(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(0)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	}, nil)

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

// Test pot distribution functionality
func TestPotDistribution(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(0)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	}, nil)

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

// TestFindWinnerEvaluatesHandStrength tests that FindWinner correctly evaluates hand strength
func TestFindWinnerEvaluatesHandStrength(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(0)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	}, nil)

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
	}, nil)

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
	potBeforeAward := table.Pot
	if potBeforeAward < 3 { // Should be at least blinds + some betting
		t.Errorf("Expected pot to be larger, got %d", potBeforeAward)
	}

	// Find winner
	winner := table.FindWinner()
	if winner == nil {
		t.Fatal("Should have a winner")
	}

	// Pot should still be intact for summary display
	potForSummary := table.Pot
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
	if table.Pot != 0 {
		t.Errorf("Pot should be 0 after awarding, got %d", table.Pot)
	}
}

func TestBettingRoundCompleteWhenAllCheck(t *testing.T) {
	// This test reproduces the infinite checking loop bug
	table := NewTable(rand.New(rand.NewSource(42)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	}, nil)

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
			callAmount := table.CurrentBet
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

func TestPostFlopCheckingRoundBug(t *testing.T) {
	// Test the post-flop scenario where CurrentBet = 0 and everyone checks
	table := NewTable(rand.New(rand.NewSource(42)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	}, nil)

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
	}, nil)

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
	}, nil)

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
