package main

import (
	"fmt"
	"os"

	"github.com/lox/pokerforbots/sdk/bot/aggressive"
	"github.com/lox/pokerforbots/sdk/bot/callingstation"
	"github.com/lox/pokerforbots/sdk/bot/complex"
	"github.com/lox/pokerforbots/sdk/bot/random"
)

type BotsCmd struct {
	CallingStation CallingStationCmd `cmd:"" aliases:"cs" help:"Run calling station bot (always calls/checks)"`
	Random         RandomCmd         `cmd:"" aliases:"rnd" help:"Run random bot (random valid actions)"`
	Aggressive     AggressiveCmd     `cmd:"" aliases:"aggro" help:"Run aggressive bot (raises often)"`
	Complex        ComplexCmd        `cmd:"" help:"Run complex bot with advanced strategy"`
	Info           InfoCmd           `cmd:"" help:"Show bot resolver information"`
}

// CallingStation bot command
type CallingStationCmd struct {
	Server string `arg:"" default:"ws://localhost:8080/ws" help:"WebSocket server URL"`
	Name   string `help:"Bot display name" default:"CallingStation"`
	Game   string `help:"Game to join" default:"default"`
}

func (c *CallingStationCmd) Run() error {
	// Set POKERFORBOTS_SEED from parent if available
	if seed := os.Getenv("POKERFORBOTS_SEED"); seed != "" {
		// The bot will pick this up automatically
	}
	return callingstation.Run(c.Server, c.Name, c.Game)
}

// Random bot command
type RandomCmd struct {
	Server string `arg:"" default:"ws://localhost:8080/ws" help:"WebSocket server URL"`
	Name   string `help:"Bot display name" default:"RandomBot"`
	Game   string `help:"Game to join" default:"default"`
}

func (c *RandomCmd) Run() error {
	if seed := os.Getenv("POKERFORBOTS_SEED"); seed != "" {
		// The bot will pick this up automatically
	}
	return random.Run(c.Server, c.Name, c.Game)
}

// Aggressive bot command
type AggressiveCmd struct {
	Server string `arg:"" default:"ws://localhost:8080/ws" help:"WebSocket server URL"`
	Name   string `help:"Bot display name" default:"AggressiveBot"`
	Game   string `help:"Game to join" default:"default"`
}

func (c *AggressiveCmd) Run() error {
	if seed := os.Getenv("POKERFORBOTS_SEED"); seed != "" {
		// The bot will pick this up automatically
	}
	return aggressive.Run(c.Server, c.Name, c.Game)
}

// Complex bot command
type ComplexCmd struct {
	Server string `arg:"" default:"ws://localhost:8080/ws" help:"WebSocket server URL"`
	Name   string `help:"Bot display name" default:"ComplexBot"`
	Game   string `help:"Game to join" default:"default"`
}

func (c *ComplexCmd) Run() error {
	if seed := os.Getenv("POKERFORBOTS_SEED"); seed != "" {
		// The bot will pick this up automatically
	}
	return complex.Run(c.Server, c.Name, c.Game)
}

// Info command shows resolver information
type InfoCmd struct{}

func (c *InfoCmd) Run() error {
	resolver := NewBotResolver()

	fmt.Printf("Execution Mode: %s\n", resolver.Mode())

	switch resolver.Mode() {
	case ModeGoRun:
		fmt.Printf("Project Root: %s\n", resolver.ProjectRoot())
		fmt.Println("Bot Execution: go run ./sdk/examples/{bot}")
	case ModeBinary:
		fmt.Printf("Binary Path: %s\n", resolver.Binary())
		fmt.Println("Bot Execution: pokerforbots bots {bot}")
	}

	fmt.Println("\nAvailable bots:")
	for _, bot := range resolver.ListAvailableBots() {
		fmt.Printf("  %s\n", bot)
	}

	fmt.Println("\nUsage examples:")
	if resolver.Mode() == ModeGoRun {
		fmt.Println("  go run ./cmd/pokerforbots bots calling-station ws://localhost:8080/ws")
		fmt.Println("  go run ./cmd/pokerforbots spawn --spec='calling-station:2,random:2'")
	} else {
		fmt.Println("  pokerforbots bots calling-station ws://localhost:8080/ws")
		fmt.Println("  pokerforbots spawn --spec='calling-station:2,random:2'")
	}

	return nil
}
