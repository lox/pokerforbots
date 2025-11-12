package main

import (
	"github.com/lox/pokerforbots/v2/internal/randutil"

	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lox/pokerforbots/v2/cmd/pokerforbots/shared"
	"github.com/lox/pokerforbots/v2/internal/fileutil"
	"github.com/lox/pokerforbots/v2/internal/server"
	"github.com/lox/pokerforbots/v2/sdk/spawner"
	"github.com/rs/zerolog"
)

type SpawnCmd struct {
	// Server configuration
	Addr            string `kong:"default='localhost:0',help='Server address, defaults to random port on localhost'"`
	SmallBlind      int    `kong:"default='5',help='Small blind'"`
	BigBlind        int    `kong:"default='10',help='Big blind'"`
	StartChips      int    `kong:"default='1000',help='Starting chip stack'"`
	TimeoutMs       int    `kong:"default='100',help='Bot decision timeout in milliseconds'"`
	MinPlayers      int    `kong:"default='0',help='Minimum players to start a hand (0 = auto, matches bot count)'"`
	MaxPlayers      int    `kong:"default='9',help='Maximum players at a table'"`
	Seed            int64  `kong:"help='Seed for deterministic testing (0 for random)'"`
	LatencyTracking bool   `kong:"help='Collect per-action latency metrics (adds overhead)'"`

	// Bot specification
	Spec   string   `kong:"default='calling-station:6',help='Bot specification (e.g. calling-station:2,random:1,aggressive:3)'"`
	BotCmd []string `kong:"help='Additional bot command to spawn (can be specified multiple times)'"`
	Count  int      `kong:"default='1',help='Number of each --bot-cmd to spawn'"`

	// Game control
	HandLimit        int  `kong:"help='Stop after N hands (0 for unlimited)'"`
	InfiniteBankroll bool `kong:"help='Players never bust out (always have chips to rebuy)'"`

	// Stats output
	WriteStats string `kong:"help='Write stats to file on exit'"`
	PrintStats bool   `kong:"help='Print stats on exit'"`

	// Output format
	Output string `kong:"default='logs',enum='logs,bot-cmd,hand-history',help='Output format: logs (all logs), bot-cmd (only custom bot logs), hand-history (pretty hand visualization)'"`

	// Logging
	LogLevel string `kong:"help='Log level (debug|info|warn|error)'"`
}

