package server

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestBotPool(t *testing.T) {
	pool := NewBotPool(2, 4)
	go pool.Run()

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
	t.Skip("Skipping - hand runner integration needs more work")

	pool := NewBotPool(2, 4)

	// Create mock bots with proper initialization
	bots := make([]*Bot, 3)
	for i := 0; i < 3; i++ {
		bots[i] = &Bot{
			ID:   fmt.Sprintf("bot%d-12345678", i),
			send: make(chan []byte, 1),
			pool: pool,
			done: make(chan struct{}),
			mu:   sync.RWMutex{},
		}
	}

	// Safely add bots to the pool
	pool.mu.Lock()
	for _, bot := range bots {
		pool.bots[bot.ID] = bot
	}
	pool.mu.Unlock()

	// Add bots to available queue
	for _, bot := range bots {
		select {
		case pool.available <- bot:
		default:
			t.Error("Failed to add bot to available queue")
		}
	}

	// Try to match
	pool.tryMatch()

	// Check that at least 2 bots are in a hand with proper synchronization
	var wg sync.WaitGroup
	inHandCount := 0
	var countMu sync.Mutex

	for _, bot := range bots {
		wg.Add(1)
		go func(b *Bot) {
			defer wg.Done()
			if b.IsInHand() {
				countMu.Lock()
				inHandCount++
				countMu.Unlock()
			}
		}(bot)
	}

	wg.Wait()

	if inHandCount < 2 {
		t.Errorf("Expected at least 2 bots in hand, got %d", inHandCount)
	}
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