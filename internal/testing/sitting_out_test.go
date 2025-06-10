package testing

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/coder/quartz"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	serverpkg "github.com/lox/pokerforbots/internal/server"
	"github.com/lox/pokerforbots/internal/tui"
)

func TestSittingOut(t *testing.T) {
	t.Run("timeout puts player in sitting out state", func(t *testing.T) {
		mockClock := quartz.NewMock(t)
		logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})

		port := findFreePort(t)
		server := startTestServer(t, port, 99999, 3, mockClock)
		defer server.Stop()

		tuiModel := tui.NewTUIModelWithOptions(logger, true)
		require.True(t, tuiModel.IsTestMode())

		serverURL := fmt.Sprintf("ws://127.0.0.1:%d", port)
		testClient := connectTestClient(t, serverURL, tuiModel)
		defer testClient.Disconnect()

		testClient.QueueActions([]string{}) // No actions - let it timeout
		testClient.EnableTimeouts()
		testClient.StartActionScript()

		err := testClient.JoinTable("table1")
		require.NoError(t, err)
		testClient.WaitForHandStart()

		time.Sleep(ActionDelay)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		mockClock.Advance(1 * time.Second).MustWait(ctx)

		require.True(t, testClient.waitForMessage(serverpkg.MessageTypePlayerTimeout))

		capturedLog := tuiModel.GetCapturedLog()
		logText := strings.Join(capturedLog, " ")
		assert.Contains(t, logText, "timed out")
		assert.Contains(t, logText, "sit-out")
	})

	t.Run("player state detection works", func(t *testing.T) {
		mockClock := quartz.NewMock(t)
		logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})

		port := findFreePort(t)
		server := startTestServer(t, port, 88888, 3, mockClock)
		defer server.Stop()

		tuiModel := tui.NewTUIModelWithOptions(logger, true)
		serverURL := fmt.Sprintf("ws://127.0.0.1:%d", port)
		testClient := connectTestClient(t, serverURL, tuiModel)
		testClient.mockClock = mockClock
		defer testClient.Disconnect()

		// Setup explicit event handling
		go func() {
			for event := range testClient.messageChan {
				switch event {
				case serverpkg.MessageTypeHandStart:
					select {
					case testClient.handStarted <- struct{}{}:
					default:
					}
				case serverpkg.MessageTypePlayerTimeout:
					select {
					case testClient.playerTimeout <- struct{}{}:
					default:
					}
				}
			}
		}()

		// Join game - player should not be sitting out initially
		err := testClient.JoinTable("table1")
		require.NoError(t, err)
		testClient.WaitForHandStart()
		testClient.AssertPlayerNotSittingOut()

		// Cause timeout to trigger sitting out
		testClient.WaitForActionRequired()
		testClient.AllowTimeout()
		testClient.WaitForPlayerTimeout()

		// Verify player is now detected as sitting out
		testClient.AssertPlayerSittingOut()
	})

	t.Run("/back command returns player to play", func(t *testing.T) {
		mockClock := quartz.NewMock(t)
		logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})

		port := findFreePort(t)
		server := startTestServer(t, port, 88888, 3, mockClock)
		defer server.Stop()

		tuiModel := tui.NewTUIModelWithOptions(logger, true)
		serverURL := fmt.Sprintf("ws://127.0.0.1:%d", port)
		testClient := connectTestClient(t, serverURL, tuiModel)
		testClient.mockClock = mockClock
		defer testClient.Disconnect()

		// Setup explicit event handling
		go func() {
			for event := range testClient.messageChan {
				switch event {
				case serverpkg.MessageTypeHandStart:
					select {
					case testClient.handStarted <- struct{}{}:
					default:
					}
				case serverpkg.MessageTypePlayerTimeout:
					select {
					case testClient.playerTimeout <- struct{}{}:
					default:
					}
				case serverpkg.MessageTypeGamePause:
					select {
					case testClient.gamePause <- struct{}{}:
					default:
					}
				}
			}
		}()

		// Get into sitting out state
		err := testClient.JoinTable("table1")
		require.NoError(t, err)
		testClient.WaitForHandStart()
		testClient.WaitForActionRequired()
		testClient.AllowTimeout()
		testClient.WaitForPlayerTimeout()
		testClient.AssertPlayerSittingOut()

		// Wait for game to pause
		testClient.WaitForGamePause()

		// Execute /back command
		err = testClient.ExecuteCommand("/back")
		require.NoError(t, err)

		time.Sleep(200 * time.Millisecond) // Processing time

		// Verify the /back command worked
		capturedLog := tuiModel.GetCapturedLog()
		logText := strings.Join(capturedLog, " ")
		assert.Contains(t, logText, "Returning to play")
	})

	t.Run("game pauses when all humans sit out", func(t *testing.T) {
		mockClock := quartz.NewMock(t)
		logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})

		port := findFreePort(t)
		server := startTestServer(t, port, 77777, 1, mockClock) // 1 bot so game can pause
		defer server.Stop()

		tuiModel := tui.NewTUIModelWithOptions(logger, true)
		serverURL := fmt.Sprintf("ws://127.0.0.1:%d", port)
		testClient := connectTestClient(t, serverURL, tuiModel)
		testClient.mockClock = mockClock
		defer testClient.Disconnect()

		// Setup explicit event handling
		go func() {
			for event := range testClient.messageChan {
				switch event {
				case serverpkg.MessageTypeHandStart:
					select {
					case testClient.handStarted <- struct{}{}:
					default:
					}
				case serverpkg.MessageTypePlayerTimeout:
					select {
					case testClient.playerTimeout <- struct{}{}:
					default:
					}
				case serverpkg.MessageTypeGamePause:
					select {
					case testClient.gamePause <- struct{}{}:
					default:
					}
				}
			}
		}()

		// Join game and wait for action
		err := testClient.JoinTable("table1")
		require.NoError(t, err)
		testClient.WaitForHandStart()
		testClient.WaitForActionRequired()

		// Allow timeout to occur
		testClient.AllowTimeout()
		testClient.WaitForPlayerTimeout()
		testClient.AssertSittingOut()

		// Wait for game to pause
		testClient.WaitForGamePause()

		// Verify expected log entries
		capturedLog := tuiModel.GetCapturedLog()
		logText := strings.Join(capturedLog, " ")
		assert.Contains(t, logText, "all sitting out")
		assert.Contains(t, logText, "pausing game")
	})
}

