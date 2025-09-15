package server

import (
	"testing"
	"time"
)

func TestBotPool(t *testing.T) {
	pool := NewBotPool(2, 4)
	go pool.Run()

	// Create mock bots
	bot1 := &Bot{ID: "bot1", send: make(chan []byte, 1)}
	bot2 := &Bot{ID: "bot2", send: make(chan []byte, 1)}
	bot3 := &Bot{ID: "bot3", send: make(chan []byte, 1)}

	// Register bots
	pool.Register(bot1)
	pool.Register(bot2)
	pool.Register(bot3)

	// Give time to process
	time.Sleep(50 * time.Millisecond)

	// Check bot count
	if pool.BotCount() != 3 {
		t.Errorf("Expected 3 bots, got %d", pool.BotCount())
	}

	// Check specific bot
	if bot, ok := pool.GetBot("bot1"); !ok || bot.ID != "bot1" {
		t.Error("Failed to get bot1")
	}

	// Unregister a bot
	pool.Unregister(bot2)
	time.Sleep(50 * time.Millisecond)

	if pool.BotCount() != 2 {
		t.Errorf("Expected 2 bots after unregister, got %d", pool.BotCount())
	}
}

func TestBotPoolMatching(t *testing.T) {
	pool := NewBotPool(2, 4)

	// Create mock bots
	bots := make([]*Bot, 3)
	for i := 0; i < 3; i++ {
		bots[i] = &Bot{
			ID:   string(rune('A' + i)),
			send: make(chan []byte, 1),
			pool: pool,
		}
	}

	// Manually test the matching logic
	// Add bots to the pool
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

	// Check that at least 2 bots are in a hand
	inHandCount := 0
	for _, bot := range bots {
		if bot.IsInHand() {
			inHandCount++
		}
	}

	if inHandCount < 2 {
		t.Errorf("Expected at least 2 bots in hand, got %d", inHandCount)
	}
}

func TestBotSendMessage(t *testing.T) {
	bot := &Bot{
		ID:   "test",
		send: make(chan []byte, 1),
	}

	// Test successful send - just test the channel mechanism
	testData := []byte("test message")

	go func() {
		select {
		case bot.send <- testData:
			// Message sent successfully
		case <-time.After(100 * time.Millisecond):
			t.Error("Send channel blocked")
		}
	}()

	// Receive the message
	select {
	case msg := <-bot.send:
		if string(msg) != "test message" {
			t.Errorf("Expected 'test message', got %s", string(msg))
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Failed to receive message")
	}
}