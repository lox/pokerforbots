# PokerForBots

A high-performance Texas Hold'em poker server optimized for bot-vs-bot play.

## Quick Start

```bash
# Build the server
task build

# Run the server
task run

# Run tests
task test

# Run demo with test bots
task spawn-bots -- -bots 6 -spawn-server
```

## Documentation

- [Design Overview](docs/design.md) - Architecture and design decisions
- [WebSocket Protocol](docs/websocket-protocol.md) - Message format and protocol specification
- [Poker Rules](docs/poker-rules.md) - No-limit Hold'em rules implementation
- [Operations Guide](docs/operations.md) - Running and monitoring the server

## Performance

The server achieves 100+ hands per minute with 6 bots, with sub-100ms decision timeouts and concurrent hand execution.

## Status

✅ Phase 2 Complete - Core infrastructure and demo ready. See [TODO.md](TODO.md) for development progress.
