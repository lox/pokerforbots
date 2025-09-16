package server

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

// TestHandLimitRespected verifies that the bot pool stops creating hands when the limit is reached
func TestHandLimitRespected(t *testing.T) {
	t.Skip("Skipping integration test with nil connections - use TestHandLimitLogic for unit testing")
	logger := testLogger()
	rng := rand.New(rand.NewSource(123))
	handLimit := uint64(2) // Only allow 2 hands

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

	// Create 4 bots - enough for 2 hands
	bots := make([]*Bot, 4)
	for i := 0; i < 4; i++ {
		bot := &Bot{
			ID:       fmt.Sprintf("test-bot-%d", i),
			send:     make(chan []byte, 256),
			done:     make(chan struct{}),
			conn:     nil, // No real connection needed for this test
			pool:     pool,
			logger:   logger,
			bankroll: 1000,
		}
		bots[i] = bot
		pool.Register(bot)
	}

	// Wait for hands to complete
	time.Sleep(500 * time.Millisecond)

	// Check that exactly handLimit hands were created
	finalHandCount := pool.handCounter
	if finalHandCount != handLimit {
		t.Errorf("Expected exactly %d hands, but got %d", handLimit, finalHandCount)
	}

	// Try to create more bots - they should be registered but no new hands should start
	moreBots := make([]*Bot, 2)
	for i := 0; i < 2; i++ {
		bot := &Bot{
			ID:       fmt.Sprintf("extra-bot-%d", i),
			send:     make(chan []byte, 256),
			done:     make(chan struct{}),
			conn:     nil,
			pool:     pool,
			logger:   logger,
			bankroll: 1000,
		}
		moreBots[i] = bot
		pool.Register(bot)
	}

	// Wait a bit more
	time.Sleep(200 * time.Millisecond)

	// Verify no additional hands were created
	if pool.handCounter != handLimit {
		t.Errorf("Hand limit was exceeded: expected %d, got %d", handLimit, pool.handCounter)
	}

	t.Logf("SUCCESS: Hand limit of %d was respected, %d hands completed", handLimit, pool.handCounter)
}

// TestUnlimitedHands verifies that 0 hand limit means unlimited
func TestUnlimitedHands(t *testing.T) {
	t.Skip("Skipping integration test with nil connections - use TestUnlimitedHandsWithZeroLimit for unit testing")
	logger := testLogger()
	rng := rand.New(rand.NewSource(456))

	// Create pool with no hand limit (0 = unlimited)
	pool := NewBotPoolWithLimit(logger, 2, 4, rng, 0)

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

	// Create and register multiple batches of bots
	totalBots := 8
	for i := 0; i < totalBots; i++ {
		bot := &Bot{
			ID:       fmt.Sprintf("unlimited-bot-%d", i),
			send:     make(chan []byte, 256),
			done:     make(chan struct{}),
			conn:     nil,
			pool:     pool,
			logger:   logger,
			bankroll: 1000,
		}
		pool.Register(bot)
	}

	// Wait for multiple hands to complete
	time.Sleep(800 * time.Millisecond)

	// Should have completed multiple hands (at least 2 with 8 bots)
	if pool.handCounter < 2 {
		t.Errorf("Expected at least 2 hands with unlimited setting, got %d", pool.handCounter)
	}

	t.Logf("SUCCESS: Unlimited hands setting allowed %d hands to complete", pool.handCounter)
}
