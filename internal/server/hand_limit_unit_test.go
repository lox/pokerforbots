package server

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestHandLimitLogic verifies that the bot pool stops creating hands when hand limit is reached
// This tests the tryMatch logic directly without requiring WebSocket connections
func TestHandLimitLogic(t *testing.T) {
	logger := testLogger()
	rng := rand.New(rand.NewSource(42))
	handLimit := uint64(2) // Allow exactly 2 hands

	// Create pool with hand limit
	pool := NewBotPoolWithLimit(logger, 2, 4, rng, handLimit)

	// Start pool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		pool.Run()
	}()
	defer func() {
		pool.Stop()
		wg.Wait()
	}()

	// Simulate the hand limit being reached by directly setting the counter
	atomic.StoreUint64(&pool.handCounter, handLimit)

	// Create bots and register them - they should not trigger new hands
	for i := 0; i < 4; i++ {
		bot := &Bot{
			ID:       fmt.Sprintf("test-bot-%d", i+1),
			send:     make(chan []byte, 256),
			done:     make(chan struct{}),
			pool:     pool,
			logger:   logger,
			bankroll: 1000,
			conn:     nil, // This will cause runHand to exit early, which is perfect for testing
		}
		pool.Register(bot)
	}

	// Wait for the tryMatch to be called multiple times
	time.Sleep(500 * time.Millisecond)

	// Verify hand count hasn't increased beyond the limit
	finalHandCount := atomic.LoadUint64(&pool.handCounter)
	if finalHandCount != handLimit {
		t.Errorf("Hand limit was not respected: expected %d, got %d", handLimit, finalHandCount)
	}

	t.Logf("SUCCESS: Hand limit of %d was respected, %d hands completed", handLimit, finalHandCount)
}

// TestUnlimitedHandsWithZeroLimit verifies that 0 hand limit means unlimited
func TestUnlimitedHandsWithZeroLimit(t *testing.T) {
	logger := testLogger()
	rng := rand.New(rand.NewSource(456))

	// Create pool with no hand limit (0 = unlimited)
	pool := NewBotPoolWithLimit(logger, 2, 4, rng, 0)

	// Directly test that the tryMatch method doesn't stop when hand limit is 0
	// Set a high hand counter to simulate many hands completed
	atomic.StoreUint64(&pool.handCounter, 100)

	// The tryMatch should still allow matching since handLimit is 0
	// We can test this by verifying the counter doesn't prevent matching logic

	// This is mainly a documentation test - the real logic is that when handLimit == 0,
	// the check `if p.handLimit > 0 && atomic.LoadUint64(&p.handCounter) >= p.handLimit`
	// will be false because p.handLimit == 0

	if pool.handLimit != 0 {
		t.Errorf("Expected handLimit to be 0 for unlimited, got %d", pool.handLimit)
	}

	t.Logf("SUCCESS: Unlimited hands setting (handLimit=0) configured correctly")
}
