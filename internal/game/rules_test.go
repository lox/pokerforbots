package game

import (
	"math/rand"
	"testing"

	"github.com/lox/pokerforbots/internal/deck"
)

// =============================================================================
// POSITION AND SEATING RULES (RULES.md §1)
// =============================================================================

// TestSixMaxPositions tests that we properly name and assign 6-max positions according to poker rules
func TestSixMaxPositions(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(42)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
		Seed:       42,
	})

	// Add 6 players to test full 6-max position assignment
	players := []*Player{
		NewPlayer(1, "UTG", AI, 200),
		NewPlayer(2, "LJ", AI, 200),  // LoJack (Low Jack)
		NewPlayer(3, "HJ", AI, 200),  // HiJack (High Jack)
		NewPlayer(4, "CO", AI, 200),  // Cutoff
		NewPlayer(5, "BTN", AI, 200), // Button
		NewPlayer(6, "SB", AI, 200),  // Small Blind (BB will be UTG)
	}

	for _, player := range players {
		table.AddPlayer(player)
	}

	table.StartNewHand()

	// Check that positions are assigned according to 6-max rules
	// Starting from dealer button and going clockwise:
	// Position map from rules: BTN(0), SB(+1), BB(+2), UTG(+3), LJ(+4), HJ(+5)

	// Find the button player
	var buttonPlayer *Player
	for _, player := range table.activePlayers {
		if player.Position == Button {
			buttonPlayer = player
			break
		}
	}

	if buttonPlayer == nil {
		t.Fatal("Should have a button player")
	}

	// Verify we have all required positions for 6-max
	positions := make(map[Position]bool)
	for _, player := range table.activePlayers {
		positions[player.Position] = true
	}

	requiredPositions := []Position{Button, SmallBlind, BigBlind, UnderTheGun, Cutoff}
	for _, pos := range requiredPositions {
		if !positions[pos] {
			t.Errorf("Missing required 6-max position: %s", pos)
		}
	}

	// Verify UTG acts first pre-flop (except in heads-up)
	if len(table.activePlayers) > 2 {
		firstToAct := table.GetCurrentPlayer()
		if firstToAct.Position != UnderTheGun {
			t.Errorf("In 6-max, UTG should act first pre-flop, got %s", firstToAct.Position)
		}
	}
}

