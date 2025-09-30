package server

import (
	"testing"
	"time"

	"github.com/lox/pokerforbots/internal/game"
	"github.com/lox/pokerforbots/internal/randutil"
	"github.com/lox/pokerforbots/protocol"
)

// TestActionRequestExactStackEqualsCall verifies the exact ActionRequest values sent
// when a player's stack exactly equals the amount they need to call.
// This reproduces the Aragorn bot scenario: stack=120, to_call=120, min_bet=1120
func TestActionRequestExactStackEqualsCall(t *testing.T) {
	logger := testLogger()
	rng := randutil.New(42)

	// Create 3 bots with specific chip amounts
	// Bot 0 (Alice): 1000 chips
	// Bot 1 (Bob): 120 chips - SHORT STACK
	// Bot 2 (Charlie): 1000 chips
	bots := []*Bot{
		newTestBot("alice", nil),
		newTestBot("bob", nil), // Short stack
		newTestBot("charlie", nil),
	}
	// Set bankrolls manually
	bots[0].bankroll = 1000
	bots[1].bankroll = 120
	bots[2].bankroll = 1000

	config := Config{
		SmallBlind: 5,
		BigBlind:   10,
		StartChips: 1000,
		Timeout:    1 * time.Second,
	}

	handID := "test-hand-1"
	button := 0 // Alice is button

	hr := NewHandRunnerWithConfig(logger, bots, handID, button, rng, config)

	// Don't run the full hand - we'll manually step through to get the exact scenario

	// Create hand state manually with Bob having 120 chips
	deckRNG := randutil.New(rng.Int64())
	hr.handState = game.NewHandState(
		deckRNG,
		[]string{"alice", "bob", "charlie"},
		button,
		5,  // small blind
		10, // big blind
		game.WithChipsByPlayer([]int{1000, 120, 1000}),
	)

	// After blinds are posted:
	// Bob (seat 1) posted SB = 5, has 115 chips left
	// Charlie (seat 2) posted BB = 10, has 990 chips left
	// Alice (seat 0) acts first

	// Alice raises to 120
	err := hr.handState.ProcessAction(game.Raise, 120)
	if err != nil {
		t.Fatalf("Alice raise failed: %v", err)
	}

	// Bob should be next to act
	// Bob has 115 chips remaining
	// Bob has 5 chips in the pot (SB)
	// CurrentBet is 120
	// toCall = 120 - 5 = 115 (exactly Bob's stack)

	if hr.handState.ActivePlayer != 1 {
		t.Fatalf("Expected Bob (seat 1) to act, got %d", hr.handState.ActivePlayer)
	}

	bob := hr.handState.Players[1]
	if bob.Chips != 115 {
		t.Fatalf("Expected Bob to have 115 chips, got %d", bob.Chips)
	}

	// Calculate what the ActionRequest should contain
	toCall := hr.handState.Betting.CurrentBet - bob.Bet
	minBet := hr.handState.Betting.CurrentBet + hr.handState.Betting.MinRaise
	minRaise := hr.handState.Betting.MinRaise

	t.Logf("Bob's state: Chips=%d, Bet=%d", bob.Chips, bob.Bet)
	t.Logf("Betting state: CurrentBet=%d, MinRaise=%d", hr.handState.Betting.CurrentBet, minRaise)
	t.Logf("ActionRequest values: ToCall=%d, MinBet=%d, MinRaise=%d", toCall, minBet, minRaise)

	// Verify calculations
	if toCall != 115 {
		t.Errorf("Expected toCall=115, got %d", toCall)
	}

	if bob.Chips != toCall {
		t.Errorf("Expected Bob's chips (%d) to equal toCall (%d)", bob.Chips, toCall)
	}

	// Get valid actions
	validActions := hr.handState.GetValidActions()
	t.Logf("Valid actions: %v", validActions)

	// Verify "raise" is NOT in valid actions
	hasRaise := false
	for _, action := range validActions {
		if action == game.Raise {
			hasRaise = true
		}
	}

	if hasRaise {
		t.Errorf("CRITICAL: Raise should NOT be in valid actions when stack == toCall")
		t.Errorf("  Bob's stack: %d", bob.Chips)
		t.Errorf("  To call: %d", toCall)
		t.Errorf("  Min bet required: %d", minBet)
		t.Errorf("  Bob would need %d more chips to raise", minBet-bob.Bet-bob.Chips)
	}

	// Verify the ActionRequest would have the right values if sent
	// (We can't actually send it because we're not running the full hand runner)
	expectedValidActions := []string{"fold", "allin"}
	actualActionStrings := make([]string, len(validActions))
	for i, a := range validActions {
		actualActionStrings[i] = a.String()
	}

	if len(actualActionStrings) != len(expectedValidActions) {
		t.Errorf("Expected %d valid actions, got %d: %v",
			len(expectedValidActions), len(actualActionStrings), actualActionStrings)
	}

	// Summary of what would be sent in ActionRequest
	t.Logf("=== ActionRequest Summary ===")
	t.Logf("ToCall: %d (amount needed to call)", toCall)
	t.Logf("MinBet: %d (minimum total bet to raise)", minBet)
	t.Logf("MinRaise: %d (minimum raise increment)", minRaise)
	t.Logf("ValidActions: %v", actualActionStrings)
	t.Logf("===========================")

	// Key assertions for Aragorn investigation
	if bob.Chips == toCall && hasRaise {
		t.Fatal("BUG FOUND: When player.Chips == toCall, raise should not be in valid_actions")
	}

	if !hasRaise {
		t.Logf("✅ VERIFIED: Raise correctly excluded when stack (%d) == toCall (%d)", bob.Chips, toCall)
	}
}

