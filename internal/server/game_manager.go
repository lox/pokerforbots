package server

import (
	"sync"
	"time"

	"github.com/lox/pokerforbots/protocol"
	"github.com/rs/zerolog"
)

// GameInstance represents a single logical game/table configuration.
type GameInstance struct {
	ID     string
	Config Config
	Pool   *BotPool
}

// GameSummary holds lightweight metadata for clients.
type GameSummary struct {
	ID               string `json:"id"`
	SmallBlind       int    `json:"small_blind"`
	BigBlind         int    `json:"big_blind"`
	StartChips       int    `json:"start_chips"`
	TimeoutMs        int    `json:"timeout_ms"`
	MinPlayers       int    `json:"min_players"`
	MaxPlayers       int    `json:"max_players"`
	InfiniteBankroll bool   `json:"infinite_bankroll"`
	ConnectedBots    int    `json:"connected_bots"`
	HandsPlayed      uint64 `json:"hands_played"`
}

// GameManager tracks available games and their bot pools.
type GameManager struct {
	logger        zerolog.Logger
	mu            sync.RWMutex
	games         map[string]*GameInstance
	defaultGameID string
}

// NewGameManager constructs an empty game manager.
func NewGameManager(logger zerolog.Logger) *GameManager {
	return &GameManager{
		logger: logger.With().Str("component", "game_manager").Logger(),
		games:  make(map[string]*GameInstance),
	}
}

// RegisterGame registers a game instance with the manager. The first
// registered game becomes the default.
func (gm *GameManager) RegisterGame(id string, pool *BotPool, config Config) *GameInstance {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if existing, ok := gm.games[id]; ok {
		existing.Config = config
		return existing
	}

	instance := &GameInstance{ID: id, Config: config, Pool: pool}
	pool.SetGameID(id)
	gm.games[id] = instance
	if gm.defaultGameID == "" {
		gm.defaultGameID = id
	}
	return instance
}

// DeleteGame removes a game by ID and returns it.
func (gm *GameManager) DeleteGame(id string) (*GameInstance, bool) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	instance, ok := gm.games[id]
	if !ok {
		return nil, false
	}

	delete(gm.games, id)
	if gm.defaultGameID == id {
		gm.defaultGameID = ""
		for newID := range gm.games {
			gm.defaultGameID = newID
			break
		}
	}
	return instance, true
}

// NPC support has been removed - use the spawner tool for bot orchestration

// GetGame retrieves a game by ID.
func (gm *GameManager) GetGame(id string) (*GameInstance, bool) {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	instance, ok := gm.games[id]
	return instance, ok
}

// DefaultGame returns the default game instance, if any.
func (gm *GameManager) DefaultGame() (*GameInstance, bool) {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	instance, ok := gm.games[gm.defaultGameID]
	return instance, ok
}

// DefaultGameID returns the ID of the default game.
func (gm *GameManager) DefaultGameID() string {
	gm.mu.RLock()
	defer gm.mu.RUnlock()
	return gm.defaultGameID
}

// ListGames returns a snapshot of available games.
func (gm *GameManager) ListGames() []GameSummary {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	summaries := make([]GameSummary, 0, len(gm.games))
	for _, game := range gm.games {
		summary := GameSummary{
			ID:               game.ID,
			SmallBlind:       game.Config.SmallBlind,
			BigBlind:         game.Config.BigBlind,
			StartChips:       game.Config.StartChips,
			TimeoutMs:        int(game.Config.Timeout / time.Millisecond),
			MinPlayers:       game.Config.MinPlayers,
			MaxPlayers:       game.Config.MaxPlayers,
			InfiniteBankroll: game.Config.InfiniteBankroll,
			ConnectedBots:    game.Pool.BotCount(),
			HandsPlayed:      game.Pool.HandCount(),
		}
		summaries = append(summaries, summary)
	}
	return summaries
}

// GameStats retrieves statistics for a game by ID.
func (gm *GameManager) GameStats(id string) (GameStats, bool) {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	instance, ok := gm.games[id]
	if !ok {
		return GameStats{}, false
	}

	stats := instance.Stats()
	return stats, true
}

// Stats returns aggregate statistics for the game instance.
func (gi *GameInstance) Stats() GameStats {
	timeoutMs := int(gi.Config.Timeout / time.Millisecond)
	handsCompleted := gi.Pool.HandCount()
	handLimit := gi.Pool.HandLimit()

	start := gi.Pool.StartTime()
	end := gi.Pool.EndTime()
	var durSec float64
	if !start.IsZero() {
		if !end.IsZero() {
			durSec = end.Sub(start).Seconds()
		} else {
			durSec = time.Since(start).Seconds()
		}
	}

	stats := GameStats{
		ID:               gi.ID,
		SmallBlind:       gi.Config.SmallBlind,
		BigBlind:         gi.Config.BigBlind,
		StartChips:       gi.Config.StartChips,
		TimeoutMs:        timeoutMs,
		MinPlayers:       gi.Config.MinPlayers,
		MaxPlayers:       gi.Config.MaxPlayers,
		InfiniteBankroll: gi.Config.InfiniteBankroll,
		HandsCompleted:   handsCompleted,
		HandLimit:        handLimit,
		HandsRemaining:   gi.Pool.HandsRemaining(),
		Timeouts:         gi.Pool.TimeoutCount(),
		HandsPerSecond:   gi.Pool.HandsPerSecond(),
		StartTime:        start,
		EndTime:          end,
		DurationSeconds:  durSec,
		Seed:             gi.Config.Seed,
		CompletionReason: gi.Pool.CompletionReason(),
	}

	// Map pool player stats into protocol players for admin JSON
	poolPlayers := gi.Pool.PlayerStats()
	players := make([]protocol.GameCompletedPlayer, 0, len(poolPlayers))
	for _, ps := range poolPlayers {
		players = append(players, ps.GameCompletedPlayer)
	}
	stats.Players = players

	return stats
}

// StartAll launches the bot pools for all registered games.
func (gm *GameManager) StartAll() {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	for _, game := range gm.games {
		go game.Pool.Run()
	}
}

// StopAll stops all game pools.
func (gm *GameManager) StopAll() {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	for _, game := range gm.games {
		game.Pool.Stop()
	}
}
