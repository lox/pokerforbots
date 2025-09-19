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

type CLI struct {
	Addr             string `kong:"default=':8080',help='Server address'"`
	Debug            bool   `kong:"help='Enable debug logging'"`
	SmallBlind       int    `kong:"default='5',help='Small blind amount'"`
	BigBlind         int    `kong:"default='10',help='Big blind amount'"`
	StartChips       int    `kong:"default='1000',help='Starting chip count'"`
	TimeoutMs        int    `kong:"default='100',help='Decision timeout in milliseconds'"`
	MinPlayers       int    `kong:"default='2',help='Minimum players per hand'"`
	MaxPlayers       int    `kong:"default='9',help='Maximum players per hand'"`
	RequirePlayer    bool   `kong:"default='true',help='Require at least one player-role bot per hand'"`
	InfiniteBankroll bool   `kong:"default='false',help='Bots never run out of chips (for simulations)'"`
	NPCBots          int    `kong:"default='0',help='Total NPC bots to spawn in default game (auto distribution)'"`
	NPCCalling       int    `kong:"default='0',help='NPC calling-station bots (overrides auto distribution)'"`
	NPCRandom        int    `kong:"default='0',help='NPC random bots (overrides auto distribution)'"`
	NPCAggro         int    `kong:"default='0',help='NPC aggressive bots (overrides auto distribution)'"`
	Seed             *int64 `kong:"help='Deterministic RNG seed for the server (optional)'"`
	Hands            uint64 `kong:"default='0',help='Maximum hands to run in the default game (0 = unlimited)'"`
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
		SmallBlind:       cli.SmallBlind,
		BigBlind:         cli.BigBlind,
		StartChips:       cli.StartChips,
		Timeout:          time.Duration(cli.TimeoutMs) * time.Millisecond,
		MinPlayers:       cli.MinPlayers,
		MaxPlayers:       cli.MaxPlayers,
		RequirePlayer:    cli.RequirePlayer,
		InfiniteBankroll: cli.InfiniteBankroll,
		HandLimit:        cli.Hands,
		Seed:             0,
	}

	// Create RNG instance for server
	seed := time.Now().UnixNano()
	if cli.Seed != nil {
		seed = *cli.Seed
	}

	rng := rand.New(rand.NewSource(seed))
	config.Seed = seed
	srv := server.NewServerWithConfig(logger, rng, config)

	if specs := computeDefaultNPCSpecs(cli.NPCBots, cli.NPCCalling, cli.NPCRandom, cli.NPCAggro); len(specs) > 0 {
		srv.AddBootstrapNPCs("default", specs)
		for _, spec := range specs {
			logger.Info().Str("game_id", "default").Str("strategy", spec.Strategy).Int("count", spec.Count).Msg("Spawning default NPC bots")
		}
	}

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
			Bool("infinite_bankroll", cli.InfiniteBankroll).
			Uint64("hand_limit", cli.Hands).
			Int64("seed", seed).
			Msg("Server starting")
		serverErr <- srv.Start(cli.Addr)
	}()

	// Wait for either server error or interrupt signal
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

func computeDefaultNPCSpecs(total, calling, random, aggro int) []server.NPCSpec {
	if total <= 0 && calling == 0 && random == 0 && aggro == 0 {
		return nil
	}

	if total > 0 && calling == 0 && random == 0 && aggro == 0 {
		base := total / 3
		remainder := total % 3
		calling = base
		random = base
		aggro = base
		if remainder >= 1 {
			calling++
		}
		if remainder >= 2 {
			random++
		}
	}

	specs := make([]server.NPCSpec, 0, 3)
	if calling > 0 {
		specs = append(specs, server.NPCSpec{Strategy: "calling", Count: calling})
	}
	if random > 0 {
		specs = append(specs, server.NPCSpec{Strategy: "random", Count: random})
	}
	if aggro > 0 {
		specs = append(specs, server.NPCSpec{Strategy: "aggressive", Count: aggro})
	}

	if len(specs) == 0 {
		return nil
	}
	return specs
}
