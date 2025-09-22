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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/lox/pokerforbots/internal/server"
	"github.com/lox/pokerforbots/internal/spawner"
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

	// Start embedded server
	srv, listener, err := startServer(logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to start server")
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	// Get WebSocket URL
	serverURL := fmt.Sprintf("ws://%s/ws", listener.Addr())
	logger.Info().Str("url", serverURL).Msg("Server started")

	// Create spawner
	var sp *spawner.BotSpawner
	if cli.Seed != 0 {
		sp = spawner.NewWithSeed(serverURL, logger, cli.Seed)
		logger.Info().Int64("seed", cli.Seed).Msg("Using deterministic seed")
	} else {
		sp = spawner.New(serverURL, logger)
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

	logger.Info().Str("spec", cli.Spec).Int("additional", len(cli.BotCmd)).Msg("Spawning bots")

	// Spawn bots
	if err := sp.SpawnMany(specs); err != nil {
		logger.Fatal().Err(err).Msg("Failed to spawn bots")
	}
	defer sp.StopAll()

	logger.Info().Int("count", sp.ActiveCount()).Msg("Bots spawned")

	// Wait for context cancellation or hand limit
	select {
	case <-ctx.Done():
	case <-srv.DefaultGameDone():
		logger.Info().Msg("Hand limit reached")
	}

	// Write stats if requested
	if cli.WriteStats != "" || cli.PrintStats {
		handleStatsOutput(listener.Addr().String(), cli.WriteStats, cli.PrintStats, logger)
	}

	// Stop bots (will be done by defer)
	logger.Info().Msg("Stopping bots...")
}

func startServer(logger zerolog.Logger) (*server.Server, net.Listener, error) {
	// Create server configuration
	seed := cli.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	rng := rand.New(rand.NewSource(seed))

	config := server.Config{
		SmallBlind:    cli.SmallBlind,
		BigBlind:      cli.BigBlind,
		StartChips:    cli.StartChips,
		Timeout:       time.Duration(cli.TimeoutMs) * time.Millisecond,
		MinPlayers:    2,
		MaxPlayers:    9,
		Seed:          seed,
		HandLimit:     uint64(cli.HandLimit),
		EnableStats:   cli.WriteStats != "" || cli.PrintStats,
		MaxStatsHands: 10000, // Default for regression testing
	}

	// Create server
	srv := server.NewServer(logger, rng, server.WithConfig(config))

	// Create listener
	listener, err := net.Listen("tcp", cli.Addr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create listener: %w", err)
	}

	// Start server in background
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.Serve(listener)
	}()

	// Wait for server to be ready
	baseURL := fmt.Sprintf("http://%s", listener.Addr())
	for i := 0; i < 50; i++ {
		resp, err := http.Get(baseURL + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return srv, listener, nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return nil, nil, fmt.Errorf("server failed to start")
}

// parseSpecString parses a specification string like "calling-station:2,random:1,aggressive:3"
func parseSpecString(spec string) ([]spawner.BotSpec, error) {
	if spec == "" {
		return nil, nil
	}

	var specs []spawner.BotSpec
	parts := strings.Split(spec, ",")

	for _, part := range parts {
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
		var stats any
		if err := json.Unmarshal(data, &stats); err != nil {
			logger.Error().Err(err).Msg("Failed to parse stats JSON")
			return
		}

		// Pretty print with indentation
		prettyJSON, err := json.MarshalIndent(stats, "", "  ")
		if err != nil {
			logger.Error().Err(err).Msg("Failed to format stats JSON")
			return
		}

		fmt.Println(string(prettyJSON))
	}
}
