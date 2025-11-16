package handhistory

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestManagerFlushOnRequest(t *testing.T) {
	baseDir := t.TempDir()
	logger := zerolog.New(io.Discard)
	mgr := NewManager(logger, ManagerConfig{
		BaseDir:          baseDir,
		FlushInterval:    time.Hour, // rely on explicit requests
		FlushHands:       1,
		IncludeHoleCards: false,
		Variant:          "NT",
		Clock:            stubClock{current: time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)},
	})
	t.Cleanup(func() { mgr.Shutdown() })

	monitor, err := mgr.CreateMonitor("default")
	if err != nil {
		t.Fatalf("CreateMonitor error: %v", err)
	}

	monitor.OnHandStart("hand-1", samplePlayers(), 0, Blinds{Small: 1, Big: 2})
	monitor.OnHandComplete(Outcome{HandID: "hand-1"})

	path := filepath.Join(baseDir, "game-default", "session.phhs")

	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("hand history file not flushed in time")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
