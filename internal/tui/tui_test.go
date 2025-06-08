package tui

import (
	"io"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTUITestMode(t *testing.T) {
	logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel}) // Quiet logger for tests

	t.Run("test mode captures log entries", func(t *testing.T) {
		tui := NewTUIModelWithOptions(logger, true)

		assert.True(t, tui.IsTestMode())
		assert.Empty(t, tui.GetCapturedLog())

		// Add some log entries
		tui.AddLogEntry("Player joins table")
		tui.AddLogEntry("*** PRE-FLOP ***")
		tui.AddBoldLogEntry("Hand #123 starting")

		// Check captured log
		captured := tui.GetCapturedLog()
		require.Len(t, captured, 3)

		// Bold entries get inserted at the beginning
		assert.Equal(t, "Hand #123 starting", captured[0])
		assert.Equal(t, "Player joins table", captured[1])
		assert.Equal(t, "*** PRE-FLOP ***", captured[2])
	})

	t.Run("production mode does not capture logs", func(t *testing.T) {
		tui := NewTUIModel(logger) // Default is production mode

		assert.False(t, tui.IsTestMode())

		tui.AddLogEntry("Some log entry")

		// Should return nil in production mode
		assert.Nil(t, tui.GetCapturedLog())
	})

	t.Run("action injection works in test mode", func(t *testing.T) {
		tui := NewTUIModelWithOptions(logger, true)

		// Inject an action
		err := tui.InjectAction("call", nil)
		require.NoError(t, err)

		// Wait for the action
		action, args, cont, err := tui.WaitForAction()
		require.NoError(t, err)

		assert.Equal(t, "call", action)
		assert.Empty(t, args)
		assert.True(t, cont)
	})

	t.Run("action injection fails in production mode", func(t *testing.T) {
		tui := NewTUIModel(logger) // Production mode

		err := tui.InjectAction("call", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "test mode")
	})

	t.Run("action injection with arguments", func(t *testing.T) {
		tui := NewTUIModelWithOptions(logger, true)

		// Inject action with arguments
		err := tui.InjectAction("raise", []string{"20"})
		require.NoError(t, err)

		// Wait for the action
		action, args, cont, err := tui.WaitForAction()
		require.NoError(t, err)

		assert.Equal(t, "raise", action)
		assert.Equal(t, []string{"20"}, args)
		assert.True(t, cont)
	})
}
