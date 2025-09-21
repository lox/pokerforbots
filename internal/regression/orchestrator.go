package regression

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
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

// StartServerWithBotsAndNPCs starts the server with bot commands and built-in NPCs
func (o *Orchestrator) StartServerWithBotsAndNPCs(ctx context.Context, seed int64, hands int, botCmds []string, npcConfig string, statsFile string) error {
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

	// Add NPCs configuration string
	if npcConfig != "" {
		args = append(args, "--npcs", npcConfig)
	}

	// Count NPCs from config string
	npcCount := 0
	if npcConfig != "" {
		parts := strings.SplitSeq(npcConfig, ",")
		for part := range parts {
			if colonPos := strings.Index(part, ":"); colonPos > 0 {
				if countStr := strings.TrimSpace(part[colonPos+1:]); countStr != "" {
					if count, err := strconv.Atoi(countStr); err == nil {
						npcCount += count
					}
				}
			}
		}
	}

	return o.startServerWithArgs(ctx, args, seed, hands, len(botCmds), npcCount)
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

	return o.startServerWithArgs(ctx, args, seed, hands, len(botCmds), len(npcCmds))
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

	// Create temporary file for stats
	statsFile := fmt.Sprintf("stats-population-%d-%d.json", seed, time.Now().Unix())
	defer os.Remove(statsFile) // Clean up after

	// Start server with bots and stats
	if err := o.StartServerWithBotsAndStats(ctx, seed, hands, botCmds, nil, statsFile); err != nil {
		return nil, fmt.Errorf("failed to start server with bots: %w", err)
	}
	defer o.StopServer()

	// Wait for completion
	if err := o.WaitForCompletion(ctx); err != nil {
		return nil, fmt.Errorf("server failed to complete: %w", err)
	}

	// Parse stats for population mode
	results, err := o.parseStatsFilePopulation(statsFile, challenger, baseline, challengerSeats, baselineSeats)
	if err != nil {
		return nil, fmt.Errorf("failed to parse stats file: %w", err)
	}

	return &BatchResult{
		Seed:    seed,
		Hands:   hands,
		Results: results,
	}, nil
}

// RunNPCBenchmarkBatch runs an NPC benchmark batch
func (o *Orchestrator) RunNPCBenchmarkBatch(ctx context.Context, bot string, botSeats int,
	npcs map[string]int, seed int64, hands int) (*BatchResult, error) {

	// Create temporary file for stats
	statsFile := fmt.Sprintf("stats-npc-%d-%d.json", seed, time.Now().Unix())
	defer os.Remove(statsFile) // Clean up after

	// Prepare bot commands
	var botCmds []string
	for range botSeats {
		botCmds = append(botCmds, bot)
	}

	// Build NPC configuration string from the map
	var npcParts []string
	for strategy, count := range npcs {
		if count > 0 {
			npcParts = append(npcParts, fmt.Sprintf("%s:%d", strategy, count))
		}
	}
	npcConfig := strings.Join(npcParts, ",")

	// Start server with bots and NPCs using the new --npcs flag
	if err := o.StartServerWithBotsAndNPCs(ctx, seed, hands, botCmds, npcConfig, statsFile); err != nil {
		return nil, fmt.Errorf("failed to start server with bots and NPCs: %w", err)
	}
	defer o.StopServer()

	// Wait for completion
	if err := o.WaitForCompletion(ctx); err != nil {
		return nil, fmt.Errorf("server failed to complete: %w", err)
	}

	// Parse stats from JSON file
	results, err := o.parseStatsFileNPC(statsFile, bot, npcs)
	if err != nil {
		return nil, fmt.Errorf("failed to parse stats file: %w", err)
	}

	return &BatchResult{
		Seed:    seed,
		Hands:   hands,
		Results: results,
	}, nil
}

