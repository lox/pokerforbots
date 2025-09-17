package server

import (
	"math/rand"
	"testing"
	"time"

	"github.com/lox/pokerforbots/internal/game"
	"github.com/lox/pokerforbots/internal/protocol"
)

func TestHandRunner(t *testing.T) {
	// Create mock bots with larger buffers
	bots := []*Bot{
		{ID: "bot1-12345678", send: make(chan []byte, 100)},
		{ID: "bot2-12345678", send: make(chan []byte, 100)},
		{ID: "bot3-12345678", send: make(chan []byte, 100)},
	}

	// Create hand runner with test RNG
	rng := rand.New(rand.NewSource(42))
	runner := NewHandRunner(testLogger(), bots, "test-hand", 0, rng)

	// Initialize hand state
	runner.handState = game.NewHandState(
		[]string{"bot1", "bot2", "bot3"},
		0,
		5,
		10,
		1000,
	)

	// Test that we can broadcast messages
	runner.broadcastHandStart()

	// Verify all bots received hand start
	for i, bot := range bots {
		select {
		case <-bot.send:
			// Message received
		default:
			t.Errorf("Bot %d did not receive hand start", i)
		}
	}
}

func TestHandRunnerMessages(t *testing.T) {
	// Create mock bots with buffered channels
	bots := []*Bot{
		{ID: "alice12345678", send: make(chan []byte, 100)},
		{ID: "bob456789012", send: make(chan []byte, 100)},
	}

	// Create hand runner
	runner := NewHandRunner(testLogger(), bots, "test-hand-2", 0, rand.New(rand.NewSource(42)))

	// Send hand start (but don't run full hand)
	runner.handState = game.NewHandState(
		[]string{"alice", "bob"},
		0,
		5,
		10,
		1000,
	)

	runner.broadcastHandStart()

	// Check that both bots received hand start messages
	for i, bot := range bots {
		select {
		case msg := <-bot.send:
			if len(msg) == 0 {
				t.Errorf("Bot %d received empty message", i)
			}
			// Message was sent successfully
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Bot %d did not receive hand start message", i)
		}
	}
}

