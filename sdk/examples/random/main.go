package main

import (
	"github.com/lox/pokerforbots/internal/randutil"

	"context"
	"flag"
	"fmt"
	rand "math/rand/v2"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lox/pokerforbots/protocol"
	"github.com/lox/pokerforbots/sdk/client"
	"github.com/lox/pokerforbots/sdk/config"
	"github.com/rs/zerolog"
)

type randomBot struct {
	rng *rand.Rand
}

func (randomBot) OnHandStart(*client.GameState, protocol.HandStart) error         { return nil }
func (randomBot) OnGameUpdate(*client.GameState, protocol.GameUpdate) error       { return nil }
func (randomBot) OnPlayerAction(*client.GameState, protocol.PlayerAction) error   { return nil }
func (randomBot) OnStreetChange(*client.GameState, protocol.StreetChange) error   { return nil }
func (randomBot) OnHandResult(*client.GameState, protocol.HandResult) error       { return nil }
func (randomBot) OnGameCompleted(*client.GameState, protocol.GameCompleted) error { return nil }

func (r randomBot) OnActionRequest(state *client.GameState, req protocol.ActionRequest) (string, int, error) {
	if len(req.ValidActions) == 0 {
		return "fold", 0, nil
	}

	choice := req.ValidActions[r.rng.IntN(len(req.ValidActions))]
	if choice == "raise" {
		amount := req.MinBet
		if req.ToCall > 0 {
			amount = req.ToCall + req.MinRaise
		}
		if amount < req.MinBet {
			amount = req.MinBet
		}
		return choice, amount, nil
	}
	return choice, 0, nil
}

func main() {
	serverURL := flag.String("server", "ws://localhost:8080/ws", "WebSocket server URL")
	flag.Parse()

	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()

	// Parse configuration from environment
	cfg, err := config.FromEnv()
	if err == nil {
		// Use environment config if available
		*serverURL = cfg.ServerURL
	}

	// Initialize RNG with seed from environment or time
	seed := time.Now().UnixNano()
	if cfg != nil && cfg.Seed != 0 {
		seed = cfg.Seed
	}
	rng := randutil.New(seed) // Create local RNG (Go 1.20+ compatible)

	// Create bot with random strategy
	id := fmt.Sprintf("random-%04d", rng.IntN(10000))
	if cfg != nil && cfg.BotID != "" {
		id = fmt.Sprintf("random-%s", cfg.BotID)
	}
	bot := client.New(id, randomBot{rng: rng}, logger)

	// Connect and run
	if err := bot.Connect(*serverURL); err != nil {
		logger.Fatal().Err(err).Msg("connect failed")
	}
	logger.Info().Msg("random bot connected")

	// Handle shutdown gracefully
	ctx, cancel := context.WithCancel(context.Background())
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	runErr := make(chan error, 1)
	go func() { runErr <- bot.Run(ctx) }()

	select {
	case <-interrupt:
		logger.Info().Msg("shutting down")
		cancel()
	case err := <-runErr:
		if err != nil {
			logger.Error().Err(err).Msg("run error")
		}
	}
}
