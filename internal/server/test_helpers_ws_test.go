package server

import (
	"testing"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
)

func sendConnectMessage(t *testing.T, conn *websocket.Conn, name, game, role string) {
	t.Helper()

	if role == "" {
		role = string(BotRolePlayer)
	}

	msg := &protocol.Connect{Type: protocol.TypeConnect, Name: name, Role: role}
	if game != "" {
		msg.Game = game
	}

	payload, err := protocol.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal connect message: %v", err)
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		t.Fatalf("failed to send connect message: %v", err)
	}
}
