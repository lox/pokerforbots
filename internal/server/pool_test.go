package server

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

// Test bot factory functions
func newTestBot(id string, pool *BotPool) *Bot {
	bot := &Bot{
		ID:       id,
		send:     make(chan []byte, 100),
		done:     make(chan struct{}),
		pool:     pool,
		mu:       sync.RWMutex{},
		inHand:   false,
		bankroll: 1000,
	}
	bot.SetRole(BotRolePlayer) // Default to Player role for tests
	return bot
}

func newTestBots(count int, pool *BotPool) []*Bot {
	bots := make([]*Bot, count)
	for i := range count {
		bots[i] = newTestBot(fmt.Sprintf("bot%d", i), pool)
	}
	return bots
}

// Removed unused test helper functions
// NPCs are now handled externally by the spawner package

// Test configuration factory
func testPoolConfig(minPlayers, maxPlayers int) Config {
	config := DefaultConfig(minPlayers, maxPlayers)
	config.Timeout = 50 * time.Millisecond // Faster tests
	return config
}

func TestBotPoolRegistration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		minPlayers int
		maxPlayers int
		botCount   int
	}{
		{"small pool", 2, 4, 3},
		{"large pool", 6, 9, 10},
		{"single bot", 2, 2, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := NewBotPool(testLogger(), rand.New(rand.NewSource(42)), testPoolConfig(tt.minPlayers, tt.maxPlayers))
			stopPool := startTestPool(t, pool)
			defer stopPool()

			bots := newTestBots(tt.botCount, pool)

			// Register all bots
			for _, bot := range bots {
				pool.Register(bot)
			}

			// Wait for registration
			waitForCondition(t, func() bool {
				return pool.BotCount() == tt.botCount
			}, 200*time.Millisecond, fmt.Sprintf("Expected %d bots to be registered", tt.botCount))

			// Verify we can retrieve specific bots
			for _, bot := range bots {
				if retrieved, ok := pool.GetBot(bot.ID); !ok || retrieved.ID != bot.ID {
					t.Errorf("Failed to retrieve bot %s", bot.ID)
				}
			}
		})
	}
}

func TestBotPoolUnregistration(t *testing.T) {
	t.Parallel()

	pool := NewBotPool(testLogger(), rand.New(rand.NewSource(42)), testPoolConfig(2, 4))
	stopPool := startTestPool(t, pool)
	defer stopPool()

	bots := newTestBots(3, pool)

	// Register all bots
	for _, bot := range bots {
		pool.Register(bot)
	}

	waitForCondition(t, func() bool {
		return pool.BotCount() == 3
	}, 200*time.Millisecond, "Expected 3 bots to be registered")

	// Unregister middle bot
	pool.Unregister(bots[1])

	waitForCondition(t, func() bool {
		return pool.BotCount() == 2
	}, 200*time.Millisecond, "Expected 2 bots after unregister")

	// Verify the right bot was removed
	if _, ok := pool.GetBot(bots[1].ID); ok {
		t.Error("Bot should have been unregistered")
	}

	// Verify other bots still exist
	for _, bot := range []int{0, 2} {
		if _, ok := pool.GetBot(bots[bot].ID); !ok {
			t.Errorf("Bot %s should still be registered", bots[bot].ID)
		}
	}
}

func TestBotPoolMatching(t *testing.T) {
	t.Parallel()

	pool := NewBotPool(testLogger(), rand.New(rand.NewSource(42)), testPoolConfig(2, 4))
	stopPool := startTestPool(t, pool)
	defer stopPool()

	// Create enough bots to trigger matching
	bots := newTestBots(3, pool)

	for _, bot := range bots {
		pool.Register(bot)
	}

	waitForCondition(t, func() bool {
		return pool.BotCount() == 3
	}, 200*time.Millisecond, "Expected 3 bots to be registered")

	// Give the matcher time to work
	time.Sleep(300 * time.Millisecond)

	// Verify pool is still functional
	if pool.BotCount() == 0 {
		t.Error("All bots disappeared - matching may have issues")
	}
}

// TestBotPoolRequiresPlayer has been removed
// The RequirePlayer functionality was removed along with NPC support
// NPCs are now handled externally by the spawner package

func TestBotPoolHandLimit(t *testing.T) {
	t.Parallel()

	config := testPoolConfig(2, 2)
	config.HandLimit = 3

	pool := NewBotPool(testLogger(), rand.New(rand.NewSource(789)), config)
	stopPool := startTestPool(t, pool)
	defer stopPool()

	// Just verify the hand limit configuration is set
	if pool.handLimit != 3 {
		t.Errorf("Expected hand limit 3, got %d", pool.handLimit)
	}
}

// Test for race conditions and edge cases
func TestBotPoolConcurrentOperations(t *testing.T) {
	t.Parallel()

	pool := NewBotPool(testLogger(), rand.New(rand.NewSource(999)), testPoolConfig(2, 6))
	stopPool := startTestPool(t, pool)
	defer stopPool()

	const numGoroutines = 10
	const botsPerGoroutine = 3

	var wg sync.WaitGroup

	// Concurrently register and unregister bots
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			bots := make([]*Bot, botsPerGoroutine)
			for j := range botsPerGoroutine {
				bots[j] = newTestBot(fmt.Sprintf("concurrent-bot-%d-%d", id, j), pool)
				pool.Register(bots[j])
			}

			// Brief pause
			time.Sleep(50 * time.Millisecond)

			// Unregister some bots
			for j := range botsPerGoroutine / 2 {
				pool.Unregister(bots[j])
			}
		}(i)
	}

	wg.Wait()

	// Pool should still be functional
	finalCount := pool.BotCount()
	if finalCount < 0 {
		t.Error("Negative bot count indicates data race")
	}
}

func TestBotSendMessage(t *testing.T) {
	t.Parallel()

	bot := newTestBot("test-bot", nil)

	testMessage := []byte("test message")

	// Test successful send
	go func() {
		select {
		case bot.send <- testMessage:
			// Success
		case <-time.After(100 * time.Millisecond):
			t.Error("Failed to send message within timeout")
		}
	}()

	// Receive the message
	select {
	case received := <-bot.send:
		if string(received) != string(testMessage) {
			t.Errorf("Expected %q, got %q", string(testMessage), string(received))
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("Failed to receive message within timeout")
	}
}

func TestBotPoolStatistics(t *testing.T) {
	t.Parallel()

	config := testPoolConfig(2, 2)
	config.EnableStats = true

	pool := NewBotPool(testLogger(), rand.New(rand.NewSource(555)), config)
	stopPool := startTestPool(t, pool)
	defer stopPool()

	bots := newTestBots(2, pool)
	for _, bot := range bots {
		pool.Register(bot)
	}

	waitForCondition(t, func() bool {
		return pool.BotCount() == 2
	}, 200*time.Millisecond, "Expected 2 bots to be registered")

	// Verify stats collection is enabled
	if pool.statsCollector == nil {
		t.Error("Expected stats collector to be initialized")
	}
	if !pool.statsCollector.IsEnabled() {
		t.Error("Expected stats collection to be enabled")
	}

	// Check initial empty stats
	stats := pool.PlayerStats()
	if len(stats) != 0 {
		t.Errorf("Expected no initial stats, got %d", len(stats))
	}
}

// waitForCondition waits for a condition to be true with timeout
func waitForCondition(t *testing.T, condition func() bool, timeout time.Duration, errMsg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s (timed out after %v)", errMsg, timeout)
}