func TestSittingOutMockTime(t *testing.T) {
	t.Run("mock time makes timeouts instant", func(t *testing.T) {
		mockClock := quartz.NewMock(t)
		logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})

		port := findFreePort(t)
		server := startTestServer(t, port, 99999, 3, mockClock)
		defer server.Stop()

		tuiModel := tui.NewTUIModelWithOptions(logger, true)
		serverURL := fmt.Sprintf("ws://127.0.0.1:%d", port)
		testClient := connectTestClient(t, serverURL, tuiModel)
		defer testClient.Disconnect()

		testClient.EnableTimeouts()
		testClient.StartActionScript()

		err := testClient.JoinTable("table1")
		require.NoError(t, err)
		testClient.WaitForHandStart()

		time.Sleep(ActionDelay)

		// Advance mock clock to trigger timeout instantly!
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		mockClock.Advance(1 * time.Second).MustWait(ctx)
		elapsed := time.Since(start)

		require.True(t, testClient.waitForMessage(serverpkg.MessageTypePlayerTimeout), "Player timeout should fire")

		// Should have timeout messages
		capturedLog := tuiModel.GetCapturedLog()
		logText := strings.Join(capturedLog, " ")
		assert.Contains(t, logText, "timed out")
		assert.Contains(t, logText, "sit-out")

		// Verify mock time worked - timeout was instant instead of 1 real second
		assert.Less(t, elapsed, 500*time.Millisecond, "Mock time should make timeout instant")
	})
}