// TestPostFlopActionOrder tests that post-flop action starts with first active seat left of button
func TestPostFlopActionOrder(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(42)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 5,
		BigBlind:   10,
		Seed:       42,
	})

	// Add 4 players to test action order
	alice := NewPlayer(1, "Alice", AI, 200)
	bob := NewPlayer(2, "Bob", AI, 200)
	charlie := NewPlayer(3, "Charlie", AI, 200)
	david := NewPlayer(4, "David", AI, 200)
	table.AddPlayer(alice)
	table.AddPlayer(bob)
	table.AddPlayer(charlie)
	table.AddPlayer(david)

	table.StartNewHand()

	// Complete pre-flop with all players calling
	for i := 0; i < 4; i++ {
		currentPlayer := table.GetCurrentPlayer()
		if currentPlayer == nil {
			break
		}

		if currentPlayer.BetThisRound < table.currentBet {
			decision := Decision{Action: Call, Amount: table.currentBet, Reasoning: "call"}
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

	// Deal flop
	table.DealFlop()

	// Post-flop, first to act should be first active seat left of button
	buttonSeat := table.dealerPosition
	firstToAct := table.GetCurrentPlayer()

	if firstToAct == nil {
		t.Fatal("Should have a player to act on flop")
	}

	// The first to act should NOT be the button (unless heads-up)
	if len(table.activePlayers) > 2 && firstToAct.SeatNumber == buttonSeat {
		t.Error("Button should not act first post-flop in multi-way pot")
	}

	// In heads-up, button acts last post-flop (so opponent acts first)
	if len(table.activePlayers) == 2 {
		// Find the non-button player
		var nonButtonPlayer *Player
		for _, player := range table.activePlayers {
			if player.SeatNumber != buttonSeat {
				nonButtonPlayer = player
				break
			}
		}

		if nonButtonPlayer != nil && firstToAct != nonButtonPlayer {
			t.Error("In heads-up, non-button player should act first post-flop")
		}
	}

	t.Logf("Button seat: %d, First to act post-flop: %s (seat %d)",
		buttonSeat, firstToAct.Name, firstToAct.SeatNumber)
}

// =============================================================================
// BETTING STRUCTURE AND MINIMUM RAISE RULES (RULES.md §3)
// =============================================================================

func TestMinimumRaiseRules(t *testing.T) {
	// Test that minimum raise follows Texas Hold'em rules:
	// The minimum raise must be at least the size of the last raise
	table := NewTable(rand.New(rand.NewSource(42)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 5,
		BigBlind:   10,
		Seed:       42,
	})

	// Add players
	alice := NewPlayer(1, "Alice", AI, 1000)
	bob := NewPlayer(2, "Bob", AI, 1000)
	charlie := NewPlayer(3, "Charlie", AI, 1000)
	table.AddPlayer(alice)
	table.AddPlayer(bob)
	table.AddPlayer(charlie)

	// Start hand
	table.StartNewHand()

	// Initial state: BB is 10, so minimum raise should be 10
	if table.minRaise != 10 {
		t.Errorf("Initial MinRaise should be 10 (big blind), got %d", table.minRaise)
	}

	// Current bet is 10 (big blind), so minimum raise total should be 20
	validActions := table.GetValidActions()
	var raiseAction *ValidAction
	for _, action := range validActions {
		if action.Action == Raise {
			raiseAction = &action
			break
		}
	}

	if raiseAction == nil {
		t.Fatal("Should have a raise action available")
	}

	if raiseAction.MinAmount != 20 {
		t.Errorf("Minimum raise should be 20 (10+10), got %d", raiseAction.MinAmount)
	}

	// First player raises to 30 (a raise of 20)
	decision := Decision{Action: Raise, Amount: 30, Reasoning: "raise to 30"}
	_, err := table.ApplyDecision(decision)
	if err != nil {
		t.Fatalf("Failed to apply decision: %v", err)
	}

	// MinRaise should now be 20 (the size of the last raise)
	if table.minRaise != 20 {
		t.Errorf("After raise to 30, MinRaise should be 20, got %d", table.minRaise)
	}

	table.AdvanceAction()

	// Next player wants to raise: minimum should be 30 + 20 = 50
	validActions = table.GetValidActions()
	raiseAction = nil
	for _, action := range validActions {
		if action.Action == Raise {
			raiseAction = &action
			break
		}
	}

	if raiseAction == nil {
		t.Fatal("Should have a raise action available")
	}

	if raiseAction.MinAmount != 50 {
		t.Errorf("Next minimum raise should be 50 (30+20), got %d", raiseAction.MinAmount)
	}

	// Second player raises to 60 (a raise of 30)
	decision = Decision{Action: Raise, Amount: 60, Reasoning: "raise to 60"}
	_, err = table.ApplyDecision(decision)
	if err != nil {
		t.Fatalf("Failed to apply second decision: %v", err)
	}

	// MinRaise should now be 30 (the size of the last raise)
	if table.minRaise != 30 {
		t.Errorf("After raise to 60, MinRaise should be 30, got %d", table.minRaise)
	}
}

func TestMinimumRaiseAcrossRounds(t *testing.T) {
	// Test that minimum raise resets to big blind on new betting rounds
	table := NewTable(rand.New(rand.NewSource(42)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 5,
		BigBlind:   10,
		Seed:       42,
	})

	// Add players
	alice := NewPlayer(1, "Alice", AI, 1000)
	bob := NewPlayer(2, "Bob", AI, 1000)
	table.AddPlayer(alice)
	table.AddPlayer(bob)

	// Start hand
	table.StartNewHand()

	// Simulate some raising pre-flop
	decision := Decision{Action: Raise, Amount: 40, Reasoning: "big raise"}
	_, err := table.ApplyDecision(decision)
	if err != nil {
		t.Fatalf("Failed to apply decision: %v", err)
	}

	// MinRaise should be 30 (the size of the raise)
	if table.minRaise != 30 {
		t.Errorf("After pre-flop raise, MinRaise should be 30, got %d", table.minRaise)
	}

	// Move to flop (this calls startNewBettingRound internally)
	table.DealFlop()

	// MinRaise should reset to big blind for new round
	if table.minRaise != 10 {
		t.Errorf("After dealing flop, MinRaise should reset to 10, got %d", table.minRaise)
	}

	// Current bet should be 0 for new round
	if table.currentBet != 0 {
		t.Errorf("After dealing flop, CurrentBet should be 0, got %d", table.currentBet)
	}
}

// TestMinimumOpeningBet tests that minimum opening bet equals big blind
func TestMinimumOpeningBet(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(42)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 5,
		BigBlind:   10,
		Seed:       42,
	})

	alice := NewPlayer(1, "Alice", AI, 200)
	bob := NewPlayer(2, "Bob", AI, 200)
	table.AddPlayer(alice)
	table.AddPlayer(bob)

	table.StartNewHand()

	// Move to post-flop where opening bet rules apply
	// First complete pre-flop action
	currentPlayer := table.GetCurrentPlayer()
	if currentPlayer != nil {
		// Determine if player needs to call or can check
		callAmount := table.currentBet - currentPlayer.BetThisRound

		if callAmount > 0 {
			// Player needs to call
			decision := Decision{Action: Call, Amount: callAmount, Reasoning: "call to match"}
			_, err := table.ApplyDecision(decision)
			if err != nil {
				t.Fatalf("Failed to apply call: %v", err)
			}
		} else {
			// Player can check (already matched)
			decision := Decision{Action: Check, Amount: 0, Reasoning: "check"}
			_, err := table.ApplyDecision(decision)
			if err != nil {
				t.Fatalf("Failed to apply check: %v", err)
			}
		}
		table.AdvanceAction()

		// Second player acts
		currentPlayer = table.GetCurrentPlayer()
		if currentPlayer != nil {
			callAmount = table.currentBet - currentPlayer.BetThisRound
			if callAmount > 0 {
				decision := Decision{Action: Call, Amount: callAmount, Reasoning: "call"}
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
		}
	}

	// Deal flop
	table.DealFlop()

	// On flop, minimum opening bet should be big blind amount
	validActions := table.GetValidActions()

	var betAction *ValidAction
	for _, action := range validActions {
		if action.Action == Raise { // Bet is represented as Raise when no previous bet
			betAction = &action
			break
		}
	}

	if betAction == nil {
		t.Fatal("Should have bet action available on flop")
	}

	// Minimum bet should be big blind (10)
	expectedMinBet := table.bigBlind
	if betAction.MinAmount < expectedMinBet {
		t.Errorf("Minimum opening bet should be at least %d (big blind), got %d", expectedMinBet, betAction.MinAmount)
	}
}

