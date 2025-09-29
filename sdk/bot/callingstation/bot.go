package callingstation

import (
	"context"
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

// Run starts the calling station bot with the given server URL and name
func Run(serverURL, name, game string) error {
	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()

	// Parse configuration from environment
	cfg, err := config.FromEnv()
	if err == nil && cfg.ServerURL != "" {
		// Use environment config if available
		serverURL = cfg.ServerURL
	}

	// Initialize RNG with seed from environment or time
	seed := time.Now().UnixNano()
	if cfg != nil && cfg.Seed != 0 {
		seed = cfg.Seed
	}
	rng := rand.New(rand.NewSource(seed))

	// Create bot with calling station strategy
	id := name
	if id == "" || id == "CallingStation" {
		id = fmt.Sprintf("calling-%04d", rng.Intn(10000))
	}
	if cfg != nil && cfg.BotID != "" {
		id = fmt.Sprintf("calling-%s", cfg.BotID)
	}

	bot := client.New(id, callingStationBot{}, logger)

	// Set game environment variable if specified
	if game != "" && game != "default" {
		os.Setenv("POKERFORBOTS_GAME", game)
	}

	// Connect to server
	if err := bot.Connect(serverURL); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	logger.Info().Str("game", game).Msg("calling station bot connected")

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
		return nil
	case err := <-runErr:
		return err
	}
}