func (c *SpawnCmd) Run() error {
	// Setup logging
	level := zerolog.InfoLevel
	switch c.LogLevel {
	case "debug":
		level = zerolog.DebugLevel
	case "info":
		level = zerolog.InfoLevel
	case "warn":
		level = zerolog.WarnLevel
	case "error":
		level = zerolog.ErrorLevel
	}

	// In hand-history mode, suppress most logs unless explicitly set
	if c.Output == "hand-history" && c.LogLevel == "" {
		level = zerolog.WarnLevel
	}

	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
		Level(level).
		With().Timestamp().Logger()

	ctx := shared.SetupSignalHandlerWithLogger(logger)

	// Configure server seed early
	seed := c.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := randutil.New(seed)

	// First, start the server to get the WebSocket URL
	// Start server on random port
	listener, err := net.Listen("tcp", c.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	wsURL := fmt.Sprintf("ws://%s/ws", listener.Addr())

	// Now parse bot specifications with the WebSocket URL
	specs, err := parseSpecString(c.Spec, wsURL, c.Output)
	if err != nil {
		return fmt.Errorf("failed to parse spec: %w", err)
	}

	// Add any additional bots specified via --bot-cmd
	// Note: We don't append wsURL to custom bot commands as they should
	// read POKERFORBOTS_SERVER from environment (set by spawner)
	for _, botCmd := range c.BotCmd {
		parts := strings.Fields(botCmd)
		if len(parts) == 0 {
			continue
		}
		// Custom bot logs are suppressed only in hand-history mode
		specs = append(specs, spawner.BotSpec{
			Command:   parts[0],
			Args:      parts[1:], // Don't append wsURL - bot should use POKERFORBOTS_SERVER
			Count:     c.Count,
			QuietLogs: c.Output == "hand-history",
		})
	}

	if len(specs) == 0 {
		return fmt.Errorf("no bots specified (use --spec to specify bots)")
	}

	// Calculate total number of bots
	totalBots := 0
	for _, spec := range specs {
		totalBots += spec.Count
	}

	// If MinPlayers is 0 (auto), set it to match bot count but at least 2
	minPlayers := c.MinPlayers
	if minPlayers == 0 {
		minPlayers = max(2, min(totalBots, c.MaxPlayers))
		if c.Output != "hand-history" {
			logger.Info().Int("min_players", minPlayers).Int("total_bots", totalBots).Msg("Auto-setting min-players to match bot count")
		}
	}

	// Configure server
	serverCfg := server.Config{
		SmallBlind:            c.SmallBlind,
		BigBlind:              c.BigBlind,
		StartChips:            c.StartChips,
		Timeout:               time.Duration(c.TimeoutMs) * time.Millisecond,
		MinPlayers:            minPlayers,
		MaxPlayers:            c.MaxPlayers,
		Seed:                  seed, // Propagate seed to server config
		HandLimit:             uint64(c.HandLimit),
		InfiniteBankroll:      c.InfiniteBankroll,
		EnableStats:           c.WriteStats != "" || c.PrintStats,
		MaxStatsHands:         10000,
		EnableLatencyTracking: c.LatencyTracking,
	}

	// Create and start server
	srv := server.NewServer(logger, rng, server.WithConfig(serverCfg))

	// Start server in background
	serverErr := make(chan error, 1)
	go func() {
		if err := srv.Serve(listener); err != nil {
			serverErr <- err
		}
	}()

	// Wait for server to be ready
	serverURL := fmt.Sprintf("http://%s", listener.Addr())
	if err := waitForHealthy(ctx, serverURL); err != nil {
		return fmt.Errorf("server failed to start: %w", err)
	}

	// Set up hand-history monitor if requested
	if c.Output == "hand-history" {
		monitor := server.NewPrettyPrintMonitor(os.Stdout)
		srv.SetHandMonitor(monitor)
	} else {
		logger.Info().Str("url", wsURL).Msg("Server started")
		if c.Seed != 0 {
			logger.Info().Int64("seed", c.Seed).Msg("Using deterministic seed")
		}
	}

	if c.Output != "hand-history" {
		logger.Info().Str("spec", c.Spec).Int("additional", len(c.BotCmd)).Int("total_bots", totalBots).Msg("Spawning bots")
	}

	// Create spawner and spawn bots
	// Note: Per-bot quiet logs are handled by BotSpec.QuietLogs, not here
	spawnerLogger := logger

	botSpawner := spawner.NewWithSeed(wsURL, spawnerLogger, seed)
	defer botSpawner.StopAll()

	if err := botSpawner.Spawn(specs...); err != nil {
		return fmt.Errorf("failed to spawn bots: %w", err)
	}

	if c.Output != "hand-history" {
		logger.Info().Int("count", botSpawner.ActiveCount()).Msg("Bots spawned")
	}

	// Monitor bot processes for unexpected exits
	botErr := make(chan error, 1)
	go func() {
		// Get all spawned processes
		processes := botSpawner.GetAllProcesses()

		// Wait for any process to exit (clean or error)
		for _, proc := range processes {
			go func(p *spawner.Process) {
				err := p.Wait()
				// Bot exited (with or without error) - signal shutdown
				// We shouldn't have bots exiting before normal shutdown
				if err != nil {
					select {
					case botErr <- fmt.Errorf("bot %s exited unexpectedly: %w", p.ID, err):
					default:
					}
				} else {
					select {
					case botErr <- fmt.Errorf("bot %s exited unexpectedly (clean exit)", p.ID):
					default:
					}
				}
			}(proc)
		}
	}()

	// Wait for context cancellation, hand limit, server error, or bot failure
	select {
	case <-ctx.Done():
		logger.Info().Msg("Shutting down...")
	case <-srv.DefaultGameDone():
		if c.Output != "hand-history" {
			logger.Info().Msg("Hand limit reached")
		}
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case err := <-botErr:
		return fmt.Errorf("bot failure: %w", err)
	}

	// Get metrics for final logging
	if metrics, ok := srv.DefaultGameMetrics(); ok {
		var duration time.Duration
		if !metrics.StartTime.IsZero() && !metrics.EndTime.IsZero() && metrics.EndTime.After(metrics.StartTime) {
			duration = metrics.EndTime.Sub(metrics.StartTime)
		}

		event := logger.Info().
			Uint64("hands_completed", metrics.HandsCompleted).
			Float64("hands_per_second", metrics.HandsPerSecond)
		if c.HandLimit > 0 {
			event = event.Int("hand_limit", c.HandLimit)
		}
		if duration > 0 {
			event = event.Dur("duration", duration)
		}
		event.Msg("Run performance")
	}

	// Write stats if requested
	if c.WriteStats != "" || c.PrintStats {
		handleStatsOutput(listener.Addr().String(), c.WriteStats, c.PrintStats, logger)
	}

	// Give bots a moment to write their own stats files before stopping them
	// This ensures external bots (like Aragorn) can flush their stats to disk
	time.Sleep(100 * time.Millisecond)

	// Stop bots (will be done by defer)
	if c.Output != "hand-history" {
		logger.Info().Msg("Stopping bots...")
	}

	return nil
}

// parseSpecString parses a specification string like "calling-station:2,random:1,aggressive:3"
func parseSpecString(spec string, wsURL string, outputMode string) ([]spawner.BotSpec, error) {
	if spec == "" {
		return nil, nil
	}

	var specs []spawner.BotSpec

	// Get path to our own binary for running bots
	pokerforbotsBin, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}

	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		strategyCount := strings.Split(part, ":")
		if len(strategyCount) != 2 {
			return nil, fmt.Errorf("invalid spec format: %q (expected strategy:count)", part)
		}

		strategy := strings.TrimSpace(strategyCount[0])
		countStr := strings.TrimSpace(strategyCount[1])

		count, err := strconv.Atoi(countStr)
		if err != nil || count <= 0 {
			return nil, fmt.Errorf("invalid count for strategy %q: %q", strategy, countStr)
		}

		// Check if it's a built-in bot or a custom command
		var command string
		var args []string
		var isBuiltIn bool

		if strings.Contains(strategy, "/") || strings.Contains(strategy, ".") {
			// Custom command (e.g., "./my-bot")
			fields := strings.Fields(strategy)
			if len(fields) == 0 {
				return nil, fmt.Errorf("invalid command in strategy: %q", strategy)
			}
			command = fields[0]
			args = fields[1:]
			args = append(args, wsURL)
			isBuiltIn = false
		} else {
			// Built-in bot - use pokerforbots bot subcommand
			command = pokerforbotsBin
			args = []string{"bot", strategy, "--server", wsURL}
			isBuiltIn = true
		}

		// Determine if logs should be suppressed for this bot
		quietLogs := false
		if isBuiltIn {
			// Suppress built-in bot logs in bot-cmd and hand-history modes
			quietLogs = (outputMode == "bot-cmd" || outputMode == "hand-history")
		} else {
			// Suppress custom bot logs only in hand-history mode
			quietLogs = (outputMode == "hand-history")
		}

		specs = append(specs, spawner.BotSpec{
			Command:   command,
			Args:      args,
			Count:     count,
			QuietLogs: quietLogs,
		})
	}

	return specs, nil
}

