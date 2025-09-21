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
	"github.com/lox/pokerforbots/protocol"
)

func TestServerHealth(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
		server := NewServer(logger, rng, WithHandLimit(10))

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
		server := NewServer(logger, rng, WithHandLimit(5))

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
	t.Parallel()
	rng := rand.New(rand.NewSource(42))
	srv := NewServer(testLogger(), rng)
	srv.pool.minPlayers = 10
	srv.pool.config.RequirePlayer = false
	var poolWg sync.WaitGroup
	poolWg.Go(func() {
		srv.pool.Run()
	})
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
	t.Parallel()
	rng := rand.New(rand.NewSource(42))
	srv := NewServer(testLogger(), rng)
	srv.pool.minPlayers = 10
	srv.pool.config.RequirePlayer = false
	var poolWg sync.WaitGroup
	poolWg.Go(func() {
		srv.pool.Run()
	})
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
	for i := range 3 {
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
	t.Parallel()
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

func TestAdminCreateAndDeleteGame(t *testing.T) {
	t.Parallel()
	srv := NewServer(testLogger(), rand.New(rand.NewSource(99)))

	createPayload := `{
		"id": "test",
		"small_blind": 10,
		"big_blind": 20,
		"start_chips": 1500,
		"timeout_ms": 200,
		"min_players": 2,
		"max_players": 6,
		"require_player": false,
		"npcs": [{"strategy": "random", "count": 2}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/admin/games", strings.NewReader(createPayload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleAdminGames(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}

	game, ok := srv.manager.GetGame("test")
	if !ok {
		t.Fatal("expected game to be registered")
	}
	if len(game.npcs) != 2 {
		t.Fatalf("expected 2 NPCs, got %d", len(game.npcs))
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/admin/games/test", nil)
	deleteRec := httptest.NewRecorder()

	srv.handleAdminGame(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", deleteRec.Code)
	}

	if _, ok := srv.manager.GetGame("test"); ok {
		t.Fatal("expected game to be removed")
	}
	if len(game.npcs) != 0 {
		t.Fatalf("expected NPCs to be stopped, still have %d", len(game.npcs))
	}
}

func TestAdminGameStatsEndpoint(t *testing.T) {
	t.Parallel()
	srv := NewServer(testLogger(), rand.New(rand.NewSource(7)))

	game, ok := srv.manager.GetGame("default")
	if !ok {
		t.Fatal("expected default game to exist")
	}

	bot1 := NewBot(testLogger(), "bot-player", nil, game.Pool)
	bot1.SetDisplayName("complex")
	bot1.SetRole(BotRolePlayer)

	bot2 := NewBot(testLogger(), "bot-npc", nil, game.Pool)
	bot2.SetDisplayName("npc-aggr")
	bot2.SetRole(BotRoleNPC)

	game.Pool.RecordHandOutcome("hand-1", []*Bot{bot1, bot2}, []int{150, -150})

	statsReq := httptest.NewRequest(http.MethodGet, "/admin/games/default/stats", nil)
	statsRec := httptest.NewRecorder()

	srv.handleAdminGame(statsRec, statsReq)

	if statsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", statsRec.Code)
	}

	var payload GameStats
	if err := json.Unmarshal(statsRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode stats response: %v", err)
	}

	if payload.ID != "default" {
		t.Fatalf("expected stats for default game, got %s", payload.ID)
	}

	if len(payload.Players) != 2 {
		t.Fatalf("expected 2 player stats, got %d", len(payload.Players))
	}

	var foundPlayer bool
	for _, ps := range payload.Players {
		if ps.DisplayName == "complex" {
			foundPlayer = true
			if ps.NetChips != 150 {
				t.Fatalf("expected complex net chips 150, got %d", ps.NetChips)
			}
			if ps.Role != string(BotRolePlayer) {
				t.Fatalf("expected complex role player, got %s", ps.Role)
			}
		}
	}

	if !foundPlayer {
		t.Fatal("expected complex bot stats present")
	}

	notFoundReq := httptest.NewRequest(http.MethodGet, "/admin/games/missing/stats", nil)
	notFoundRec := httptest.NewRecorder()

	srv.handleAdminGame(notFoundRec, notFoundReq)

	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing game, got %d", notFoundRec.Code)
	}
}

// TestHandLimitLogic verifies that the bot pool stops creating hands when hand limit is reached
// This tests the tryMatch logic directly without requiring WebSocket connections
func TestHandLimitLogic(t *testing.T) {
	t.Parallel()
	logger := testLogger()
	rng := rand.New(rand.NewSource(42))
	handLimit := uint64(2) // Allow exactly 2 hands

	// Create pool with hand limit
	config := DefaultConfig(2, 4)
	config.HandLimit = handLimit
	pool := NewBotPool(logger, rng, config)
	pool.SetGameID("test-hand-limit")

	// Start pool
	var wg sync.WaitGroup
	wg.Go(func() {
		pool.Run()
	})
	defer func() {
		pool.Stop()
		wg.Wait()
	}()

	// Simulate the hand limit being reached by directly setting the counter
	atomic.StoreUint64(&pool.handCounter, handLimit)

	// Create bots and register them - they should not trigger new hands
	bots := make([]*Bot, 0, 4)
	for i := range 4 {
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
		bots = append(bots, bot)
	}

	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		if atomic.LoadUint64(&pool.handCounter) == handLimit {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Verify hand count hasn't increased beyond the limit
	finalHandCount := atomic.LoadUint64(&pool.handCounter)
	if finalHandCount != handLimit {
		t.Errorf("Hand limit was not respected: expected %d, got %d", handLimit, finalHandCount)
	}

	if !pool.HandLimitNotified() {
		deadline := time.Now().Add(200 * time.Millisecond)
		for time.Now().Before(deadline) {
			if pool.HandLimitNotified() {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if !pool.HandLimitNotified() {
			t.Fatalf("expected hand limit notification to be sent")
		}
	}

	decodeGameCompleted := func(raw []byte) {
		var completed protocol.GameCompleted
		if err := protocol.Unmarshal(raw, &completed); err != nil {
			t.Fatalf("failed to decode game_completed message: %v", err)
		}
		if completed.Type != protocol.TypeGameCompleted {
			t.Fatalf("expected game_completed type, got %s", completed.Type)
		}
		if completed.HandLimit != handLimit {
			t.Fatalf("expected hand limit %d, got %d", handLimit, completed.HandLimit)
		}
		if completed.GameID == "" {
			t.Fatal("expected game id to be set")
		}
		if completed.Reason != "hand_limit_reached" {
			t.Fatalf("expected reason hand_limit_reached, got %s", completed.Reason)
		}
	}

	messageReceived := false
	deadlineMsg := time.Now().Add(150 * time.Millisecond)
	for !messageReceived && time.Now().Before(deadlineMsg) {
		for _, bot := range bots {
			select {
			case msg := <-bot.send:
				decodeGameCompleted(msg)
				messageReceived = true
			default:
			}
		}
		if !messageReceived {
			time.Sleep(5 * time.Millisecond)
		}
	}

	if !messageReceived {
		t.Fatal("expected game_completed message to be delivered")
	}

	t.Logf("SUCCESS: Hand limit of %d was respected, %d hands completed", handLimit, finalHandCount)
}

// TestUnlimitedHandsWithZeroLimit verifies that 0 hand limit means unlimited
func TestUnlimitedHandsWithZeroLimit(t *testing.T) {
	t.Parallel()
	logger := testLogger()
	rng := rand.New(rand.NewSource(456))

	// Create pool with no hand limit (0 = unlimited)
	config := DefaultConfig(2, 4)
	config.HandLimit = 0
	pool := NewBotPool(logger, rng, config)

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
