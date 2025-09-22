package spawner

import (
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestSpawnerBasic(t *testing.T) {
	logger := zerolog.New(zerolog.NewTestWriter(t))
	spawner := New("ws://localhost:8080/ws", logger)

	// Test spawning a simple echo command
	spec := BotSpec{
		Command: "echo",
		Args:    []string{"hello", "world"},
		Count:   1,
	}

	if err := spawner.Spawn(spec); err != nil {
		t.Fatalf("Failed to spawn bot: %v", err)
	}

	// Wait for process to complete
	time.Sleep(100 * time.Millisecond)

	// Check active count
	if count := spawner.ActiveCount(); count != 0 {
		t.Errorf("Expected 0 active processes after echo completes, got %d", count)
	}

	// Clean up
	if err := spawner.StopAll(); err != nil {
		t.Fatalf("Failed to stop all: %v", err)
	}
}

func TestSpawnerMultiple(t *testing.T) {
	logger := zerolog.New(zerolog.NewTestWriter(t))
	spawner := New("ws://localhost:8080/ws", logger)

	// Spawn multiple processes
	spec := BotSpec{
		Command: "sleep",
		Args:    []string{"0.1"},
		Count:   3,
	}

	if err := spawner.Spawn(spec); err != nil {
		t.Fatalf("Failed to spawn bots: %v", err)
	}

	// Check active count immediately
	if count := spawner.ActiveCount(); count != 3 {
		t.Errorf("Expected 3 active processes, got %d", count)
	}

	// Wait for processes to complete
	time.Sleep(200 * time.Millisecond)

	// Check they're done
	if count := spawner.ActiveCount(); count != 0 {
		t.Errorf("Expected 0 active processes after sleep completes, got %d", count)
	}

	// Clean up
	spawner.StopAll()
}

func TestSpawnerEnvironment(t *testing.T) {
	logger := zerolog.New(zerolog.NewTestWriter(t))
	spawner := NewWithSeed("ws://localhost:8080/ws", logger, 42)

	// Create temp script to echo environment
	script := `#!/bin/sh
echo "SERVER=$POKERFORBOTS_SERVER"
echo "GAME=$POKERFORBOTS_GAME"
echo "ID=$POKERFORBOTS_BOT_ID"
echo "SEED=$POKERFORBOTS_SEED"
`
	tmpfile := t.TempDir() + "/test.sh"
	if err := os.WriteFile(tmpfile, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	// Spawn with custom environment
	spec := BotSpec{
		Command: "sh",
		Args:    []string{tmpfile},
		Count:   1,
		GameID:  "test-game",
		Env: map[string]string{
			"CUSTOM_VAR": "custom_value",
		},
	}

	if err := spawner.Spawn(spec); err != nil {
		t.Fatalf("Failed to spawn bot: %v", err)
	}

	// Wait for completion
	spawner.Wait()

	// TODO: Capture and verify output
	// For now, just ensure it ran without error
}

func TestSpawnerStop(t *testing.T) {
	logger := zerolog.New(zerolog.NewTestWriter(t))
	spawner := New("ws://localhost:8080/ws", logger)

	// Use a shell script that handles signals properly
	script := `#!/bin/sh
trap 'exit 0' INT TERM
sleep 10
`
	tmpfile := t.TempDir() + "/sleeper.sh"
	if err := os.WriteFile(tmpfile, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	// Spawn long-running process
	spec := BotSpec{
		Command: "sh",
		Args:    []string{tmpfile},
		Count:   1,
	}

	if err := spawner.Spawn(spec); err != nil {
		t.Fatalf("Failed to spawn bot: %v", err)
	}

	// Should be running
	if count := spawner.ActiveCount(); count != 1 {
		t.Errorf("Expected 1 active process, got %d", count)
	}

	// Stop it
	if err := spawner.StopAll(); err != nil {
		t.Errorf("Stop error (can be ignored on some systems): %v", err)
	}

	// Wait a bit for cleanup
	time.Sleep(100 * time.Millisecond)

	// Should be stopped
	if count := spawner.ActiveCount(); count != 0 {
		t.Errorf("Expected 0 active processes after stop, got %d", count)
	}
}
