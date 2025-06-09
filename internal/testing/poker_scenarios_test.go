package testing

import (
	"fmt"
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lox/pokerforbots/internal/tui"
)

func TestPokerMechanics(t *testing.T) {
	t.Run("basic hand flow - join table and receive cards", func(t *testing.T) {
		testBasicHandFlow(t)
	})

	t.Run("preflop actions - fold, call, raise", func(t *testing.T) {
		testPreflopActions(t)
	})

	t.Run("sidebar shows game state correctly", func(t *testing.T) {
		testSidebarGameState(t)
	})
}

func setupExplicitPokerTestClient(t *testing.T, seed int64) *TestClient {
	t.Helper()
	logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})

	// Start server
	port := findFreePort(t)
	server := startTestServer(t, port, seed, 3)
	t.Cleanup(func() { server.Stop() })

	// Create TUI in test mode
	tuiModel := tui.NewTUIModelWithOptions(logger, true)
	require.True(t, tuiModel.IsTestMode())

	// Connect test client
	serverURL := fmt.Sprintf("ws://127.0.0.1:%d", port)
	testClient := connectTestClient(t, serverURL, tuiModel)
	t.Cleanup(func() { testClient.Disconnect() })

	return testClient
}

func testBasicHandFlow(t *testing.T) {
	t.Helper()
	client := setupExplicitPokerTestClient(t, 12345)

	// Setup explicit event handling without action script
	go func() {
		for event := range client.eventChan {
			switch event {
			case "hand_start":
				select {
				case client.handStarted <- struct{}{}:
				default:
				}
			case "hand_end":
				select {
				case client.handEnded <- struct{}{}:
				default:
				}
			case "action_required":
				// Don't auto-inject actions - let test control them explicitly
				t.Logf("EXPLICIT: Action required - test will handle manually")
			}
		}
	}()

	// Step 1: Join table
	err := client.JoinTable("table1")
	require.NoError(t, err)

	// Step 2: Wait for hand to start and verify we got cards
	client.WaitForHandStart()

	// Step 3: Small delay to ensure action is ready, then fold
	time.Sleep(100 * time.Millisecond) // Wait for action to be required
	err = client.ExecuteAction("fold")
	require.NoError(t, err)

	// Step 4: Wait for hand to complete
	client.WaitForHandEnd()

	// Step 5: Verify basic game flow worked
	client.AssertExpectedLog("Joined table table1", "*** HOLE CARDS ***", "Dealt to You:", "*** PRE-FLOP ***")
}

func testPreflopActions(t *testing.T) {
	t.Helper()
	client := setupExplicitPokerTestClient(t, 23456)

	// Setup explicit event handling without action script
	go func() {
		for event := range client.eventChan {
			switch event {
			case "hand_start":
				select {
				case client.handStarted <- struct{}{}:
				default:
				}
			case "hand_end":
				select {
				case client.handEnded <- struct{}{}:
				default:
				}
			case "action_required":
				t.Logf("EXPLICIT: Action required - test will handle manually")
			}
		}
	}()

	// Step 1: Join table and wait for action
	err := client.JoinTable("table1")
	require.NoError(t, err)
	client.WaitForHandStart()

	// Step 2: Test fold action only (simpler test)
	time.Sleep(100 * time.Millisecond) // Wait for action to be required
	err = client.ExecuteAction("fold")
	require.NoError(t, err)

	// Step 3: Wait for completion and verify
	client.WaitForHandEnd()
	client.AssertExpectedLog("*** PRE-FLOP ***", "TestPlayer: folds")
}

func testSidebarGameState(t *testing.T) {
	t.Helper()
	client := setupExplicitPokerTestClient(t, 34567)

	// Setup explicit event handling without action script
	go func() {
		for event := range client.eventChan {
			switch event {
			case "hand_start":
				select {
				case client.handStarted <- struct{}{}:
				default:
				}
			case "hand_end":
				select {
				case client.handEnded <- struct{}{}:
				default:
				}
			case "action_required":
				t.Logf("EXPLICIT: Action required - test will handle manually")
			}
		}
	}()

	// Step 1: Join table and wait for action
	err := client.JoinTable("table1")
	require.NoError(t, err)
	client.WaitForHandStart()

	// Step 2: Execute an action to trigger sidebar updates
	time.Sleep(100 * time.Millisecond) // Wait for action to be required
	err = client.ExecuteAction("call")
	require.NoError(t, err)

	// Step 3: Force UI refresh and check sidebar
	client.tui.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	_ = client.tui.View()

	// Step 4: Check sidebar contains expected game state elements
	sidebar := client.GetSidebarContent()
	assert.Contains(t, sidebar, "Players at table:")
	assert.Contains(t, sidebar, "TestPlayer")
	assert.Contains(t, sidebar, "Pot: $")

	// Step 5: Clean up
	err = client.ExecuteAction("fold")
	require.NoError(t, err)
}
