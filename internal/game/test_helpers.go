package game

import (
	"io"
	"math/rand"

	"github.com/charmbracelet/log"
)

// TestTableOption configures test table creation
type TestTableOption func(*testTableBuilder)

type testTableBuilder struct {
	seed     int64
	config   TableConfig
	eventBus EventBus
	players  []string
}

// Test table options
func WithSeed(seed int64) TestTableOption {
	return func(b *testTableBuilder) { b.seed = seed }
}

func WithMaxSeats(seats int) TestTableOption {
	return func(b *testTableBuilder) { b.config.MaxSeats = seats }
}

func WithBlinds(small, big int) TestTableOption {
	return func(b *testTableBuilder) {
		b.config.SmallBlind = small
		b.config.BigBlind = big
	}
}

func WithEventBus(eventBus EventBus) TestTableOption {
	return func(b *testTableBuilder) { b.eventBus = eventBus }
}

func WithPlayers(names ...string) TestTableOption {
	return func(b *testTableBuilder) { b.players = names }
}

func WithHandHistoryWriter(writer HandHistoryWriter) TestTableOption {
	return func(b *testTableBuilder) { b.config.HandHistoryWriter = writer }
}

// NewTestTable creates a table for testing with sensible defaults
func NewTestTable(opts ...TestTableOption) *Table {
	builder := &testTableBuilder{
		seed: 42,
		config: TableConfig{
			MaxSeats:          6,
			SmallBlind:        10,
			BigBlind:          20,
			HandHistoryWriter: &NoOpHandHistoryWriter{},
		},
		eventBus: NewEventBus(),
	}

	for _, opt := range opts {
		opt(builder)
	}

	rng := rand.New(rand.NewSource(builder.seed))
	table := NewTable(rng, builder.eventBus, builder.config)

	// Add players if specified
	for i, name := range builder.players {
		player := NewPlayer(i+1, name, AI, 1000)
		table.AddPlayer(player)
	}

	return table
}

// NewTestGameEngine creates a table and engine for testing
func NewTestGameEngine(opts ...TestTableOption) (*Table, *GameEngine) {
	table := NewTestTable(opts...)
	engine := NewGameEngine(table, log.New(io.Discard))
	return table, engine
}

// Convenience functions for common scenarios
func HeadsUpTable() *Table {
	return NewTestTable(
		WithMaxSeats(2),
		WithPlayers("Alice", "Bob"),
	)
}

func SixMaxTable() *Table {
	return NewTestTable(
		WithPlayers("Alice", "Bob", "Charlie", "Diana", "Eve", "Frank"),
	)
}

// AddTestPlayers is a helper to add players to an existing table
func AddTestPlayers(table *Table, names ...string) []*Player {
	players := make([]*Player, len(names))
	for i, name := range names {
		player := NewPlayer(i+1, name, AI, 1000)
		players[i] = player
		table.AddPlayer(player)
	}
	return players
}
