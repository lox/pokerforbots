# Poker for Bots ♠️♣️♦️♥️🤖

A client/server Texas Hold'em poker platform designed for bot development and testing, with human-playable interfaces and comprehensive poker tools.

## Features

### Server Architecture
- **WebSocket-based multiplayer**: Real-time game server supporting multiple concurrent tables
- **Bot ecosystem**: Multiple AI strategies (chart-based, TAG, maniac, calling station)
- **HCL configuration**: Flexible configuration for tables, bots, and server settings
- **Docker support**: Complete containerized setup for easy deployment and testing

### Client Interfaces
- **Interactive CLI client**: Professional terminal interface for human players
- **Automated bot clients**: Configurable AI players that connect and play autonomously
- **TUI design**: Bubble Tea-powered interface with game log and action panels
- **Real-time gameplay**: Live updates, hand progression, and showdown results

### Game Features
- **Complete poker simulation**: Full Texas Hold'em with all betting rounds
- **Multiple table support**: Concurrent games with different stakes and configurations
- **Position-aware gameplay**: Proper blinds, button rotation, and positional strategy
- **Hand history tracking**: Complete game logs and hand records
- **Professional poker rules**: Exact implementation of 6-max $1/$2 No Limit Hold'em

### Developer Tools
- **Poker odds calculator**: Monte Carlo simulation for hand equity analysis
- **Testing framework**: Automated game testing with deterministic results
- **Clean architecture**: Separated game logic, networking, and presentation layers

## Quick Start

### Server Setup

```bash
# Start server with default configuration
./bin/holdem-server

# Or use Docker for complete setup
./scripts/docker-test.sh test

# Custom configuration
./bin/holdem-server --config holdem-server.hcl
```

### Client Connection

```bash
# Interactive human player
./bin/holdem-client --player "YourName"

# Connect to remote server
./bin/holdem-client --server ws://remote:8080 --player "Alice"

# Docker interactive client
./scripts/docker-test.sh client Alice
```

### Development/Testing

```bash
# Complete integration test
./scripts/docker-test.sh test

# Bot vs bot action
./scripts/docker-test.sh full
```

### Poker Odds Calculator

```bash
# See comprehensive help and examples
go run cmd/poker-odds/main.go --help
```

## Gameplay

### Game Commands
- `/list` - List available tables
- `/join <table_id>` - Join a table with configurable buy-in
- `/leave` - Leave current table
- `/quit` - Quit the game

### Poker Actions
- `call`, `c` - Call the current bet
- `raise 50`, `r 50` - Raise to 50
- `fold`, `f` - Fold hand
- `check`, `k` - Check (when no bet)
- `allin`, `a` - Go all-in

### Game Features
- Standard 6-max tables (2-10 players supported)
- Configurable buy-ins (default: 50-500 big blinds)
- Small blind: $1, Big blind: $2
- Complete hand progression: Pre-flop → Flop → Turn → River → Showdown
- Real-time multiplayer with WebSocket communication
- Hand history and game logs

## Game Rules

- Standard Texas Hold'em rules
- No Limit betting structure
- Blinds remain static throughout session
- Bankrolls are ephemeral (reset each session)



## Architecture

```
┌─────────────────┐    WebSocket    ┌─────────────────┐
│   Client 1      │◄──────────────► │                 │
│   (Human/Bot)   │                 │                 │
├─────────────────┤                 │  Holdem Server  │
│   Client 2      │◄──────────────► │                 │
│   (Human/Bot)   │                 │  - GameService  │
├─────────────────┤                 │  - Tables       │
│   Client N      │◄──────────────► │  - Bots         │
│   (Human/Bot)   │                 │                 │
└─────────────────┘                 └─────────────────┘
```

## Project Structure

```
cmd/
├── holdem-server/   # WebSocket game server
├── holdem-client/   # Interactive client
└── poker-odds/      # Poker odds calculator tool

internal/
├── game/           # Core game logic and state management
├── deck/           # Card and deck implementation
├── evaluator/      # Hand strength evaluation and equity calculation
├── bot/            # AI player strategies
├── tui/            # Terminal UI components (Bubble Tea)
├── server/         # WebSocket server and protocol
└── client/         # Client networking and game interface

docs/               # Protocol documentation and setup guides
scripts/            # Docker and deployment scripts
```

## Configuration

The system uses HCL configuration files for flexible setup:

- **`holdem-server.hcl`** - Server configuration (tables, bots, networking)
- **`holdem-client.hcl`** - Client configuration (player settings, UI preferences)

See [`docs/configuration.md`](docs/configuration.md) for complete configuration options.

## Development

### Commands

- **Server**: `./bin/holdem-server [--config server.hcl]`
- **Client**: `./bin/holdem-client [--config client.hcl]`
- **Tests**: `go test ./...` or `./bin/task test`
- **Docker**: `./scripts/docker-test.sh [test|full|client]`

### Contributing

- **Commits**: Use conventional commits with prefixes `chore`, `feat`, `fix`, or area-specific like `fix(server)`, `feat(client)`, `chore(docs)`

### Documentation

- [WebSocket Protocol](docs/websocket-protocol.md) - Client/server communication
- [Configuration Guide](docs/configuration.md) - HCL setup and options
- [Docker Setup](docs/docker-setup.md) - Containerized deployment
- [Poker Rules](docs/poker-rules.md) - Game implementation specification
