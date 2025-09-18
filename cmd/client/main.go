package main

import (
	"strings"

	"github.com/alecthomas/kong"
	"github.com/lox/pokerforbots/internal/client"
)

type cli struct {
	Server string `kong:"default='ws://localhost:8080/ws',help='WebSocket server URL'"`
	Name   string `kong:"default='',help='Display name (defaults to $USER or \"Player\")'"`
}

func main() {
	var c cli
	ctx := kong.Parse(&c,
		kong.Name("pokerforbots-client"),
		kong.Description("Interactive CLI client for PokerForBots"),
		kong.UsageOnError(),
	)

	config := client.Config{
		Server: strings.TrimSpace(c.Server),
		Name:   strings.TrimSpace(c.Name),
	}

	if err := client.Run(config); err != nil {
		ctx.FatalIfErrorf(err)
	}
}
