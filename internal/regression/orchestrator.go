package regression

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/lox/pokerforbots/internal/server"
	"github.com/rs/zerolog"
)

// Orchestrator uses spawner for bot management instead of managing processes directly
type Orchestrator struct {
	config        *Config
	healthMonitor *HealthMonitor
	logger        zerolog.Logger
	serverCmd     *exec.Cmd
	useSpawner    bool // Flag to use new spawner-based approach
}

// NewOrchestrator creates a new orchestrator that uses server bot commands
func NewOrchestrator(config *Config, healthMonitor *HealthMonitor) *Orchestrator {
	// Check if spawner command exists in PATH or as ./cmd/spawner
	useSpawner := false
	if _, err := exec.LookPath("spawner"); err == nil {
		useSpawner = true
		config.Logger.Info().Msg("Using spawner for bot orchestration")
	} else if _, err := os.Stat("./cmd/spawner/main.go"); err == nil {
		useSpawner = true
		config.Logger.Info().Msg("Using spawner (via go run) for bot orchestration")
	} else {
		config.Logger.Info().Msg("Using legacy server-based bot orchestration (spawner not found)")
	}

	return &Orchestrator{
		config:        config,
		healthMonitor: healthMonitor,
		logger:        config.Logger,
		useSpawner:    useSpawner,
	}
}

// ExecuteBatches runs multiple batches using the provided strategy
func (o *Orchestrator) ExecuteBatches(ctx context.Context, strategy BatchStrategy, totalHands int) ([]BatchResult, error) {
	if totalHands <= 0 {
		return nil, fmt.Errorf("totalHands must be positive, got %d", totalHands)
	}

	// Create strategy-specific health monitor with policy limits
	healthPolicy := strategy.GetHealthPolicy()
	originalHealthMonitor := o.healthMonitor
	o.healthMonitor = NewHealthMonitor(
		healthPolicy.MaxCrashesPerBot,
		healthPolicy.MaxTimeoutsPerBot,
		time.Duration(healthPolicy.RestartDelayMs)*time.Millisecond,
		o.logger,
	)
	// Ensure we restore the original health monitor when done
	defer func() {
		o.healthMonitor = originalHealthMonitor
	}()

	o.logger.Info().
		Str("strategy", strategy.Name()).
		Int("total_hands", totalHands).
		Int("batch_size", o.config.BatchSize).
		Int("max_crashes", healthPolicy.MaxCrashesPerBot).
		Int("max_timeouts", healthPolicy.MaxTimeoutsPerBot).
		Msg("Starting batch execution with strategy-specific health policy")

	// Calculate number of batches
	handsPerBatch := o.config.BatchSize
	remainingHands := totalHands

	// Generate seeds if needed
	seeds := o.config.Seeds
	if len(seeds) == 0 {
		seeds = []int64{42} // Default seed
	}

	var allBatches []BatchResult
	batchNum := 0
	totalHandsCompleted := 0

	for remainingHands > 0 {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return allBatches, ctx.Err()
		default:
		}

		batchHands := min(handsPerBatch, remainingHands)

		// Get or generate seed for this batch
		var seed int64
		if batchNum < len(seeds) {
			seed = seeds[batchNum]
		} else {
			// Generate additional seed based on last available seed
			seed = seeds[len(seeds)-1] + int64(batchNum-len(seeds)+1)*1000
		}

		o.logger.Info().
			Int("batch", batchNum+1).
			Int64("seed", seed).
			Int("hands", batchHands).
			Msg("Running batch")

		// Configure batch using strategy
		batchConfig := strategy.ConfigureBatch(batchNum, seed)
		batchConfig.Hands = batchHands // Override with actual hands for this batch

		// Run the batch
		batch, err := o.runSingleBatch(ctx, strategy, batchConfig)
		if err != nil {
			return allBatches, fmt.Errorf("batch %d failed: %w", batchNum+1, err)
		}

		allBatches = append(allBatches, *batch)
		remainingHands -= batchHands
		totalHandsCompleted += batchHands
		batchNum++

		// Check for early stopping
		if o.config.EarlyStopping && totalHandsCompleted >= o.config.MinHands {
			// Aggregate current results for early stopping check
			aggregated := o.aggregateBatchResults(allBatches, strategy)
			if strategy.ShouldStopEarly(aggregated, totalHandsCompleted) {
				o.logger.Info().
					Int("hands_completed", totalHandsCompleted).
					Msg("Early stopping criteria met")
				break
			}
		}
	}

	if len(allBatches) == 0 {
		return nil, fmt.Errorf("no batches completed")
	}

	o.logger.Info().
		Int("batches_completed", len(allBatches)).
		Int("hands_completed", totalHandsCompleted).
		Msg("Batch execution completed")

	return allBatches, nil
}

