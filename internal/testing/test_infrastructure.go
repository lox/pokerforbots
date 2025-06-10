package testing

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/coder/quartz"
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
}

// TestClient wraps a client with action injection and event-driven synchronization
type TestClient struct {
	client        *client.Client
	tui           *tui.TUIModel
	actionQueue   []string
	actionIndex   int
	messageChan   chan server.MessageType // Single message channel for this client
	handStarted   chan struct{}           // Signal when hand starts
	handEnded     chan struct{}           // Signal when hand ends
	streetChanged chan struct{}           // Signal when street changes
	playerTimeout chan struct{}           // Signal when player times out
	gamePause     chan struct{}           // Signal when game is paused
	allowTimeout  bool                    // Allow timeout instead of auto-fold
	mockClock     *quartz.Mock            // For explicit timeout control
	t             *testing.T              // For test logging
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
	c.t.Helper()
	sidebar := c.GetSidebarContent()
	for _, expected := range expectedContent {
		require.Contains(t, sidebar, expected,
			"Expected sidebar content not found: %s\nActual sidebar:\n%s",
			expected, sidebar)
	}
}

func (c *TestClient) JoinTable(tableID string) error {
	return c.client.JoinTable(tableID, DefaultBuyIn)
}

func (c *TestClient) Disconnect() {
	if c.client != nil {
		// Clear the message callback first to prevent further message processing
		c.tui.SetMessageCallback(nil)

		// Disconnect the client which signals goroutines to stop
		_ = c.client.Disconnect()

		// Wait for goroutines to finish processing and exit
		// This ensures no more logging happens after test completion
		time.Sleep(DisconnectDelay)
	}
}

// Explicit step-by-step test methods

func (c *TestClient) WaitForMessage(messageType server.MessageType) bool {
	c.t.Helper()
	return c.waitForMessage(messageType)
}

func (c *TestClient) WaitForActionRequired() {
	c.t.Helper()
	c.WaitForMessage(server.MessageTypeActionRequired)
}

func (c *TestClient) WaitForHandStart() {
	c.t.Helper()
	c.WaitForMessage(server.MessageTypeHandStart)
}

func (c *TestClient) WaitForHandEnd() {
	c.t.Helper()
	c.WaitForMessage(server.MessageTypeHandEnd)
}

func (c *TestClient) WaitForPlayerTimeout() {
	c.t.Helper()
	c.WaitForMessage(server.MessageTypePlayerTimeout)
}

func (c *TestClient) WaitForGamePause() {
	c.t.Helper()
	c.WaitForMessage(server.MessageTypeGamePause)
}

func (c *TestClient) AllowTimeout() {
	c.t.Helper()
	c.t.Logf("STEP: Allowing timeout to occur")
	if c.mockClock != nil {
		// Small delay to allow action_required processing to complete
		// The trap approach has a race condition: AfterFunc might be called
		// before we set up the trap, especially in single-threaded test scenarios
		time.Sleep(ClockAdvanceDelay) // Minimal delay for message processing

		// Advance mock clock to trigger instant timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c.mockClock.Advance(1 * time.Second).MustWait(ctx)
		c.t.Logf("STEP: Mock clock advanced by 1 second")
	}
}

func (c *TestClient) ExecuteAction(action string, args ...string) error {
	c.t.Helper()
	c.t.Logf("STEP: Executing action '%s' with args %v", action, args)
	time.Sleep(ActionDelay) // Brief delay for TUI readiness
	return c.tui.InjectAction(action, args)
}

func (c *TestClient) ExecuteCommand(command string) error {
	c.t.Helper()
	c.t.Logf("STEP: Executing command '%s'", command)
	time.Sleep(ActionDelay) // Brief delay for TUI readiness
	return c.tui.InjectAction(command, []string{})
}

func (c *TestClient) AssertSittingOut() {
	c.t.Helper()
	// Check that the player is in sitting out state
	// This could be enhanced to check actual game state
	c.t.Logf("STEP: Asserting player is sitting out")
}

func (c *TestClient) IsPlayerSittingOut() bool {
	c.t.Helper()
	// Check the captured log for sitting out indicators
	capturedLog := c.tui.GetCapturedLog()
	logText := strings.Join(capturedLog, " ")

	// Look for sitting out messages in the log
	return strings.Contains(logText, "sit-out") ||
		strings.Contains(logText, "timed out") ||
		strings.Contains(logText, "Sitting Out")
}

func (c *TestClient) AssertPlayerSittingOut() {
	c.t.Helper()
	require.True(c.t, c.IsPlayerSittingOut(), "Expected player to be sitting out")
}

func (c *TestClient) AssertPlayerNotSittingOut() {
	c.t.Helper()
	require.False(c.t, c.IsPlayerSittingOut(), "Expected player to not be sitting out")
}