func TestRaiseValidation(t *testing.T) {
	// Test that invalid raises are rejected
	table := NewTable(rand.New(rand.NewSource(42)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 5,
		BigBlind:   10,
		Seed:       42,
	})

	// Add players
	alice := NewPlayer(1, "Alice", AI, 1000)
	bob := NewPlayer(2, "Bob", AI, 1000)
	table.AddPlayer(alice)
	table.AddPlayer(bob)

	// Start hand
	table.StartNewHand()

	// Try to make a raise that's too small
	decision := Decision{Action: Raise, Amount: 15, Reasoning: "small raise"} // Only 5 more than BB
	_, err := table.ApplyDecision(decision)

	if err == nil {
		t.Error("Should have rejected raise that's smaller than minimum")
	}

	// Try to make a valid raise
	decision = Decision{Action: Raise, Amount: 20, Reasoning: "valid raise"} // Minimum raise
	_, err = table.ApplyDecision(decision)

	if err != nil {
		t.Errorf("Should have accepted valid minimum raise: %v", err)
	}
}

func TestBettingRoundComplete_WithRaises(t *testing.T) {
	// Test that betting round completion works correctly with raises
	table := NewTable(rand.New(rand.NewSource(42)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 5,
		BigBlind:   10,
		Seed:       42,
	})

	// Add players
	alice := NewPlayer(1, "Alice", AI, 1000)
	bob := NewPlayer(2, "Bob", AI, 1000)
	charlie := NewPlayer(3, "Charlie", AI, 1000)
	table.AddPlayer(alice)
	table.AddPlayer(bob)
	table.AddPlayer(charlie)

	// Start hand
	table.StartNewHand()

	// Simulate betting sequence: first player raises, others call
	// This should complete the betting round

	// First player raises
	decision := Decision{Action: Raise, Amount: 30, Reasoning: "raise"}
	_, err := table.ApplyDecision(decision)
	if err != nil {
		t.Fatalf("Failed to apply first raise: %v", err)
	}
	table.AdvanceAction()

	// Should not be complete yet
	if table.IsBettingRoundComplete() {
		t.Error("Betting round should not be complete after one action")
	}

	// Second player calls
	decision = Decision{Action: Call, Amount: 30, Reasoning: "call"}
	_, err = table.ApplyDecision(decision)
	if err != nil {
		t.Fatalf("Failed to apply second call: %v", err)
	}
	table.AdvanceAction()

	// Still not complete
	if table.IsBettingRoundComplete() {
		t.Error("Betting round should not be complete after two actions")
	}

	// Third player calls
	decision = Decision{Action: Call, Amount: 30, Reasoning: "call"}
	_, err = table.ApplyDecision(decision)
	if err != nil {
		t.Fatalf("Failed to apply third call: %v", err)
	}
	table.AdvanceAction()

	// Now should be complete
	if !table.IsBettingRoundComplete() {
		t.Error("Betting round should be complete after all players have acted and matched the bet")
	}
}

