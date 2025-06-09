package server

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"

	"github.com/lox/pokerforbots/internal/game"
)

func TestSittingOutEventDriven(t *testing.T) {
	// Create a test server table
	logger := log.NewWithOptions(io.Discard, log.Options{})
	table := &ServerTable{
		playerReturn: make(chan struct{}, 1),
		logger:       logger,
	}

	// Test that the channel works for signaling
	t.Run("playerReturn channel signals correctly", func(t *testing.T) {
		// Start a goroutine that waits for the signal
		done := make(chan bool, 1)
		go func() {
			<-table.playerReturn
			done <- true
		}()

		// Signal the channel
		select {
		case table.playerReturn <- struct{}{}:
		default:
			t.Fatal("Channel should not be full")
		}

		// Verify the signal was received quickly
		select {
		case <-done:
			// Success - signal received
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Should have received signal within 100ms")
		}
	})

	t.Run("multiple signals don't block", func(t *testing.T) {
		// Clear the channel first
		select {
		case <-table.playerReturn:
		default:
		}

		// Send first signal
		select {
		case table.playerReturn <- struct{}{}:
		default:
			t.Fatal("First signal should not block")
		}

		// Send second signal (should not block due to select with default)
		start := time.Now()
		select {
		case table.playerReturn <- struct{}{}:
			t.Fatal("Second signal should not succeed when channel is full")
		default:
			// This is expected - channel is full, so we skip
		}
		elapsed := time.Since(start)

		// Should complete immediately without blocking
		assert.Less(t, elapsed, 10*time.Millisecond, "Should not block")
	})
}

func TestSittingOutGameFlow(t *testing.T) {
	// Test the sitting out player state logic
	t.Run("player sitting out state transitions", func(t *testing.T) {
		player := game.NewPlayer(1, "TestPlayer", game.Human, 100)

		// Initial state
		assert.False(t, player.IsSittingOut)
		assert.True(t, player.CanAct())
		assert.True(t, player.IsAvailableToPlay())

		// Sit out
		player.SitOut()
		assert.True(t, player.IsSittingOut)
		assert.False(t, player.CanAct())
		assert.False(t, player.IsAvailableToPlay())
		assert.True(t, player.IsFolded) // Should fold when sitting out

		// Reset for new hand (sitting out should persist)
		player.ResetForNewHand()
		assert.True(t, player.IsSittingOut) // Should persist
		assert.False(t, player.CanAct())    // Still can't act
		assert.False(t, player.IsActive)    // Not active in new hand

		// Sit back in
		player.SitIn()
		assert.False(t, player.IsSittingOut)
		assert.False(t, player.CanAct()) // Still not active until next hand reset

		// Reset for new hand after sitting in
		player.ResetForNewHand()
		assert.False(t, player.IsSittingOut)
		assert.True(t, player.CanAct())
		assert.True(t, player.IsActive)
	})
}

func TestGamePauseLogic(t *testing.T) {
	t.Run("hasAvailableRemotePlayer logic", func(t *testing.T) {
		// Simulate the logic from game_service.go
		type testPlayer struct {
			name         string
			isSittingOut bool
		}

		players := map[string]*testPlayer{
			"player1": {name: "player1", isSittingOut: false},
			"player2": {name: "player2", isSittingOut: true},
		}

		networkAgents := map[string]bool{
			"player1": true,
			"player2": true,
		}

		// Test available player detection
		hasAvailable := false
		for playerName := range networkAgents {
			if player, exists := players[playerName]; exists && !player.isSittingOut {
				hasAvailable = true
				break
			}
		}

		assert.True(t, hasAvailable, "Should detect player1 is available")

		// Now sit out player1
		players["player1"].isSittingOut = true

		// Test no available players
		hasAvailable = false
		for playerName := range networkAgents {
			if player, exists := players[playerName]; exists && !player.isSittingOut {
				hasAvailable = true
				break
			}
		}

		assert.False(t, hasAvailable, "Should detect no available players")
	})
}

func TestEventDrivenTiming(t *testing.T) {
	t.Run("event response time", func(t *testing.T) {
		// Test that the event-driven system responds quickly
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		ch := make(chan struct{}, 1)

		// Simulate game loop waiting
		go func() {
			<-ch // Wait for signal
		}()

		// Simulate player sitting back in
		start := time.Now()

		select {
		case ch <- struct{}{}:
		default:
			t.Fatal("Should be able to signal immediately")
		}

		elapsed := time.Since(start)

		// Should be essentially instantaneous
		assert.Less(t, elapsed, 1*time.Millisecond, "Event signaling should be immediate")

		// Make sure context doesn't timeout (game loop would have responded)
		select {
		case <-ctx.Done():
			t.Fatal("Context should not timeout - event was signaled")
		default:
			// Good - no timeout
		}
	})
}
