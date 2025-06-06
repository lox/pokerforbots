package commands

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/lox/pokerforbots/internal/client"
)

// GlobalFlags holds common configuration for all commands
type GlobalFlags struct {
	Config   string `short:"c" long:"config" default:"holdem-client.hcl" help:"Path to HCL configuration file"`
	Server   string `short:"s" long:"server" help:"Server URL to connect to (overrides config)"`
	Player   string `short:"p" long:"player" help:"Player name (overrides config)"`
	LogLevel string `short:"l" long:"log-level" help:"Log level (overrides config)"`
	LogFile  string `long:"log-file" help:"Log file path (overrides config)"`
}

// SetupClient creates and connects a client with the given configuration
func SetupClient(config *GlobalFlags) (*client.Client, *client.ClientConfig, *log.Logger, error) {
	client, cfg, logger, err := setupClientCommon(config, os.Stderr)
	return client, cfg, logger, err
}

// SetupClientWithFileLogging creates and connects a client with the given configuration and file logging
func SetupClientWithFileLogging(config *GlobalFlags) (*client.Client, *client.ClientConfig, *log.Logger, func(), error) {
	// Load configuration first to get log file name
	cfg, err := client.LoadClientConfig(config.Config)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("error loading config: %w", err)
	}

	// Apply command line overrides
	if config.Server != "" {
		cfg.Server.URL = config.Server
	}
	if config.Player != "" {
		cfg.Player.Name = config.Player
	}
	if config.LogLevel != "" {
		cfg.UI.LogLevel = config.LogLevel
	}
	if config.LogFile != "" {
		cfg.UI.LogFile = config.LogFile
	}

	// Setup logging to file (overwrite each time)
	logFile, err := os.OpenFile(cfg.UI.LogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to open log file: %w", err)
	}

	// Create client with file logger
	wsClient, finalCfg, logger, err := setupClientConfigured(cfg, logFile)
	if err != nil {
		logFile.Close()
		return nil, nil, nil, nil, err
	}

	// Create cleanup function
	cleanup := func() {
		_ = wsClient.Disconnect()
		_ = logFile.Close()
	}

	return wsClient, finalCfg, logger, cleanup, nil
}

// setupClientConfigured creates a client with an already loaded config and log writer
func setupClientConfigured(cfg *client.ClientConfig, logWriter io.Writer) (*client.Client, *client.ClientConfig, *log.Logger, error) {
	// Get player name if not set
	if cfg.Player.Name == "" {
		fmt.Print("Enter your player name: ")
		var input string
		_, _ = fmt.Scanln(&input)
		cfg.Player.Name = strings.TrimSpace(input)
		if cfg.Player.Name == "" {
			return nil, nil, nil, fmt.Errorf("player name is required")
		}
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, nil, nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Create logger with the provided writer
	logger := log.New(logWriter)
	switch cfg.UI.LogLevel {
	case "debug":
		logger.SetLevel(log.DebugLevel)
	case "info":
		logger.SetLevel(log.InfoLevel)
	case "warn":
		logger.SetLevel(log.WarnLevel)
	case "error":
		logger.SetLevel(log.ErrorLevel)
	default:
		logger.SetLevel(log.WarnLevel) // Default to warn to reduce noise
	}

	// Create WebSocket client
	wsClient := client.NewClient(cfg.Server.URL, logger)

	// Connect to server
	err := wsClient.Connect()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to connect to server: %w", err)
	}

	// Authenticate
	err = wsClient.Auth(cfg.Player.Name)
	if err != nil {
		_ = wsClient.Disconnect()
		return nil, nil, nil, fmt.Errorf("failed to authenticate: %w", err)
	}

	return wsClient, cfg, logger, nil
}

// setupClientCommon handles the common client setup logic
func setupClientCommon(config *GlobalFlags, logWriter io.Writer) (*client.Client, *client.ClientConfig, *log.Logger, error) {
	// Load configuration
	cfg, err := client.LoadClientConfig(config.Config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("error loading config: %w", err)
	}

	// Apply command line overrides
	if config.Server != "" {
		cfg.Server.URL = config.Server
	}
	if config.Player != "" {
		cfg.Player.Name = config.Player
	}
	if config.LogLevel != "" {
		cfg.UI.LogLevel = config.LogLevel
	}
	if config.LogFile != "" {
		cfg.UI.LogFile = config.LogFile
	}

	// Use the common setup logic
	return setupClientConfigured(cfg, logWriter)
}
