package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/lox/pokerforbots/protocol"
	"github.com/lox/pokerforbots/sdk/client"
	"github.com/lox/pokerforbots/sdk/config"
	"github.com/rs/zerolog"
)

type aggressiveBot struct {
	rng *rand.Rand
}

func (aggressiveBot) OnHandStart(*client.GameState, protocol.HandStart) error         { return nil }
func (aggressiveBot) OnGameUpdate(*client.GameState, protocol.GameUpdate) error       { return nil }
func (aggressiveBot) OnPlayerAction(*client.GameState, protocol.PlayerAction) error   { return nil }
func (aggressiveBot) OnStreetChange(*client.GameState, protocol.StreetChange) error   { return nil }
func (aggressiveBot) OnHandResult(*client.GameState, protocol.HandResult) error       { return nil }
func (aggressiveBot) OnGameCompleted(*client.GameState, protocol.GameCompleted) error { return nil }

func (a aggressiveBot) OnActionRequest(state *client.GameState, req protocol.ActionRequest) (string, int, error) {
	// Aggressive strategy: raise 70% of the time
	if a.rng.Float32() < 0.7 {
		for _, action := range req.ValidActions {
			if action == "raise" {
				minRequired := max(req.MinRaise, req.MinBet)
				amount := minRequired

				// Sometimes bet pot-sized or more
				if req.Pot > 0 {
					amount = req.Pot * (2 + a.rng.Intn(2))
				}

				// Ensure amount meets minimum requirements
				if amount < minRequired {
					amount = minRequired
				}

				// Cap at our chip stack
				if amount > state.Chips {
					amount = state.Chips
				}

				// If we can't meet minimum, go all-in if possible
				if state.Chips < minRequired {
					if slices.Contains(req.ValidActions, "allin") {
						return "allin", 0, nil
					}
					// Fall back to call/check
					if slices.Contains(req.ValidActions, "call") {
						return "call", 0, nil
					}
					if slices.Contains(req.ValidActions, "check") {
						return "check", 0, nil
					}
					return "fold", 0, nil
				}

				return "raise", amount, nil
			}
		}
		// If we can't raise, try all-in
		if slices.Contains(req.ValidActions, "allin") {
			return "allin", 0, nil
		}
	}

	// Fall back to call/check
	if slices.Contains(req.ValidActions, "call") {
		return "call", 0, nil
	}
	if slices.Contains(req.ValidActions, "check") {
		return "check", 0, nil
	}
	return "fold", 0, nil
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
	rng := rand.New(rand.NewSource(seed))

	// Create bot with aggressive strategy
	id := fmt.Sprintf("aggressive-%04d", rng.Intn(10000))
	if cfg != nil && cfg.BotID != "" {
		id = fmt.Sprintf("aggressive-%s", cfg.BotID)
	}
	bot := client.New(id, aggressiveBot{rng: rng}, logger)

	// Connect and run
	if err := bot.Connect(*serverURL); err != nil {
		logger.Fatal().Err(err).Msg("connect failed")
	}
	logger.Info().Msg("aggressive bot connected")

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
