# Claude Project Context

## Project Overview

PokerForBots is a high-performance poker server designed specifically for bot-vs-bot play. The key design principle is speed and simplicity - each hand is independent with no persistent state, optimizing for rapid gameplay and minimizing collusion opportunities.

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

Phase 2 Complete ✅ - Core infrastructure complete, moving to polish phase

## Commit Strategy

Use conventional commits and commit after each numbered milestone in TODO.md:
- After 1. Project Setup → `feat: initial project setup with dependencies`
- After 2. Protocol Layer → `feat: add msgpack protocol definitions`
- After 3. Card & Game Logic → `feat: implement core game logic with bit-packed cards`
- After 4. Server Core → `feat: add websocket server and bot management`
- After 5. Hand Execution → `feat: implement hand runner and game integration`
- After 6. Test Bots → `feat: add test bot implementations`
- After 7. Demo Runner → `feat: add demo script and statistics`
- After 8. Integration Testing → `test: add integration and load tests`

Each commit should include all tests for that milestone.

Update @TODO.md after each task, and make sure all tests are passing before commit.

YOU MUST NEVER COMMIT FAILING TESTS, EVEN IF THE TEST IS UNRELATED TO YOUR CHANGES.

## Key Implementation Patterns

### Dependency Injection
All components requiring randomness accept *rand.Rand in constructors for deterministic testing.

### Stateless Hand Design
- Each hand is independent with random bot selection
- Button position is randomized, not rotated
- No persistent state between hands

### Testing Strategy
- Unit tests for core logic
- Integration tests for complete scenarios
- Race detection enabled throughout
- Deterministic testing via seeded RNG
