package server

import (
	"testing"
	"time"

	"github.com/lox/pokerforbots/internal/game"
	"github.com/lox/pokerforbots/internal/protocol"
)

// TestListenForActionGoroutineLeak verifies listenForAction doesn't leak goroutines
func TestListenForActionGoroutineLeak(t *testing.T) {
	// Count goroutines before
	// Note: In a real test we'd use runtime.NumGoroutine() but let's focus on the logic

	bot := &Bot{
		ID:         "test-bot",
		send:       make(chan []byte, 10),
		actionChan: make(chan protocol.Action, 1),
	}

	hr := NewHandRunner([]*Bot{bot}, "test-hand", 0)
	hr.handState = game.NewHandState([]string{"player1"}, 0, 5, 10, 1000)

	// Simulate multiple calls to waitForAction
	// Each one starts a listenForAction goroutine
	doneChannels := make([]chan struct{}, 5)
	for i := 0; i < 5; i++ {
		done := make(chan struct{})
		doneChannels[i] = done
		// Start listenForAction in goroutine (like waitForAction does)
		go hr.listenForAction(0, done)

		// Give it a moment to start
		time.Sleep(10 * time.Millisecond)
	}

	// Now we potentially have 5 goroutines running listenForAction
	// They will keep timing out every 100ms and sending fold actions!

	// Try to read from the actions channel
	actionsReceived := 0
	timeout := time.After(500 * time.Millisecond)

	for {
		select {
		case action := <-hr.actions:
			actionsReceived++
			t.Logf("Received action %d: %v", actionsReceived, action)
			if actionsReceived > 5 {
				t.Error("Received more actions than expected - goroutines are leaking!")
				return
			}
		case <-timeout:
			t.Logf("Total actions received: %d", actionsReceived)

			// Close all done channels to stop the goroutines
			for _, done := range doneChannels {
				if done != nil {
					close(done)
				}
			}

			// With the fix, we should only get one action per goroutine
			if actionsReceived != 5 {
				t.Errorf("Expected exactly 5 actions (one per goroutine), got %d", actionsReceived)
			}
			return
		}
	}
}

// TestWaitForActionRepeatedCalls simulates what happens in the actual game loop
func TestWaitForActionRepeatedCalls(t *testing.T) {
	bot1 := &Bot{
		ID:         "bot1",
		send:       make(chan []byte, 10),
		actionChan: make(chan protocol.Action, 1),
	}

	bot2 := &Bot{
		ID:         "bot2",
		send:       make(chan []byte, 10),
		actionChan: make(chan protocol.Action, 1),
	}

	hr := NewHandRunner([]*Bot{bot1, bot2}, "test-hand", 0)
	hr.handState = game.NewHandState([]string{"bot1", "bot2"}, 0, 5, 10, 1000)

	// Simulate the game loop calling waitForAction multiple times
	// This happens in the Run() method's for loop

	// First action request to bot1
	go func() {
		action, _ := hr.waitForAction(0)
		t.Logf("Bot1 action: %v", action)
	}()

	time.Sleep(50 * time.Millisecond)

	// Second action request to bot2
	go func() {
		action, _ := hr.waitForAction(1)
		t.Logf("Bot2 action: %v", action)
	}()

	time.Sleep(50 * time.Millisecond)

	// Third action request back to bot1
	go func() {
		action, _ := hr.waitForAction(0)
		t.Logf("Bot1 second action: %v", action)
	}()

	// Now we have multiple listenForAction goroutines running
	// They might interfere with each other!

	// Wait and see if we get spurious actions
	time.Sleep(300 * time.Millisecond)

	// Try to drain the actions channel to see how many we got
	drainedActions := 0
	for {
		select {
		case <-hr.actions:
			drainedActions++
		default:
			t.Logf("Drained %d actions from channel", drainedActions)
			if drainedActions > 3 {
				t.Error("More actions than expected - goroutines are interfering")
			}
			return
		}
	}
}
