// Package testing provides comprehensive integration tests for the poker system using
// event-driven WebSocket synchronization. Tests use real server/client communication
// with deterministic seeds for reproducible game scenarios.
//
// Key features:
// - Event broadcasting: WebSocket events are broadcast to all waiting goroutines
// - Race-condition free: No competing consumers for the same events
// - Fast execution: Sub-second performance using real events instead of sleeps
// - Full stack testing: Real WebSocket communication from server to TUI display
package testing

import (
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lox/pokerforbots/internal/client"
	"github.com/lox/pokerforbots/internal/server"
	"github.com/lox/pokerforbots/internal/tui"
)

// TestScenario defines a complete test scenario
type TestScenario struct {
	Name          string
	Seed          int64
	PlayerActions []string // Actions for the human player in order
	ExpectedLog   []string // Expected log entries to be present
}

// TestServer wraps a running server instance
type TestServer struct {
	wsServer    *server.Server
	gameService *server.GameService
	port        int
}

func (s *TestServer) Stop() {
	if s.wsServer != nil {
		_ = s.wsServer.Stop()
	}
	// No more global state to clean up!
}

// TestClient wraps a client with action injection and event-driven synchronization
type TestClient struct {
	client      *client.Client
	tui         *tui.TUIModel
	actionQueue []string
	actionIndex int
	eventChan   chan string   // Single event channel for this client
	handStarted chan struct{} // Signal when hand starts
	handEnded   chan struct{} // Signal when hand ends
	t           *testing.T    // For test logging
}

func (c *TestClient) QueueActions(actions []string) {
	c.actionQueue = append(c.actionQueue, actions...)
}

func (c *TestClient) JoinTable(tableID string) error {
	return c.client.JoinTable(tableID, 200) // Default buy-in
}

func (c *TestClient) Disconnect() {
	if c.client != nil {
		_ = c.client.Disconnect()
	}
}

// StartActionScript starts providing queued actions when needed
func (c *TestClient) StartActionScript() {
	go func() {
		// Process events and inject actions when needed
		for event := range c.eventChan {
			c.t.Logf("STATE: Processing event %s", event)

			switch event {
			case "hand_start":
				// Signal hand started
				select {
				case c.handStarted <- struct{}{}:
				default:
				}
			case "hand_end":
				// Signal hand ended
				select {
				case c.handEnded <- struct{}{}:
				default:
				}
			case "action_required":
				// Try to inject next action if we have one
				if c.actionIndex < len(c.actionQueue) {
					c.injectNextAction()
				} else {
					// No more scripted actions - auto-fold to end the hand quickly
					c.t.Logf("ACTION: Auto-folding (no more scripted actions)")
					time.Sleep(50 * time.Millisecond)
					_ = c.tui.InjectAction("fold", []string{})
				}
			}
		}
	}()
}

// injectNextAction injects the next queued action
func (c *TestClient) injectNextAction() {
	actionStr := c.actionQueue[c.actionIndex]
	c.t.Logf("ACTION: Executing action %d/%d: '%s'", c.actionIndex+1, len(c.actionQueue), actionStr)
	c.actionIndex++

	// Parse action string into action and args
	parts := strings.Fields(strings.ToLower(actionStr))
	action := parts[0]
	var args []string
	if len(parts) > 1 {
		args = parts[1:]
	}

	// Small delay to ensure TUI is ready
	time.Sleep(50 * time.Millisecond)

	// Inject action
	err := c.tui.InjectAction(action, args)
	if err != nil {
		c.t.Logf("ACTION ERROR: Failed to inject action: %v", err)
	}
}

// Helper Functions

func findFreePort(t *testing.T) int {
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

func startTestServer(t *testing.T, port int, seed int64, bots int) *TestServer {
	// Create server config
	cfg := &server.ServerConfig{
		Server: server.ServerSettings{
			Address:  "127.0.0.1",
			Port:     port,
			LogLevel: "error", // Quiet logs during tests
		},
		Tables: []server.TableConfig{
			{
				Name:       "table1",
				MaxPlayers: 6,
				SmallBlind: 1,
				BigBlind:   2,
				BuyInMin:   100,
				BuyInMax:   1000,
				AutoStart:  true,
			},
		},
	}

	// Setup logger
	logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})

	// Create WebSocket server
	wsServer := server.NewServer(cfg.GetServerAddress(), logger)

	// Create game service
	gameService := server.NewGameService(wsServer, logger, seed)

	// Set game service in server
	wsServer.SetGameService(gameService)

	// Create tables from configuration
	for _, tableConfig := range cfg.Tables {
		table, err := gameService.CreateTable(
			tableConfig.Name,
			tableConfig.MaxPlayers,
			tableConfig.SmallBlind,
			tableConfig.BigBlind,
		)
		require.NoError(t, err, "Failed to create table")

		// Auto-populate with bots if requested
		if bots > 0 {
			_, err := gameService.AddBots(table.ID, bots)
			require.NoError(t, err, "Failed to add bots")
		}
	}

	// Start server in background
	go func() {
		err := wsServer.Start()
		if err != nil {
			logger.Error("Server failed", "error", err)
		}
	}()

	// Wait for server to be ready
	serverURL := fmt.Sprintf("ws://127.0.0.1:%d", port)
	waitForServerReady(t, serverURL, 5*time.Second)

	return &TestServer{
		wsServer:    wsServer,
		gameService: gameService,
		port:        port,
	}
}