func TestHandRunnerActionRequest(t *testing.T) {
	// Create mock bot
	bot := &Bot{
		ID:   "test-bot-12345678",
		send: make(chan []byte, 10),
	}

	// Create hand runner
	runner := NewHandRunner(testLogger(), []*Bot{bot}, "test-hand-3", 0, rand.New(rand.NewSource(42)))
	runner.handState = game.NewHandState(
		[]string{"player1"},
		0,
		5,
		10,
		1000,
	)

	// Send action request
	validActions := []game.Action{game.Fold, game.Call, game.Raise}
	err := runner.sendActionRequest(bot, 0, validActions)

	if err != nil {
		t.Errorf("Failed to send action request: %v", err)
	}

	// Check that bot received action request
	select {
	case msg := <-bot.send:
		if len(msg) == 0 {
			t.Error("Received empty action request")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Bot did not receive action request")
	}
}

func TestHandRunnerTimeout(t *testing.T) {
	// Test that bots timeout and auto-fold when they don't respond
	bots := []*Bot{
		{ID: "timeout-bot1", send: make(chan []byte, 100), actionChan: make(chan ActionEnvelope, 1), bankroll: 100},
		{ID: "timeout-bot2", send: make(chan []byte, 100), actionChan: make(chan ActionEnvelope, 1), bankroll: 100},
	}

	runner := NewHandRunner(testLogger(), bots, "timeout-test", 0, rand.New(rand.NewSource(42)))

	// Start the hand runner in background
	done := make(chan struct{})
	go func() {
		defer close(done)
		runner.Run()
	}()

	// Don't send any actions - both bots should timeout and auto-fold

	// Wait for hand to complete (should be quick due to timeouts)
	select {
	case <-done:
		// Hand completed successfully
		if !runner.handState.IsComplete() {
			t.Error("Expected hand to be complete after timeouts")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Hand did not complete within timeout period")
	}
}

func TestHandRunnerComplete(t *testing.T) {
	// Create a simple 2-player scenario
	bots := []*Bot{
		{ID: "alice12345678", send: make(chan []byte, 100)},
		{ID: "bob1234567890", send: make(chan []byte, 100)},
	}

	runner := NewHandRunner(testLogger(), bots, "complete-test", 0, rand.New(rand.NewSource(42)))

	// Manually setup a simple scenario where one player folds
	runner.handState = game.NewHandState(
		[]string{"alice", "bob"},
		0,
		5,
		10,
		1000,
	)

	// Process alice folding
	runner.processAction(0, game.Fold, 0)

	// Hand should be complete (only one player left)
	if !runner.handState.IsComplete() {
		t.Error("Hand should be complete after one player folds in heads-up")
	}

	// Resolve and broadcast results
	winners := runner.resolveHand()
	runner.broadcastHandResult(winners)

	// Both bots should receive hand result
	for _, bot := range bots {
		select {
		case <-bot.send:
			// Received message
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Bot %s did not receive hand result", bot.ID)
		}
	}
}

func TestHandRunnerForceFoldOnDisconnect(t *testing.T) {
	// Two bots, bot1 will disconnect
	bots := []*Bot{
		{ID: "bot1", send: make(chan []byte, 10), done: make(chan struct{})},
		{ID: "bot2", send: make(chan []byte, 10), done: make(chan struct{})},
	}

	runner := NewHandRunner(testLogger(), bots, "disconnect-test", 0, rand.New(rand.NewSource(42)))
	runner.handState = game.NewHandState([]string{"bot1", "bot2"}, 0, 5, 10, 1000)
	runner.playerLabels = []string{"bot1", "bot2"}
	runner.lastStreet = runner.handState.Street

	// Simulate bot2 disconnecting
	bots[1].mu.Lock()
	bots[1].closed = true
	bots[1].mu.Unlock()
	close(bots[1].done)

	runner.foldDisconnectedPlayers(-1)

	if !runner.handState.Players[1].Folded {
		t.Fatal("expected seat 1 to be folded after disconnect")
	}

	// Connected bot should observe at least one message (player_action or game_update)
	select {
	case <-bots[0].send:
		// ok
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected player_action broadcast to connected bot")
	}
}

// TestValidActionsGeneration tests that valid actions are always generated correctly
func TestValidActionsGeneration(t *testing.T) {
	tests := []struct {
		name          string
		setupHand     func() *HandRunner
		expectedValid []string
		description   string
	}{
		{
			name: "preflop_utg_first_action",
			setupHand: func() *HandRunner {
				bot1 := &Bot{ID: "bot1", send: make(chan []byte, 10)}
				bot2 := &Bot{ID: "bot2", send: make(chan []byte, 10)}
				hr := NewHandRunner(testLogger(), []*Bot{bot1, bot2}, "test-hand", 0, rand.New(rand.NewSource(42)))
				hr.handState = game.NewHandState([]string{"bot1", "bot2"}, 0, 5, 10, 1000)
				hr.handState.Street = game.Preflop
				hr.handState.ActivePlayer = 0
				return hr
			},
			expectedValid: []string{"fold", "call", "raise"},
			description:   "UTG should be able to fold, call, or raise",
		},
		{
			name: "preflop_bb_after_call",
			setupHand: func() *HandRunner {
				bot1 := &Bot{ID: "bot1", send: make(chan []byte, 10)}
				bot2 := &Bot{ID: "bot2", send: make(chan []byte, 10)}
				hr := NewHandRunner(testLogger(), []*Bot{bot1, bot2}, "test-hand", 0, rand.New(rand.NewSource(42)))
				hr.handState = game.NewHandState([]string{"bot1", "bot2"}, 0, 5, 10, 1000)
				hr.handState.Street = game.Preflop
				// Simulate SB calling
				hr.handState.Players[0].Bet = 10
				hr.handState.Players[0].Chips = 995
				hr.handState.ActivePlayer = 1 // BB to act
				return hr
			},
			expectedValid: []string{"fold", "check", "raise"},
			description:   "BB should be able to check or raise after SB calls",
		},
		{
			name: "flop_first_to_act",
			setupHand: func() *HandRunner {
				bot1 := &Bot{ID: "bot1", send: make(chan []byte, 10)}
				bot2 := &Bot{ID: "bot2", send: make(chan []byte, 10)}
				hr := NewHandRunner(testLogger(), []*Bot{bot1, bot2}, "test-hand", 0, rand.New(rand.NewSource(42)))
				hr.handState = game.NewHandState([]string{"bot1", "bot2"}, 0, 5, 10, 1000)
				hr.handState.Street = game.Flop
				hr.handState.CurrentBet = 0
				hr.handState.MinRaise = 10
				hr.handState.Players[0].Bet = 0
				hr.handState.Players[1].Bet = 0
				hr.handState.ActivePlayer = 1 // In heads-up, BB acts first post-flop
				return hr
			},
			expectedValid: []string{"fold", "check", "raise"},
			description:   "First to act on flop should be able to check or bet",
		},
		{
			name: "facing_all_in",
			setupHand: func() *HandRunner {
				bot1 := &Bot{ID: "bot1", send: make(chan []byte, 10)}
				bot2 := &Bot{ID: "bot2", send: make(chan []byte, 10)}
				hr := NewHandRunner(testLogger(), []*Bot{bot1, bot2}, "test-hand", 0, rand.New(rand.NewSource(42)))
				hr.handState = game.NewHandState([]string{"bot1", "bot2"}, 0, 5, 10, 1000)
				hr.handState.Street = game.Flop
				hr.handState.CurrentBet = 1000
				hr.handState.Players[0].Bet = 1000
				hr.handState.Players[0].Chips = 0
				hr.handState.Players[0].AllInFlag = true
				hr.handState.Players[1].Bet = 0
				hr.handState.Players[1].Chips = 990
				hr.handState.ActivePlayer = 1
				return hr
			},
			expectedValid: []string{"fold", "allin"},
			description:   "Facing all-in should only be able to fold or call all-in",
		},
		{
			name: "short_stacked_cant_raise",
			setupHand: func() *HandRunner {
				bot1 := &Bot{ID: "bot1", send: make(chan []byte, 10)}
				bot2 := &Bot{ID: "bot2", send: make(chan []byte, 10)}
				hr := NewHandRunner(testLogger(), []*Bot{bot1, bot2}, "test-hand", 0, rand.New(rand.NewSource(42)))
				hr.handState = game.NewHandState([]string{"bot1", "bot2"}, 0, 5, 10, 1000)
				hr.handState.Street = game.Preflop
				hr.handState.CurrentBet = 10
				hr.handState.MinRaise = 10
				hr.handState.Players[0].Bet = 0
				hr.handState.Players[0].Chips = 15 // Only 15 chips left
				hr.handState.ActivePlayer = 0
				return hr
			},
			expectedValid: []string{"fold", "call", "allin"},
			description:   "Short stack should be able to call or go all-in but not raise",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hr := tt.setupHand()

			// Get valid actions from the hand state
			validActions := hr.handState.GetValidActions()

			// Convert to strings
			actionStrings := make([]string, len(validActions))
			for i, a := range validActions {
				actionStrings[i] = a.String()
			}

			// Check we have the expected actions
			if len(actionStrings) != len(tt.expectedValid) {
				t.Errorf("%s: expected %d valid actions, got %d. Actions: %v",
					tt.description, len(tt.expectedValid), len(actionStrings), actionStrings)
			}

			// Check each expected action is present
			for _, expected := range tt.expectedValid {
				found := false
				for _, actual := range actionStrings {
					if actual == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("%s: expected action '%s' not found in %v",
						tt.description, expected, actionStrings)
				}
			}

			// Ensure we never have empty valid actions
			if len(actionStrings) == 0 {
				t.Errorf("%s: CRITICAL - no valid actions generated!", tt.description)
			}
		})
	}
}

// TestActionRequestMessagePopulation verifies all ActionRequest fields are populated
func TestActionRequestMessagePopulation(t *testing.T) {
	// Setup
	bot := &Bot{ID: "test-bot", send: make(chan []byte, 10)}
	hr := NewHandRunner(testLogger(), []*Bot{bot}, "test-hand", 0, rand.New(rand.NewSource(42)))
	hr.handState = game.NewHandState([]string{"player1"}, 0, 5, 10, 1000)

	// Get valid actions
	validActions := hr.handState.GetValidActions()

	// Send action request
	err := hr.sendActionRequest(bot, 0, validActions)
	if err != nil {
		t.Fatalf("Failed to send action request: %v", err)
	}

	// Get the message
	select {
	case msgBytes := <-bot.send:
		// Unmarshal to verify contents
		var actionReq protocol.ActionRequest
		if err := protocol.Unmarshal(msgBytes, &actionReq); err != nil {
			t.Fatalf("Failed to unmarshal action request: %v", err)
		}

		// Verify all critical fields
		if len(actionReq.ValidActions) == 0 {
			t.Error("ValidActions is empty - this causes bots to fold!")
		}

		if actionReq.Type != "action_request" {
			t.Errorf("Type = %s, want 'action_request'", actionReq.Type)
		}

		if actionReq.HandID == "" {
			t.Error("HandID is empty")
		}

		// Check the valid actions match what we expect
		expectedActions := make([]string, len(validActions))
		for i, a := range validActions {
			expectedActions[i] = a.String()
		}

		if len(actionReq.ValidActions) != len(expectedActions) {
			t.Errorf("ValidActions length mismatch: got %d, want %d",
				len(actionReq.ValidActions), len(expectedActions))
		}

	case <-time.After(100 * time.Millisecond):
		t.Fatal("No message received")
	}
}

// TestActivePlayerAfterActions tests ActivePlayer is valid after various actions
func TestActivePlayerAfterActions(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() *game.HandState
		verify func(*testing.T, *game.HandState)
	}{
		{
			name: "after_fold_in_3way",
			setup: func() *game.HandState {
				h := game.NewHandState([]string{"p1", "p2", "p3"}, 0, 5, 10, 1000)
				h.ProcessAction(game.Fold, 0) // p1 folds
				return h
			},
			verify: func(t *testing.T, h *game.HandState) {
				if h.ActivePlayer != 1 && h.ActivePlayer != 2 {
					t.Errorf("ActivePlayer should be 1 or 2 after p1 folds, got %d", h.ActivePlayer)
				}
				if h.Players[h.ActivePlayer].Folded {
					t.Error("ActivePlayer has folded!")
				}
			},
		},
		{
			name: "after_all_in",
			setup: func() *game.HandState {
				h := game.NewHandState([]string{"p1", "p2"}, 0, 5, 10, 1000)
				h.ProcessAction(game.AllIn, 0) // p1 goes all-in
				return h
			},
			verify: func(t *testing.T, h *game.HandState) {
				if !h.IsComplete() {
					// If not complete, verify active player
					if h.ActivePlayer < 0 || h.ActivePlayer >= len(h.Players) {
						t.Errorf("Invalid ActivePlayer: %d", h.ActivePlayer)
					}
					if h.Players[h.ActivePlayer].AllInFlag {
						t.Error("ActivePlayer is all-in!")
					}
				}
			},
		},
		{
			name: "street_transition_headsup",
			setup: func() *game.HandState {
				h := game.NewHandState([]string{"p1", "p2"}, 0, 5, 10, 1000)
				// Complete preflop betting
				h.ProcessAction(game.Call, 0)  // SB calls
				h.ProcessAction(game.Check, 0) // BB checks
				return h
			},
			verify: func(t *testing.T, h *game.HandState) {
				if h.Street != game.Flop {
					t.Errorf("Should be on flop, but on %v", h.Street)
				}
				// In heads-up, BB acts first post-flop
				if h.ActivePlayer != 1 {
					t.Errorf("BB should act first on flop in heads-up, got player %d", h.ActivePlayer)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.setup()
			tt.verify(t, h)

			// Always verify we have valid actions if hand not complete
			if !h.IsComplete() {
				actions := h.GetValidActions()
				if len(actions) == 0 {
					t.Error("No valid actions available!")
				}
			}
		})
	}
}

// TestEmptyValidActionsScenarios specifically tests scenarios that might produce empty valid actions
func TestEmptyValidActionsScenarios(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() *game.HandState
		shouldErr bool
	}{
		{
			name: "negative_active_player",
			setup: func() *game.HandState {
				h := game.NewHandState([]string{"p1", "p2"}, 0, 5, 10, 1000)
				h.ActivePlayer = -1 // Invalid state
				return h
			},
			shouldErr: true,
		},
		{
			name: "out_of_bounds_active_player",
			setup: func() *game.HandState {
				h := game.NewHandState([]string{"p1", "p2"}, 0, 5, 10, 1000)
				h.ActivePlayer = 5 // Out of bounds
				return h
			},
			shouldErr: true,
		},
		{
			name: "all_players_folded",
			setup: func() *game.HandState {
				h := game.NewHandState([]string{"p1", "p2", "p3"}, 0, 5, 10, 1000)
				h.Players[0].Folded = true
				h.Players[1].Folded = true
				// p3 should win
				return h
			},
			shouldErr: false, // Hand should be complete
		},
		{
			name: "all_players_allin",
			setup: func() *game.HandState {
				h := game.NewHandState([]string{"p1", "p2"}, 0, 5, 10, 1000)
				h.Players[0].AllInFlag = true
				h.Players[1].AllInFlag = true
				return h
			},
			shouldErr: false, // Should run out remaining cards
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := tt.setup()

			// Protect against panic
			defer func() {
				if r := recover(); r != nil && !tt.shouldErr {
					t.Errorf("Unexpected panic: %v", r)
				}
			}()

			if h.IsComplete() {
				// Hand is complete, no actions needed
				return
			}

			// Check for valid active player
			if h.ActivePlayer < 0 || h.ActivePlayer >= len(h.Players) {
				if !tt.shouldErr {
					t.Errorf("Invalid ActivePlayer %d but error not expected", h.ActivePlayer)
				}
				return
			}

			// Get valid actions
			actions := h.GetValidActions()

			if len(actions) == 0 && !tt.shouldErr {
				t.Error("Got empty valid actions when they should exist!")
			}
		})
	}
}

