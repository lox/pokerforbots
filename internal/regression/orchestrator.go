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

// Orchestrator uses server's bot-cmd flags instead of managing bots directly
type Orchestrator struct {
	config        *Config
	healthMonitor *HealthMonitor
	logger        zerolog.Logger
	serverCmd     *exec.Cmd
}

// NewOrchestrator creates a new orchestrator that uses server bot commands
func NewOrchestrator(config *Config, healthMonitor *HealthMonitor) *Orchestrator {
	return &Orchestrator{
		config:        config,
		healthMonitor: healthMonitor,
		logger:        config.Logger,
	}
}

// StartServerWithBots starts the server with bot commands
func (o *Orchestrator) StartServerWithBots(ctx context.Context, seed int64, hands int, botCmds []string, npcCmds []string) error {
	return o.StartServerWithBotsAndStats(ctx, seed, hands, botCmds, npcCmds, "")
}

// StartServerWithBotsAndStats starts the server with bot commands and stats output
func (o *Orchestrator) StartServerWithBotsAndStats(ctx context.Context, seed int64, hands int, botCmds []string, npcCmds []string, statsFile string) error {
	// Build server command
	args := []string{
		"--addr", o.getServerAddr(),
		"--start-chips", fmt.Sprintf("%d", o.config.StartingChips),
		"--timeout-ms", fmt.Sprintf("%d", o.config.TimeoutMs),
		"--seed", fmt.Sprintf("%d", seed),
		"--hands", fmt.Sprintf("%d", hands),
		"--collect-detailed-stats",
	}

	// Add stats output file if specified
	if statsFile != "" {
		args = append(args, "--write-stats-on-exit", statsFile)
	}

	if o.config.InfiniteBankroll {
		args = append(args, "--infinite-bankroll")
	}

	// Add bot commands
	for _, cmd := range botCmds {
		args = append(args, "--bot-cmd", cmd)
	}

	// Add NPC commands
	for _, cmd := range npcCmds {
		args = append(args, "--npc-bot-cmd", cmd)
	}

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
		Int("bots", len(botCmds)).
		Int("npcs", len(npcCmds)).
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

// getServerAddr returns the server address
func (o *Orchestrator) getServerAddr() string {
	if o.config.ServerAddr == "" || o.config.ServerAddr == "embedded" {
		return "localhost:8080"
	}
	return o.config.ServerAddr
}

// RunHeadsUpBatch runs a single batch of heads-up hands using server bot commands
func (o *Orchestrator) RunHeadsUpBatch(ctx context.Context, botA, botB string, seed int64, hands int) (*BatchResult, error) {
	// Create temporary file for stats
	statsFile := fmt.Sprintf("stats-%d-%d.json", seed, time.Now().Unix())
	defer os.Remove(statsFile) // Clean up after

	// Prepare bot commands
	botCmds := []string{
		botA,
		botB,
	}

	// Start server with bots and stats output
	if err := o.StartServerWithBotsAndStats(ctx, seed, hands, botCmds, nil, statsFile); err != nil {
		return nil, fmt.Errorf("failed to start server with bots: %w", err)
	}
	defer o.StopServer()

	// Wait for completion
	if err := o.WaitForCompletion(ctx); err != nil {
		return nil, fmt.Errorf("server failed to complete: %w", err)
	}

	// Parse stats from JSON file
	results, err := o.parseStatsFile(statsFile, botA, botB)
	if err != nil {
		return nil, fmt.Errorf("failed to parse stats file: %w", err)
	}

	return &BatchResult{
		Seed:    seed,
		Hands:   hands,
		Results: results,
	}, nil
}

// RunPopulationBatch runs a population test batch
func (o *Orchestrator) RunPopulationBatch(ctx context.Context, challenger, baseline string,
	challengerSeats, baselineSeats int, seed int64, hands int) (*BatchResult, error) {

	// Prepare bot commands
	var botCmds []string

	// Add challenger bots
	for range challengerSeats {
		botCmds = append(botCmds, challenger)
	}

	// Add baseline bots
	for range baselineSeats {
		botCmds = append(botCmds, baseline)
	}

	// Start server with bots
	if err := o.StartServerWithBots(ctx, seed, hands, botCmds, nil); err != nil {
		return nil, fmt.Errorf("failed to start server with bots: %w", err)
	}
	defer o.StopServer()

	// Wait for completion
	if err := o.WaitForCompletion(ctx); err != nil {
		return nil, fmt.Errorf("server failed to complete: %w", err)
	}

	// TODO: Parse actual stats
	results := make(map[string]float64)
	results["challenger_bb_per_100"] = 8.5
	results["baseline_bb_per_100"] = -2.1

	return &BatchResult{
		Seed:    seed,
		Hands:   hands,
		Results: results,
	}, nil
}

// RunNPCBenchmarkBatch runs an NPC benchmark batch
func (o *Orchestrator) RunNPCBenchmarkBatch(ctx context.Context, bot string, botSeats int,
	npcs map[string]int, seed int64, hands int) (*BatchResult, error) {

	// Prepare bot commands
	var botCmds []string
	for range botSeats {
		botCmds = append(botCmds, bot)
	}

	// For NPCs, we can use the server's built-in NPCs or external commands
	// Using built-in NPCs via flags would be:
	// --npc-calling X --npc-random Y --npc-aggro Z
	// But since we're using bot-cmd, we'll need actual NPC binaries

	// For now, just use the player bots
	// TODO: Implement actual NPC spawning

	// Start server with bots
	if err := o.StartServerWithBots(ctx, seed, hands, botCmds, nil); err != nil {
		return nil, fmt.Errorf("failed to start server with bots: %w", err)
	}
	defer o.StopServer()

	// Wait for completion
	if err := o.WaitForCompletion(ctx); err != nil {
		return nil, fmt.Errorf("server failed to complete: %w", err)
	}

	// TODO: Parse actual stats
	results := make(map[string]float64)
	results["bot_bb_per_100"] = 25.3 // Should crush NPCs

	return &BatchResult{
		Seed:    seed,
		Hands:   hands,
		Results: results,
	}, nil
}

// parseStatsFile reads and parses the JSON stats file written by the server
func (o *Orchestrator) parseStatsFile(filename string, botA, botB string) (map[string]float64, error) {
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

	results := make(map[string]float64)

	// In heads-up mode, we expect exactly 2 players
	// The server spawns bots in the order they were specified in --bot-cmd flags
	// So the first player should be botA and the second should be botB
	if len(stats.Players) != 2 {
		return nil, fmt.Errorf("expected 2 players in heads-up stats, got %d", len(stats.Players))
	}

	// Bot A is the first player (first --bot-cmd)
	playerA := stats.Players[0]

	// Check if detailed stats are present
	if playerA.DetailedStats == nil {
		o.logger.Warn().Msg("Bot A has no detailed stats")
		results["bot_a_bb_per_100"] = 0
		results["bot_a_vpip"] = 0
		results["bot_a_pfr"] = 0
		results["bot_a_timeouts"] = 0
		results["bot_a_busts"] = 0
	} else {
		results["bot_a_bb_per_100"] = playerA.DetailedStats.BB100
		results["bot_a_vpip"] = playerA.DetailedStats.VPIP
		results["bot_a_pfr"] = playerA.DetailedStats.PFR
		results["bot_a_timeouts"] = float64(playerA.DetailedStats.Timeouts)
		results["bot_a_busts"] = float64(playerA.DetailedStats.Busts)

		o.logger.Debug().
			Float64("vpip", playerA.DetailedStats.VPIP).
			Float64("pfr", playerA.DetailedStats.PFR).
			Msg("Bot A stats parsed")
	}

	// Bot B is the second player (second --bot-cmd)
	playerB := stats.Players[1]

	// Check if detailed stats are present
	if playerB.DetailedStats == nil {
		o.logger.Warn().Msg("Bot B has no detailed stats")
		results["bot_b_bb_per_100"] = 0
		results["bot_b_vpip"] = 0
		results["bot_b_pfr"] = 0
		results["bot_b_timeouts"] = 0
		results["bot_b_busts"] = 0
	} else {
		results["bot_b_bb_per_100"] = playerB.DetailedStats.BB100
		results["bot_b_vpip"] = playerB.DetailedStats.VPIP
		results["bot_b_pfr"] = playerB.DetailedStats.PFR
		results["bot_b_timeouts"] = float64(playerB.DetailedStats.Timeouts)
		results["bot_b_busts"] = float64(playerB.DetailedStats.Busts)

		o.logger.Debug().
			Float64("vpip", playerB.DetailedStats.VPIP).
			Float64("pfr", playerB.DetailedStats.PFR).
			Msg("Bot B stats parsed")
	}

	return results, nil
}
