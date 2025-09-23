// Package spawner provides bot process management for poker server testing and orchestration.
package spawner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strconv"
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

// Spawn spawns one or more bots according to the spec.
func (s *BotSpawner) Spawn(spec BotSpec) error {
	if spec.Count <= 0 {
		spec.Count = 1
	}
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
		proc, err := s.spawnOne(spec, i)
		if err != nil {
			s.logger.Error().Err(err).Int("index", i).Msg("Failed to spawn bot")
			// Stop previously spawned bots on error
			s.StopAll()
			return fmt.Errorf("failed to spawn bot %d: %w", i, err)
		}

		s.mu.Lock()
		s.processes[proc.ID] = proc
		s.mu.Unlock()
	}

	return nil
}

// SpawnMany spawns multiple bot specs.
func (s *BotSpawner) SpawnMany(specs []BotSpec) error {
	for _, spec := range specs {
		if err := s.Spawn(spec); err != nil {
			return err
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

// spawnOne spawns a single bot instance.
func (s *BotSpawner) spawnOne(spec BotSpec, index int) (*Process, error) {
	// Build environment
	env := s.buildEnv(spec, index)

	// Create and start process
	proc := NewProcess(s.ctx, spec.Command, spec.Args, env, s.logger)
	if err := proc.Start(); err != nil {
		return nil, err
	}

	return proc, nil
}

// buildEnv builds the environment variables for a bot.
func (s *BotSpawner) buildEnv(spec BotSpec, _ int) map[string]string {
	env := make(map[string]string)

	// Core environment
	env[config.EnvServer] = s.serverURL
	env[config.EnvGame] = spec.GameID

	// Increment bot sequence and use for ID
	s.mu.Lock()
	s.botSeq++
	botID := s.botSeq
	s.mu.Unlock()

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

	// Generate deterministic bot ID
	botID := fmt.Sprintf("bot-%d", s.botSeq)
	s.botSeq++

	// Build environment
	env := s.buildEnv(spec, 0)
	env[config.EnvBotID] = botID

	// Create and start the process
	proc := NewProcess(s.ctx, spec.Command, spec.Args, env, s.logger)
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

// ServerConfig defines configuration for spawning a server subprocess.
type ServerConfig struct {
	Command     string   // Command to execute (default: "go")
	Args        []string // Arguments (default: ["run", "./cmd/server"])
	Addr        string   // Server address
	SmallBlind  int
	BigBlind    int
	StartChips  int
	TimeoutMs   int
	MinPlayers  int
	MaxPlayers  int
	Seed        int64
	HandLimit   int
	EnableStats bool
	Env         map[string]string // Additional environment variables
}

// SpawnServer spawns a poker server as a subprocess and returns its process handle.
// This is useful for isolation and testing different server versions.
func (s *BotSpawner) SpawnServer(config ServerConfig) (*Process, error) {
	// Set defaults
	if config.Command == "" {
		config.Command = "go"
		config.Args = []string{"run", "./cmd/server"}
	}

	// Build command line arguments
	args := append([]string{}, config.Args...)

	if config.Addr != "" {
		args = append(args, "--addr", config.Addr)
	}
	if config.SmallBlind > 0 {
		args = append(args, "--small-blind", strconv.Itoa(config.SmallBlind))
	}
	if config.BigBlind > 0 {
		args = append(args, "--big-blind", strconv.Itoa(config.BigBlind))
	}
	if config.StartChips > 0 {
		args = append(args, "--start-chips", strconv.Itoa(config.StartChips))
	}
	if config.TimeoutMs > 0 {
		args = append(args, "--timeout-ms", strconv.Itoa(config.TimeoutMs))
	}
	if config.MinPlayers > 0 {
		args = append(args, "--min-players", strconv.Itoa(config.MinPlayers))
	}
	if config.MaxPlayers > 0 {
		args = append(args, "--max-players", strconv.Itoa(config.MaxPlayers))
	}
	if config.Seed != 0 {
		args = append(args, "--seed", strconv.FormatInt(config.Seed, 10))
	}
	if config.HandLimit > 0 {
		args = append(args, "--hand-limit", strconv.Itoa(config.HandLimit))
	}
	if config.EnableStats {
		args = append(args, "--enable-stats")
	}

	// Build environment
	env := make(map[string]string)
	maps.Copy(env, config.Env)

	// Create and start the process
	proc := NewProcess(s.ctx, config.Command, args, env, s.logger)
	if err := proc.Start(); err != nil {
		return nil, fmt.Errorf("failed to start server: %w", err)
	}

	// Register as a special process
	s.mu.Lock()
	s.processes["__server__"] = proc
	s.mu.Unlock()

	s.logger.Info().
		Str("addr", config.Addr).
		Msg("Server spawned")

	return proc, nil
}

// WaitForServer waits for a server to be ready by polling its health endpoint.
func WaitForServer(url string, timeout time.Duration) error {
	// Convert WebSocket URL to HTTP
	httpURL := strings.Replace(url, "ws://", "http://", 1)
	httpURL = strings.Replace(httpURL, "wss://", "https://", 1)
	httpURL = strings.TrimSuffix(httpURL, "/ws")

	healthURL := httpURL + "/health"
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := http.Get(healthURL)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("server not ready after %v", timeout)
}

// GameStats represents aggregate game statistics.
type GameStats struct {
	ID              string    `json:"id"`
	SmallBlind      int       `json:"small_blind"`
	BigBlind        int       `json:"big_blind"`
	StartChips      int       `json:"start_chips"`
	TimeoutMs       int       `json:"timeout_ms"`
	MinPlayers      int       `json:"min_players"`
	MaxPlayers      int       `json:"max_players"`
	HandsCompleted  int       `json:"hands_completed"`
	HandLimit       int       `json:"hand_limit"`
	HandsRemaining  int       `json:"hands_remaining"`
	Timeouts        int       `json:"timeouts"`
	HandsPerSecond  float64   `json:"hands_per_second"`
	StartTime       time.Time `json:"start_time"`
	EndTime         time.Time `json:"end_time"`
	DurationSeconds float64   `json:"duration_seconds"`
	Seed            int64     `json:"seed"`
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
