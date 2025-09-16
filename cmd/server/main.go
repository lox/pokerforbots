package main

import (
	"context"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/lox/pokerforbots/internal/server"
	"github.com/rs/zerolog"
)

type CLI struct {
	Addr       string `kong:"default=':8080',help='Server address'"`
	Debug      bool   `kong:"help='Enable debug logging'"`
	SmallBlind int    `kong:"default='5',help='Small blind amount'"`
	BigBlind   int    `kong:"default='10',help='Big blind amount'"`
	StartChips int    `kong:"default='1000',help='Starting chip count'"`
	TimeoutMs  int    `kong:"default='100',help='Decision timeout in milliseconds'"`
	MinPlayers int    `kong:"default='2',help='Minimum players per hand'"`
	MaxPlayers int    `kong:"default='9',help='Maximum players per hand'"`
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
		SmallBlind: cli.SmallBlind,
		BigBlind:   cli.BigBlind,
		StartChips: cli.StartChips,
		Timeout:    time.Duration(cli.TimeoutMs) * time.Millisecond,
		MinPlayers: cli.MinPlayers,
		MaxPlayers: cli.MaxPlayers,
	}

	// Create RNG instance for server
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	srv := server.NewServerWithConfig(logger, rng, config)

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
			Msg("Server starting")
		serverErr <- srv.Start(cli.Addr)
	}()

	// Wait for either server error or interrupt signal
	select {
	case err := <-serverErr:
		if err != nil {
			ctx.FatalIfErrorf(err)
		}
	case sig := <-sigChan:
		logger.Info().Str("signal", sig.String()).Msg("Received signal, shutting down gracefully...")

		// Give server a moment to finish current operations
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Note: server.Stop() would need to be implemented for full graceful shutdown
		// For now, we just exit after a brief delay
		select {
		case <-shutdownCtx.Done():
			logger.Error().Msg("Shutdown timeout exceeded, forcing exit")
		case <-time.After(500 * time.Millisecond):
			logger.Info().Msg("Server shutdown complete")
		}
	}
}
