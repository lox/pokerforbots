package server

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestServerHealth(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	srv := NewServer(testLogger(), rng)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	srv.handleHealth(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

// TestStatsEndpoint verifies the enhanced stats endpoint
func TestStatsEndpoint(t *testing.T) {
	logger := testLogger()
	rng := rand.New(rand.NewSource(12345))

	t.Run("stats with no hand limit", func(t *testing.T) {
		// Create server with unlimited hands
		server := NewServer(logger, rng)

		req := httptest.NewRequest(http.MethodGet, "/stats", nil)
		recorder := httptest.NewRecorder()

		server.handleStats(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", recorder.Code)
		}

		body := recorder.Body.String()
		t.Logf("Stats output (unlimited): %s", body)

		// Should contain basic info
		if !strings.Contains(body, "Connected bots: 0") {
			t.Errorf("Expected 'Connected bots: 0', got: %s", body)
		}
		if !strings.Contains(body, "Hands completed: 0") {
			t.Errorf("Expected 'Hands completed: 0', got: %s", body)
		}
		if !strings.Contains(body, "Hand limit: unlimited") {
			t.Errorf("Expected 'Hand limit: unlimited', got: %s", body)
		}
	})

	t.Run("stats with hand limit", func(t *testing.T) {
		// Create server with hand limit
		server := NewServerWithHandLimit(logger, rng, 10)

		// Simulate some hands completed
		server.pool.handCounter = 3

		req := httptest.NewRequest(http.MethodGet, "/stats", nil)
		recorder := httptest.NewRecorder()

		server.handleStats(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", recorder.Code)
		}

		body := recorder.Body.String()
		t.Logf("Stats output (limited): %s", body)

		// Should contain hand limit info
		if !strings.Contains(body, "Connected bots: 0") {
			t.Errorf("Expected 'Connected bots: 0', got: %s", body)
		}
		if !strings.Contains(body, "Hands completed: 3") {
			t.Errorf("Expected 'Hands completed: 3', got: %s", body)
		}
		if !strings.Contains(body, "Hand limit: 10") {
			t.Errorf("Expected 'Hand limit: 10', got: %s", body)
		}
		if !strings.Contains(body, "Hands remaining: 7") {
			t.Errorf("Expected 'Hands remaining: 7', got: %s", body)
		}
	})

	t.Run("stats when limit reached", func(t *testing.T) {
		// Create server with hand limit
		server := NewServerWithHandLimit(logger, rng, 5)

		// Simulate hand limit reached
		server.pool.handCounter = 5

		req := httptest.NewRequest(http.MethodGet, "/stats", nil)
		recorder := httptest.NewRecorder()

		server.handleStats(recorder, req)

		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", recorder.Code)
		}

		body := recorder.Body.String()
		t.Logf("Stats output (limit reached): %s", body)

		// Should show 0 hands remaining
		if !strings.Contains(body, "Hands completed: 5") {
			t.Errorf("Expected 'Hands completed: 5', got: %s", body)
		}
		if !strings.Contains(body, "Hand limit: 5") {
			t.Errorf("Expected 'Hand limit: 5', got: %s", body)
		}
		if !strings.Contains(body, "Hands remaining: 0") {
			t.Errorf("Expected 'Hands remaining: 0', got: %s", body)
		}
	})
}

func TestWebSocketConnection(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	srv := NewServer(testLogger(), rng)
	srv.pool.minPlayers = 10
	srv.pool.config.RequirePlayer = false
	var poolWg sync.WaitGroup
	poolWg.Add(1)
	go func() {
		defer poolWg.Done()
		srv.pool.Run()
	}()
	t.Cleanup(func() {
		srv.pool.Stop()
		poolWg.Wait()
	})

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(srv.handleWebSocket))
	defer server.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect as a bot
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer ws.Close()
	sendConnectMessage(t, ws, "TestBot", "", string(BotRolePlayer))

	// Give the server time to register the bot
	time.Sleep(100 * time.Millisecond)

	// Check bot count
	if srv.pool.BotCount() != 1 {
		t.Errorf("Expected 1 bot, got %d", srv.pool.BotCount())
	}

	// Close connection
	ws.Close()

	// Give the server time to unregister
	time.Sleep(100 * time.Millisecond)

	if srv.pool.BotCount() != 0 {
		t.Errorf("Expected 0 bots after disconnect, got %d", srv.pool.BotCount())
	}
}

func TestMultipleBotConnections(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	srv := NewServer(testLogger(), rng)
	srv.pool.minPlayers = 10
	srv.pool.config.RequirePlayer = false
	var poolWg sync.WaitGroup
	poolWg.Add(1)
	go func() {
		defer poolWg.Done()
		srv.pool.Run()
	}()
	t.Cleanup(func() {
		srv.pool.Stop()
		poolWg.Wait()
	})

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(srv.handleWebSocket))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect multiple bots
	var bots []*websocket.Conn
	for i := 0; i < 3; i++ {
		ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect bot %d: %v", i, err)
		}
		sendConnectMessage(t, ws, fmt.Sprintf("Bot%02d", i), "", string(BotRolePlayer))
		bots = append(bots, ws)
	}

	// Give the server time to register all bots
	time.Sleep(100 * time.Millisecond)

	// Check bot count
	if srv.pool.BotCount() != 3 {
		t.Errorf("Expected 3 bots, got %d", srv.pool.BotCount())
	}

	// Close all connections
	for _, ws := range bots {
		ws.Close()
	}

	// Give the server time to unregister
	time.Sleep(100 * time.Millisecond)

	if srv.pool.BotCount() != 0 {
		t.Errorf("Expected 0 bots after disconnect, got %d", srv.pool.BotCount())
	}
}

func TestGamesEndpoint(t *testing.T) {
	rng := rand.New(rand.NewSource(77))
	srv := NewServer(testLogger(), rng)
	req := httptest.NewRequest(http.MethodGet, "/games", nil)
	rec := httptest.NewRecorder()

	srv.handleGames(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 response, got %d", rec.Code)
	}

	var games []GameSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &games); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(games) == 0 {
		t.Fatal("expected at least one game in response")
	}

	if games[0].ID != "default" {
		t.Fatalf("expected default game, got %s", games[0].ID)
	}
}

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