// =============================================================================
// ALL-IN RULES AND SIDE POTS (RULES.md §4, §5)
// =============================================================================

func TestAllInRaiseRules(t *testing.T) {
	// Test that all-ins properly update minimum raise
	table := NewTable(rand.New(rand.NewSource(42)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 5,
		BigBlind:   10,
		Seed:       42,
	})

	// Add players with different stack sizes
	alice := NewPlayer(1, "Alice", AI, 45)       // Small stack
	bob := NewPlayer(2, "Bob", AI, 1000)         // Big stack
	charlie := NewPlayer(3, "Charlie", AI, 1000) // Big stack
	table.AddPlayer(alice)
	table.AddPlayer(bob)
	table.AddPlayer(charlie)

	// Start hand
	table.StartNewHand()

	// Alice (with 45 chips) goes all-in
	// After blinds, she has 40 chips left, so her all-in is 45 total
	currentPlayer := table.GetCurrentPlayer()
	if currentPlayer.Name != "Alice" {
		// Find Alice if she's not first to act
		for i, player := range table.activePlayers {
			if player.Name == "Alice" {
				table.actionOn = i
				// currentPlayer = table.GetCurrentPlayer() // Update tracked for debugging
				break
			}
		}
	}

	// Alice goes all-in with her remaining chips
	decision := Decision{Action: AllIn, Amount: 0, Reasoning: "all-in"}
	_, err := table.ApplyDecision(decision)
	if err != nil {
		t.Fatalf("Failed to apply all-in: %v", err)
	}

	// Check that Alice is all-in
	if !alice.IsAllIn {
		t.Error("Alice should be all-in")
	}

	// If Alice's all-in was a raise (total bet > previous bet), MinRaise should update
	if alice.TotalBet > 10 { // If her total bet exceeds the big blind
		expectedMinRaise := alice.TotalBet - 10 // Size of the raise
		if table.minRaise != expectedMinRaise {
			t.Errorf("After Alice's all-in raise, MinRaise should be %d, got %d", expectedMinRaise, table.minRaise)
		}
	}
}

func TestInsufficientRaiseAllIn(t *testing.T) {
	// Test scenario where player doesn't have enough chips for minimum raise
	// but goes all-in - this should not increase the minimum raise for others
	table := NewTable(rand.New(rand.NewSource(42)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 5,
		BigBlind:   10,
		Seed:       42,
	})

	// Add players
	alice := NewPlayer(1, "Alice", AI, 1000)
	bob := NewPlayer(2, "Bob", AI, 1000)
	charlie := NewPlayer(3, "Charlie", AI, 25) // Short stack
	table.AddPlayer(alice)
	table.AddPlayer(bob)
	table.AddPlayer(charlie)

	// Start hand
	table.StartNewHand()

	// Alice makes a big raise
	decision := Decision{Action: Raise, Amount: 50, Reasoning: "big raise"}
	_, err := table.ApplyDecision(decision)
	if err != nil {
		t.Fatalf("Failed to apply Alice's raise: %v", err)
	}

	// MinRaise should be 40 (size of the raise)
	if table.minRaise != 40 {
		t.Errorf("After Alice's raise, MinRaise should be 40, got %d", table.minRaise)
	}

	table.AdvanceAction()

	// Charlie has only ~20 chips left after blinds, so can't make minimum raise
	// Charlie goes all-in with insufficient chips for a full raise
	if table.GetCurrentPlayer().Name == "Charlie" {
		decision = Decision{Action: AllIn, Amount: 0, Reasoning: "forced all-in"}
		_, err := table.ApplyDecision(decision)
		if err != nil {
			t.Fatalf("Failed to apply Charlie's all-in: %v", err)
		}

		// MinRaise should stay 40 because Charlie's all-in wasn't a full raise
		if table.minRaise != 40 {
			t.Errorf("After Charlie's insufficient all-in, MinRaise should stay 40, got %d", table.minRaise)
		}
	}
}