func waitForHealthy(ctx context.Context, baseURL string) error {
	healthURL := fmt.Sprintf("%s/health", baseURL)
	client := &http.Client{Timeout: 100 * time.Millisecond}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			return err
		}

		resp, err := client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}

	return fmt.Errorf("server failed to become healthy within timeout")
}

func handleStatsOutput(addr, statsFile string, printStats bool, logger zerolog.Logger) {
	baseURL := fmt.Sprintf("http://%s", addr)
	url := fmt.Sprintf("%s/admin/games/default/stats", baseURL)

	// Fetch JSON stats
	resp, err := http.Get(url)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to fetch stats")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Error().Int("status", resp.StatusCode).Msg("Failed to fetch stats")
		return
	}

	// Read the JSON response
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to read stats")
		return
	}

	// Write to file if requested (using atomic write to avoid races)
	if statsFile != "" {
		if err := fileutil.WriteFileAtomic(statsFile, data, 0644); err != nil {
			logger.Error().Err(err).Str("file", statsFile).Msg("Failed to write stats file")
			return
		}
		logger.Info().Str("file", statsFile).Msg("Stats written to file")
	}

	// Pretty print stats if requested
	if printStats {
		printFormattedStats(data, logger)
	}
}

// GameStats represents the JSON structure from the server
type GameStats struct {
	ID               string   `json:"id"`
	SmallBlind       int      `json:"small_blind"`
	BigBlind         int      `json:"big_blind"`
	StartChips       int      `json:"start_chips"`
	HandsCompleted   uint64   `json:"hands_completed"`
	HandLimit        uint64   `json:"hand_limit"`
	HandsPerSecond   float64  `json:"hands_per_second"`
	DurationSeconds  float64  `json:"duration_seconds"`
	CompletionReason string   `json:"completion_reason"`
	Players          []Player `json:"players"`
}

