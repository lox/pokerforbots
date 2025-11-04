# AI Project Context

## Project Overview

PokerForBots is a high-performance poker server designed specifically for bot-vs-bot play. The key design principle is speed and simplicity - each hand is independent with no persistent state, optimizing for rapid gameplay and minimizing collusion opportunities.

**Status**: v1.0 complete - production-ready infrastructure for bot tournaments.

## Key Documents

Please refer to these documents for context:

1. **[docs/design.md](docs/design.md)** - Complete system design and architecture
2. **[docs/websocket-protocol.md](docs/websocket-protocol.md)** - Binary msgpack protocol specification
3. **[docs/poker-rules.md](docs/poker-rules.md)** - No-limit Hold'em rules reference
4. **[TODO.md](TODO.md)** - Implementation roadmap with test requirements

## Performance Considerations

- Use bit-packed card representation (see `../poker-bot-project/src/poker/card.zig` for reference implementation)
- Target 100ms decision timeouts
- Optimize for concurrent hand execution
- Keep it simple - avoid over-engineering

## Development Approach

1. Start simple and iterate
2. Test at each stage (see TODO.md for specific test requirements)
3. Use pragmatic optimizations (bit-packing cards, channels for queues)
4. Focus on getting a working demo first, optimize later

## Current Phase

**v1.0 Complete** ✅ - Production-ready poker server with built-in bots and testing tools

Focus is now on:
- Bug fixes and stability
- Performance improvements
- Additional bot implementations (optional)
- Documentation improvements

## Commit Strategy

Use conventional commits and commit after each numbered milestone in TODO.md:
- After 1. Project Setup → `feat: initial project setup with dependencies`
- After 2. Protocol Layer → `feat: add msgpack protocol definitions`
- After 3. Card & Game Logic → `feat: implement core game logic with bit-packed cards`
etc.

Each commit should include all tests for that milestone.

Update @TODO.md after each task, and make sure all tests are passing before commit.

YOU MUST NEVER COMMIT FAILING TESTS, EVEN IF THE TEST IS UNRELATED TO YOUR CHANGES.

YOU MUST NEVER DISABLE THE LINTER TO ALLOW COMMITS. ASK THE USER WHAT TO DO IN THIS CASE.

## Key Implementation Patterns

### Dependency Injection
All components requiring randomness accept *rand.Rand in constructors for deterministic testing.

### Stateless Hand Design
- Each hand is independent with random bot selection
- Button position is randomized, not rotated
- No persistent state between hands

### Bot Architecture
- All bots implement `sdk/bot.Bot` interface with `Run(ctx, serverURL, name, game) error`
- Built-in bots in `sdk/bots/` package compiled into main binary
- `pokerforbots bot <name>` subcommand runs bots
- `pokerforbots spawn` uses bot subcommand internally for process spawning

### Testing Strategy
- Unit tests for core logic
- Integration tests with complete scenarios
- Race detection enabled throughout
- Deterministic testing via seeded RNG
- All tests passing, zero race conditions
