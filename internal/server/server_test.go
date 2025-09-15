package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestServerHealth(t *testing.T) {
	srv := NewServer()

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	srv.handleHealth(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestServerStats(t *testing.T) {
	srv := NewServer()

	req := httptest.NewRequest("GET", "/stats", nil)
	w := httptest.NewRecorder()

	srv.handleStats(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Connected bots: 0") {
		t.Errorf("Expected stats to show 0 bots, got: %s", body)
	}
}

func TestWebSocketConnection(t *testing.T) {
	srv := NewServer()
	go srv.pool.Run()

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
	srv := NewServer()
	go srv.pool.Run()

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