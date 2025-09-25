// Package spawner provides bot process management for poker server testing and orchestration.
package spawner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lox/pokerforbots/sdk/config"
	"github.com/rs/zerolog"
)

// BotSpawner manages the lifecycle of bot processes.
type BotSpawner struct {
	serverURL string
	processes map[string]*Process
	mu        sync.RWMutex
	logger    zerolog.Logger
	ctx       context.Context
	cancel    context.CancelFunc
	seed      int64 // Base seed for deterministic testing
	botSeq    int   // Bot sequence counter
}

// BotSpec defines a bot to spawn.
type BotSpec struct {
	Command string            // Command to execute (e.g. "go")
	Args    []string          // Arguments (e.g. ["run", "./sdk/examples/calling-station"])
	Count   int               // Number to spawn
	GameID  string            // Target game (default: "default")
	Env     map[string]string // Additional environment variables
}

// GameStats represents game statistics from the server.
// This is a public DTO that doesn't leak internal types.
type GameStats struct {
	ID               string                        `json:"id"`
	SmallBlind       int                           `json:"small_blind"`
	BigBlind         int                           `json:"big_blind"`
	StartChips       int                           `json:"start_chips"`
	TimeoutMs        int                           `json:"timeout_ms"`
	MinPlayers       int                           `json:"min_players"`
	MaxPlayers       int                           `json:"max_players"`
	InfiniteBankroll bool                          `json:"infinite_bankroll"`
	HandsCompleted   uint64                        `json:"hands_completed"`
	HandLimit        uint64                        `json:"hand_limit"`
	HandsRemaining   uint64                        `json:"hands_remaining"`
	Timeouts         uint64                        `json:"timeouts"`
	HandsPerSecond   float64                       `json:"hands_per_second"`
	StartTime        time.Time                     `json:"start_time"`
	EndTime          time.Time                     `json:"end_time"`
	BotStatistics    map[string]map[string]float64 `json:"bot_statistics"`
	ActiveBots       int                           `json:"active_bots"`
	TotalBots        int                           `json:"total_bots"`
	CompletionReason string                        `json:"completion_reason"`
}

