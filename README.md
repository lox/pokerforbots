# PokerForBots

A high-performance Texas Hold'em poker server optimized for bot-vs-bot play.

## Quick Start

```bash
# Build the server
task build

# Run the server in the foreground (use tmux for background)
task server

# Run tests
task test
```

## Documentation

- [Design Overview](docs/design.md) - Architecture and design decisions
- [WebSocket Protocol](docs/websocket-protocol.md) - Message format and protocol specification
- [Poker Rules](docs/poker-rules.md) - No-limit Hold'em rules implementation
- [Operations Guide](docs/operations.md) - Running and monitoring the server
- [Development Workflow](docs/development-workflow.md) - Developing the server and the bots in examples

## Human CLI Client

Run the minimal interactive client from `cmd/client` to play a hand yourself:

```bash
go run ./cmd/client --server ws://localhost:8080/ws --name Alice
```

For comfortable play, start the server with a higher timeout (for example `-timeout-ms 10000`). While acting you can type `info` to reprint the latest table state.

The client renders cards with suit emojis, color-coded chip counts, and a hand-history style log driven by live player-action events.
