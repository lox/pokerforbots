# PokerForBots

A high-performance Texas Hold'em poker server optimized for bot-vs-bot play.

## Features

- WebSocket server with msgpack binary protocol
- Sub-100ms decision timeouts
- Concurrent hand execution
- Automatic bot matchmaking
- No persistent state between hands

## Quick Start

```bash
# Install Task (if not already installed)
# brew install go-task/tap/go-task

# Build the server
task build

# Run the server
task run

# Run tests
task test

# Connect bots (see docs/websocket-protocol.md for protocol details)
```

## Documentation

- [Design Overview](docs/design.md) - Architecture and design decisions
- [WebSocket Protocol](docs/websocket-protocol.md) - Message format and protocol specification
- [Poker Rules](docs/poker-rules.md) - No-limit Hold'em rules implementation
- [TODO](TODO.md) - Implementation roadmap

## Performance Goals

- Handle 100ms decision timeouts reliably
- Run many concurrent hands
- 100+ hands per minute with 6 bots

## Status

🚧 Under active development - see [TODO.md](TODO.md) for progress