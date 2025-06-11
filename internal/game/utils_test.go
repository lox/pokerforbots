package game

import (
	"math/rand"
	"strings"
	"testing"
)

func TestValidatePlayerForTable(t *testing.T) {
	t.Run("nil table", func(t *testing.T) {
		err := ValidatePlayerForTable(nil, NewPlayer(1, "Alice", Human, 200))
		if err == nil || !strings.Contains(err.Error(), "table is nil") {
			t.Errorf("expected 'table is nil' error, got: %v", err)
		}
	})

	t.Run("nil player", func(t *testing.T) {
		eventBus := NewEventBus()
		table := NewTable(rand.New(rand.NewSource(0)), eventBus, TableConfig{
			MaxSeats:   6,
			SmallBlind: 1,
			BigBlind:   2,
		})

		err := ValidatePlayerForTable(table, nil)
		if err == nil || !strings.Contains(err.Error(), "player is nil") {
			t.Errorf("expected 'player is nil' error, got: %v", err)
		}
	})

	t.Run("valid player on empty table", func(t *testing.T) {
		eventBus := NewEventBus()
		table := NewTable(rand.New(rand.NewSource(0)), eventBus, TableConfig{
			MaxSeats:   6,
			SmallBlind: 1,
			BigBlind:   2,
		})

		err := ValidatePlayerForTable(table, NewPlayer(1, "Alice", Human, 200))
		if err != nil {
			t.Errorf("expected no error for valid player, got: %v", err)
		}
	})

	t.Run("duplicate player ID", func(t *testing.T) {
		eventBus := NewEventBus()
		table := NewTable(rand.New(rand.NewSource(0)), eventBus, TableConfig{
			MaxSeats:   6,
			SmallBlind: 1,
			BigBlind:   2,
		})

		// Add Alice first
		alice := NewPlayer(1, "Alice", Human, 200)
		table.AddPlayer(alice)

		// Try to add Bob with same ID
		bob := NewPlayer(1, "Bob", AI, 200)
		err := ValidatePlayerForTable(table, bob)
		if err == nil || !strings.Contains(err.Error(), "player with ID 1 already exists") {
			t.Errorf("expected duplicate ID error, got: %v", err)
		}
	})

	t.Run("duplicate player name", func(t *testing.T) {
		eventBus := NewEventBus()
		table := NewTable(rand.New(rand.NewSource(0)), eventBus, TableConfig{
			MaxSeats:   6,
			SmallBlind: 1,
			BigBlind:   2,
		})

		// Add Alice first
		alice := NewPlayer(1, "Alice", Human, 200)
		table.AddPlayer(alice)

		// Try to add another Alice with different ID
		alice2 := NewPlayer(2, "Alice", AI, 200)
		err := ValidatePlayerForTable(table, alice2)
		if err == nil || !strings.Contains(err.Error(), "player with name 'Alice' already exists") {
			t.Errorf("expected duplicate name error, got: %v", err)
		}
	})

	t.Run("valid unique player", func(t *testing.T) {
		eventBus := NewEventBus()
		table := NewTable(rand.New(rand.NewSource(0)), eventBus, TableConfig{
			MaxSeats:   6,
			SmallBlind: 1,
			BigBlind:   2,
		})

		// Add Alice first
		alice := NewPlayer(1, "Alice", Human, 200)
		table.AddPlayer(alice)

		// Add Bob with unique ID and name
		bob := NewPlayer(2, "Bob", AI, 200)
		err := ValidatePlayerForTable(table, bob)
		if err != nil {
			t.Errorf("expected no error for unique player, got: %v", err)
		}
	})
}

func TestValidatePlayerForTable_TableFull(t *testing.T) {
	eventBus := NewEventBus()
	table := NewTable(rand.New(rand.NewSource(0)), eventBus, TableConfig{
		MaxSeats:   2, // Small table for easier testing
		SmallBlind: 1,
		BigBlind:   2,
	})

	// Fill the table
	player1 := NewPlayer(1, "Alice", Human, 200)
	player2 := NewPlayer(2, "Bob", AI, 200)
	table.AddPlayer(player1)
	table.AddPlayer(player2)

	// Try to add a third player
	player3 := NewPlayer(3, "Charlie", AI, 200)
	err := ValidatePlayerForTable(table, player3)

	if err == nil {
		t.Error("expected error for full table but got none")
		return
	}

	if !strings.Contains(err.Error(), "table is full") {
		t.Errorf("expected error message to contain 'table is full', got '%s'", err.Error())
	}

	if !strings.Contains(err.Error(), "2/2") {
		t.Errorf("expected error message to show seat count 2/2, got '%s'", err.Error())
	}
}
