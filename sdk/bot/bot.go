package bot

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/lox/pokerforbots/sdk/client"
	"github.com/lox/pokerforbots/sdk/config"
	"github.com/rs/zerolog"
)

// RunOption configures the bot runner
type RunOption func(*runConfig)

type runConfig struct {
	logger    zerolog.Logger
	rng       *rand.Rand
	prefix    string
	useEnvCfg bool
}

// WithLogger sets a custom logger
func WithLogger(logger zerolog.Logger) RunOption {
	return func(cfg *runConfig) {
		cfg.logger = logger
	}
}

// WithRNG sets a custom random number generator
func WithRNG(rng *rand.Rand) RunOption {
	return func(cfg *runConfig) {
		cfg.rng = rng
	}
}

// WithPrefix sets the bot name prefix for ID generation
func WithPrefix(prefix string) RunOption {
	return func(cfg *runConfig) {
		cfg.prefix = prefix
	}
}

// WithEnvConfig enables reading configuration from environment variables
func WithEnvConfig() RunOption {
	return func(cfg *runConfig) {
		cfg.useEnvCfg = true
	}
}

// Run connects a bot handler to the poker server and plays until the context
// is cancelled or the game completes.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - handler: Bot implementation that handles game events
//   - serverURL: WebSocket server URL (e.g., "ws://localhost:8080/ws")
//   - name: Display name for this bot instance (empty for auto-generated)
//   - game: Game ID to join (empty defaults to "default")
//   - opts: Optional configuration (logger, RNG, prefix, etc.)
//
// Returns an error if connection fails or the bot encounters a fatal error.
func Run(ctx context.Context, handler client.Handler, serverURL, name, game string, opts ...RunOption) error {
	// Apply options
	cfg := &runConfig{
		logger:    zerolog.New(os.Stderr).With().Timestamp().Logger(),
		prefix:    "bot",
		useEnvCfg: true, // Default to reading env config
	}
	for _, opt := range opts {
		opt(cfg)
	}

	// Parse environment config if enabled
	var envCfg *config.BotConfig
	if cfg.useEnvCfg {
		envCfg, _ = config.FromEnv()
		if envCfg != nil && envCfg.ServerURL != "" {
			serverURL = envCfg.ServerURL
		}
	}

	// Initialize RNG
	if cfg.rng == nil {
		seed := time.Now().UnixNano()
		if envCfg != nil && envCfg.Seed != 0 {
			seed = envCfg.Seed
		}
		cfg.rng = rand.New(rand.NewSource(seed))
	}

	// Generate bot ID
	id := name
	if id == "" {
		id = fmt.Sprintf("%s-%04d", cfg.prefix, cfg.rng.Intn(10000))
	}
	if envCfg != nil && envCfg.BotID != "" {
		id = fmt.Sprintf("%s-%s", cfg.prefix, envCfg.BotID)
	}

	// Create client
	c := client.New(id, handler, cfg.logger)

	// Set game env var if specified
	if game != "" && game != "default" {
		os.Setenv("POKERFORBOTS_GAME", game)
	}

	// Connect
	if err := c.Connect(serverURL); err != nil {
		return fmt.Errorf("connect failed: %w", err)
	}
	cfg.logger.Info().Str("prefix", cfg.prefix).Msg("bot connected")

	// Run
	return c.Run(ctx)
}