// parseStatsFile reads and parses the JSON stats file written by the server
func (o *Orchestrator) parseStatsFile(filename string, _, _ string) (map[string]float64, error) {
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

// parseStatsFilePopulation reads and parses the JSON stats file for population mode
func (o *Orchestrator) parseStatsFilePopulation(filename string, _, _ string,
	challengerSeats, baselineSeats int) (map[string]float64, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read stats file: %w", err)
	}

	var stats server.GameStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, fmt.Errorf("failed to parse stats JSON: %w", err)
	}

	o.logger.Debug().
		Int("total_players", len(stats.Players)).
		Str("stats_file", filename).
		Msg("Parsing population stats")

	results := make(map[string]float64)

	// In population mode, we have multiple instances of each bot type
	// We need to aggregate stats for each type separately
	var challengerNetChips int64
	var challengerHands int
	var challengerVPIP, challengerPFR float64
	var challengerTimeouts, challengerBusts int
	challengerCount := 0

	var baselineNetChips int64
	var baselineHands int
	var baselineVPIP, baselinePFR float64
	var baselineTimeouts, baselineBusts int
	baselineCount := 0

	// The server spawns bots in the order they were specified
	// First challengerSeats players are challengers, next baselineSeats are baseline
	for i, player := range stats.Players {
		if i < challengerSeats {
			// This is a challenger bot
			challengerNetChips += player.NetChips
			challengerHands += player.Hands
			challengerCount++
			if player.DetailedStats != nil {
				challengerVPIP += player.DetailedStats.VPIP
				challengerPFR += player.DetailedStats.PFR
				challengerTimeouts += player.DetailedStats.Timeouts
				challengerBusts += player.DetailedStats.Busts
			}
		} else if i < challengerSeats+baselineSeats {
			// This is a baseline bot
			baselineNetChips += player.NetChips
			baselineHands += player.Hands
			baselineCount++
			if player.DetailedStats != nil {
				baselineVPIP += player.DetailedStats.VPIP
				baselinePFR += player.DetailedStats.PFR
				baselineTimeouts += player.DetailedStats.Timeouts
				baselineBusts += player.DetailedStats.Busts
			}
		}
	}

	// Calculate aggregate BB/100 for challenger
	bigBlind := float64(stats.BigBlind)
	if challengerHands > 0 && bigBlind > 0 {
		results["challenger_bb_per_100"] = (float64(challengerNetChips) / bigBlind) / float64(challengerHands) * 100
	} else {
		results["challenger_bb_per_100"] = 0
	}

	// Calculate aggregate BB/100 for baseline
	if baselineHands > 0 && bigBlind > 0 {
		results["baseline_bb_per_100"] = (float64(baselineNetChips) / bigBlind) / float64(baselineHands) * 100
	} else {
		results["baseline_bb_per_100"] = 0
	}

	// Average the strategy metrics across instances
	if challengerCount > 0 {
		results["challenger_vpip"] = challengerVPIP / float64(challengerCount)
		results["challenger_pfr"] = challengerPFR / float64(challengerCount)
		results["challenger_timeouts"] = float64(challengerTimeouts) / float64(challengerCount)
		results["challenger_busts"] = float64(challengerBusts) / float64(challengerCount)
	}

	if baselineCount > 0 {
		results["baseline_vpip"] = baselineVPIP / float64(baselineCount)
		results["baseline_pfr"] = baselinePFR / float64(baselineCount)
		results["baseline_timeouts"] = float64(baselineTimeouts) / float64(baselineCount)
		results["baseline_busts"] = float64(baselineBusts) / float64(baselineCount)
	}

	// Store hands for weighting
	results["challenger_hands"] = float64(challengerHands)
	results["baseline_hands"] = float64(baselineHands)

	o.logger.Debug().
		Float64("challenger_bb_per_100", results["challenger_bb_per_100"]).
		Float64("baseline_bb_per_100", results["baseline_bb_per_100"]).
		Int("challenger_seats", challengerSeats).
		Int("baseline_seats", baselineSeats).
		Msg("Population stats calculated")

	return results, nil
}

// parseStatsFileNPC reads and parses the JSON stats file for NPC benchmark mode
func (o *Orchestrator) parseStatsFileNPC(filename string, _ string, _ map[string]int) (map[string]float64, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read stats file: %w", err)
	}

	var stats server.GameStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, fmt.Errorf("failed to parse stats JSON: %w", err)
	}

	o.logger.Debug().
		Int("total_players", len(stats.Players)).
		Str("stats_file", filename).
		Msg("Parsing NPC benchmark stats")

	results := make(map[string]float64)

	// Aggregate all non-NPC bot stats (for multi-seat configurations)
	var totalNetChips int64
	var totalHands int
	var totalVPIP, totalPFR float64
	var totalTimeouts, totalBusts int
	botCount := 0

	for i, player := range stats.Players {
		if strings.HasPrefix(player.DisplayName, "npc-") {
			continue // Skip NPCs
		}
		// This is one of our test bot instances
		totalNetChips += player.NetChips
		totalHands += player.Hands
		botCount++

		o.logger.Debug().
			Int("player_index", i).
			Str("player_name", player.DisplayName).
			Int("hands", player.Hands).
			Int64("net_chips", player.NetChips).
			Msg("Found test bot stats")

		// Aggregate detailed stats if available
		if player.DetailedStats != nil {
			totalVPIP += player.DetailedStats.VPIP
			totalPFR += player.DetailedStats.PFR
			totalTimeouts += player.DetailedStats.Timeouts
			totalBusts += player.DetailedStats.Busts
		}
	}

	if botCount == 0 {
		return nil, fmt.Errorf("could not find test bot stats in file")
	}

	// Calculate aggregate BB/100 from total net chips and hands
	bigBlind := float64(stats.BigBlind)
	if totalHands > 0 && bigBlind > 0 {
		results["bot_bb_per_100"] = (float64(totalNetChips) / bigBlind) / float64(totalHands) * 100
	} else {
		results["bot_bb_per_100"] = 0
	}

	// Average the strategy metrics across bot instances
	if botCount > 0 {
		results["bot_vpip"] = totalVPIP / float64(botCount)
		results["bot_pfr"] = totalPFR / float64(botCount)
		results["bot_timeouts"] = float64(totalTimeouts) / float64(botCount)
		results["bot_busts"] = float64(totalBusts) / float64(botCount)
	}

	// Basic metrics
	results["bot_hands"] = float64(totalHands)
	results["bot_net_chips"] = float64(totalNetChips)
	results["bot_seats"] = float64(botCount)

	// Calculate aggregate NPC performance for comparison
	var npcNetChipsSum int64
	var npcHandsSum int
	var npcCount int
	for _, player := range stats.Players {
		if strings.HasPrefix(player.DisplayName, "npc-") {
			npcNetChipsSum += player.NetChips
			npcHandsSum += player.Hands
			npcCount++
		}
	}

	if npcHandsSum > 0 && bigBlind > 0 {
		results["npc_avg_bb_per_100"] = (float64(npcNetChipsSum) / bigBlind) / float64(npcHandsSum) * 100
	} else {
		results["npc_avg_bb_per_100"] = 0
	}

	o.logger.Debug().
		Int("npc_count", npcCount).
		Float64("bot_bb_per_100", results["bot_bb_per_100"]).
		Float64("npc_avg_bb_per_100", results["npc_avg_bb_per_100"]).
		Msg("NPC benchmark stats calculated")

	return results, nil
}
