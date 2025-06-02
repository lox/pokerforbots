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
	"github.com/lox/holdem-cli/internal/client"
	"github.com/lox/holdem-cli/internal/game"
	"github.com/lox/holdem-cli/internal/tui"
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
		fmt.Scanln(&input)
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
	defer logFile.Close()

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

	// Create TUI adapter
	tuiAdapter := tui.NewNetworkTUIAdapter(tuiModel)

	// Create network agent (handles server events and user input)
	_ = client.NewNetworkAgent(wsClient, tuiAdapter, logger)

	// Connect to server
	err = wsClient.Connect()
	if err != nil {
		fmt.Printf("Failed to connect to server: %v\n", err)
		ctx.Exit(1)
	}
	defer wsClient.Disconnect()

	// Authenticate
	err = wsClient.Auth(cfg.Player.Name)
	if err != nil {
		fmt.Printf("Failed to authenticate: %v\n", err)
		ctx.Exit(1)
	}

	// Wait for auth response (simple wait)
	// In a real implementation, you'd want better synchronization
	// time.Sleep(1 * time.Second)

	// Start TUI
	program := tea.NewProgram(tuiModel, tea.WithAltScreen())

	// Add initial welcome message
	tuiModel.AddLogEntry("=== Texas Hold'em Client ===")
	tuiModel.AddLogEntry("Connected to server: " + cfg.Server.URL)
	tuiModel.AddLogEntry("Player: " + cfg.Player.Name)
	tuiModel.AddLogEntry("")
	tuiModel.AddLogEntry("Commands:")
	tuiModel.AddLogEntry("  /list - List available tables")
	tuiModel.AddLogEntry("  /join <table_id> - Join a table")
	tuiModel.AddLogEntry("  /leave - Leave current table")
	tuiModel.AddLogEntry("  /quit - Quit the game")
	tuiModel.AddLogEntry("")

	// Set up command handler
	go handleCommands(wsClient, tuiModel, logger, cfg)

	// Run TUI
	if _, err := program.Run(); err != nil {
		fmt.Printf("Error running TUI: %v\n", err)
		ctx.Exit(1)
	}

	// Cleanup
	wsClient.Disconnect()
}

// handleCommands processes special commands from user input
func handleCommands(wsClient *client.Client, tuiModel *tui.TUIModel, logger *log.Logger, cfg *client.ClientConfig) {
	for {
		action, args, shouldContinue, err := tuiModel.WaitForAction()
		if err != nil {
			logger.Error("Error waiting for action", "error", err)
			continue
		}

		if !shouldContinue {
			break
		}

		// Handle special commands
		if strings.HasPrefix(action, "/") {
			switch action {
			case "/list":
				err := wsClient.ListTables()
				if err != nil {
					tuiModel.AddLogEntry(fmt.Sprintf("Error listing tables: %v", err))
				}

			case "/join":
				if len(args) < 1 {
					tuiModel.AddLogEntry("Usage: /join <table_id>")
					continue
				}
				tableID := args[0]
				buyIn := cfg.Player.DefaultBuyIn
				if len(args) > 1 {
					// Parse buy-in if provided
					// buyIn = parseBuyIn(args[1])
				}

				err := wsClient.JoinTable(tableID, buyIn)
				if err != nil {
					tuiModel.AddLogEntry(fmt.Sprintf("Error joining table: %v", err))
				}

			case "/leave":
				tableID := wsClient.GetTableID()
				if tableID == "" {
					tuiModel.AddLogEntry("You're not at a table")
					continue
				}

				err := wsClient.LeaveTable(tableID)
				if err != nil {
					tuiModel.AddLogEntry(fmt.Sprintf("Error leaving table: %v", err))
				}

			case "/quit":
				return

			default:
				tuiModel.AddLogEntry(fmt.Sprintf("Unknown command: %s", action))
			}
		}
		// Non-command actions are handled by the NetworkAgent via game events
	}
}
