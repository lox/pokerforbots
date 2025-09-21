package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"syscall"

	"github.com/lox/pokerforbots/protocol"
	"github.com/lox/pokerforbots/sdk/client"
	"github.com/rs/zerolog"
)

type randomBot struct{}

func (randomBot) OnHandStart(*client.GameState, protocol.HandStart) error         { return nil }
func (randomBot) OnGameUpdate(*client.GameState, protocol.GameUpdate) error       { return nil }
func (randomBot) OnPlayerAction(*client.GameState, protocol.PlayerAction) error   { return nil }
func (randomBot) OnStreetChange(*client.GameState, protocol.StreetChange) error   { return nil }
func (randomBot) OnHandResult(*client.GameState, protocol.HandResult) error       { return nil }
func (randomBot) OnGameCompleted(*client.GameState, protocol.GameCompleted) error { return nil }

func (randomBot) OnActionRequest(state *client.GameState, req protocol.ActionRequest) (string, int, error) {
	if len(req.ValidActions) == 0 {
		return "fold", 0, nil
	}

	choice := req.ValidActions[rand.Intn(len(req.ValidActions))]
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

	// Create bot with random strategy
	id := fmt.Sprintf("random-%04d", rand.Intn(10000))
	bot := client.New(id, randomBot{}, logger)

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