// runSingleBatch runs a single batch with the given configuration
func (o *Orchestrator) runSingleBatch(ctx context.Context, strategy BatchStrategy, config BatchConfiguration) (*BatchResult, error) {
	// Create temporary file for stats
	statsFile := fmt.Sprintf("stats-%s-%d-%d.json", strategy.Name(), config.Seed, time.Now().Unix())
	defer os.Remove(statsFile) // Clean up after

	// Build server configuration
	serverConfig := &ServerConfig{
		Seed:        config.Seed,
		Hands:       config.Hands,
		StatsFile:   statsFile,
		BotCommands: config.BotCommands,
		NPCConfig:   config.NPCConfig,
	}

	// Log the exact reproduction command for debugging
	reproCmd := serverConfig.BuildReproCommand(o.config)
	o.logger.Info().
		Str("repro_cmd", reproCmd).
		Msg("To reproduce this batch, run")

	// Start server with consolidated configuration
	if err := o.StartServer(ctx, serverConfig); err != nil {
		return nil, fmt.Errorf("failed to start server: %w", err)
	}
	defer o.StopServer()

	// Wait for completion
	if err := o.WaitForCompletion(ctx); err != nil {
		return nil, fmt.Errorf("server failed to complete: %w", err)
	}

	// Parse stats from JSON file
	stats, err := o.parseStatsFile(statsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse stats file: %w", err)
	}

	// Aggregate stats using strategy
	results, err := strategy.AggregateStats(stats)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate stats: %w", err)
	}

	// Extract standard deviations from detailed stats
	stdDevs := make(map[string]float64)
	for i, player := range stats.Players {
		if player.DetailedStats != nil {
			// Map standard deviations to match result keys using standardized prefixes
			switch i {
			case 0:
				// First player is always challenger
				stdDevs["challenger_std_dev"] = player.DetailedStats.StdDev
			case 1:
				// Second player is baseline in heads-up mode
				stdDevs["baseline_std_dev"] = player.DetailedStats.StdDev
			}
			// Also store by player index for multi-player modes
			stdDevs[fmt.Sprintf("player_%d_std_dev", i)] = player.DetailedStats.StdDev
		}
	}

	return &BatchResult{
		Seed:    config.Seed,
		Hands:   config.Hands,
		Results: results,
		StdDevs: stdDevs,
	}, nil
}

// aggregateBatchResults aggregates results from multiple batches
func (o *Orchestrator) aggregateBatchResults(batches []BatchResult, _ BatchStrategy) map[string]float64 {
	// This is a simplified aggregation for early stopping checks
	// The actual aggregation logic depends on the strategy
	if len(batches) == 0 {
		return make(map[string]float64)
	}

	// For now, just return the last batch's results
	// TODO: Implement proper weighted averaging based on strategy
	return batches[len(batches)-1].Results
}

// StartServer starts the server with the given configuration
func (o *Orchestrator) StartServer(ctx context.Context, serverConfig *ServerConfig) error {
	if o.useSpawner {
		return o.startServerWithSpawner(ctx, serverConfig)
	}

	// Build arguments using the server configuration
	args := serverConfig.BuildServerArgs(o.config)

	// Start the server
	return o.startServerWithArgs(ctx, args, serverConfig.Seed, serverConfig.Hands,
		len(serverConfig.BotCommands), serverConfig.CountNPCs())
}

