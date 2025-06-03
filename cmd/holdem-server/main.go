package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/log"
	"github.com/lox/pokerforbots/internal/server"
)

var CLI struct {
	Config   string `short:"c" long:"config" default:"holdem-server.hcl" help:"Path to HCL configuration file"`
	Addr     string `short:"a" long:"addr" help:"Server address to bind to (overrides config)"`
	LogLevel string `short:"l" long:"log-level" help:"Log level (overrides config)"`
	Tables   int    `short:"t" long:"tables" help:"Number of tables to create (legacy mode)"`
}

func main() {
	ctx := kong.Parse(&CLI)

	// Load configuration
	cfg, err := server.LoadServerConfig(CLI.Config)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		ctx.Exit(1)
	}

	// Apply command line overrides
	if CLI.Addr != "" {
		// Parse addr into host:port
		cfg.Server.Address = CLI.Addr
	}
	if CLI.LogLevel != "" {
		cfg.Server.LogLevel = CLI.LogLevel
	}

	// Handle legacy mode (command line tables)
	if CLI.Tables > 0 {
		// Use legacy mode - override config with command line args
		cfg.Tables = []server.TableConfig{}

		tables := CLI.Tables
		if tables == 0 {
			tables = 1
		}

		for i := 0; i < tables; i++ {
			tableName := fmt.Sprintf("table%d", i+1)
			cfg.Tables = append(cfg.Tables, server.TableConfig{
				Name:       tableName,
				MaxPlayers: 6,
				SmallBlind: 1,
				BigBlind:   2,
				BuyInMin:   100,
				BuyInMax:   1000,
				AutoStart:  true,
			})
		}
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fmt.Printf("Invalid configuration: %v\n", err)
		ctx.Exit(1)
	}

	// Setup logging
	logger := log.New(os.Stderr)
	switch cfg.Server.LogLevel {
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

	logger.Info("Starting Holdem Server",
		"addr", cfg.GetServerAddress(),
		"tables", len(cfg.Tables))

	// Create WebSocket server
	wsServer := server.NewServer(cfg.GetServerAddress(), logger)

	// Create game service
	gameService := server.NewGameService(wsServer, logger)

	// Set game service in connection handlers
	server.SetGameService(gameService)

	// Create tables from configuration
	tableIDMap := make(map[string]string) // name -> ID mapping
	for _, tableConfig := range cfg.Tables {
		table, err := gameService.CreateTable(
			tableConfig.Name,
			tableConfig.MaxPlayers,
			tableConfig.SmallBlind,
			tableConfig.BigBlind,
		)
		if err != nil {
			logger.Error("Failed to create table", "error", err, "table", tableConfig.Name)
			ctx.Exit(1)
		}

		tableIDMap[tableConfig.Name] = table.ID
		logger.Info("Created table",
			"id", table.ID,
			"name", tableConfig.Name,
			"stakes", fmt.Sprintf("$%d/$%d", tableConfig.SmallBlind, tableConfig.BigBlind),
			"maxPlayers", tableConfig.MaxPlayers)
	}

	// Handle graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		logger.Info("Shutting down server...")
		_ = wsServer.Stop()
		os.Exit(0)
	}()

	// Start server (this blocks)
	if err := wsServer.Start(); err != nil {
		logger.Error("Server failed", "error", err)
		ctx.Exit(1)
	}
}
