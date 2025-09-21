package server

import (
	"io"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/protocol"
	"github.com/rs/zerolog"
)

// testLogger creates a logger that discards output for tests
func testLogger() zerolog.Logger {
	return zerolog.New(io.Discard).Level(zerolog.Disabled)
}

// startTestPool starts a bot pool in a goroutine and returns cleanup function
func startTestPool(t *testing.T, pool *BotPool) func() {
	t.Helper()
	var wg sync.WaitGroup
	wg.Go(func() {
		pool.Run()
	})
	return func() {
		pool.Stop()
		wg.Wait()
	}
}

// dialAndConnect creates a WebSocket connection and sends connect message
func dialAndConnect(t *testing.T, url, name, game, role string) *websocket.Conn {
	t.Helper()

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("failed to dial %s: %v", url, err)
	}

	sendConnectMessage(t, conn, name, game, role)
	return conn
}

// sendConnectMessage sends a connect message over WebSocket
func sendConnectMessage(t *testing.T, conn *websocket.Conn, name, game, role string) {
	t.Helper()

	connectMsg := &protocol.Connect{
		Type: "connect",
		Name: name,
		Game: game,
		Role: role,
	}

	data, err := protocol.Marshal(connectMsg)
	if err != nil {
		t.Fatalf("failed to marshal connect message: %v", err)
	}

	err = conn.WriteMessage(websocket.BinaryMessage, data)
	if err != nil {
		t.Fatalf("failed to send connect message: %v", err)
	}
}
