// Package spawner provides bot process management for poker server testing and orchestration.
package spawner

import (
	"context"
	"fmt"
	"strings"
	"sync"

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
func (s *BotSpawner) buildEnv(spec BotSpec, index int) map[string]string {
	env := make(map[string]string)

	// Core environment
	env["POKERFORBOTS_SERVER"] = s.serverURL
	env["POKERFORBOTS_GAME"] = spec.GameID

	// Increment bot sequence and use for ID
	s.mu.Lock()
	s.botSeq++
	botID := s.botSeq
	s.mu.Unlock()

	env["POKERFORBOTS_BOT_ID"] = fmt.Sprintf("bot-%d", botID)

	// Add seed derivation for deterministic testing
	if s.seed != 0 {
		botSeed := s.seed + int64(botID)
		env["POKERFORBOTS_SEED"] = fmt.Sprintf("%d", botSeed)
	}

	// Add custom environment variables
	for k, v := range spec.Env {
		env[k] = v
	}

	return env
}
