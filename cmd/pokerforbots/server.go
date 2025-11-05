package main

import (
	"github.com/lox/pokerforbots/v2/internal/randutil"

	"context"
	"errors"
	rand "math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/lox/pokerforbots/v2/cmd/pokerforbots/shared"
	"github.com/lox/pokerforbots/v2/internal/auth"
	"github.com/lox/pokerforbots/v2/internal/server"
)

// ServerCmd contains core server configuration
type ServerCmd struct {
	Addr            string `kong:"default=':8080',help='Server address'"`
	Debug           bool   `kong:"help='Enable debug logging'"`
	AuthURL         string `kong:"env='AUTH_URL',help='Authentication service URL (optional, disables auth if empty)'"`
	AdminSecret     string `kong:"env='ADMIN_SECRET',help='Shared secret for auth service (optional)'"`
	AuthRequired    bool   `kong:"env='AUTH_REQUIRED',help='Fail closed on auth unavailable (default: fail open)'"`
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

	// Setup authentication
	var validator server.AuthValidator
	if c.AuthURL != "" {
		// Warn if using HTTP (not HTTPS) with admin secret in production
		if c.AdminSecret != "" && !strings.HasPrefix(c.AuthURL, "https://") &&
			!strings.HasPrefix(c.AuthURL, "http://localhost") &&
			!strings.HasPrefix(c.AuthURL, "http://127.0.0.1") {
			logger.Warn().
				Str("auth_url", c.AuthURL).
				Msg("WARNING: Using HTTP (not HTTPS) with admin secret - secret will be sent in plaintext. Use HTTPS in production!")
		}

		httpValidator := auth.NewHTTPValidator(c.AuthURL, c.AdminSecret)
		validator = auth.NewAdapter(httpValidator)
		logger.Info().
			Str("auth_url", c.AuthURL).
			Bool("auth_required", c.AuthRequired).
			Msg("authentication enabled")
	} else {
		noopValidator := auth.NewNoopValidator()
		validator = auth.NewAdapter(noopValidator)
		logger.Info().Msg("authentication disabled (dev mode)")
	}

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
		AuthRequired:          c.AuthRequired,
	}

	// Create and start server
	s := server.NewServer(logger, rng, server.WithConfig(cfg), server.WithAuthValidator(validator))

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
