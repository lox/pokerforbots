package regression

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/lox/pokerforbots/internal/server"
	"github.com/lox/pokerforbots/sdk/spawner"
	"github.com/rs/zerolog"
)

// Orchestrator manages server and bot lifecycle for regression testing
// ProgressReporter receives progress updates during test execution
type ProgressReporter interface {
	OnBatchStart(batchNum int, totalBatches int, handsInBatch int)
	OnBatchComplete(batchNum int, handsCompleted int)
	OnHandsProgress(handsCompleted int, totalHands int)
}

type Orchestrator struct {
	config           *Config
	healthMonitor    *HealthMonitor
	logger           zerolog.Logger
	serverCmd        *exec.Cmd           // For legacy mode
	botSpawner       *spawner.BotSpawner // For spawner mode
	embeddedServer   *server.Server      // For embedded server mode
	serverListener   net.Listener        // For embedded server mode
	serverURL        string              // WebSocket URL for bots
	currentStatsFile string              // Current batch's stats file path
	progressReporter ProgressReporter    // Optional progress reporting
	handMonitor      server.HandMonitor  // Optional server hand monitor
}

// OrchestratorOption configures an Orchestrator
type OrchestratorOption func(*Orchestrator)

// WithOrchestratorProgressReporter sets a progress reporter
func WithOrchestratorProgressReporter(reporter ProgressReporter) OrchestratorOption {
	return func(o *Orchestrator) {
		o.progressReporter = reporter
	}
}

// WithOrchestratorHandMonitor sets a hand monitor
func WithOrchestratorHandMonitor(monitor server.HandMonitor) OrchestratorOption {
	return func(o *Orchestrator) {
		o.handMonitor = monitor
	}
}

// NewOrchestrator creates a new orchestrator with options
func NewOrchestrator(config *Config, healthMonitor *HealthMonitor, opts ...OrchestratorOption) *Orchestrator {
	orchestrator := &Orchestrator{
		config:        config,
		healthMonitor: healthMonitor,
		logger:        config.Logger,
	}

	// Apply options
	for _, opt := range opts {
		opt(orchestrator)
	}

	return orchestrator
}

// SetProgressReporter sets the progress reporter (used by Runner options)
func (o *Orchestrator) SetProgressReporter(reporter ProgressReporter) {
	o.progressReporter = reporter
}

// SetHandMonitor sets the hand monitor (used by Runner options)
func (o *Orchestrator) SetHandMonitor(monitor server.HandMonitor) {
	o.handMonitor = monitor
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

		// Notify progress reporter of batch start
		if o.progressReporter != nil {
			totalBatches := (totalHands + o.config.BatchSize - 1) / o.config.BatchSize
			o.progressReporter.OnBatchStart(batchNum+1, totalBatches, batchHands)
		}

		// Run the batch
		batch, err := o.runSingleBatch(ctx, strategy, batchConfig)
		if err != nil {
			return allBatches, fmt.Errorf("batch %d failed: %w", batchNum+1, err)
		}

		allBatches = append(allBatches, *batch)
		remainingHands -= batchHands
		totalHandsCompleted += batchHands

		// Notify progress reporter of batch completion
		if o.progressReporter != nil {
			o.progressReporter.OnBatchComplete(batchNum+1, totalHandsCompleted)
			o.progressReporter.OnHandsProgress(totalHandsCompleted, totalHands)
		}

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
	o.currentStatsFile = statsFile // Store for embedded server to use
	defer os.Remove(statsFile)     // Clean up after

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
	// Try embedded server mode first (fastest)
	if err := o.startEmbeddedServer(ctx, serverConfig); err == nil {
		return nil
	}

	// Fall back to legacy subprocess mode
	o.logger.Info().Msg("Using legacy server subprocess mode")
	args := serverConfig.BuildServerArgs(o.config)
	return o.startServerWithArgs(ctx, args, serverConfig.Seed, serverConfig.Hands,
		len(serverConfig.BotCommands), serverConfig.CountNPCs())
}

// startEmbeddedServer starts an embedded server with spawner for bot management
func (o *Orchestrator) startEmbeddedServer(ctx context.Context, serverConfig *ServerConfig) error {
	o.logger.Info().Msg("Starting embedded server with spawner library")

	// Create RNG with seed
	rng := rand.New(rand.NewSource(serverConfig.Seed))

	// Create server configuration
	srvConfig := server.Config{
		SmallBlind:  5,
		BigBlind:    10,
		StartChips:  o.config.StartingChips,
		Timeout:     time.Duration(o.config.TimeoutMs) * time.Millisecond,
		MinPlayers:  2,
		MaxPlayers:  9,
		HandLimit:   uint64(serverConfig.Hands),
		EnableStats: true,
	}

	// Create embedded server
	o.embeddedServer = server.NewServer(o.logger, rng, server.WithConfig(srvConfig))

	// Set hand monitor if available
	if o.handMonitor != nil {
		if err := o.embeddedServer.SetHandMonitor(o.handMonitor); err != nil {
			o.logger.Warn().Err(err).Msg("Failed to set hand monitor")
		}
	}

	// Find a free port
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return fmt.Errorf("failed to find free port: %w", err)
	}
	o.serverListener = listener

	// Start server in background
	go func() {
		err := o.embeddedServer.Serve(listener)
		// Only log actual errors, not normal shutdown
		if err != nil && err != http.ErrServerClosed {
			o.logger.Error().Err(err).Msg("Server stopped with error")
		}
	}()

	// Build WebSocket URL
	o.serverURL = fmt.Sprintf("ws://%s/ws", listener.Addr().String())
	o.logger.Info().Str("url", o.serverURL).Msg("Embedded server started")

	// Create bot spawner
	o.botSpawner = spawner.New(o.serverURL, o.logger)

	// Spawn bot processes
	var specs []spawner.BotSpec

	// Add external bot commands
	for _, botCmd := range serverConfig.BotCommands {
		parts := strings.Fields(botCmd)
		if len(parts) > 0 {
			spec := spawner.BotSpec{
				Command: parts[0],
				Args:    parts[1:],
				Count:   1,
				Env: map[string]string{
					"POKERFORBOTS_SEED": fmt.Sprintf("%d", serverConfig.Seed),
				},
			}
			specs = append(specs, spec)
		}
	}

	// Convert NPC config to bot specs
	if serverConfig.NPCConfig != "" {
		npcSpecs, err := o.parseNPCConfig(serverConfig.NPCConfig, serverConfig.Seed)
		if err != nil {
			return fmt.Errorf("failed to parse NPC config: %w", err)
		}
		specs = append(specs, npcSpecs...)
	}

	// Spawn all bots
	if err := o.botSpawner.Spawn(specs...); err != nil {
		return fmt.Errorf("failed to spawn bots: %w", err)
	}

	// Wait for bots to connect
	time.Sleep(2 * time.Second)

	return nil
}

