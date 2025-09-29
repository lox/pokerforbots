package random

import (
	"context"
	"fmt"
	"math/rand"
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

func (b randomBot) OnActionRequest(state *client.GameState, req protocol.ActionRequest) (string, int, error) {
	// Random strategy: pick a random valid action
	if len(req.ValidActions) == 0 {
		return "fold", 0, nil
	}

	// Pick a random action
	action := req.ValidActions[b.rng.Intn(len(req.ValidActions))]

	// If it's a raise/bet, pick a random amount between min and max
	amount := 0
	if action == "raise" || action == "bet" {
		// Max bet is player's remaining chips
		maxBet := state.Chips
		if maxBet > req.MinBet {
			amount = req.MinBet + b.rng.Intn(maxBet-req.MinBet+1)
		} else {
			amount = req.MinBet
		}
	}

	return action, amount, nil
}

// Run starts the random bot with the given server URL and name
func Run(serverURL, name, game string) error {
	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()

	// Parse configuration from environment
	cfg, err := config.FromEnv()
	if err == nil && cfg.ServerURL != "" {
		serverURL = cfg.ServerURL
	}

	// Initialize RNG with seed from environment or time
	seed := time.Now().UnixNano()
	if cfg != nil && cfg.Seed != 0 {
		seed = cfg.Seed
	}
	rng := rand.New(rand.NewSource(seed))

	// Create bot ID
	id := name
	if id == "" || id == "RandomBot" {
		id = fmt.Sprintf("random-%04d", rng.Intn(10000))
	}
	if cfg != nil && cfg.BotID != "" {
		id = fmt.Sprintf("random-%s", cfg.BotID)
	}

	bot := client.New(id, randomBot{rng: rng}, logger)

	// Set game environment variable if specified
	if game != "" && game != "default" {
		os.Setenv("POKERFORBOTS_GAME", game)
	}

	// Connect to server
	if err := bot.Connect(serverURL); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	logger.Info().Str("game", game).Msg("random bot connected")

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
