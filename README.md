# PokerForBots

A high-performance Texas Hold'em poker server optimized for bot-vs-bot play.

## Installation

### Option 1: Install Binary (Recommended)
```bash
# Build and install to ~/bin
go build -o ~/bin/pokerforbots ./cmd/pokerforbots

# Verify installation
pokerforbots --help
```

### Option 2: Run from Source
```bash
# During development, use go run
go run ./cmd/pokerforbots spawn --spec "calling-station:4,random:2"
```

## Quick Start

```bash
# Quick demo with bots
pokerforbots spawn --spec "calling-station:3,random:3" --hand-limit 1000

# Run your bot against opponents
pokerforbots spawn --spec "calling-station:5" \
  --bot-cmd "./my-bot" --hand-limit 1000 --print-stats

# Test bot improvements
pokerforbots regression --mode heads-up --hands 5000 \
  --challenger "./my-new-bot" --baseline "./my-old-bot"

# Interactive play
pokerforbots client --name Alice
```

## Documentation

### Getting Started
- [Quick Start Guide](docs/quickstart.md) - Getting up and running quickly
- [Testing Guide](docs/testing.md) - Regression testing and validation
- [Command Reference](docs/reference.md) - Complete CLI reference

### Technical Documentation
- [Design Overview](docs/design.md) - Architecture and design decisions
- [WebSocket Protocol](docs/websocket-protocol.md) - Message format specification
- [Poker Rules](docs/poker-rules.md) - No-limit Hold'em rules implementation
- [Go SDK](docs/sdk.md) - Bot development SDK
- [Operations](docs/operations.md) - Server operation and monitoring
- [HTTP API](docs/http-api.md) - REST endpoints for stats and control
- [Benchmarking](docs/benchmarking.md) - Performance testing

## Command Overview

The `pokerforbots` CLI provides these sub-commands:

- **`spawn`** - Quick testing with bots (most common)
- **`bot`** - Run a built-in bot (calling-station, random, aggressive, complex)
- **`regression`** - Statistical bot comparison
- **`server`** - Standalone poker server
- **`client`** - Interactive human client

Run `pokerforbots <command> --help` for detailed options.

## Common Workflows

### Testing Your Bot
```bash
# Quick test against passive opponents
pokerforbots spawn --spec "calling-station:5" \
  --bot-cmd "./my-bot" --hand-limit 1000

# Test against mixed strategies
pokerforbots spawn --spec "calling-station:2,random:2,aggressive:2" \
  --bot-cmd "./my-bot" --hand-limit 1000
```

### Validating Improvements
```bash
# Compare bot versions statistically
pokerforbots regression --mode heads-up --hands 10000 \
  --challenger "./bot-v2" --baseline "./bot-v1"
```

### Running Built-in Bots
```bash
# Run built-in bots
pokerforbots bot calling-station
pokerforbots bot random
pokerforbots bot aggressive
pokerforbots bot complex
```

## Architecture

The codebase is organized into public packages for shared types and internal packages for server implementation:

- `poker/` - Core poker primitives (cards, deck, evaluator)
- `protocol/` - WebSocket protocol messages (msgpack encoded)
- `internal/game/` - Game logic and state management
- `internal/server/` - WebSocket server and bot management
- `sdk/` - Go SDK for bot development
  - `sdk/bot/` - Bot interface
  - `sdk/bots/` - Built-in bot implementations
- `cmd/pokerforbots/` - Unified CLI tool

## Development

```bash
# Run tests
task test

# Run with race detection
task test:race

# Generate code (msgpack)
task generate

# Build binary
task build
```

## License

MIT