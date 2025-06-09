package testing

import (
	"fmt"
	"io"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/require"

	"github.com/lox/pokerforbots/internal/client"
	"github.com/lox/pokerforbots/internal/tui"
)

func TestBasicConnection(t *testing.T) {
	// Use stderr for debugging
	logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})

	// 1. Start server on random port
	port := findFreePort(t)
	server := startTestServer(t, port, 12345, 1) // Start with 1 bot
	defer server.Stop()

	// 2. Create TUI in test mode
	tuiModel := tui.NewTUIModelWithOptions(logger, true)

	// 3. Create client connection
	serverURL := fmt.Sprintf("ws://127.0.0.1:%d", port)
	wsClient := client.NewClient(serverURL, logger)

	err := wsClient.Connect()
	require.NoError(t, err, "Failed to connect")

	err = wsClient.Auth("TestPlayer")
	require.NoError(t, err, "Failed to authenticate")

	// Set up real bridge and command handler
	setupTUIBridge(wsClient, tuiModel)
	tui.StartCommandHandler(wsClient, tuiModel, 200)

	// Try to join table
	err = wsClient.JoinTable("table1", 200)
	require.NoError(t, err, "Failed to join table")

	// Create a temporary test client to wait for events
	tempClient := &TestClient{
		client:        wsClient,
		tui:           tuiModel,
		eventChan:     make(chan string, 100),
		handStarted:   make(chan struct{}, 1),
		handEnded:     make(chan struct{}, 1),
		streetChanged: make(chan struct{}, 1),
		playerTimeout: make(chan struct{}, 1),
		gamePause:     make(chan struct{}, 1),
		t:             t,
	}
	tempClient.setupEventCallback()
	tempClient.StartActionScript() // Start processing events

	// Wait for some game events
	tempClient.WaitForHandStart()

	// Disconnect
	_ = wsClient.Disconnect()

	// Check if we got any events
	capturedLog := tuiModel.GetCapturedLog()
	for _, logEntry := range capturedLog {
		t.Log(logEntry)
	}
}