// parseNPCConfig converts NPC configuration string to bot specs
func (o *Orchestrator) parseNPCConfig(npcConfig string, seed int64) ([]spawner.BotSpec, error) {
	var specs []spawner.BotSpec

	// Parse config like "aggressive:3,calling:2,random:1"
	for _, part := range strings.Split(npcConfig, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		strategyCount := strings.Split(part, ":")
		if len(strategyCount) != 2 {
			return nil, fmt.Errorf("invalid NPC config format: %q", part)
		}

		strategy := strings.TrimSpace(strategyCount[0])
		count := 0
		if _, err := fmt.Sscanf(strategyCount[1], "%d", &count); err != nil {
			return nil, fmt.Errorf("invalid count in NPC config: %q", part)
		}

		// Map strategy names to bot commands
		var command string
		var args []string

		switch strategy {
		case "calling", "calling-station", "callbot":
			command = "go"
			args = []string{"run", "./sdk/examples/calling-station"}
		case "aggressive", "aggro":
			command = "go"
			args = []string{"run", "./sdk/examples/aggressive"}
		case "random":
			command = "go"
			args = []string{"run", "./sdk/examples/random"}
		default:
			return nil, fmt.Errorf("unknown NPC strategy: %q", strategy)
		}

		spec := spawner.BotSpec{
			Command: command,
			Args:    args,
			Count:   count,
			Env: map[string]string{
				"POKERFORBOTS_SEED": fmt.Sprintf("%d", seed),
			},
		}
		specs = append(specs, spec)
	}

	return specs, nil
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
	// Handle embedded server mode
	if o.embeddedServer != nil {
		// Wait for the default game to complete (hand limit reached)
		select {
		case <-ctx.Done():
			o.logger.Info().Msg("Context cancelled")
			return ctx.Err()
		case <-o.embeddedServer.DefaultGameDone():
			o.logger.Info().Msg("Server completed successfully")
			// Write stats to file for later parsing
			if err := o.writeEmbeddedServerStats(); err != nil {
				return fmt.Errorf("failed to write stats: %w", err)
			}
			return nil
		}
	}

	// Handle legacy subprocess mode
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
	// Stop spawned bots first
	if o.botSpawner != nil {
		o.logger.Info().Msg("Stopping spawned bots")
		if err := o.botSpawner.StopAll(); err != nil {
			o.logger.Warn().Err(err).Msg("Failed to stop some bots")
		}
	}

	// Stop embedded server
	if o.embeddedServer != nil {
		o.logger.Info().Msg("Shutting down embedded server")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := o.embeddedServer.Shutdown(ctx); err != nil {
			o.logger.Warn().Err(err).Msg("Failed to shutdown server gracefully")
		}
		if o.serverListener != nil {
			o.serverListener.Close()
		}
		return nil
	}

	// Stop legacy subprocess server
	if o.serverCmd != nil && o.serverCmd.Process != nil {
		o.logger.Info().Msg("Stopping server subprocess")

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

// writeEmbeddedServerStats fetches stats from the embedded server and writes to file
func (o *Orchestrator) writeEmbeddedServerStats() error {
	if o.serverURL == "" || o.embeddedServer == nil {
		return fmt.Errorf("no embedded server running")
	}

	// Get the stats file path from server config (stored during runSingleBatch)
	statsFile := o.currentStatsFile
	if statsFile == "" {
		return fmt.Errorf("no stats file configured")
	}

	// Convert WebSocket URL to HTTP URL
	baseURL := strings.Replace(o.serverURL, "ws://", "http://", 1)
	baseURL = strings.Replace(baseURL, "/ws", "", 1)
	url := fmt.Sprintf("%s/admin/games/default/stats", baseURL)

	// Fetch stats from HTTP API
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch stats: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch stats: status %d", resp.StatusCode)
	}

	// Read the JSON response
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read stats: %w", err)
	}

	// Write to file
	if err := os.WriteFile(statsFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write stats file: %w", err)
	}

	o.logger.Info().Str("file", statsFile).Msg("Stats written to file")
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
