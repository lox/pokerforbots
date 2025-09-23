// Package config provides configuration parsing for PokerForBots SDK clients.
// It defines the standard environment variables used by the spawner and bots.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Environment variable names used by the spawner and bots
const (
	// EnvServer specifies the WebSocket URL for the poker server
	EnvServer = "POKERFORBOTS_SERVER"

	// EnvSeed provides a random seed for deterministic testing
	EnvSeed = "POKERFORBOTS_SEED"

	// EnvBotID provides a unique identifier for the bot
	EnvBotID = "POKERFORBOTS_BOT_ID"

	// EnvGame specifies the target game ID (defaults to "default")
	EnvGame = "POKERFORBOTS_GAME"
)

// BotConfig holds configuration parsed from environment variables
type BotConfig struct {
	// ServerURL is the WebSocket URL for connecting to the server
	ServerURL string

	// Seed is the random seed for deterministic behavior (0 means not set)
	Seed int64

	// BotID is the unique identifier for this bot instance
	BotID string

	// GameID is the target game to join (defaults to "default")
	GameID string
}

// FromEnv parses configuration from environment variables.
// Returns an error if required variables are missing or invalid.
func FromEnv() (*BotConfig, error) {
	cfg := &BotConfig{
		GameID: "default", // Default game if not specified
	}

	// Parse server URL (required)
	cfg.ServerURL = os.Getenv(EnvServer)
	if cfg.ServerURL == "" {
		return nil, fmt.Errorf("%s environment variable is required", EnvServer)
	}

	// Parse seed (optional)
	if seedStr := os.Getenv(EnvSeed); seedStr != "" {
		seed, err := strconv.ParseInt(seedStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid %s value: %w", EnvSeed, err)
		}
		cfg.Seed = seed
	}

	// Parse bot ID (optional but recommended)
	cfg.BotID = os.Getenv(EnvBotID)

	// Parse game ID (optional, defaults to "default")
	if gameID := os.Getenv(EnvGame); gameID != "" {
		cfg.GameID = gameID
	}

	return cfg, nil
}

// SetEnv sets an environment variable for the spawner to use.
// This is a helper for setting up bot processes.
func SetEnv(env []string, key, value string) []string {
	return append(env, fmt.Sprintf("%s=%s", key, value))
}