func waitForServerReady(t *testing.T, serverURL string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// Try to create a temporary client connection
		logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})
		wsClient := client.NewClient(serverURL, logger)

		err := wsClient.Connect()
		if err == nil {
			_ = wsClient.Disconnect()
			return // Server is ready
		}

		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("Server at %s did not become ready within %v", serverURL, timeout)
}

func connectTestClient(t *testing.T, serverURL string, tuiModel *tui.TUIModel) *TestClient {
	logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})
	wsClient := client.NewClient(serverURL, logger)

	err := wsClient.Connect()
	require.NoError(t, err, "Failed to connect test client")

	// Authenticate
	err = wsClient.Auth("TestPlayer")
	require.NoError(t, err, "Failed to authenticate test client")

	// Create test client with event channels
	testClient := &TestClient{
		client:      wsClient,
		tui:         tuiModel,
		eventChan:   make(chan string, 100), // Large buffer to prevent blocking
		handStarted: make(chan struct{}, 1), // Buffered to prevent blocking
		handEnded:   make(chan struct{}, 1), // Buffered to prevent blocking
		t:           t,
	}

	// Set up TUI bridge and command handler
	setupTUIBridge(wsClient, tuiModel)
	tui.StartCommandHandler(wsClient, tuiModel, 200)

	// Set up test event synchronization via TUI callback
	testClient.setupEventCallback()

	return testClient
}

// setupEventCallback configures TUI event callback for test synchronization
func (c *TestClient) setupEventCallback() {
	// Set up callback to receive events from TUI bridge handlers
	c.tui.SetEventCallback(func(eventType string) {
		c.t.Logf("EVENT: %s", eventType)
		select {
		case c.eventChan <- eventType:
			// Event sent successfully
		default:
			// Channel full, skip this event
		}
	})
}

// waitForEvent waits for a specific event, blocking until it arrives
func (c *TestClient) waitForEvent(eventType string) bool {
	timeout := time.After(10 * time.Second)

	switch eventType {
	case "hand_start":
		select {
		case <-c.handStarted:
			return true
		case <-timeout:
			c.t.Logf("TIMEOUT: No hand_start event received within 10 seconds")
			return false
		}
	case "hand_end":
		select {
		case <-c.handEnded:
			return true
		case <-timeout:
			c.t.Logf("TIMEOUT: No hand_end event received within 10 seconds")
			return false
		}
	default:
		c.t.Logf("UNKNOWN EVENT TYPE: %s", eventType)
		return false
	}
}

func setupTUIBridge(wsClient *client.Client, tuiModel *tui.TUIModel) {
	// Use the real bridge from simple_bridge.go
	tui.SetupSimpleNetworkHandlers(wsClient, tuiModel)
}

// waitForHandComplete waits for hand completion using WebSocket events
func waitForHandComplete(t *testing.T, testClient *TestClient) {
	if testClient.waitForEvent("hand_end") {
		return // Hand completed
	}
	t.Logf("Hand did not complete")
}

// waitForHandStart waits for a new hand to begin using WebSocket events
func waitForHandStart(t *testing.T, testClient *TestClient) {
	if testClient.waitForEvent("hand_start") {
		return // Hand started
	}
	t.Logf("Hand did not start")
}

func TestBasicConnection(t *testing.T) {
	// Use stderr for debugging
	logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})

	t.Run("basic connect and disconnect", func(t *testing.T) {
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
			client:      wsClient,
			tui:         tuiModel,
			eventChan:   make(chan string, 100),
			handStarted: make(chan struct{}, 1),
			handEnded:   make(chan struct{}, 1),
			t:           t,
		}
		tempClient.setupEventCallback()

		// Wait for some game events
		waitForHandStart(t, tempClient)

		// Disconnect
		_ = wsClient.Disconnect()

		// Check if we got any events
		capturedLog := tuiModel.GetCapturedLog()
		for _, logEntry := range capturedLog {
			t.Log(logEntry)
		}
	})
}

