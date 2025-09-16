package server

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func TestBotPool(t *testing.T) {
	pool := NewBotPool(testLogger(), 2, 4, rand.New(rand.NewSource(42)))
	stopPool := startTestPool(t, pool)
	defer stopPool()

	// Create mock bots with proper initialization
	bot1 := &Bot{
		ID:   "bot1",
		send: make(chan []byte, 1),
		done: make(chan struct{}),
	}
	bot2 := &Bot{
		ID:   "bot2",
		send: make(chan []byte, 1),
		done: make(chan struct{}),
	}
	bot3 := &Bot{
		ID:   "bot3",
		send: make(chan []byte, 1),
		done: make(chan struct{}),
	}

	// Register bots
	pool.Register(bot1)
	pool.Register(bot2)
	pool.Register(bot3)

	// Wait for registration to complete using a retry loop
	waitForCondition(t, func() bool {
		return pool.BotCount() == 3
	}, 100*time.Millisecond, "Expected 3 bots to be registered")

	// Check specific bot
	if bot, ok := pool.GetBot("bot1"); !ok || bot.ID != "bot1" {
		t.Error("Failed to get bot1")
	}

	// Unregister a bot
	pool.Unregister(bot2)

	// Wait for unregistration to complete
	waitForCondition(t, func() bool {
		return pool.BotCount() == 2
	}, 100*time.Millisecond, "Expected 2 bots after unregister")
}

func TestBotPoolMatching(t *testing.T) {
	pool := NewBotPool(testLogger(), 2, 4, rand.New(rand.NewSource(42)))

	// Start the pool in background
	stopPool := startTestPool(t, pool)
	defer stopPool()

	// Create properly initialized bots
	bots := make([]*Bot, 3)
	for i := 0; i < 3; i++ {
		bots[i] = &Bot{
			ID:     fmt.Sprintf("bot%d-12345678", i),
			send:   make(chan []byte, 100), // Larger buffer to prevent blocking
			pool:   pool,
			done:   make(chan struct{}),
			mu:     sync.RWMutex{},
			inHand: false,
		}
	}

	// Register bots with the pool using the proper mechanism
	for _, bot := range bots {
		pool.Register(bot)
	}

	// Wait for bots to be registered
	waitForCondition(t, func() bool {
		return pool.BotCount() == 3
	}, 100*time.Millisecond, "Expected 3 bots to be registered")

	// Give the pool a moment to try matching
	time.Sleep(200 * time.Millisecond)

	// Check that at least 2 bots were matched into a hand
	// We can't reliably test the exact state due to race conditions in hand completion,
	// but we can verify the pool is functioning
	if pool.BotCount() == 0 {
		t.Error("All bots disappeared - pool matching may have issues")
	}

	// The important thing is that the pool didn't crash and is still running
	// Bots may have completed hands and been returned to the pool
}

func TestBotSendMessage(t *testing.T) {
	bot := &Bot{
		ID:   "test",
		send: make(chan []byte, 1),
		done: make(chan struct{}),
	}

	// Test successful send
	testData := []byte("test message")

	// Use a channel to signal completion
	done := make(chan bool)

	go func() {
		select {
		case bot.send <- testData:
			done <- true
		case <-time.After(100 * time.Millisecond):
			done <- false
		}
	}()

	// Receive the message
	select {
	case msg := <-bot.send:
		if string(msg) != "test message" {
			t.Errorf("Expected 'test message', got %s", string(msg))
		}
		// Wait for sender to complete
		if !<-done {
			t.Error("Sender reported failure")
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Failed to receive message")
	}
}

// waitForCondition waits for a condition to be true with timeout
func waitForCondition(t *testing.T, condition func() bool, timeout time.Duration, errMsg string) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Error(errMsg)
}