// TestActionRequestMessageContent verifies the actual protocol.ActionRequest message
func TestActionRequestMessageContent(t *testing.T) {
	logger := testLogger()
	rng := randutil.New(42)

	// Create a simple 2-player scenario
	bots := []*Bot{
		newTestBot("alice", nil),
		newTestBot("bob", nil),
	}
	bots[0].bankroll = 200
	bots[1].bankroll = 120

	config := Config{
		SmallBlind: 5,
		BigBlind:   10,
		StartChips: 1000,
		Timeout:    100 * time.Millisecond,
	}

	hr := NewHandRunnerWithConfig(logger, bots, "test", 0, rng, config)

	// Manually create the hand state
	deckRNG := randutil.New(rng.Int64())
	hr.handState = game.NewHandState(
		deckRNG,
		[]string{"alice", "bob"},
		0, // Alice is button
		5,
		10,
		game.WithChipsByPlayer([]int{200, 120}),
	)

	// Heads-up: Button (Alice) posts SB, Bob posts BB
	// Alice has 195 chips, bet=5
	// Bob has 110 chips, bet=10
	// Alice to act first

	alice := hr.handState.Players[0]
	bob := hr.handState.Players[1]

	t.Logf("Initial state:")
	t.Logf("  Alice: chips=%d, bet=%d", alice.Chips, alice.Bet)
	t.Logf("  Bob: chips=%d, bet=%d", bob.Chips, bob.Bet)

	// Alice raises to 120 (all of Bob's stack + his BB)
	err := hr.handState.ProcessAction(game.Raise, 120)
	if err != nil {
		t.Fatalf("Alice raise failed: %v", err)
	}

	t.Logf("\nAfter Alice raises to 120:")
	t.Logf("  Alice: chips=%d, bet=%d", alice.Chips, alice.Bet)
	t.Logf("  Bob: chips=%d, bet=%d", bob.Chips, bob.Bet)

	// Now Bob faces a decision
	// Bob has 110 chips remaining
	// Bob has 10 chips already in (BB)
	// CurrentBet is 120
	// toCall = 120 - 10 = 110 (exactly Bob's stack!)

	toCall := hr.handState.Betting.CurrentBet - bob.Bet

	if toCall != 110 {
		t.Fatalf("Expected toCall=110, got %d", toCall)
	}

	if bob.Chips != 110 {
		t.Fatalf("Expected Bob chips=110, got %d", bob.Chips)
	}

	// Create the ActionRequest that would be sent to Bob
	validActions := hr.handState.GetValidActions()
	actionStrings := make([]string, len(validActions))
	for i, a := range validActions {
		actionStrings[i] = a.String()
	}

	pot := 0
	for _, p := range hr.handState.GetPots() {
		pot += p.Amount
	}

	msg := &protocol.ActionRequest{
		Type:          "action_request",
		HandID:        "test",
		Pot:           pot,
		ToCall:        toCall,
		MinBet:        hr.handState.Betting.CurrentBet + hr.handState.Betting.MinRaise,
		MinRaise:      hr.handState.Betting.MinRaise,
		ValidActions:  actionStrings,
		TimeRemaining: 100,
	}

	t.Logf("\n=== ActionRequest Message Content ===")
	t.Logf("ToCall: %d", msg.ToCall)
	t.Logf("MinBet: %d", msg.MinBet)
	t.Logf("MinRaise: %d", msg.MinRaise)
	t.Logf("Pot: %d", msg.Pot)
	t.Logf("ValidActions: %v", msg.ValidActions)
	t.Logf("=====================================")

	// Verify "raise" is not in valid actions
	hasRaise := false
	for _, action := range msg.ValidActions {
		if action == "raise" {
			hasRaise = true
		}
	}

	if hasRaise {
		t.Errorf("BUG: 'raise' should not be in ValidActions when stack == toCall")
		t.Errorf("  Bob's stack: %d", bob.Chips)
		t.Errorf("  ToCall: %d", msg.ToCall)
		t.Errorf("  MinBet required: %d", msg.MinBet)
	} else {
		t.Logf("✅ VERIFIED: 'raise' correctly excluded from ValidActions")
	}

	// Verify what the bot would see in GameUpdate for Bob
	// The Player.Chips field would show 110 (remaining chips, not including the 10 bet)
	t.Logf("\n=== GameUpdate Player Data (Bob) ===")
	t.Logf("Chips: %d (remaining, not in pot)", bob.Chips)
	t.Logf("Bet: %d (already in pot this street)", bob.Bet)
	t.Logf("Total starting: %d (Chips + Bet)", bob.Chips+bob.Bet)
	t.Logf("====================================")
}
