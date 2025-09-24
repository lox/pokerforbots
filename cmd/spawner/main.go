package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/lox/pokerforbots/internal/server"
	"github.com/lox/pokerforbots/sdk/spawner"
	"github.com/rs/zerolog"
)

var cli struct {
	// Server configuration
	Addr       string `kong:"default='localhost:0',help='Server address, defaults to random port on localhost'"`
	SmallBlind int    `kong:"default='5',help='Small blind'"`
	BigBlind   int    `kong:"default='10',help='Big blind'"`
	StartChips int    `kong:"default='1000',help='Starting chip stack'"`
	TimeoutMs  int    `kong:"default='100',help='Bot decision timeout in milliseconds'"`
	Seed       int64  `kong:"help='Seed for deterministic testing (0 for random)'"`

	// Bot specification
	Spec   string   `kong:"default='calling-station:6',help='Bot specification (e.g. calling-station:2,random:1,aggressive:3)'"`
	BotCmd []string `kong:"help='Additional bot command to spawn (can be specified multiple times)'"`
	Count  int      `kong:"default='1',help='Number of each --bot-cmd to spawn'"`

	// Game control
	HandLimit int `kong:"help='Stop after N hands (0 for unlimited)'"`

	// Stats output
	WriteStats string `kong:"help='Write stats to file on exit'"`
	PrintStats bool   `kong:"help='Print stats on exit'"`

	// Display mode
	Pretty bool `kong:"help='Display hands in pretty format instead of logs'"`

	// Logging
	LogLevel string `kong:"help='Log level (debug|info|warn|error)'"`
}

func main() {
	kong.Parse(&cli)

	// Setup logging
	level := zerolog.InfoLevel
	switch cli.LogLevel {
	case "debug":
		level = zerolog.DebugLevel
	case "info":
		level = zerolog.InfoLevel
	case "warn":
		level = zerolog.WarnLevel
	case "error":
		level = zerolog.ErrorLevel
	}

	// In pretty mode, suppress most logs unless explicitly set
	if cli.Pretty && cli.LogLevel == "" {
		level = zerolog.WarnLevel
	}

	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
		Level(level).
		With().Timestamp().Logger()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		logger.Info().Msg("Shutting down...")
		cancel()
	}()

	// Configure and start embedded server with spawner
	seed := cli.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	serverCfg := server.Config{
		SmallBlind:    cli.SmallBlind,
		BigBlind:      cli.BigBlind,
		StartChips:    cli.StartChips,
		Timeout:       time.Duration(cli.TimeoutMs) * time.Millisecond,
		MinPlayers:    2,
		MaxPlayers:    9,
		Seed:          seed,
		HandLimit:     uint64(cli.HandLimit),
		EnableStats:   cli.WriteStats != "" || cli.PrintStats,
		MaxStatsHands: 10000,
	}

	// Start embedded server
	srv := server.NewServer(logger, rand.New(rand.NewSource(seed)), server.WithConfig(serverCfg))
	listener, err := net.Listen("tcp", cli.Addr)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to listen")
	}

	go srv.Serve(listener)
	defer srv.Shutdown(ctx)

	// Wait for server to be ready
	serverURL := fmt.Sprintf("http://%s", listener.Addr())
	if err := server.WaitForHealthy(ctx, serverURL); err != nil {
		logger.Fatal().Err(err).Msg("Server failed to start")
	}

	wsURL := fmt.Sprintf("ws://%s/ws", listener.Addr())

	// Set up pretty printer if requested
	if cli.Pretty {
		monitor := server.NewPrettyPrintMonitor(os.Stdout)
		if err := srv.SetHandMonitor(monitor); err != nil {
			logger.Error().Err(err).Msg("Failed to set pretty print monitor")
		}
	} else {
		logger.Info().Str("url", wsURL).Msg("Server started")
		if cli.Seed != 0 {
			logger.Info().Int64("seed", cli.Seed).Msg("Using deterministic seed")
		}
	}

	// Parse bot specifications
	specs, err := parseSpecString(cli.Spec)
	if err != nil {
		logger.Fatal().Err(err).Str("spec", cli.Spec).Msg("Failed to parse spec")
	}

	// Add any additional bots specified via --bot
	for _, botCmd := range cli.BotCmd {
		parts := strings.Fields(botCmd)
		if len(parts) == 0 {
			continue
		}
		specs = append(specs, spawner.BotSpec{
			Command: parts[0],
			Args:    parts[1:],
			Count:   cli.Count,
		})
	}

	if len(specs) == 0 {
		logger.Fatal().Msg("No bots specified (use --spec to specify bots)")
	}

	if !cli.Pretty {
		logger.Info().Str("spec", cli.Spec).Int("additional", len(cli.BotCmd)).Msg("Spawning bots")
	}

	// Create spawner and spawn bots
	spawnerLogger := logger
	if cli.Pretty {
		// Create a quiet logger for bot processes in pretty mode
		spawnerLogger = zerolog.New(io.Discard).Level(zerolog.Disabled)
	}
	botSpawner := spawner.NewWithSeed(wsURL, spawnerLogger, seed)
	defer botSpawner.StopAll()

	if err := botSpawner.Spawn(specs...); err != nil {
		logger.Fatal().Err(err).Msg("Failed to spawn bots")
	}

	if !cli.Pretty {
		logger.Info().Int("count", botSpawner.ActiveCount()).Msg("Bots spawned")
	}

	// Wait for context cancellation or hand limit
	select {
	case <-ctx.Done():
	case <-srv.DefaultGameDone():
		if !cli.Pretty {
			logger.Info().Msg("Hand limit reached")
		}
	}

	// Write stats if requested
	if cli.WriteStats != "" || cli.PrintStats {
		handleStatsOutput(listener.Addr().String(), cli.WriteStats, cli.PrintStats, logger)
	}

	// Stop bots (will be done by defer)
	if !cli.Pretty {
		logger.Info().Msg("Stopping bots...")
	}
}