// New creates a new BotSpawner.
func New(serverURL string, logger zerolog.Logger) *BotSpawner {
	ctx, cancel := context.WithCancel(context.Background())
	return &BotSpawner{
		serverURL: serverURL,
		processes: make(map[string]*Process),
		logger:    logger.With().Str("component", "spawner").Logger(),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// NewWithSeed creates a new BotSpawner with a base seed for deterministic testing.
func NewWithSeed(serverURL string, logger zerolog.Logger, seed int64) *BotSpawner {
	spawner := New(serverURL, logger)
	spawner.seed = seed
	return spawner
}

// Spawn spawns one or more bot specs.
func (s *BotSpawner) Spawn(specs ...BotSpec) error {
	// Pre-allocate bot IDs deterministically based on spec order
	totalBots := 0
	for _, spec := range specs {
		if spec.Count <= 0 {
			spec.Count = 1
		}
		totalBots += spec.Count
	}

	botID := 0
	for _, spec := range specs {
		if spec.GameID == "" {
			spec.GameID = "default"
		}

		s.logger.Info().
			Str("command", spec.Command).
			Strs("args", spec.Args).
			Int("count", spec.Count).
			Str("game", spec.GameID).
			Msg("Spawning bots")

		for i := 0; i < spec.Count; i++ {
			botID++
			// Build environment with deterministic bot ID
			env := s.buildEnvWithID(spec, botID)

			// Create and start process
			proc := NewProcess(s.ctx, spec.Command, spec.Args, env, s.logger)
			if err := proc.Start(); err != nil {
				s.logger.Error().Err(err).Int("index", i).Msg("Failed to spawn bot")
				// Stop previously spawned bots on error
				s.StopAll()
				return fmt.Errorf("failed to spawn bot %d: %w", i, err)
			}

			s.mu.Lock()
			s.processes[proc.ID] = proc
			s.mu.Unlock()
		}
	}

	return nil
}

// StopAll stops all spawned bots.
func (s *BotSpawner) StopAll() error {
	s.logger.Info().Msg("Stopping all bots")
	s.cancel() // Cancel context to signal all processes

	s.mu.Lock()
	defer s.mu.Unlock()

	var lastErr error
	for id, proc := range s.processes {
		if err := proc.Stop(); err != nil {
			// Only log as error if it's a real failure
			if !strings.Contains(err.Error(), "process already finished") {
				s.logger.Error().Err(err).Str("bot_id", id).Msg("Failed to stop bot")
				lastErr = err
			}
		}
	}

	// Clear process map
	s.processes = make(map[string]*Process)

	// Create a new context for future spawns
	s.ctx, s.cancel = context.WithCancel(context.Background())

	return lastErr
}

// Wait waits for all bots to finish.
func (s *BotSpawner) Wait() error {
	s.mu.RLock()
	procs := make([]*Process, 0, len(s.processes))
	for _, p := range s.processes {
		procs = append(procs, p)
	}
	s.mu.RUnlock()

	for _, proc := range procs {
		proc.Wait()
	}
	return nil
}

// ActiveCount returns the number of active bot processes.
func (s *BotSpawner) ActiveCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, proc := range s.processes {
		if proc.IsAlive() {
			count++
		}
	}
	return count
}

// buildEnv builds the environment variables for a bot.
func (s *BotSpawner) buildEnv(spec BotSpec, _ int) map[string]string {
	// This method is deprecated, use buildEnvWithID instead
	s.mu.Lock()
	s.botSeq++
	botID := s.botSeq
	s.mu.Unlock()
	return s.buildEnvWithID(spec, botID)
}

func (s *BotSpawner) buildEnvWithID(spec BotSpec, botID int) map[string]string {
	env := make(map[string]string)

	// Core environment
	env[config.EnvServer] = s.serverURL
	env[config.EnvGame] = spec.GameID
	env[config.EnvBotID] = fmt.Sprintf("bot-%d", botID)

	// Add seed derivation for deterministic testing
	if s.seed != 0 {
		botSeed := s.seed + int64(botID)
		env[config.EnvSeed] = fmt.Sprintf("%d", botSeed)
	}

	// Add custom environment variables
	maps.Copy(env, spec.Env)

	return env
}

// SpawnBot spawns a single bot and returns its process handle.
// This is useful when you need to track individual bots.
func (s *BotSpawner) SpawnBot(spec BotSpec) (*Process, error) {
	if spec.Count != 1 {
		return nil, fmt.Errorf("SpawnBot expects Count=1, got %d", spec.Count)
	}

	// Build environment (this already increments botSeq and sets bot ID)
	env := s.buildEnv(spec, 0)
	botID := env[config.EnvBotID]

	// Create and start the process
	proc := NewProcess(s.ctx, spec.Command, spec.Args, env, s.logger)
	proc.ID = botID // Set the process ID to match bot ID
	if err := proc.Start(); err != nil {
		return nil, fmt.Errorf("failed to start bot: %w", err)
	}

	// Register the process
	s.mu.Lock()
	s.processes[botID] = proc
	s.mu.Unlock()

	s.logger.Info().
		Str("bot_id", botID).
		Str("command", spec.Command).
		Msg("Bot spawned")

	return proc, nil
}

// GetProcess retrieves a process by its bot ID.
func (s *BotSpawner) GetProcess(botID string) (*Process, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	proc, ok := s.processes[botID]
	return proc, ok
}

// CollectStats fetches game statistics from the server.
func CollectStats(serverURL string, gameID string) (*GameStats, error) {
	// Convert WebSocket URL to HTTP
	httpURL := strings.Replace(serverURL, "ws://", "http://", 1)
	httpURL = strings.Replace(httpURL, "wss://", "https://", 1)
	httpURL = strings.TrimSuffix(httpURL, "/ws")

	statsURL := fmt.Sprintf("%s/admin/games/%s/stats", httpURL, gameID)

	resp, err := http.Get(statsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch stats: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stats request failed with status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read stats response: %w", err)
	}

	var stats GameStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, fmt.Errorf("failed to parse stats: %w", err)
	}

	return &stats, nil
}