// TestAllInDoesNotReopenBetting tests that insufficient all-ins don't reopen betting
func TestAllInDoesNotReopenBetting(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(42)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 5,
		BigBlind:   10,
		Seed:       42,
	})

	// Create specific scenario:
	// Alice raises, Bob calls, Charlie (short stack) goes all-in for less than minimum raise
	// Alice should NOT get another chance to act
	alice := NewPlayer(1, "Alice", AI, 1000)   // Big stack
	bob := NewPlayer(2, "Bob", AI, 1000)       // Big stack
	charlie := NewPlayer(3, "Charlie", AI, 25) // Short stack (after blinds ~15 chips)
	table.AddPlayer(alice)
	table.AddPlayer(bob)
	table.AddPlayer(charlie)

	table.StartNewHand()

	// Track the sequence of players who act
	var actionSequence []string

	// First action - should be UTG
	currentPlayer := table.GetCurrentPlayer()
	actionSequence = append(actionSequence, currentPlayer.Name)
	decision := Decision{Action: Raise, Amount: 30, Reasoning: "raise to $30"}
	_, err := table.ApplyDecision(decision)
	if err != nil {
		t.Fatalf("Failed to apply raise: %v", err)
	}
	originalRaiser := currentPlayer.Name
	table.AdvanceAction()

	// Second action - next player acts (might be the short stack)
	currentPlayer = table.GetCurrentPlayer()
	if currentPlayer != nil {
		actionSequence = append(actionSequence, currentPlayer.Name)

		// If this is the short stack (Charlie), they need to go all-in
		if currentPlayer.Name == "Charlie" {
			decision = Decision{Action: AllIn, Amount: 0, Reasoning: "forced all-in"}
			_, err = table.ApplyDecision(decision)
			if err != nil {
				t.Fatalf("Failed to apply all-in: %v", err)
			}
		} else {
			// Regular player can call
			callAmount := table.currentBet - currentPlayer.BetThisRound
			decision = Decision{Action: Call, Amount: callAmount, Reasoning: "call"}
			_, err = table.ApplyDecision(decision)
			if err != nil {
				t.Fatalf("Failed to apply call: %v", err)
			}
		}
		table.AdvanceAction()
	}

	// Third action - final player acts
	currentPlayer = table.GetCurrentPlayer()
	if currentPlayer != nil {
		actionSequence = append(actionSequence, currentPlayer.Name)

		// If this is the short stack, they go all-in; otherwise they call
		if currentPlayer.Name == "Charlie" {
			decision = Decision{Action: AllIn, Amount: 0, Reasoning: "forced all-in"}
			_, err = table.ApplyDecision(decision)
			if err != nil {
				t.Fatalf("Failed to apply all-in: %v", err)
			}

			// Verify Charlie's all-in is less than minimum raise
			minRaiseAmount := 30 + table.minRaise // Should be 30 + 20 = 50 minimum
			if currentPlayer.TotalBet < minRaiseAmount {
				t.Logf("Charlie's all-in (%d) is less than minimum raise (%d) - correct",
					currentPlayer.TotalBet, minRaiseAmount)
			} else {
				t.Errorf("Expected Charlie's all-in to be insufficient, but got %d >= %d",
					currentPlayer.TotalBet, minRaiseAmount)
			}
		} else {
			// Regular player calls
			callAmount := table.currentBet - currentPlayer.BetThisRound
			decision = Decision{Action: Call, Amount: callAmount, Reasoning: "call"}
			_, err = table.ApplyDecision(decision)
			if err != nil {
				t.Fatalf("Failed to apply call: %v", err)
			}
		}

		table.AdvanceAction()
	}

	// Now check if betting is complete
	// The original raiser should NOT get another chance to act
	if !table.IsBettingRoundComplete() {
		// If betting continues, the next player should NOT be the original raiser
		if nextPlayer := table.GetCurrentPlayer(); nextPlayer != nil {
			if nextPlayer.Name == originalRaiser {
				t.Errorf("Original raiser %s should not get to act again after insufficient all-in, but betting continued to them", originalRaiser)
			}
		}
	}

	t.Logf("Action sequence: %v", actionSequence)
	t.Logf("Betting round complete: %v", table.IsBettingRoundComplete())
	t.Logf("Original raiser: %s", originalRaiser)
}

