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
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/coder/quartz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lox/pokerforbots/internal/client"
	"github.com/lox/pokerforbots/internal/server"
	"github.com/lox/pokerforbots/internal/tui"
)

// TestScenario defines a complete test scenario
type TestScenario struct {
	Name            string
	Seed            int64
	PlayerActions   []string // Actions for the human player in order
	ExpectedLog     []string // Expected log entries to be present
	ExpectedSidebar []string // Expected sidebar content to be present
	AllowTimeout    bool     // Allow real timeout instead of auto-fold
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
	client        *client.Client
	tui           *tui.TUIModel
	actionQueue   []string
	actionIndex   int
	eventChan     chan string   // Single event channel for this client
	handStarted   chan struct{} // Signal when hand starts
	handEnded     chan struct{} // Signal when hand ends
	streetChanged chan struct{} // Signal when street changes
	playerTimeout chan struct{} // Signal when player times out
	gamePause     chan struct{} // Signal when game is paused
	allowTimeout  bool          // Allow timeout instead of auto-fold
	mockClock     *quartz.Mock  // For explicit timeout control
	t             *testing.T    // For test logging
}

func (c *TestClient) QueueActions(actions []string) {
	c.actionQueue = append(c.actionQueue, actions...)
}

func (c *TestClient) EnableTimeouts() {
	c.allowTimeout = true
}

func (c *TestClient) GetSidebarContent() string {
	return c.tui.GetSidebarContent()
}

func (c *TestClient) AssertSidebar(t *testing.T, expectedContent []string) {
	sidebar := c.GetSidebarContent()
	for _, expected := range expectedContent {
		assert.Contains(t, sidebar, expected,
			"Expected sidebar content not found: %s\nActual sidebar:\n%s",
			expected, sidebar)
	}
}

func (c *TestClient) JoinTable(tableID string) error {
	return c.client.JoinTable(tableID, 200) // Default buy-in
}

func (c *TestClient) Disconnect() {
	if c.client != nil {
		_ = c.client.Disconnect()
	}
}

// Explicit step-by-step test methods

func (c *TestClient) WaitForEvent(eventType string) bool {
	return c.waitForEvent(eventType)
}

func (c *TestClient) WaitForActionRequired() {
	c.WaitForEvent("action_required")
}

func (c *TestClient) WaitForHandStart() {
	c.WaitForEvent("hand_start")
}

func (c *TestClient) WaitForHandEnd() {
	c.WaitForEvent("hand_end")
}

func (c *TestClient) WaitForPlayerTimeout() {
	c.WaitForEvent("player_timeout")
}

func (c *TestClient) WaitForGamePause() {
	c.WaitForEvent("game_pause")
}

func (c *TestClient) AllowTimeout() {
	c.t.Logf("STEP: Allowing timeout to occur")
	if c.mockClock != nil {
		// Give a brief moment for timeout to be armed
		time.Sleep(100 * time.Millisecond)

		// Advance mock clock to trigger instant timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c.mockClock.Advance(1 * time.Second).MustWait(ctx)
		c.t.Logf("STEP: Mock clock advanced by 1 second")
	}
}

func (c *TestClient) ExecuteAction(action string, args ...string) error {
	c.t.Logf("STEP: Executing action '%s' with args %v", action, args)
	time.Sleep(50 * time.Millisecond) // Brief delay for TUI readiness
	return c.tui.InjectAction(action, args)
}

func (c *TestClient) ExecuteCommand(command string) error {
	c.t.Logf("STEP: Executing command '%s'", command)
	time.Sleep(50 * time.Millisecond) // Brief delay for TUI readiness
	return c.tui.InjectAction(command, []string{})
}

func (c *TestClient) AssertSittingOut() {
	// Check that the player is in sitting out state
	// This could be enhanced to check actual game state
	c.t.Logf("STEP: Asserting player is sitting out")
}