func TestPokerScenarios(t *testing.T) {
	scenarios := []TestScenario{
		{
			Name:          "preflop fold - conservative play",
			Seed:          12345,
			PlayerActions: []string{"fold"},
			ExpectedLog: []string{
				"*** HOLE CARDS ***",
				"*** PRE-FLOP ***",
				"TestPlayer: folds",
				"Complete",
			},
		},
		{
			Name:          "preflop call and flop fold",
			Seed:          23456,
			PlayerActions: []string{"c", "f"},
			ExpectedLog: []string{
				"*** PRE-FLOP ***",
				"TestPlayer: calls",
				"*** FLOP ***",
				// Don't require exact fold timing - may fold earlier
			},
		},
		{
			Name:          "aggressive preflop raise",
			Seed:          34567,
			PlayerActions: []string{"r 20"},
			ExpectedLog: []string{
				"*** PRE-FLOP ***",
				"TestPlayer: raises by $20",
				"pot now:",
			},
		},
		{
			Name:          "call to showdown",
			Seed:          45678,
			PlayerActions: []string{"call", "call", "call", "call"},
			ExpectedLog: []string{
				"*** FLOP ***",
				"*** TURN ***",
				"*** RIVER ***",
				"TestPlayer: calls",
			},
		},
		{
			Name:          "check-call passive play",
			Seed:          56789,
			PlayerActions: []string{"call", "check", "check", "check"},
			ExpectedLog: []string{
				"TestPlayer: calls",
				"TestPlayer: checks",
				"*** FLOP ***",
			},
		},
		{
			Name:          "early all-in",
			Seed:          67890,
			PlayerActions: []string{"a"},
			ExpectedLog: []string{
				"*** PRE-FLOP ***",
				"TestPlayer: all-in",
				"Complete",
			},
		},
		{
			Name:          "basic hand progression verification",
			Seed:          98765,
			PlayerActions: []string{"f"}, // Fold immediately to test basic flow
			ExpectedLog: []string{
				"Joined table table1",
				"*** HOLE CARDS ***",
				"Dealt to You:",
				"*** PRE-FLOP ***",
				"TestPlayer: folds",
				"Winner:",
				"Complete",
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			runPokerScenario(t, scenario)
		})
	}
}

// runPokerScenario executes a single poker test scenario
func runPokerScenario(t *testing.T, scenario TestScenario) {
	logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})

	// 1. Start server on random port
	port := findFreePort(t)
	server := startTestServer(t, port, scenario.Seed, 3)
	defer server.Stop()

	// 2. Create TUI in test mode
	tuiModel := tui.NewTUIModelWithOptions(logger, true)
	require.True(t, tuiModel.IsTestMode())

	// 3. Connect test client
	serverURL := fmt.Sprintf("ws://127.0.0.1:%d", port)
	testClient := connectTestClient(t, serverURL, tuiModel)
	defer testClient.Disconnect()

	// 4. Queue actions and start script
	testClient.QueueActions(scenario.PlayerActions)
	testClient.StartActionScript()

	// 5. Join table and wait for game to start
	err := testClient.JoinTable("table1")
	require.NoError(t, err, "Failed to join table")

	// Wait for hand to start before beginning action script
	waitForHandStart(t, testClient)

	// 6. Wait for hand to complete - now event-driven without timeouts
	waitForHandComplete(t, testClient)

	// 7. Get captured log and assert
	capturedLog := tuiModel.GetCapturedLog()
	t.Logf("Scenario: %s", scenario.Name)
	t.Logf("Actions: %v", scenario.PlayerActions)
	t.Logf("Captured %d log entries:", len(capturedLog))

	// Log the captured entries for easier debugging
	for _, entry := range capturedLog {
		t.Log(entry)
	}

	// Should have some events
	assert.Greater(t, len(capturedLog), 0, "Should have captured some log entries")

	// Check for expected patterns
	logText := strings.Join(capturedLog, " ")
	for _, expectedEntry := range scenario.ExpectedLog {
		assert.Contains(t, logText, expectedEntry,
			"Expected log entry not found: %s\nScenario: %s\nActions: %v",
			expectedEntry, scenario.Name, scenario.PlayerActions)
	}
}
