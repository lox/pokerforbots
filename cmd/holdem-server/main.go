package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/log"
	"github.com/lox/pokerforbots/internal/server"
	"github.com/muesli/termenv"
)

var CLI struct {
	Config   string `short:"c" long:"config" default:"holdem-server.hcl" help:"Path to HCL configuration file"`
	Addr     string `short:"a" long:"addr" help:"Server address to bind to (overrides config)"`
	LogLevel string `short:"l" long:"log-level" help:"Log level (overrides config)"`
	LogFile  string `short:"f" long:"log-file" help:"Log file path (overrides config)"`
	Tables   int    `short:"t" long:"tables" help:"Number of tables to create (legacy mode)"`
	Bots     int    `short:"b" long:"bots" help:"Number of bots to add to each table"`
	Seed     int64  `short:"s" long:"seed" help:"Random seed for deterministic table IDs"`
}

// stripANSIWriter is a writer that strips ANSI color codes before writing to the underlying writer
type stripANSIWriter struct {
	writer *os.File
}

func (s *stripANSIWriter) Write(p []byte) (n int, err error) {
	// Simple ANSI escape sequence regex pattern
	// This strips color codes but keeps the basic text
	stripped := make([]byte, 0, len(p))
	inEscape := false
	for i := 0; i < len(p); i++ {
		if p[i] == '\x1b' && i+1 < len(p) && p[i+1] == '[' {
			inEscape = true
			i++ // Skip the '['
			continue
		}
		if inEscape {
			if (p[i] >= 'A' && p[i] <= 'Z') || (p[i] >= 'a' && p[i] <= 'z') {
				inEscape = false
			}
			continue
		}
		stripped = append(stripped, p[i])
	}
	return s.writer.Write(stripped)
}

// multiTargetWriter writes to both terminal and file, with different processing
type multiTargetWriter struct {
	termWriter *os.File
	fileWriter *stripANSIWriter
}

func (m *multiTargetWriter) Write(p []byte) (n int, err error) {
	// Write to terminal with colors
	_, err1 := m.termWriter.Write(p)
	// Write to file without colors
	_, err2 := m.fileWriter.Write(p)

	// Return the length and any error
	if err1 != nil {
		return len(p), err1
	}
	if err2 != nil {
		return len(p), err2
	}
	return len(p), nil
}

func main() {
	ctx := kong.Parse(&CLI)

	// Set seed from time
	if CLI.Seed == 0 {
		CLI.Seed = time.Now().Unix()
	}

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
	if CLI.LogFile != "" {
		cfg.Server.LogFile = CLI.LogFile
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
				Name:           tableName,
				MaxPlayers:     6,
				SmallBlind:     1,
				BigBlind:       2,
				BuyInMin:       100,
				BuyInMax:       1000,
				AutoStart:      true,
				TimeoutSeconds: 60,
			})
		}
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fmt.Printf("Invalid configuration: %v\n", err)
		ctx.Exit(1)
	}

	// Setup logging with colors for terminal and plain text for file
	var logger *log.Logger

	if cfg.Server.LogFile != "" {
		// Open log file
		logFile, err := os.OpenFile(cfg.Server.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Printf("Error opening log file: %v\n", err)
			ctx.Exit(1)
		}
		defer func() {
			if err := logFile.Close(); err != nil {
				fmt.Printf("Error closing log file: %v\n", err)
			}
		}()

		// Create a writer that strips ANSI codes for the file
		fileWriter := &stripANSIWriter{writer: logFile}

		// Use MultiWriter to write to both stderr (with colors) and file (without colors)
		dualWriter := &multiTargetWriter{
			termWriter: os.Stderr,
			fileWriter: fileWriter,
		}
		logger = log.New(dualWriter)
		logger.SetColorProfile(termenv.TrueColor)
	} else {
		// Just use stderr with colors
		logger = log.New(os.Stderr)
		logger.SetColorProfile(termenv.TrueColor)
	}

	// Set log level
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
	gameService := server.NewGameService(wsServer, logger, CLI.Seed)

	// Set game service in server
	wsServer.SetGameService(gameService)

	// Create tables from configuration
	tableIDMap := make(map[string]string) // name -> ID mapping
	for _, tableConfig := range cfg.Tables {
		table, err := gameService.CreateTable(
			tableConfig.Name,
			tableConfig.MaxPlayers,
			tableConfig.SmallBlind,
			tableConfig.BigBlind,
			tableConfig.TimeoutSeconds,
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

		// Auto-populate with bots if requested
		if CLI.Bots > 0 {
			logger.Info("Auto-populating table with bots", "tableId", table.ID, "count", CLI.Bots)
			botNames, err := gameService.AddBots(table.ID, CLI.Bots)
			if err != nil {
				logger.Error("Failed to add bots", "error", err)
			} else {
				logger.Info("Added bots to table", "bots", botNames, "tableId", table.ID)
			}
		}
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
