package server

import (
	"sync"

	"github.com/rs/zerolog"
)

// GameInstance represents a single logical game/table configuration.
type GameInstance struct {
	ID            string
	Config        Config
	Pool          *BotPool
	RequirePlayer bool
}

// GameSummary holds lightweight metadata for clients.
type GameSummary struct {
	ID     string
	Config Config
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
		return existing
	}

	instance := &GameInstance{ID: id, Config: config, Pool: pool}
	gm.games[id] = instance
	if gm.defaultGameID == "" {
		gm.defaultGameID = id
	}
	return instance
}

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
		summaries = append(summaries, GameSummary{ID: game.ID, Config: game.Config})
	}
	return summaries
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
