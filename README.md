# Poker for Bots ♠️♣️♦️♥️🤖

A WebSocket-based Texas Hold'em poker server with SDK for bot development.

## Overview

This is a client/server poker platform where:

- **Server**: Runs poker games and manages tables
- **SDK**: Provides types and utilities for building poker bots
- **External Clients**: Connect via WebSocket to play (humans or bots)

The server handles game logic while bots connect as external clients using the provided SDK.

## Quick Start

### Start Server

```bash
./bin/task build  # Build binaries
./dist/holdem-server  # Start poker server on :8080
```

### Build a Bot (using SDK)

```go
package main

import (
    "github.com/lox/pokerforbots/sdk"
    "github.com/lox/pokerforbots/sdk/deck"
)

type SimpleBot struct{}

func (b *SimpleBot) MakeDecision(state sdk.TableState, actions []sdk.ValidAction) sdk.Decision {
    // Always call/check if possible, otherwise fold
    for _, action := range actions {
        if action.Action == sdk.ActionCall || action.Action == sdk.ActionCheck {
            return sdk.NewDecision(action.Action, 0, "Simple strategy")
        }
    }
    return sdk.NewFoldDecision("No good options")
}

func main() {
    bot := sdk.NewBotClient("ws://localhost:8080", "SimpleBot", &SimpleBot{}, logger)
    bot.Connect(context.Background(), "SimpleBot")
    bot.JoinTable("table-1", 1000)
    // Bot will now play automatically
}
```

## SDK Features

### Core Types

- **`deck.Card`**: Cards with integer rank (2-14) and suit (0-3)
- **`TableState`**: Current game state (pot, cards, players, etc.)
- **`ValidAction`**: Legal actions with min/max amounts
- **`Decision`**: Bot's chosen action with reasoning

### Bot Interface

```go
type Agent interface {
    MakeDecision(tableState TableState, validActions []ValidAction) Decision
}
```

### WebSocket Protocol

- **Message Types**: JSON messages for game events and actions
- **Event Handlers**: Register callbacks for different message types
- **Automatic Reconnection**: Built-in connection management

## Project Structure

```text
cmd/holdem-server/   # WebSocket poker server
sdk/                 # Bot development SDK
├── deck/           # Card and deck types
├── examples/       # Example bot implementations
├── protocol.go     # WebSocket message types
├── enums.go        # Action, position, round enums
├── types.go        # Table and player state types
├── client.go       # Bot client wrapper
└── ws_client.go    # WebSocket client

internal/           # Server implementation
├── game/          # Game logic and rules
├── server/        # WebSocket server
└── evaluator/     # Hand evaluation
```

## Development

```bash
# Build and test
./bin/task build
./bin/task test
./bin/task lint

# Run server
./dist/holdem-server

# Test with example bots
go run sdk/examples/simple/main.go
```