// parseSpecString parses a specification string like "calling-station:2,random:1,aggressive:3"
func parseSpecString(spec string) ([]spawner.BotSpec, error) {
	if spec == "" {
		return nil, nil
	}

	var specs []spawner.BotSpec

	for part := range strings.SplitSeq(spec, ",") {
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

		// Map strategy names to bot commands
		var command string
		var args []string

		switch strategy {
		case "calling-station", "calling":
			command = "go"
			args = []string{"run", "./sdk/examples/calling-station"}
		case "random":
			command = "go"
			args = []string{"run", "./sdk/examples/random"}
		case "aggressive", "aggro":
			command = "go"
			args = []string{"run", "./sdk/examples/aggressive"}
		case "complex":
			command = "go"
			args = []string{"run", "./sdk/examples/complex"}
		default:
			// Support paths directly if they contain / or .
			if strings.Contains(strategy, "/") || strings.Contains(strategy, ".") {
				// Treat as a command/path
				fields := strings.Fields(strategy)
				if len(fields) > 0 {
					command = fields[0]
					args = fields[1:]
				} else {
					return nil, fmt.Errorf("invalid command in strategy: %q", strategy)
				}
			} else {
				return nil, fmt.Errorf("unknown strategy: %q (valid: calling-station, random, aggressive, complex)", strategy)
			}
		}

		specs = append(specs, spawner.BotSpec{
			Command: command,
			Args:    args,
			Count:   count,
		})
	}

	return specs, nil
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

	// Write to file if requested
	if statsFile != "" {
		if err := os.WriteFile(statsFile, data, 0644); err != nil {
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
	ID              string   `json:"id"`
	SmallBlind      int      `json:"small_blind"`
	BigBlind        int      `json:"big_blind"`
	StartChips      int      `json:"start_chips"`
	HandsCompleted  uint64   `json:"hands_completed"`
	HandLimit       uint64   `json:"hand_limit"`
	HandsPerSecond  float64  `json:"hands_per_second"`
	DurationSeconds float64  `json:"duration_seconds"`
	Players         []Player `json:"players"`
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
	Hands             int                     `json:"hands"`
	NetBB             float64                 `json:"net_bb"`
	BBPer100          float64                 `json:"bb_per_100"`
	Mean              float64                 `json:"mean"`
	Median            float64                 `json:"median"`
	StdDev            float64                 `json:"std_dev"`
	CI95Low           float64                 `json:"ci_95_low"`
	CI95High          float64                 `json:"ci_95_high"`
	WinningHands      int                     `json:"winning_hands"`
	WinRate           float64                 `json:"win_rate"`
	ShowdownWins      int                     `json:"showdown_wins"`
	NonShowdownWins   int                     `json:"non_showdown_wins"`
	ShowdownWinRate   float64                 `json:"showdown_win_rate"`
	ShowdownBB        float64                 `json:"showdown_bb"`
	NonShowdownBB     float64                 `json:"non_showdown_bb"`
	VPIP              float64                 `json:"vpip"`
	PFR               float64                 `json:"pfr"`
	PositionStats     map[string]PositionStat `json:"position_stats"`
	StreetStats       map[string]StreetStat   `json:"street_stats"`
	HandCategoryStats map[string]CategoryStat `json:"hand_category_stats"`
}

type PositionStat struct {
	Hands     int     `json:"hands"`
	NetBB     float64 `json:"net_bb"`
	BBPerHand float64 `json:"bb_per_hand"`
}

type StreetStat struct {
	HandsEnded int     `json:"hands_ended"`
	NetBB      float64 `json:"net_bb"`
	BBPerHand  float64 `json:"bb_per_hand"`
}

type CategoryStat struct {
	Hands     int     `json:"hands"`
	NetBB     float64 `json:"net_bb"`
	BBPerHand float64 `json:"bb_per_hand"`
}

func printFormattedStats(data []byte, logger zerolog.Logger) {
	var stats GameStats
	if err := json.Unmarshal(data, &stats); err != nil {
		logger.Error().Err(err).Msg("Failed to parse stats JSON")
		return
	}

	fmt.Println("\n=== GAME SUMMARY ===")
	fmt.Printf("Game ID: %s\n", stats.ID)
	fmt.Printf("Hands completed: %d", stats.HandsCompleted)
	if stats.HandLimit > 0 {
		fmt.Printf(" / %d (%.1f%%)", stats.HandLimit, float64(stats.HandsCompleted)/float64(stats.HandLimit)*100)
	}
	fmt.Println()

	if stats.DurationSeconds > 0 {
		fmt.Printf("Duration: %.1f seconds\n", stats.DurationSeconds)
		fmt.Printf("Hands per second: %.1f\n", stats.HandsPerSecond)
	}
	fmt.Printf("Stakes: %d/%d (BB = %d)\n", stats.SmallBlind, stats.BigBlind, stats.BigBlind)
	fmt.Println()

	// Sort players by net chips
	players := stats.Players
	sort.Slice(players, func(i, j int) bool {
		return players[i].NetChips > players[j].NetChips
	})

	fmt.Println("=== LEADERBOARD ===")
	for i, p := range players {
		prefix := "  "
		if i == 0 {
			prefix = "👑"
		}
		netBB := float64(p.NetChips) / float64(stats.BigBlind)
		fmt.Printf("%s %s: %+d chips (%+.1f BB, %.2f BB/hand)\n",
			prefix, p.DisplayName, p.NetChips, netBB, p.AvgPerHand)
	}
	fmt.Println()

	// Print detailed stats for each player if available
	for _, p := range players {
		if p.DetailedStats != nil {
			printPlayerDetails(p.DisplayName, p.DetailedStats)
		}
	}
}

func printPlayerDetails(name string, stats *DetailedStats) {
	fmt.Printf("=== %s DETAILS ===\n", strings.ToUpper(name))
	fmt.Printf("Hands played: %d\n", stats.Hands)
	fmt.Printf("Net result: %.2f BB (%.2f BB/100)\n", stats.NetBB, stats.BBPer100)
	fmt.Printf("Mean: %.4f BB/hand | Median: %.4f BB/hand\n", stats.Mean, stats.Median)
	fmt.Printf("Std Dev: %.4f BB | 95%% CI: [%.4f, %.4f]\n", stats.StdDev, stats.CI95Low, stats.CI95High)
	fmt.Println()

	// Win/loss breakdown
	fmt.Println("Win/Loss Breakdown:")
	fmt.Printf("  Winning hands: %d (%.1f%%)\n", stats.WinningHands, stats.WinRate)
	fmt.Printf("    Showdown wins: %d | Non-showdown wins: %d\n", stats.ShowdownWins, stats.NonShowdownWins)
	losingHands := stats.Hands - stats.WinningHands
	fmt.Printf("  Losing hands: %d (%.1f%%)\n", losingHands, 100-stats.WinRate)
	fmt.Printf("  VPIP: %.1f%% | PFR: %.1f%%\n", stats.VPIP*100, stats.PFR*100)
	fmt.Println()

	// Showdown analysis
	totalShowdowns := stats.ShowdownWins + (stats.Hands - stats.WinningHands - stats.NonShowdownWins)
	if totalShowdowns > 0 {
		fmt.Println("Showdown Analysis:")
		fmt.Printf("  Went to showdown: %d hands\n", totalShowdowns)
		fmt.Printf("  Showdown win rate: %.1f%%\n", stats.ShowdownWinRate)
		fmt.Printf("  Showdown BB: %.2f | Non-showdown BB: %.2f\n", stats.ShowdownBB, stats.NonShowdownBB)
		fmt.Println()
	}

	// Position analysis if available
	if len(stats.PositionStats) > 0 {
		fmt.Println("Position Analysis:")
		positions := []string{"Button", "Cutoff", "Middle", "Early", "SB", "BB"}
		for _, pos := range positions {
			if stat, ok := stats.PositionStats[pos]; ok && stat.Hands > 0 {
				fmt.Printf("  %s: %d hands, %.3f BB/hand (%.1f BB/100)\n",
					pos, stat.Hands, stat.BBPerHand, stat.BBPerHand*100)
			}
		}
		fmt.Println()
	}

	// Street analysis if available
	if len(stats.StreetStats) > 0 {
		fmt.Println("Street Analysis:")
		streets := []string{"preflop", "flop", "turn", "river"}
		for _, street := range streets {
			if stat, ok := stats.StreetStats[street]; ok && stat.HandsEnded > 0 {
				// Capitalize first letter of street name
				streetName := strings.ToUpper(street[:1]) + street[1:]
				fmt.Printf("  %s: %d hands ended, %.3f BB/hand\n",
					streetName, stat.HandsEnded, stat.BBPerHand)
			}
		}
		fmt.Println()
	}
}
