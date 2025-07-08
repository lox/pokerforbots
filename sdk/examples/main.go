package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/log"
	"github.com/lox/pokerforbots/sdk"
	"github.com/lox/pokerforbots/sdk/examples/chart"
	"github.com/lox/pokerforbots/sdk/examples/simple"
)

func main() {
	var (
		serverURL = flag.String("server", "ws://localhost:8080/ws", "Server WebSocket URL")
		botName   = flag.String("name", "ExampleBot", "Bot name")
		botType   = flag.String("type", "chart", "Bot type: call, fold, random, chart")
		tableID   = flag.String("table", "", "Table ID to join (required)")
		buyIn     = flag.Int("buyin", 200, "Buy-in amount")
	)
	flag.Parse()

	if *tableID == "" {
		log.Error("Table ID is required")
		flag.Usage()
		os.Exit(1)
	}

	logger := log.New(os.Stdout)

	// Create bot based on type
	var bot sdk.Agent
	switch *botType {
	case "call":
		bot = simple.NewCallBot(*botName)
	case "fold":
		bot = simple.NewFoldBot(*botName)
	case "random":
		bot = simple.NewRandomBot(*botName)
	case "chart":
		bot = chart.NewPreflopChartBot(*botName)
	default:
		logger.Error("Unknown bot type", "type", *botType)
		os.Exit(1)
	}

	logger.Info("Starting bot", "name", *botName, "type", *botType, "server", *serverURL)

	// Create client
	client := sdk.NewBotClient(*serverURL, *botName, bot, logger)

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		logger.Info("Shutting down bot...")
		cancel()
		_ = client.Disconnect()
		os.Exit(0)
	}()

	// Connect and authenticate
	if err := client.Connect(ctx, *botName); err != nil {
		logger.Error("Failed to connect", "error", err)
		os.Exit(1)
	}
	defer func() { _ = client.Disconnect() }()

	logger.Info("Connected to server, joining table", "tableId", *tableID, "buyIn", *buyIn)

	// Join table
	if err := client.JoinTable(*tableID, *buyIn); err != nil {
		logger.Error("Failed to join table", "error", err)
		os.Exit(1)
	}

	logger.Info("Bot is ready and waiting for game events...")

	// Keep running until interrupted
	<-ctx.Done()
}