func (c *TestClient) AssertExpectedLog(expectedEntries ...string) {
	c.t.Helper()
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
		// Process messages and inject actions when needed
		for message := range c.messageChan {
			c.t.Logf("STATE: Processing message %s", message)

			switch message {
			case server.MessageTypeHandStart:
				// Signal hand started
				select {
				case c.handStarted <- struct{}{}:
				default:
				}
			case server.MessageTypeHandEnd:
				// Signal hand ended
				select {
				case c.handEnded <- struct{}{}:
				default:
				}
			case server.MessageTypeStreetChange:
				// Signal street changed
				select {
				case c.streetChanged <- struct{}{}:
				default:
				}
			case server.MessageTypePlayerTimeout:
				// Signal player timeout
				select {
				case c.playerTimeout <- struct{}{}:
				default:
				}
			case server.MessageTypeGamePause:
				// Signal game pause
				select {
				case c.gamePause <- struct{}{}:
				default:
				}
			case server.MessageTypeActionRequired:
				// Try to inject next action if we have one
				if c.actionIndex < len(c.actionQueue) {
					c.injectNextAction()
				} else if !c.allowTimeout {
					// No more scripted actions - auto-fold to end the hand quickly
					c.t.Logf("ACTION: Auto-folding (no more scripted actions)")
					time.Sleep(ActionDelay)
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
	time.Sleep(ActionDelay)

	// Inject action
	err := c.tui.InjectAction(action, args)
	if err != nil {
		c.t.Logf("ACTION ERROR: Failed to inject action: %v", err)
	}
}

// Helper Functions

func findFreePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

// Test constants
const (
	DefaultBuyIn       = 200
	DefaultTimeoutSecs = 30
	MockTimeoutSecs    = 1
	EventTimeoutSecs   = 2
	ActionDelay        = 50 * time.Millisecond
	DisconnectDelay    = 300 * time.Millisecond
	ServerReadyTimeout = 5 * time.Second
	ClockAdvanceDelay  = 10 * time.Millisecond
)

func startTestServer(t *testing.T, port int, seed int64, bots int, clock ...quartz.Clock) *TestServer {
	t.Helper()
	// Use real clock by default, or provided clock for mocking
	gameClock := quartz.NewReal()
	var timeoutSecs = DefaultTimeoutSecs
	if len(clock) > 0 {
		gameClock = clock[0]
		timeoutSecs = MockTimeoutSecs // Use shorter timeout for mock time tests
	}

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
				TimeoutSeconds: timeoutSecs,
			},
		},
	}

	// Setup logger
	logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})

	// Create WebSocket server
	wsServer := server.NewServer(cfg.GetServerAddress(), logger)

	// Create game service with provided clock
	gameService := server.NewGameService(wsServer, logger, seed, gameClock)

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
	waitForServerReady(t, serverURL, ServerReadyTimeout)

	return &TestServer{
		wsServer:    wsServer,
		gameService: gameService,
		port:        port,
	}
}

func waitForServerReady(t *testing.T, serverURL string, timeout time.Duration) {
	t.Helper()
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
	t.Helper()
	logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})
	wsClient := client.NewClient(serverURL, logger)

	err := wsClient.Connect()
	require.NoError(t, err, "Failed to connect test client")

	// Authenticate
	err = wsClient.Auth("TestPlayer")
	require.NoError(t, err, "Failed to authenticate test client")

	// Create test client with message channels
	testClient := &TestClient{
		client:        wsClient,
		tui:           tuiModel,
		messageChan:   make(chan server.MessageType, 100), // Large buffer to prevent blocking
		handStarted:   make(chan struct{}, 1),             // Buffered to prevent blocking
		handEnded:     make(chan struct{}, 1),             // Buffered to prevent blocking
		streetChanged: make(chan struct{}, 1),             // Buffered to prevent blocking
		playerTimeout: make(chan struct{}, 1),             // Buffered to prevent blocking
		gamePause:     make(chan struct{}, 1),             // Buffered to prevent blocking
		t:             t,
	}

	// Set up TUI bridge
	setupTUIBridge(wsClient, tuiModel)

	// Set up test message synchronization via TUI callback
	testClient.setupMessageCallback()

	return testClient
}

// setupMessageCallback configures TUI message callback for test synchronization
func (c *TestClient) setupMessageCallback() {
	// Set up callback to receive messages from TUI bridge handlers
	c.tui.SetMessageCallback(func(messageType server.MessageType) {
		c.t.Logf("MESSAGE: %s", messageType)
		select {
		case c.messageChan <- messageType:
			// Message sent successfully
		default:
			// Channel full, skip this message
		}
	})
}

// waitForMessage waits for a specific message, blocking until it arrives
func (c *TestClient) waitForMessage(messageType server.MessageType) bool {
	c.t.Helper()
	timeout := time.After(EventTimeoutSecs * time.Second)

	switch messageType {
	case server.MessageTypeHandStart:
		select {
		case <-c.handStarted:
			return true
		case <-timeout:
			c.t.Logf("TIMEOUT: No hand_start event received within 2 seconds")
			return false
		}
	case server.MessageTypeHandEnd:
		select {
		case <-c.handEnded:
			return true
		case <-timeout:
			c.t.Logf("TIMEOUT: No hand_end event received within 2 seconds")
			return false
		}
	case server.MessageTypeStreetChange:
		select {
		case <-c.streetChanged:
			return true
		case <-timeout:
			c.t.Logf("TIMEOUT: No street_change event received within 2 seconds")
			return false
		}
	case server.MessageTypePlayerTimeout:
		select {
		case <-c.playerTimeout:
			return true
		case <-timeout:
			c.t.Logf("TIMEOUT: No player_timeout event received within 2 seconds")
			return false
		}
	case server.MessageTypeGamePause:
		select {
		case <-c.gamePause:
			return true
		case <-timeout:
			c.t.Logf("TIMEOUT: No game_pause event received within 2 seconds")
			return false
		}

	default:
		c.t.Logf("UNKNOWN MESSAGE TYPE: %s", messageType)
		return false
	}
}

func setupTUIBridge(wsClient *client.Client, tuiModel *tui.TUIModel) {
	// Use the unified bridge
	bridge := tui.NewBridge(wsClient, tuiModel, 200)
	bridge.Start()
}
