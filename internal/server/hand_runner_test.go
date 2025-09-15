package server

import (
	"testing"
	"time"

	"github.com/lox/pokerforbots/internal/game"
)

func TestHandRunner(t *testing.T) {
	// Create mock bots with larger buffers
	bots := []*Bot{
		{ID: "bot1-12345678", send: make(chan []byte, 100)},
		{ID: "bot2-12345678", send: make(chan []byte, 100)},
		{ID: "bot3-12345678", send: make(chan []byte, 100)},
	}

	// Create hand runner
	runner := NewHandRunner(bots, "test-hand", 0)

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
	runner := NewHandRunner(bots, "test-hand-2", 0)

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
	runner := NewHandRunner([]*Bot{bot}, "test-hand-3", 0)
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
	// This test is skipped because listenForAction currently
	// auto-responds with call for testing
	t.Skip("Skipping timeout test - listenForAction auto-responds")
}

func TestHandRunnerComplete(t *testing.T) {
	// Create a simple 2-player scenario
	bots := []*Bot{
		{ID: "alice12345678", send: make(chan []byte, 100)},
		{ID: "bob1234567890", send: make(chan []byte, 100)},
	}

	runner := NewHandRunner(bots, "complete-test", 0)

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
	runner.resolveHand()
	runner.broadcastHandResult()

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