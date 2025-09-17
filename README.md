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

## Human CLI Client

Run the minimal interactive client from `cmd/client` to play a hand yourself:

```bash
go run ./cmd/client --server ws://localhost:8080/ws --name Alice
```

For comfortable play, start the server with a higher timeout (for example `-timeout-ms 10000`). While acting you can type `info` to reprint the latest table state.

The client renders cards with suit emojis, color-coded chip counts, and a hand-history style log driven by live player-action events.

## Performance

The server achieves 100+ hands per minute with 6 bots, with sub-100ms decision timeouts and concurrent hand execution.

## Status

✅ Phase 2 Complete - Core infrastructure and demo ready. See [TODO.md](TODO.md) for development progress.
