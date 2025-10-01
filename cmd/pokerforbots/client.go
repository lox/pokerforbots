package main

import (
	"strings"

	"github.com/lox/pokerforbots/v2/internal/client"
)

type ClientCmd struct {
	Server string `kong:"default='ws://localhost:8080/ws',help='WebSocket server URL'"`
	Name   string `kong:"default='',help='Display name (defaults to $USER or \"Player\")'"`
	Game   string `kong:"default='default',help='Game/table identifier to join'"`
}

func (c *ClientCmd) Run() error {
	config := client.Config{
		Server: strings.TrimSpace(c.Server),
		Name:   strings.TrimSpace(c.Name),
		Game:   strings.TrimSpace(c.Game),
	}

	return client.Run(config)
}
