package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	rand "math/rand/v2"

	"github.com/lox/pokerforbots/cmd/pokerforbots/shared"
	"github.com/lox/pokerforbots/internal/randutil"
	"github.com/lox/pokerforbots/internal/server"
)

// ServerCmd contains core server configuration
type ServerCmd struct {
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

func (c *ServerCmd) Run() error {
	// Configure logging
	logger := shared.SetupLogger(c.Debug)

	// Setup RNG and seed
	var rng *rand.Rand
	var seed int64
	if c.Seed != nil {
		seed = *c.Seed
		logger.Info().Int64("seed", seed).Msg("Using deterministic seed")
		rng = randutil.New(seed)
	} else {
		seed = time.Now().UnixNano()
		logger.Info().Int64("seed", seed).Msg("Using random seed")
		rng = randutil.New(seed)
	}

	// Create server config
	cfg := server.Config{
		SmallBlind:            c.SmallBlind,
		BigBlind:              c.BigBlind,
		StartChips:            c.StartChips,
		Timeout:               time.Duration(c.TimeoutMs) * time.Millisecond,
		MinPlayers:            c.MinPlayers,
		MaxPlayers:            c.MaxPlayers,
		Seed:                  seed, // Propagate seed to config
		EnableStats:           c.EnableStats,
		MaxStatsHands:         c.MaxStatsHands,
		EnableLatencyTracking: c.LatencyTracking,
	}

	// Create and start server
	s := server.NewServer(logger, rng, server.WithConfig(cfg))

	logger.Info().
		Str("address", c.Addr).
		Int("small_blind", cfg.SmallBlind).
		Int("big_blind", cfg.BigBlind).
		Int("starting_chips", cfg.StartChips).
		Dur("decision_timeout", cfg.Timeout).
		Int("min_players", cfg.MinPlayers).
		Int("max_players", cfg.MaxPlayers).
		Bool("enable_stats", cfg.EnableStats).
		Bool("latency_tracking", cfg.EnableLatencyTracking).
		Msg("Starting PokerForBots server")

	// Setup graceful shutdown
	ctx := shared.SetupSignalHandlerWithLogger(logger)

	// Start server in background
	serverErr := make(chan error, 1)
	go func() {
		if err := s.Start(c.Addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Wait for shutdown or error
	select {
	case <-ctx.Done():
		logger.Info().Msg("Shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.Shutdown(shutdownCtx)
	case err := <-serverErr:
		return err
	}
}
