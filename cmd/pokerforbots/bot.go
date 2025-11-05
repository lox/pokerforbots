package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/lox/pokerforbots/v2/sdk/bot"
	"github.com/lox/pokerforbots/v2/sdk/client"
	"github.com/rs/zerolog"

	// Bots
	"github.com/lox/pokerforbots/v2/sdk/bots/aggressive"
	"github.com/lox/pokerforbots/v2/sdk/bots/callingstation"
	"github.com/lox/pokerforbots/v2/sdk/bots/complex"
	"github.com/lox/pokerforbots/v2/sdk/bots/random"
)

type BotCmd struct {
	Name     string `arg:"" help:"Bot type (calling-station, random, aggressive, complex)"`
	Server   string `default:"ws://localhost:8080/ws" help:"WebSocket server URL"`
	Game     string `default:"default" help:"Game to join"`
	LogLevel string `default:"info" help:"Log level (debug|info|warn|error)"`
	LogJSON  bool   `help:"Output JSON logs instead of console format"`
}

// botHandlers maps bot names to their handler constructors
var botHandlers = map[string]func(zerolog.Logger) client.Handler{
	"calling-station": func(zerolog.Logger) client.Handler { return &callingstation.Handler{} },
	"random":          func(zerolog.Logger) client.Handler { return random.NewHandler() },
	"aggressive":      func(zerolog.Logger) client.Handler { return aggressive.NewHandler() },
	"complex":         func(logger zerolog.Logger) client.Handler { return complex.NewHandlerWithLogger(logger) },
}

// botPrefixes maps bot names to their ID prefixes
var botPrefixes = map[string]string{
	"calling-station": "calling",
	"random":          "random",
	"aggressive":      "aggressive",
	"complex":         "complex",
}

func (c *BotCmd) Run() error {
	// Look up the bot handler constructor
	handlerFn, ok := botHandlers[c.Name]
	if !ok {
		return fmt.Errorf("unknown bot: %s (available: calling-station, random, aggressive, complex)", c.Name)
	}

	// Setup logger
	logger := createBotLogger(c.LogLevel, c.LogJSON)

	// Create handler
	handler := handlerFn(logger)

	// Setup context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	runErr := make(chan error, 1)
	go func() {
		runErr <- bot.Run(
			ctx,
			handler,
			c.Server,
			"",     // name auto-generated
			c.Game, // game from flag
			bot.WithPrefix(botPrefixes[c.Name]),
			bot.WithLogger(logger),
		)
	}()

	select {
	case <-interrupt:
		cancel()
		return nil
	case err := <-runErr:
		return err
	}
}

func createBotLogger(level string, jsonFormat bool) zerolog.Logger {
	// Parse log level
	var zLevel zerolog.Level
	switch level {
	case "debug":
		zLevel = zerolog.DebugLevel
	case "info":
		zLevel = zerolog.InfoLevel
	case "warn":
		zLevel = zerolog.WarnLevel
	case "error":
		zLevel = zerolog.ErrorLevel
	default:
		zLevel = zerolog.InfoLevel
	}

	// Create logger with appropriate format
	var logger zerolog.Logger
	if jsonFormat {
		logger = zerolog.New(os.Stderr)
	} else {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	return logger.Level(zLevel).With().Timestamp().Logger()
}