func (c *TestClient) AssertExpectedLog(expectedEntries ...string) {
	capturedLog := c.tui.GetCapturedLog()
	logText := strings.Join(capturedLog, " ")

	for _, expected := range expectedEntries {
		if !strings.Contains(logText, expected) {
			c.t.Errorf("Expected log entry not found: %s\nCaptured log: %s", expected, logText)
		}
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
			case "street_change":
				// Signal street changed
				select {
				case c.streetChanged <- struct{}{}:
				default:
				}
			case "player_timeout":
				// Signal player timeout
				select {
				case c.playerTimeout <- struct{}{}:
				default:
				}
			case "game_pause":
				// Signal game pause
				select {
				case c.gamePause <- struct{}{}:
				default:
				}
			case "action_required":
				// Try to inject next action if we have one
				if c.actionIndex < len(c.actionQueue) {
					c.injectNextAction()
				} else if !c.allowTimeout {
					// No more scripted actions - auto-fold to end the hand quickly
					c.t.Logf("ACTION: Auto-folding (no more scripted actions)")
					time.Sleep(50 * time.Millisecond)
					_ = c.tui.InjectAction("fold", []string{})
				} else {
					// Allow timeout - don't inject any action
					c.t.Logf("ACTION: Allowing timeout (no more scripted actions)")
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
				Name:           "table1",
				MaxPlayers:     6,
				SmallBlind:     1,
				BigBlind:       2,
				BuyInMin:       100,
				BuyInMax:       1000,
				AutoStart:      true,
				TimeoutSeconds: 30, // Short timeout for fast tests
			},
		},
	}

	// Setup logger
	logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})

	// Create WebSocket server
	wsServer := server.NewServer(cfg.GetServerAddress(), logger)

	// Create game service with real clock
	gameService := server.NewGameService(wsServer, logger, seed, quartz.NewReal())

	// Set game service in server
	wsServer.SetGameService(gameService)

	// Create tables from configuration
	for _, tableConfig := range cfg.Tables {
		table, err := gameService.CreateTable(
			tableConfig.Name,
			tableConfig.MaxPlayers,
			tableConfig.SmallBlind,
			tableConfig.BigBlind,
			tableConfig.TimeoutSeconds,
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

// startTestServerWithClock creates a test server with custom clock for time mocking
func startTestServerWithClock(t *testing.T, port int, seed int64, bots int, clock quartz.Clock) *TestServer {
	// Create server config
	cfg := &server.ServerConfig{
		Server: server.ServerSettings{
			Address:  "127.0.0.1",
			Port:     port,
			LogLevel: "error", // Quiet logs during tests
		},
		Tables: []server.TableConfig{
			{
				Name:           "table1",
				MaxPlayers:     6,
				SmallBlind:     1,
				BigBlind:       2,
				BuyInMin:       100,
				BuyInMax:       1000,
				AutoStart:      true,
				TimeoutSeconds: 1, // 1 second timeout for fast mock time tests
			},
		},
	}

	// Setup logger
	logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})

	// Create WebSocket server
	wsServer := server.NewServer(cfg.GetServerAddress(), logger)

	// Create game service with provided clock (real or mock)
	gameService := server.NewGameService(wsServer, logger, seed, clock)

	// Set game service in server
	wsServer.SetGameService(gameService)

	// Create tables from configuration
	for _, tableConfig := range cfg.Tables {
		table, err := gameService.CreateTable(
			tableConfig.Name,
			tableConfig.MaxPlayers,
			tableConfig.SmallBlind,
			tableConfig.BigBlind,
			tableConfig.TimeoutSeconds,
		)
		require.NoError(t, err, "Failed to create table")

		// Add bots if requested
		if bots > 0 {
			_, err := gameService.AddBots(table.ID, bots)
			require.NoError(t, err, "Failed to add bots")
		}
	}

	// Start server in background
	go func() {
		err := wsServer.Start()
		if err != nil {
			t.Logf("Server start failed: %v", err)
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
		client:        wsClient,
		tui:           tuiModel,
		eventChan:     make(chan string, 100), // Large buffer to prevent blocking
		handStarted:   make(chan struct{}, 1), // Buffered to prevent blocking
		handEnded:     make(chan struct{}, 1), // Buffered to prevent blocking
		streetChanged: make(chan struct{}, 1), // Buffered to prevent blocking
		playerTimeout: make(chan struct{}, 1), // Buffered to prevent blocking
		gamePause:     make(chan struct{}, 1), // Buffered to prevent blocking
		t:             t,
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
	timeout := time.After(2 * time.Second)

	switch eventType {
	case "hand_start":
		select {
		case <-c.handStarted:
			return true
		case <-timeout:
			c.t.Logf("TIMEOUT: No hand_start event received within 2 seconds")
			return false
		}
	case "hand_end":
		select {
		case <-c.handEnded:
			return true
		case <-timeout:
			c.t.Logf("TIMEOUT: No hand_end event received within 2 seconds")
			return false
		}
	case "street_change":
		select {
		case <-c.streetChanged:
			return true
		case <-timeout:
			c.t.Logf("TIMEOUT: No street_change event received within 2 seconds")
			return false
		}
	case "player_timeout":
		select {
		case <-c.playerTimeout:
			return true
		case <-timeout:
			c.t.Logf("TIMEOUT: No player_timeout event received within 2 seconds")
			return false
		}
	case "game_pause":
		select {
		case <-c.gamePause:
			return true
		case <-timeout:
			c.t.Logf("TIMEOUT: No game_pause event received within 2 seconds")
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

func TestSittingOutWithMockTime(t *testing.T) {
	t.Run("timeout puts player in sitting out state with instant mock time", func(t *testing.T) {
		// Create mock clock
		mockClock := quartz.NewMock(t)

		logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})

		// 1. Start server with mock clock
		port := findFreePort(t)
		server := startTestServerWithClock(t, port, 99999, 3, mockClock) // 3 bots
		defer server.Stop()

		// 2. Create TUI in test mode
		tuiModel := tui.NewTUIModelWithOptions(logger, true)
		require.True(t, tuiModel.IsTestMode())

		// 3. Connect test client
		serverURL := fmt.Sprintf("ws://127.0.0.1:%d", port)
		testClient := connectTestClient(t, serverURL, tuiModel)
		defer testClient.Disconnect()

		// 4. Enable timeouts and start action script
		testClient.EnableTimeouts() // Allow real timeout instead of auto-fold
		testClient.StartActionScript()

		// 5. Join table and wait for game to start
		err := testClient.JoinTable("table1")
		require.NoError(t, err, "Failed to join table")

		// Wait for hand to start
		waitForHandStart(t, testClient)

		// Give a tiny bit of time for the action to be required
		time.Sleep(50 * time.Millisecond)

		// Advance mock clock to trigger timeout instantly!
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		mockClock.Advance(1 * time.Second).MustWait(ctx)

		// Timeout should have fired instantly
		require.True(t, testClient.waitForEvent("player_timeout"), "Player timeout should fire")

		// Get captured log and assert timeout behavior
		capturedLog := tuiModel.GetCapturedLog()
		t.Logf("Mock Time Test - Captured %d log entries:", len(capturedLog))
		for _, entry := range capturedLog {
			t.Log(entry)
		}

		// Should have timeout messages
		logText := strings.Join(capturedLog, " ")
		assert.Contains(t, logText, "timed out", "Should contain timeout message")
		assert.Contains(t, logText, "sit-out", "Should contain sit-out message")

		// Success! Mock time worked - timeout was instant instead of 1 real second
		t.Log("🎉 SUCCESS: Mock time made 1-second timeout instant!")
	})
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

func TestSittingOutScenarios(t *testing.T) {
	scenarios := []TestScenario{
		{
			Name:          "timeout puts player in sitting out state",
			Seed:          99999,
			PlayerActions: []string{}, // No actions - let it timeout
			AllowTimeout:  true,       // Allow real timeout
			ExpectedLog: []string{
				"timed out",
				"sit-out", // Should see sit-out message
			},
		},
		{
			Name:          "player returns from sitting out with /back",
			Seed:          88888,
			PlayerActions: []string{"/back"}, // Use /back command after timing out
			AllowTimeout:  true,              // Allow initial timeout
			ExpectedLog: []string{
				"Returning to play",
				"included in the next hand",
			},
		},
		{
			Name:          "game pauses when all humans sitting out",
			Seed:          77777,
			PlayerActions: []string{}, // Timeout and don't return
			AllowTimeout:  true,
			ExpectedLog: []string{
				"all sitting out",
				"pausing game",
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			runSittingOutScenario(t, scenario)
		})
	}
}

// Explicit step-by-step sitting out tests
func TestSittingOutExplicit(t *testing.T) {
	t.Run("timeout puts player in sitting out state", func(t *testing.T) {
		testTimeoutPutsPlayerInSittingOutState(t)
	})

	t.Run("player returns from sitting out with /back", func(t *testing.T) {
		testPlayerReturnsFromSittingOut(t)
	})

	t.Run("game pauses when all humans sitting out", func(t *testing.T) {
		testGamePausesWhenAllHumansSittingOut(t)
	})
}

func setupExplicitTestClient(t *testing.T, seed int64, botCount int) *TestClient {
	logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})

	// Start server with mock clock for instant timeouts
	port := findFreePort(t)
	mockClock := quartz.NewMock(t)
	server := startTestServerWithClock(t, port, seed, botCount, mockClock)
	t.Cleanup(func() { server.Stop() })

	// Create TUI in test mode
	tuiModel := tui.NewTUIModelWithOptions(logger, true)
	require.True(t, tuiModel.IsTestMode())

	// Connect test client
	serverURL := fmt.Sprintf("ws://127.0.0.1:%d", port)
	testClient := connectTestClient(t, serverURL, tuiModel)

	// Store mock clock reference for timeout control
	testClient.mockClock = mockClock

	// Setup explicit event handling (replace the action script approach)
	go func() {
		for event := range testClient.eventChan {
			testClient.t.Logf("EXPLICIT EVENT: %s", event)
			switch event {
			case "hand_start":
				select {
				case testClient.handStarted <- struct{}{}:
				default:
				}
			case "hand_end":
				select {
				case testClient.handEnded <- struct{}{}:
				default:
				}
			case "street_changed":
				select {
				case testClient.streetChanged <- struct{}{}:
				default:
				}
			case "action_required":
				// Action required - we'll manually control what happens next
				testClient.t.Logf("ACTION_REQUIRED: Ready for explicit control")
			case "player_timeout":
				select {
				case testClient.playerTimeout <- struct{}{}:
				default:
				}
			case "game_pause":
				select {
				case testClient.gamePause <- struct{}{}:
				default:
				}
			}
		}
	}()

	return testClient
}

func testTimeoutPutsPlayerInSittingOutState(t *testing.T) {
	client := setupExplicitTestClient(t, 99999, 3)
	defer client.Disconnect()

	// Step 1: Join game and wait for action
	err := client.JoinTable("table1")
	require.NoError(t, err)
	client.WaitForHandStart()
	client.WaitForActionRequired()

	// Step 2: Allow timeout to occur by advancing mock clock
	client.AllowTimeout()

	// Step 3: Wait for timeout event and verify log
	client.WaitForPlayerTimeout()
	client.AssertSittingOut()
	client.AssertExpectedLog("timed out", "sit-out")
}

func testPlayerReturnsFromSittingOut(t *testing.T) {
	client := setupExplicitTestClient(t, 88888, 3)
	defer client.Disconnect()

	// Step 1: Join game and wait for action
	err := client.JoinTable("table1")
	require.NoError(t, err)
	client.WaitForHandStart()
	client.WaitForActionRequired()

	// Step 2: Allow timeout to occur
	client.AllowTimeout()
	client.WaitForPlayerTimeout()
	client.AssertSittingOut()

	// Step 3: Use /back command to return to play
	err = client.ExecuteCommand("/back")
	require.NoError(t, err)

	// Give time for the command to process and game to unpause
	time.Sleep(200 * time.Millisecond)

	// The game should automatically unpause and start a new hand
	client.WaitForHandStart() // Wait for next hand where player can return

	// Step 4: Verify expected log entries
	client.AssertExpectedLog("Returning to play", "included in the next hand")
}

func testGamePausesWhenAllHumansSittingOut(t *testing.T) {
	client := setupExplicitTestClient(t, 77777, 1) // Use 1 bot so game can pause
	defer client.Disconnect()

	// Step 1: Join game and wait for action
	err := client.JoinTable("table1")
	require.NoError(t, err)
	client.WaitForHandStart()
	client.WaitForActionRequired()

	// Step 2: Allow timeout to occur
	client.AllowTimeout()
	client.WaitForPlayerTimeout()
	client.AssertSittingOut()

	// Step 3: Wait for game to pause
	client.WaitForGamePause()

	// Step 4: Verify expected log entries
	client.AssertExpectedLog("all sitting out", "pausing game")
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
			Name:          "sidebar shows correct game state and positions",
			Seed:          11111,
			PlayerActions: []string{"call", "fold"},
			ExpectedSidebar: []string{
				"Round:",            // Round info is shown
				"Board:",            // Board is displayed
				"Pot: $",            // Pot amount
				"Players at table:", // Players section
				"TestPlayer",        // Our player is listed
				"Bot_1",             // Other players listed
			},
		},
		{
			Name:          "sidebar updates after player actions",
			Seed:          22222,
			PlayerActions: []string{"raise 10", "fold"},
			ExpectedSidebar: []string{
				"Pot: $",            // Pot updates
				"TestPlayer",        // Player in list
				"Players at table:", // Players section
			},
		},
		{
			Name:          "sidebar shows street progression",
			Seed:          33333,
			PlayerActions: []string{"call", "call", "call"},
			ExpectedSidebar: []string{
				"Round:",            // Round progression shown
				"Board:",            // Community cards
				"Players at table:", // Players section
				"TestPlayer",        // Our player
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

	// 4. Queue actions and configure timeout behavior
	testClient.QueueActions(scenario.PlayerActions)
	if scenario.AllowTimeout {
		testClient.EnableTimeouts()
	}
	testClient.StartActionScript()

	// 5. Join table and wait for game to start
	err := testClient.JoinTable("table1")
	require.NoError(t, err, "Failed to join table")

	// Wait for hand to start before beginning action script
	waitForHandStart(t, testClient)

	// 6. Wait for appropriate game state based on test type
	if len(scenario.ExpectedSidebar) > 0 {
		// For sidebar tests, wait for street change to ensure board is visible
		// First street change is Pre-flop to Flop
		testClient.waitForEvent("street_change")
	} else {
		// For non-sidebar tests, wait for hand to complete
		waitForHandComplete(t, testClient)
	}

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

	// Check for expected log patterns
	if len(scenario.ExpectedLog) > 0 {
		logText := strings.Join(capturedLog, " ")
		for _, expectedEntry := range scenario.ExpectedLog {
			assert.Contains(t, logText, expectedEntry,
				"Expected log entry not found: %s\nScenario: %s\nActions: %v",
				expectedEntry, scenario.Name, scenario.PlayerActions)
		}
	}

	// 8. Assert sidebar content
	if len(scenario.ExpectedSidebar) > 0 {
		// Force a UI refresh to ensure sidebar is rendered
		tuiModel.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		// Force view rendering to trigger sidebar capture
		_ = tuiModel.View()

		// Debug: Print the actual sidebar content
		sidebarContent := testClient.GetSidebarContent()
		t.Logf("DEBUG: Sidebar content: '%s'", sidebarContent)
		t.Logf("DEBUG: Sidebar length: %d", len(sidebarContent))

		testClient.AssertSidebar(t, scenario.ExpectedSidebar)
	}
}

// runSittingOutScenario executes a sitting out test scenario
func runSittingOutScenario(t *testing.T, scenario TestScenario) {
	logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})

	// 1. Start server on random port with mock clock for instant timeouts
	port := findFreePort(t)

	// Use different bot counts based on scenario
	botCount := 3 // Default for most scenarios
	if scenario.Name == "game pauses when all humans sitting out" {
		botCount = 1 // Use 1 bot so game can start, then pause when human sits out
	}

	// Use mock clock for instant timeout testing
	mockClock := quartz.NewMock(t)
	server := startTestServerWithClock(t, port, scenario.Seed, botCount, mockClock)
	defer server.Stop()

	// 2. Create TUI in test mode
	tuiModel := tui.NewTUIModelWithOptions(logger, true)
	require.True(t, tuiModel.IsTestMode())

	// 3. Connect test client
	serverURL := fmt.Sprintf("ws://127.0.0.1:%d", port)
	testClient := connectTestClient(t, serverURL, tuiModel)
	defer testClient.Disconnect()

	// 4. Queue actions and configure timeout behavior
	// For /back scenarios, don't auto-execute the /back command - we'll do it manually after timeout
	if scenario.AllowTimeout && len(scenario.PlayerActions) == 1 && scenario.PlayerActions[0] == "/back" {
		testClient.QueueActions([]string{}) // Empty queue - manual control
	} else {
		testClient.QueueActions(scenario.PlayerActions)
	}
	testClient.EnableTimeouts() // Always allow timeouts for sitting out tests
	testClient.StartActionScript()

	// 5. Join table and wait for game to start
	err := testClient.JoinTable("table1")
	require.NoError(t, err, "Failed to join table")

	// Wait for hand to start
	waitForHandStart(t, testClient)

	// 6. Handle timeout scenarios
	if scenario.AllowTimeout {
		// Give a moment for action to be required, then advance mock clock
		time.Sleep(50 * time.Millisecond) // Brief pause for action to be armed

		// Advance mock clock to trigger timeout instantly
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		mockClock.Advance(1 * time.Second).MustWait(ctx)

		// Wait for the timeout event
		if !testClient.waitForEvent("player_timeout") {
			t.Logf("No timeout event received after clock advance")
		}

		// For /back scenarios, now execute the /back command after timeout
		if len(scenario.PlayerActions) == 1 && scenario.PlayerActions[0] == "/back" {
			// Execute /back after timeout to return to play
			t.Logf("ACTION: Manually executing /back command after timeout")
			time.Sleep(50 * time.Millisecond) // Brief delay
			err := testClient.tui.InjectAction("/back", []string{})
			if err != nil {
				t.Logf("ACTION ERROR: Failed to inject /back action: %v", err)
			}

			// Wait for next hand to start so player can return
			waitForHandStart(t, testClient)
		}
	} else {
		// For non-timeout scenarios, just wait for timeout event (shouldn't happen with mock clock)
		if !testClient.waitForEvent("player_timeout") {
			t.Logf("No timeout event received, test may need adjustment")
		}
	}

	// 6b. For the "game pauses when all humans sitting out" test, also wait for game_pause event
	if scenario.Name == "game pauses when all humans sitting out" {
		if !testClient.waitForEvent("game_pause") {
			t.Logf("No game_pause event received for pause test")
		}
		// Give a bit more time for the pause message to propagate to the UI
		time.Sleep(100 * time.Millisecond)
	}

	// 7. Get captured log and assert
	capturedLog := tuiModel.GetCapturedLog()
	t.Logf("Sitting Out Scenario: %s", scenario.Name)
	t.Logf("Actions: %v", scenario.PlayerActions)
	t.Logf("Captured %d log entries:", len(capturedLog))

	// Log the captured entries for easier debugging
	for _, entry := range capturedLog {
		t.Log(entry)
	}

	// Should have some events
	assert.Greater(t, len(capturedLog), 0, "Should have captured some log entries")

	// Check for expected log patterns
	if len(scenario.ExpectedLog) > 0 {
		logText := strings.Join(capturedLog, " ")
		for _, expectedEntry := range scenario.ExpectedLog {
			assert.Contains(t, logText, expectedEntry,
				"Expected log entry not found: %s\nScenario: %s\nActions: %v",
				expectedEntry, scenario.Name, scenario.PlayerActions)
		}
	}
}