// startServerWithSpawner starts the server using the spawner tool
func (o *Orchestrator) startServerWithSpawner(ctx context.Context, serverConfig *ServerConfig) error {
	var args []string

	// Core server configuration
	args = append(args,
		"--addr", o.config.ServerAddr,
		"--seed", fmt.Sprintf("%d", serverConfig.Seed),
		"--hand-limit", fmt.Sprintf("%d", serverConfig.Hands),
		"--write-stats-on-exit", serverConfig.StatsFile,
	)

	// Add debug flag if logger is in debug mode
	if o.logger.GetLevel() == zerolog.DebugLevel {
		args = append(args, "--debug")
	}

	// Add bot commands
	for _, botCmd := range serverConfig.BotCommands {
		args = append(args, "--bot", botCmd)
	}

	// Add NPC configuration if present
	if serverConfig.NPCConfig != "" {
		// Parse NPC config and convert to spawner demo mode
		switch {
		case serverConfig.NPCConfig == "aggressive:3,calling:3":
			args = append(args, "--demo", "mixed", "--num-bots", "6")
		case strings.Contains(serverConfig.NPCConfig, "aggressive"):
			args = append(args, "--demo", "aggressive", "--num-bots", fmt.Sprintf("%d", serverConfig.CountNPCs()))
		case strings.Contains(serverConfig.NPCConfig, "calling"):
			args = append(args, "--demo", "simple", "--num-bots", fmt.Sprintf("%d", serverConfig.CountNPCs()))
		case strings.Contains(serverConfig.NPCConfig, "random"):
			args = append(args, "--demo", "mixed", "--num-bots", fmt.Sprintf("%d", serverConfig.CountNPCs()))
		}
	}

	// Determine spawner command
	spawnerCmd := "spawner"
	if _, err := exec.LookPath(spawnerCmd); err != nil {
		// Try using go run
		spawnerCmd = "go"
		args = append([]string{"run", "./cmd/spawner"}, args...)
	}

	o.serverCmd = exec.CommandContext(ctx, spawnerCmd, args...)
	o.serverCmd.Stdout = os.Stdout
	o.serverCmd.Stderr = os.Stderr

	o.logger.Debug().
		Str("command", spawnerCmd).
		Strs("args", args).
		Msg("Starting spawner")

	if err := o.serverCmd.Start(); err != nil {
		return fmt.Errorf("failed to start spawner: %w", err)
	}

	// Wait for server to be ready
	time.Sleep(2 * time.Second)
	return nil
}

// startServerWithArgs starts the server with the given arguments
func (o *Orchestrator) startServerWithArgs(ctx context.Context, args []string, seed int64, hands int, numBots, numNPCs int) error {
	// Parse server command (supports both simple binary and complex commands like "go run ./cmd/server")
	serverCmdParts := strings.Fields(o.config.ServerCmd)
	if len(serverCmdParts) == 0 {
		return fmt.Errorf("server command is empty")
	}

	// Combine server command with server arguments
	allArgs := make([]string, 0, len(serverCmdParts)-1+len(args))
	allArgs = append(allArgs, serverCmdParts[1:]...)
	allArgs = append(allArgs, args...)
	o.serverCmd = exec.CommandContext(ctx, serverCmdParts[0], allArgs...)

	// Capture output for parsing stats later
	o.serverCmd.Stdout = os.Stdout // Show bot output with prefixes
	o.serverCmd.Stderr = os.Stderr

	o.logger.Info().
		Str("binary", serverCmdParts[0]).
		Int64("seed", seed).
		Int("hands", hands).
		Int("bots", numBots).
		Int("npcs", numNPCs).
		Msg("Starting server with bots")

	if err := o.serverCmd.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	// Server will manage everything - bots, hands, and clean exit
	return nil
}

// WaitForCompletion waits for the server to complete all hands
func (o *Orchestrator) WaitForCompletion(ctx context.Context) error {
	if o.serverCmd == nil || o.serverCmd.Process == nil {
		return fmt.Errorf("server not started")
	}

	done := make(chan error, 1)
	go func() {
		done <- o.serverCmd.Wait()
	}()

	select {
	case <-ctx.Done():
		o.logger.Info().Msg("Context cancelled, stopping server")
		o.StopServer()
		return ctx.Err()
	case err := <-done:
		if err != nil {
			return fmt.Errorf("server exited with error: %w", err)
		}
		o.logger.Info().Msg("Server completed successfully")
		return nil
	}
}

// StopServer stops the server
func (o *Orchestrator) StopServer() error {
	if o.serverCmd != nil && o.serverCmd.Process != nil {
		o.logger.Info().Msg("Stopping server")

		// Try graceful shutdown first
		o.serverCmd.Process.Signal(os.Interrupt)

		// Wait up to 5 seconds for graceful shutdown
		done := make(chan error, 1)
		go func() {
			done <- o.serverCmd.Wait()
		}()

		select {
		case <-time.After(5 * time.Second):
			// Force kill if not stopped
			o.logger.Warn().Msg("Force killing server")
			o.serverCmd.Process.Kill()
		case <-done:
			o.logger.Info().Msg("Server stopped gracefully")
		}
	}

	return nil
}

// parseStatsFile reads and parses the JSON stats file written by the server
// This is the single source of truth for parsing server statistics
func (o *Orchestrator) parseStatsFile(filename string) (*server.GameStats, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read stats file: %w", err)
	}

	o.logger.Debug().
		Int("json_bytes", len(data)).
		Str("filename", filename).
		Msg("Read stats file")

	var stats server.GameStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, fmt.Errorf("failed to parse stats JSON: %w", err)
	}

	o.logger.Debug().
		Int("players", len(stats.Players)).
		Uint64("hands", stats.HandsCompleted).
		Msg("Parsed stats JSON")

	return &stats, nil
}
