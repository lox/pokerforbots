# PokerForBots - Project Status

## Status: v1.0 Ready ✅

PokerForBots is a complete, production-ready poker server for bot-vs-bot play.

## Core Features Complete

### Server Infrastructure ✅
- High-performance WebSocket server with msgpack protocol
- Bit-packed card representation for speed
- Complete Texas Hold'em game logic with all betting rounds
- Hand evaluation and winner determination
- Side pot management for all-in scenarios
- Handles 350+ hands/second with 6 concurrent bots
- 100ms decision timeouts with graceful handling

### Bot Framework ✅
- Clean `sdk/bot.Bot` interface for building bots
- Built-in bots: calling-station, random, aggressive, complex
- Simple WebSocket protocol (msgpack binary)
- Go SDK with handlers for all game events

### Developer Tools ✅
- `pokerforbots spawn` - Quick testing with bots
- `pokerforbots bot` - Run built-in bots
- `pokerforbots regression` - Statistical bot comparison
- `pokerforbots server` - Standalone server
- `pokerforbots client` - Interactive human client
- Multi-game support with independent hand limits
- Deterministic testing with seed support

### Testing ✅
- Comprehensive unit tests for all core logic
- Integration tests with real bots
- Race detection enabled throughout
- Load tested with 20+ bots
- All edge cases covered (all-ins, everyone folds, etc.)

## Architecture Decisions

### Stateless Design
Each hand is independent with random bot selection and randomized button position. No persistent bankroll or state between hands. This optimizes for:
- Rapid gameplay (no state management overhead)
- Minimized collusion opportunities
- Easy testing and deterministic replay

### Performance
- Bit-packed cards (uint64 bitset, like Zig reference implementation)
- Msgpack binary protocol (~20-50ns per message)
- Goroutine per connection
- Channel-based bot pool for matching

### Bot Distribution
All bots compiled into single `pokerforbots` binary using `sdk/bots` package. No external dependencies or build steps required. Spawn uses `pokerforbots bot <name>` subcommand internally.

## What's Not Included

These were explicitly excluded from v1.0:

- **CFR/GTO Solver** - Moved to separate Zig project (Aragorn)
- **Persistent bankrolls** - By design (stateless hands)
- **Authentication** - Not needed for bot tournaments
- **Database** - No persistent state to store
- **Neural networks** - Users bring their own AI

## Usage Examples

```bash
# Quick test with 6 bots, 100 hands
pokerforbots spawn --spec "calling-station:2,random:2,aggressive:2" --hand-limit 100

# Test your bot vs built-ins
pokerforbots spawn --spec "calling-station:5" --bot-cmd "./my-bot" --hand-limit 1000

# Statistical comparison
pokerforbots regression --mode heads-up --hands 10000 \
  --challenger "./my-bot-v2" --baseline "./my-bot-v1"

# Run standalone server
pokerforbots server --addr :8080

# Connect a bot
pokerforbots bot complex --server ws://localhost:8080/ws
```

## Future Possibilities

Not planned for near-term, but possible:

- Mirror mode (replay hands with seat rotation for evaluation)
- More sophisticated complex bot implementation
- Example bots in other languages (Python, JS)
- Tournament brackets
- Hand history export

## Key Documents

- [Design Overview](docs/design.md) - Architecture and decisions
- [WebSocket Protocol](docs/websocket-protocol.md) - Message format
- [Go SDK](docs/sdk.md) - Bot development guide
- [Operations](docs/operations.md) - Running the server

## Performance Metrics

Achieved on Apple M1:
- 350+ hands/second with 6 bots (target: 100 hands/sec)
- 100ms timeout handling (target: 100ms)
- Zero race conditions under load
- Graceful handling of rapid connect/disconnect

## Contributing

This is research infrastructure. Contributions welcome for:
- Bug fixes
- Performance improvements
- Additional test coverage
- Documentation improvements
- Example bot implementations

Not accepting:
- Feature creep (authentication, databases, etc.)
- Breaking protocol changes
- Non-deterministic behavior