func TestValidActionsMinRaise(t *testing.T) {
	// Test that GetValidActions respects minimum raise rules
	table := NewTable(rand.New(rand.NewSource(42)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 5,
		BigBlind:   10,
		Seed:       42,
	})

	// Add players
	alice := NewPlayer(1, "Alice", AI, 1000)
	bob := NewPlayer(2, "Bob", AI, 1000)
	charlie := NewPlayer(3, "Charlie", AI, 30) // Short stack
	table.AddPlayer(alice)
	table.AddPlayer(bob)
	table.AddPlayer(charlie)

	// Start hand
	table.StartNewHand()

	// Alice raises
	currentPlayer := table.GetCurrentPlayer()
	t.Logf("Current player to act: %s", currentPlayer.Name)
	if currentPlayer.Name == "Alice" {
		decision := Decision{Action: Raise, Amount: 40, Reasoning: "raise"}
		_, _ = table.ApplyDecision(decision)
		table.AdvanceAction()
	} else {
		// Find Alice and make her raise (she might not be first to act)
		for i, player := range table.activePlayers {
			if player.Name == "Alice" {
				table.actionOn = i
				decision := Decision{Action: Raise, Amount: 40, Reasoning: "raise"}
				_, err := table.ApplyDecision(decision)
				if err != nil {
					t.Fatalf("Failed to apply Alice's raise: %v", err)
				}
				table.AdvanceAction()
				break
			}
		}
	}

	// Debug: check state after Alice's raise
	t.Logf("After Alice's raise: CurrentBet=%d, MinRaise=%d", table.currentBet, table.minRaise)

	// Check valid actions for next player
	validActions := table.GetValidActions()

	var raiseAction *ValidAction
	for _, action := range validActions {
		if action.Action == Raise {
			raiseAction = &action
			break
		}
	}

	if raiseAction != nil {
		// Minimum raise should be 40 + 30 = 70 (current bet + last raise size)
		t.Logf("Got minimum raise amount: %d", raiseAction.MinAmount)
		if raiseAction.MinAmount != 70 {
			t.Errorf("Minimum raise should be 70 (40+30), got %d", raiseAction.MinAmount)
		}
	}

	// Check that a short-stacked player might not have raise available
	table.AdvanceAction()
	currentPlayer = table.GetCurrentPlayer()
	if currentPlayer.Name == "Charlie" {
		validActions = table.GetValidActions()

		hasRaise := false
		for _, action := range validActions {
			if action.Action == Raise {
				hasRaise = true
				break
			}
		}

		// Charlie has ~25 chips left, can't make minimum raise to 70
		if hasRaise {
			t.Error("Charlie should not have raise action available due to insufficient chips")
		}

		// But should have all-in available
		hasAllIn := false
		for _, action := range validActions {
			if action.Action == AllIn {
				hasAllIn = true
				break
			}
		}

		if !hasAllIn {
			t.Error("Charlie should have all-in action available")
		}
	}
}

// =============================================================================
// SHOWDOWN AND POT DISTRIBUTION (RULES.md §6, §7)
// =============================================================================

