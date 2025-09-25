package main

import (
	"context"
	"errors"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/lox/pokerforbots/internal/server"
	"github.com/rs/zerolog"
)

// SimpleCLI contains only core server configuration
type CLI struct {
	Addr            string `kong:"default=':8080',help='Server address'"`
	Debug           bool   `kong:"help='Enable debug logging'"`
	SmallBlind      int    `kong:"default='5',help='Small blind amount'"`
	BigBlind        int    `kong:"default='10',help='Big blind amount'"`
	StartChips      int    `kong:"default='1000',help='Starting chip count'"`
	TimeoutMs       int    `kong:"default='100',help='Decision timeout in milliseconds'"`
	MinPlayers      int    `kong:"default='2',help='Minimum players per hand'"`
	MaxPlayers      int    `kong:"default='9',help='Maximum players per hand'"`
	Seed            *int64 `kong:"help='Deterministic RNG seed for the server (optional)'"`
	EnableStats     bool   `kong:"help='Enable statistics collection'"`
	MaxStatsHands   int    `kong:"default='10000',help='Maximum hands to track in statistics (memory limit)'"`
	LatencyTracking bool   `kong:"help='Collect per-action latency metrics'"`
}

func main() {
	var cli CLI
	ctx := kong.Parse(&cli,
		kong.Name("pokerforbots-server"),
		kong.Description("High-performance poker server for bot-vs-bot play"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
	)

	// Configure zerolog for pretty console output
	level := zerolog.InfoLevel
	if cli.Debug {
		level = zerolog.DebugLevel
	}
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
		Level(level).
		With().
		Timestamp().
		Logger()

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create server configuration
	config := server.Config{
		SmallBlind:            cli.SmallBlind,
		BigBlind:              cli.BigBlind,
		StartChips:            cli.StartChips,
		Timeout:               time.Duration(cli.TimeoutMs) * time.Millisecond,
		MinPlayers:            cli.MinPlayers,
		MaxPlayers:            cli.MaxPlayers,
		EnableStats:           cli.EnableStats,
		MaxStatsHands:         cli.MaxStatsHands,
		EnableLatencyTracking: cli.LatencyTracking,
	}

	// Create RNG instance for server
	seed := time.Now().UnixNano()
	if cli.Seed != nil {
		seed = *cli.Seed
	}

	rng := rand.New(rand.NewSource(seed))
	config.Seed = seed
	srv := server.NewServer(logger, rng, server.WithConfig(config))

	// Start server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		logger.Info().
			Str("addr", cli.Addr).
			Int("small_blind", cli.SmallBlind).
			Int("big_blind", cli.BigBlind).
			Int("start_chips", cli.StartChips).
			Int("timeout_ms", cli.TimeoutMs).
			Int("min_players", cli.MinPlayers).
			Int("max_players", cli.MaxPlayers).
			Int64("seed", seed).
			Bool("enable_stats", cli.EnableStats).
			Bool("enable_latency_tracking", cli.LatencyTracking).
			Msg("Server starting")
		serverErr <- srv.Start(cli.Addr)
	}()

	// Wait for server error or interrupt signal
	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			ctx.FatalIfErrorf(err)
		}
	case sig := <-sigChan:
		logger.Info().Str("signal", sig.String()).Msg("Received signal, shutting down gracefully...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error().Err(err).Msg("Graceful shutdown failed")
		}

		if err := <-serverErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error().Err(err).Msg("Server exited with error")
		} else {
			logger.Info().Msg("Server shutdown complete")
		}
	}
}
