package aggressive

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

type aggressiveBot struct {
	rng *rand.Rand
}

func (aggressiveBot) OnHandStart(*client.GameState, protocol.HandStart) error         { return nil }
func (aggressiveBot) OnGameUpdate(*client.GameState, protocol.GameUpdate) error       { return nil }
func (aggressiveBot) OnPlayerAction(*client.GameState, protocol.PlayerAction) error   { return nil }
func (aggressiveBot) OnStreetChange(*client.GameState, protocol.StreetChange) error   { return nil }
func (aggressiveBot) OnHandResult(*client.GameState, protocol.HandResult) error       { return nil }
func (aggressiveBot) OnGameCompleted(*client.GameState, protocol.GameCompleted) error { return nil }

func (b aggressiveBot) OnActionRequest(state *client.GameState, req protocol.ActionRequest) (string, int, error) {
	// Aggressive strategy: always raise/bet if possible, otherwise call/check

	// First preference: raise or bet
	if slices.Contains(req.ValidActions, "raise") {
		// Raise between 50% and 100% of pot
		potSize := state.Pot
		if potSize == 0 {
			potSize = 20 // Default to 2BB
		}
		amount := req.MinBet + potSize/2 + b.rng.Intn(potSize/2+1)
		// Max bet is player's remaining chips
		maxBet := state.Chips
		if amount > maxBet {
			amount = maxBet
		}
		return "raise", amount, nil
	}

	if slices.Contains(req.ValidActions, "bet") {
		// Bet between 50% and 100% of pot
		potSize := state.Pot
		if potSize == 0 {
			potSize = 20
		}
		amount := potSize/2 + b.rng.Intn(potSize/2+1)
		// Max bet is player's remaining chips
		maxBet := state.Chips
		if amount > maxBet {
			amount = maxBet
		}
		if amount < req.MinBet {
			amount = req.MinBet
		}
		return "bet", amount, nil
	}

	// Second preference: call or check
	if slices.Contains(req.ValidActions, "call") {
		return "call", 0, nil
	}
	if slices.Contains(req.ValidActions, "check") {
		return "check", 0, nil
	}

	// Last resort: fold
	return "fold", 0, nil
}

// Run starts the aggressive bot with the given server URL and name
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
	if id == "" || id == "AggressiveBot" {
		id = fmt.Sprintf("aggressive-%04d", rng.Intn(10000))
	}
	if cfg != nil && cfg.BotID != "" {
		id = fmt.Sprintf("aggressive-%s", cfg.BotID)
	}

	bot := client.New(id, aggressiveBot{rng: rng}, logger)

	// Set game environment variable if specified
	if game != "" && game != "default" {
		os.Setenv("POKERFORBOTS_GAME", game)
	}

	// Connect to server
	if err := bot.Connect(serverURL); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	logger.Info().Str("game", game).Msg("aggressive bot connected")

	// Handle shutdown gracefully
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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
