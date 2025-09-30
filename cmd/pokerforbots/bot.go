package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/lox/pokerforbots/sdk/bot"
	"github.com/lox/pokerforbots/sdk/client"

	// Bots
	"github.com/lox/pokerforbots/sdk/bots/aggressive"
	"github.com/lox/pokerforbots/sdk/bots/callingstation"
	"github.com/lox/pokerforbots/sdk/bots/complex"
	"github.com/lox/pokerforbots/sdk/bots/random"
)

type BotCmd struct {
	Name   string `arg:"" help:"Bot type (calling-station, random, aggressive, complex)"`
	Server string `default:"ws://localhost:8080/ws" help:"WebSocket server URL"`
	Game   string `default:"default" help:"Game to join"`
}

// botHandlers maps bot names to their handler constructors
var botHandlers = map[string]func() client.Handler{
	"calling-station": func() client.Handler { return &callingstation.Handler{} },
	"random":          func() client.Handler { return random.NewHandler() },
	"aggressive":      func() client.Handler { return aggressive.NewHandler() },
	"complex":         func() client.Handler { return complex.NewHandler() },
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

	// Create handler
	handler := handlerFn()

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