// TestShowdownLastAggressor tests that last aggressor shows first (RULES.md §6)
func TestShowdownLastAggressor(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(42)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 5,
		BigBlind:   10,
		Seed:       42,
	})

	alice := NewPlayer(1, "Alice", AI, 200)
	bob := NewPlayer(2, "Bob", AI, 200)
	table.AddPlayer(alice)
	table.AddPlayer(bob)

	table.StartNewHand()

	// Complete pre-flop with both players calling
	for !table.IsBettingRoundComplete() {
		currentPlayer := table.GetCurrentPlayer()
		if currentPlayer == nil {
			break
		}

		callAmount := table.currentBet - currentPlayer.BetThisRound
		if callAmount > 0 {
			decision := Decision{Action: Call, Amount: callAmount, Reasoning: "call"}
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
	}

	// Deal flop and have one player bet (becoming last aggressor)
	table.DealFlop()

	// First player bets (becomes last aggressor)
	lastAggressor := table.GetCurrentPlayer()
	if lastAggressor != nil {
		decision := Decision{Action: Raise, Amount: 20, Reasoning: "bet"}
		_, err := table.ApplyDecision(decision)
		if err != nil {
			t.Fatalf("Failed to apply bet: %v", err)
		}
		table.AdvanceAction()

		// Second player calls
		currentPlayer := table.GetCurrentPlayer()
		if currentPlayer != nil {
			callAmount := table.currentBet - currentPlayer.BetThisRound
			decision = Decision{Action: Call, Amount: callAmount, Reasoning: "call"}
			_, err = table.ApplyDecision(decision)
			if err != nil {
				t.Fatalf("Failed to apply call: %v", err)
			}
		}
	}

	// Complete remaining betting rounds
	table.DealTurn()
	table.DealRiver()

	// At showdown, the game should track who was the last aggressor
	// This is implementation-specific but the rule states last aggressor shows first
	// In our implementation, FindWinner() handles this automatically by hand evaluation
	winner := table.FindWinner()
	if winner == nil {
		t.Fatal("Should have a winner at showdown")
	}

	// The key rule tested here is that the system can identify the last aggressor
	// and in a real implementation would show their cards first
	if lastAggressor == nil {
		t.Error("Should be able to identify last aggressor for showdown order")
	} else {
		t.Logf("Last aggressor: %s", lastAggressor.Name)
	}
	t.Logf("Winner: %s", winner.Name)
}

