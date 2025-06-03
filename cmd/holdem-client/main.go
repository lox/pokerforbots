package main

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/lox/pokerforbots/internal/client"
	"github.com/lox/pokerforbots/internal/game"
	"github.com/lox/pokerforbots/internal/tui"
)

var CLI struct {
	Config   string `short:"c" long:"config" default:"holdem-client.hcl" help:"Path to HCL configuration file"`
	Server   string `short:"s" long:"server" help:"Server URL to connect to (overrides config)"`
	Player   string `short:"p" long:"player" help:"Player name (overrides config)"`
	LogLevel string `short:"l" long:"log-level" help:"Log level (overrides config)"`
	LogFile  string `long:"log-file" help:"Log file path (overrides config)"`
}

func main() {
	ctx := kong.Parse(&CLI)

	// Load configuration
	cfg, err := client.LoadClientConfig(CLI.Config)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		ctx.Exit(1)
	}

	// Apply command line overrides
	if CLI.Server != "" {
		cfg.Server.URL = CLI.Server
	}
	if CLI.Player != "" {
		cfg.Player.Name = CLI.Player
	}
	if CLI.LogLevel != "" {
		cfg.UI.LogLevel = CLI.LogLevel
	}
	if CLI.LogFile != "" {
		cfg.UI.LogFile = CLI.LogFile
	}

	// Get player name if not set
	if cfg.Player.Name == "" {
		fmt.Print("Enter your player name: ")
		var input string
		_, _ = fmt.Scanln(&input)
		cfg.Player.Name = strings.TrimSpace(input)
		if cfg.Player.Name == "" {
			fmt.Println("Player name is required")
			ctx.Exit(1)
		}
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fmt.Printf("Invalid configuration: %v\n", err)
		ctx.Exit(1)
	}

	// Setup logging to file
	logFile, err := os.OpenFile(cfg.UI.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("Failed to open log file: %v\n", err)
		ctx.Exit(1)
	}
	defer func() { _ = logFile.Close() }()

	logger := log.New(logFile)
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
		logger.SetLevel(log.InfoLevel)
	}

	logger.Info("Starting Holdem Client",
		"server", cfg.Server.URL,
		"player", cfg.Player.Name,
		"config", CLI.Config)

	// Create a dummy table for TUI initialization (won't be used in network mode)
	dummyRng := rand.New(rand.NewSource(time.Now().UnixNano()))
	dummyTable := game.NewTable(dummyRng, game.TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
		Seed:       time.Now().UnixNano(),
	})

	// Create TUI model
	tuiModel := tui.NewTUIModel(dummyTable, logger)

	// Create WebSocket client
	wsClient := client.NewClient(cfg.Server.URL, logger)

	// Set up simple bridge between client and TUI
	tui.SetupSimpleNetworkHandlers(wsClient, tuiModel)

	// Connect to server
	err = wsClient.Connect()
	if err != nil {
		fmt.Printf("Failed to connect to server: %v\n", err)
		ctx.Exit(1)
	}
	defer func() { _ = wsClient.Disconnect() }()

	// Authenticate
	err = wsClient.Auth(cfg.Player.Name)
	if err != nil {
		fmt.Printf("Failed to authenticate: %v\n", err)
		ctx.Exit(1)
	}

	// Start TUI
	program := tea.NewProgram(tuiModel, tea.WithAltScreen())

	// Add initial welcome message
	tuiModel.AddLogEntry("=== Texas Hold'em Client ===")
	tuiModel.AddLogEntry("Connected to server: " + cfg.Server.URL)
	tuiModel.AddLogEntry("Player: " + cfg.Player.Name)
	tuiModel.AddLogEntry("")
	tuiModel.AddLogEntry("Commands:")
	tuiModel.AddLogEntry("  \033[1m/list\033[0m - List available tables")
	tuiModel.AddLogEntry("  \033[1m/join <table_id>\033[0m - Join a table")
	tuiModel.AddLogEntry("  \033[1m/leave\033[0m - Leave current table")
	tuiModel.AddLogEntry("  \033[1m/addbot [count]\033[0m - Add bots to table (1-5)")
	tuiModel.AddLogEntry("  \033[1m/kickbot <name>\033[0m - Remove a bot from table")
	tuiModel.AddLogEntry("  \033[1m/quit\033[0m - Quit the game")
	tuiModel.AddLogEntry("")

	// Start command handler in TUI package
	tui.StartCommandHandler(wsClient, tuiModel, cfg.Player.DefaultBuyIn)

	// Run TUI
	if _, err := program.Run(); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		ctx.Exit(1)
	}

	// Cleanup
	_ = wsClient.Disconnect()
}
