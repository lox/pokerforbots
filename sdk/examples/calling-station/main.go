package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"syscall"
	"time"

	"github.com/lox/pokerforbots/protocol"
	"github.com/lox/pokerforbots/sdk/client"
	"github.com/rs/zerolog"
)

type callingStationBot struct{}

func (callingStationBot) OnHandStart(*client.GameState, protocol.HandStart) error         { return nil }
func (callingStationBot) OnGameUpdate(*client.GameState, protocol.GameUpdate) error       { return nil }
func (callingStationBot) OnPlayerAction(*client.GameState, protocol.PlayerAction) error   { return nil }
func (callingStationBot) OnStreetChange(*client.GameState, protocol.StreetChange) error   { return nil }
func (callingStationBot) OnHandResult(*client.GameState, protocol.HandResult) error       { return nil }
func (callingStationBot) OnGameCompleted(*client.GameState, protocol.GameCompleted) error { return nil }

func (callingStationBot) OnActionRequest(state *client.GameState, req protocol.ActionRequest) (string, int, error) {
	// Calling station strategy: always check or call, never raise
	if slices.Contains(req.ValidActions, "check") {
		return "check", 0, nil
	}
	if slices.Contains(req.ValidActions, "call") {
		return "call", 0, nil
	}
	return "fold", 0, nil
}

func main() {
	serverURL := flag.String("server", "ws://localhost:8080/ws", "WebSocket server URL")
	flag.Parse()

	// Check for environment variable override
	if envURL := os.Getenv("POKERFORBOTS_SERVER"); envURL != "" {
		*serverURL = envURL
	}

	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()

	// Initialize RNG with seed from environment or time
	seed := time.Now().UnixNano()
	if envSeed := os.Getenv("POKERFORBOTS_SEED"); envSeed != "" {
		if parsedSeed, err := strconv.ParseInt(envSeed, 10, 64); err == nil {
			seed = parsedSeed
		}
	}
	rng := rand.New(rand.NewSource(seed))

	// Create bot with calling station strategy
	id := fmt.Sprintf("calling-%04d", rng.Intn(10000))
	if envID := os.Getenv("POKERFORBOTS_BOT_ID"); envID != "" {
		id = fmt.Sprintf("calling-%s", envID)
	}
	bot := client.New(id, callingStationBot{}, logger)

	// Connect and run
	if err := bot.Connect(*serverURL); err != nil {
		logger.Fatal().Err(err).Msg("connect failed")
	}
	logger.Info().Msg("calling station bot connected")

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