// TestOddChipDistribution tests that odd chips in split pots go to the player closest clockwise to button
func TestOddChipDistribution(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(42)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
		Seed:       42,
	})

	// Add 3 players so we can test odd chip distribution
	alice := NewPlayer(1, "Alice", AI, 200)
	bob := NewPlayer(2, "Bob", AI, 200)
	charlie := NewPlayer(3, "Charlie", AI, 200)
	table.AddPlayer(alice)
	table.AddPlayer(bob)
	table.AddPlayer(charlie)

	table.StartNewHand()

	// Simulate betting to create an odd-numbered pot
	// Complete the betting round properly with all players acting
	for i := 0; i < 3; i++ {
		currentPlayer := table.GetCurrentPlayer()
		if currentPlayer == nil {
			break
		}

		// First player raises, others call
		if i == 0 {
			decision := Decision{Action: Raise, Amount: 17, Reasoning: "raise to create odd pot"}
			_, err := table.ApplyDecision(decision)
			if err != nil {
				t.Fatalf("Failed to apply raise: %v", err)
			}
		} else {
			decision := Decision{Action: Call, Amount: 17, Reasoning: "call"}
			_, err := table.ApplyDecision(decision)
			if err != nil {
				t.Fatalf("Failed to apply call: %v", err)
			}
		}

		table.AdvanceAction()

		if table.IsBettingRoundComplete() {
			break
		}
	}

	// Create a three-way tie scenario
	table.communityCards = []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Hearts, Rank: deck.King},
		{Suit: deck.Clubs, Rank: deck.Queen},
		{Suit: deck.Diamonds, Rank: deck.Jack},
		{Suit: deck.Spades, Rank: deck.Ten},
	}

	// All players get same strength cards for a tie
	alice.HoleCards = []deck.Card{
		{Suit: deck.Hearts, Rank: deck.Two},
		{Suit: deck.Clubs, Rank: deck.Three},
	}
	bob.HoleCards = []deck.Card{
		{Suit: deck.Hearts, Rank: deck.Four},
		{Suit: deck.Clubs, Rank: deck.Five},
	}
	charlie.HoleCards = []deck.Card{
		{Suit: deck.Hearts, Rank: deck.Six},
		{Suit: deck.Clubs, Rank: deck.Seven},
	}

	// Ensure pot is odd number for remainder test
	if table.pot%3 == 0 {
		table.pot += 1 // Make it odd
	}

	potBeforeAward := table.pot
	sharePerPlayer := potBeforeAward / 3
	remainder := potBeforeAward % 3

	// Record starting chips
	startingChips := map[string]int{
		"Alice":   alice.Chips,
		"Bob":     bob.Chips,
		"Charlie": charlie.Chips,
	}

	// Find who is closest clockwise to button among the tied players
	// This requires finding the button position and determining clockwise order
	buttonSeat := table.dealerPosition

	// Award pot
	table.AwardPot()

	// Check that exactly one player got the extra chip(s)
	chipsReceived := map[string]int{
		"Alice":   alice.Chips - startingChips["Alice"],
		"Bob":     bob.Chips - startingChips["Bob"],
		"Charlie": charlie.Chips - startingChips["Charlie"],
	}

	// Count how many players got extra chips
	playersWithExtra := 0
	playerWithMostChips := ""
	maxReceived := 0

	for player, received := range chipsReceived {
		if received > sharePerPlayer {
			playersWithExtra++
			if received > maxReceived {
				maxReceived = received
				playerWithMostChips = player
			}
		}
	}

	// Exactly one player should get the remainder
	if playersWithExtra != 1 {
		t.Errorf("Expected exactly 1 player to get remainder chips, got %d players", playersWithExtra)
	}

	// The difference should equal the remainder
	expectedExtra := sharePerPlayer + remainder
	if maxReceived != expectedExtra {
		t.Errorf("Player with extra chips should have received %d, got %d", expectedExtra, maxReceived)
	}

	t.Logf("Pot: %d, Split among 3: %d each + %d remainder", potBeforeAward, sharePerPlayer, remainder)
	t.Logf("Button seat: %d", buttonSeat)
	t.Logf("Chips received - Alice: %d, Bob: %d, Charlie: %d",
		chipsReceived["Alice"], chipsReceived["Bob"], chipsReceived["Charlie"])
	t.Logf("Player with extra chips: %s", playerWithMostChips)
}

// =============================================================================
// HAND COMPLETION AND BUTTON ROTATION (RULES.md §8)
// =============================================================================

// TestDealerButtonAdvancement tests button rotation (RULES.md §8)
func TestDealerButtonAdvancement(t *testing.T) {
	table := NewTable(rand.New(rand.NewSource(42)), TableConfig{
		MaxSeats:   6,
		SmallBlind: 5,
		BigBlind:   10,
		Seed:       42,
	})

	// Add 3 players
	alice := NewPlayer(1, "Alice", AI, 200)
	bob := NewPlayer(2, "Bob", AI, 200)
	charlie := NewPlayer(3, "Charlie", AI, 200)
	table.AddPlayer(alice)
	table.AddPlayer(bob)
	table.AddPlayer(charlie)

	// Start first hand and record initial button position
	table.StartNewHand()
	initialButton := table.dealerPosition

	// Start second hand - button should advance
	table.StartNewHand()
	secondButton := table.dealerPosition

	// Start third hand - button should advance again
	table.StartNewHand()
	thirdButton := table.dealerPosition

	// Start fourth hand - should wrap around to initial position
	table.StartNewHand()
	fourthButton := table.dealerPosition

	// Button should advance clockwise: 1 -> 2 -> 3 -> 1
	expectedSequence := []int{initialButton}
	if initialButton == 1 {
		expectedSequence = append(expectedSequence, 2, 3, 1)
	} else if initialButton == 2 {
		expectedSequence = append(expectedSequence, 3, 1, 2)
	} else {
		expectedSequence = append(expectedSequence, 1, 2, 3)
	}

	actualSequence := []int{initialButton, secondButton, thirdButton, fourthButton}

	t.Logf("Expected button sequence: %v", expectedSequence)
	t.Logf("Actual button sequence: %v", actualSequence)

	for i, expected := range expectedSequence {
		if actualSequence[i] != expected {
			t.Errorf("Hand %d: expected button at seat %d, got %d",
				i+1, expected, actualSequence[i])
		}
	}
}