func TestWrongBotActionRejection(t *testing.T) {
	// Test that actions from the wrong bot are ignored
	bots := []*Bot{
		{ID: "bot1", send: make(chan []byte, 100), actionChan: make(chan ActionEnvelope, 1), bankroll: 100},
		{ID: "bot2", send: make(chan []byte, 100), actionChan: make(chan ActionEnvelope, 1), bankroll: 100},
	}

	runner := NewHandRunner(testLogger(), bots, "wrong-bot-test", 0, rand.New(rand.NewSource(42)))

	// Inject a wrong bot action into the channel
	wrongBotEnvelope := ActionEnvelope{
		BotID: "bot2", // Bot 2 trying to act when it's bot 1's turn
		Action: protocol.Action{
			Type:   "action",
			Action: "call",
			Amount: 0,
		},
	}

	// Start hand runner
	handDone := make(chan bool)
	go func() {
		runner.Run()
		handDone <- true
	}()

	// Wait a bit for hand to start
	time.Sleep(10 * time.Millisecond)

	// Bot 1 should receive an action request despite bot 2's interference
	select {
	case msg := <-bots[0].send:
		var actionReq protocol.ActionRequest
		if err := protocol.Unmarshal(msg, &actionReq); err == nil && actionReq.Type == "action_request" {
			// Bot 1 got action request - inject wrong bot action
			select {
			case runner.botActionChan <- wrongBotEnvelope:
				// Wrong bot action sent
			default:
			}

			// Wait a bit then send correct action
			time.Sleep(20 * time.Millisecond)
			correctEnvelope := ActionEnvelope{
				BotID: "bot1",
				Action: protocol.Action{
					Type:   "action",
					Action: "fold",
					Amount: 0,
				},
			}
			select {
			case runner.botActionChan <- correctEnvelope:
				// Correct action sent
			default:
			}
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Bot 1 never received action request")
	}

	// Wait for hand to complete
	select {
	case <-handDone:
		// Good - hand completed normally despite wrong bot interference
	case <-time.After(500 * time.Millisecond):
		t.Error("Hand did not complete after correct action was sent")
	}
}

func TestBankrollDeltaCalculation(t *testing.T) {
	// Test that bankroll changes are calculated correctly based on actual buy-ins

	// Start bots with different bankrolls
	bot1 := &Bot{
		ID:       "rich-bot",
		send:     make(chan []byte, 100),
		bankroll: 1000, // Rich bot
	}
	bot2 := &Bot{
		ID:       "poor-bot",
		send:     make(chan []byte, 100),
		bankroll: 50, // Poor bot can only buy in for 50
	}

	bots := []*Bot{bot1, bot2}
	runner := NewHandRunner(testLogger(), bots, "bankroll-test", 0, rand.New(rand.NewSource(42)))

	// Record initial bankrolls
	initialBankroll1 := bot1.bankroll
	_ = bot2.bankroll // initialBankroll2 not used since bot2 loses everything

	// Record the buy-ins (what each bot actually brought to the table)
	buyIn1 := bot1.GetBuyIn() // Should be 100 (capped)
	buyIn2 := bot2.GetBuyIn() // Should be 50 (all they have)

	if buyIn1 != 100 {
		t.Errorf("Bot 1 buy-in = %d, expected 100", buyIn1)
	}
	if buyIn2 != 50 {
		t.Errorf("Bot 2 buy-in = %d, expected 50", buyIn2)
	}

	// Simulate hand completion where bot1 wins everything
	// Bot1 starts with 100, bot2 starts with 50
	// After blinds: bot1 has 95 (small blind 5), bot2 has 40 (big blind 10)
	// If bot2 goes all-in with 40 and bot1 calls:
	// - Main pot = 85 (40 from each + 5 from bot1's SB)
	// - Side pot = 55 (bot1's remaining chips)
	// If bot1 wins, final chips: bot1 = 140, bot2 = 0

	// Manually set final chip counts to simulate bot1 winning
	runner.handState = &game.HandState{
		Players: []*game.Player{
			{Chips: 140}, // Bot1 won the pot
			{Chips: 0},   // Bot2 lost everything
		},
	}
	runner.seatBuyIns = []int{buyIn1, buyIn2}

	// Apply the results
	for i, bot := range runner.bots {
		finalChips := runner.handState.Players[i].Chips
		delta := finalChips - runner.seatBuyIns[i]
		bot.ApplyResult(delta)
	}

	// Check bankroll changes
	// Bot1: started with 1000, bought in for 100 (now 900), won 140, delta = +40
	expectedBankroll1 := initialBankroll1 + 40
	if bot1.bankroll != expectedBankroll1 {
		t.Errorf("Bot1 bankroll = %d, expected %d (initial %d + delta %d)",
			bot1.bankroll, expectedBankroll1, initialBankroll1, 40)
	}

	// Bot2: started with 50, bought in for 50 (now 0), won 0, delta = -50
	expectedBankroll2 := 0 // Lost everything
	if bot2.bankroll != expectedBankroll2 {
		t.Errorf("Bot2 bankroll = %d, expected %d", bot2.bankroll, expectedBankroll2)
	}
}