// Player represents a player's statistics
type Player struct {
	BotID         string         `json:"bot_id"`
	DisplayName   string         `json:"display_name"`
	Hands         int            `json:"hands"`
	NetChips      int            `json:"net_chips"`
	AvgPerHand    float64        `json:"avg_per_hand"`
	DetailedStats *DetailedStats `json:"detailed_stats,omitempty"`
}

// DetailedStats contains detailed player statistics
type DetailedStats struct {
	Hands            int     `json:"hands"`
	NetBB            float64 `json:"net_bb"`
	BBPer100         float64 `json:"bb_per_100"`
	Mean             float64 `json:"mean"`
	Median           float64 `json:"median"`
	StdDev           float64 `json:"std_dev"`
	CI95Low          float64 `json:"ci_95_low"`
	CI95High         float64 `json:"ci_95_high"`
	WinningHands     int     `json:"winning_hands"`
	WinRate          float64 `json:"win_rate"`
	ShowdownWins     int     `json:"showdown_wins"`
	NonShowdownWins  int     `json:"non_showdown_wins"`
	ShowdownWinRate  float64 `json:"showdown_win_rate"`
	ShowdownBB       float64 `json:"showdown_bb"`
	NonShowdownBB    float64 `json:"non_showdown_bb"`
	VPIP             float64 `json:"vpip"`
	PFR              float64 `json:"pfr"`
	ResponsesTracked int     `json:"responses_tracked"`
	AvgResponseMs    float64 `json:"avg_response_ms"`
	P95ResponseMs    float64 `json:"p95_response_ms"`
}

func printFormattedStats(data []byte, logger zerolog.Logger) {
	var stats GameStats
	if err := json.Unmarshal(data, &stats); err != nil {
		logger.Error().Err(err).Msg("Failed to parse stats")
		return
	}

	fmt.Println("\n=== Game Statistics ===")
	fmt.Printf("Hands Completed: %d", stats.HandsCompleted)
	if stats.HandLimit > 0 {
		fmt.Printf(" (limit: %d)", stats.HandLimit)
	}
	fmt.Println()

	if stats.HandsPerSecond > 0 {
		fmt.Printf("Hands/Second: %.1f\n", stats.HandsPerSecond)
	}
	if stats.DurationSeconds > 0 {
		fmt.Printf("Duration: %.1fs\n", stats.DurationSeconds)
	}

	if len(stats.Players) > 0 {
		fmt.Println("\n=== Player Rankings ===")

		// Sort players by net chips (descending)
		sort.Slice(stats.Players, func(i, j int) bool {
			return stats.Players[i].NetChips > stats.Players[j].NetChips
		})

		for i, p := range stats.Players {
			fmt.Printf("%d. %s: %+d chips", i+1, p.DisplayName, p.NetChips)
			if p.AvgPerHand != 0 {
				fmt.Printf(" (%.1f/hand)", p.AvgPerHand)
			}
			if p.DetailedStats != nil && p.DetailedStats.BBPer100 != 0 {
				fmt.Printf(" %.1f BB/100", p.DetailedStats.BBPer100)
			}
			fmt.Println()
		}
	}
}
