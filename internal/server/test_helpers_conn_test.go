package server

import (
	"testing"

	"github.com/gorilla/websocket"
)

func dialAndConnect(t *testing.T, url, name, game, role string) *websocket.Conn {
	t.Helper()

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("failed to dial %s: %v", url, err)
	}

	sendConnectMessage(t, conn, name, game, role)
	return conn
}
