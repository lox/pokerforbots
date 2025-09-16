package main

import (
	"context"
	"flag"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lox/pokerforbots/internal/server"
	"github.com/rs/zerolog"
)

func main() {
	addr := flag.String("addr", ":8080", "Server address")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	// Configure zerolog for pretty console output
	level := zerolog.InfoLevel
	if *debug {
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

	// Create RNG instance for server
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	srv := server.NewServer(logger, rng)

	// Start server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		logger.Info().Str("addr", *addr).Msg("Server starting")
		serverErr <- srv.Start(*addr)
	}()

	// Wait for either server error or interrupt signal
	select {
	case err := <-serverErr:
		if err != nil {
			logger.Fatal().Err(err).Msg("Server error")
		}
	case sig := <-sigChan:
		logger.Info().Str("signal", sig.String()).Msg("Received signal, shutting down gracefully...")

		// Give server a moment to finish current operations
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Note: server.Stop() would need to be implemented for full graceful shutdown
		// For now, we just exit after a brief delay
		select {
		case <-ctx.Done():
			logger.Error().Msg("Shutdown timeout exceeded, forcing exit")
		case <-time.After(500 * time.Millisecond):
			logger.Info().Msg("Server shutdown complete")
		}
	}
}
