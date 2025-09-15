# PokerForBots Design Document

## Overview

PokerForBots is a high-performance Texas Hold'em poker server optimized for bot-vs-bot play. The design prioritizes speed, fairness, and simplicity while minimizing opportunities for collusion.

### Core Principles

1. **Speed First**: Sub-second hand completion with 100ms decision timeouts
2. **Stateless Hands**: Each hand is independent - no chip carryover, no persistent state
3. **Continuous Play**: Bots immediately re-enter the pool after each hand
4. **Simple Protocol**: msgpack binary protocol for minimal latency
5. **Fair Matching**: Random bot selection for each hand

## Architecture

### System Components

```
Bot → WebSocket → Server → Bot Pool → Hand Runner → Game Engine
```

- **WebSocket Server**: Manages bot connections
- **Bot Pool**: Queue of available bots waiting for hands
- **Hand Runner**: Executes individual poker hands in parallel
- **Game Engine**: Core poker rules and hand evaluation

### Hand Lifecycle

1. Bot connects and enters available pool
2. When 2+ bots available, server starts new hand
3. Bots receive hole cards and play through streets
4. Hand completes (fold or showdown)
5. Bots immediately return to pool

## Protocol

### Transport

- WebSocket with binary msgpack messages
- Zero-allocation marshaling via code generation (msgp)
- Message size: 20-100 bytes typical

### Message Types

**Client → Server:**
- `Connect`: Join server with bot name
- `Action`: Send poker decision (fold/call/raise)

**Server → Client:**
- `HandStart`: Deal hole cards, announce players
- `ActionRequest`: Request decision with timeout
- `GameUpdate`: Broadcast other player actions
- `HandResult`: Announce winner(s)

### Timeout Handling

- Configurable timeout per action (default 100ms)
- Auto-fold on timeout
- No reconnection support - simplicity over resilience

## Game Rules

### Texas Hold'em Variations

- No Limit Hold'em
- Fixed starting chips (e.g., 1000)
- Fixed blinds (e.g., 5/10)
- 2-9 players per hand (configurable)

### Hand Execution

1. **Preflop**: Deal 2 cards to each player
2. **Flop**: Deal 3 community cards
3. **Turn**: Deal 1 community card
4. **River**: Deal 1 community card
5. **Showdown**: Evaluate best 5-card hands

Each street includes a betting round with standard actions:
- Fold
- Check/Call
- Raise (min raise = previous bet, max = all-in)

## Anti-Collusion Design

### Information Isolation

- No hand history during play
- No player statistics visible
- Hole cards only revealed at showdown
- No chat or communication channel

### Temporal Isolation

- Ultra-fast hands (2-10 seconds total)
- Random processing delays (0-50ms)
- No predictable scheduling

### Statistical Noise

- High volume dilutes any edge
- Random starting positions
- Random seat assignments

## Implementation

### Technology Stack

- **Language**: Go 1.21+
- **Protocol**: msgpack with msgp code generation
- **Transport**: gorilla/websocket
- **Concurrency**: Goroutines with channels

### Performance Targets

- 1000+ concurrent hands
- 100ms decision timeout
- <1ms message processing
- 10,000+ hands/second throughput

### Project Structure

```
pokerforbots/
├── cmd/server/          # Server entry point
├── internal/
│   ├── protocol/        # msgpack message definitions
│   ├── game/           # Poker game logic
│   └── server/         # WebSocket and bot management
└── docs/               # Documentation
```

## Configuration

```yaml
server:
  port: 8080
  max_connections: 10000

game:
  min_players: 2
  max_players: 9
  starting_chips: 1000
  small_blind: 5
  big_blind: 10
  decision_timeout_ms: 100

pool:
  match_interval_ms: 10
  random_delay_max_ms: 50
```

## Client Implementation

Bots need to:

1. Connect via WebSocket
2. Send `Connect` message with name
3. Wait for `HandStart` message
4. Respond to `ActionRequest` within timeout
5. Process `GameUpdate` messages
6. Handle `HandResult` and prepare for next hand

Example in Python:
```python
import msgpack
import websocket

ws = websocket.WebSocket()
ws.connect("ws://localhost:8080")
ws.send_binary(msgpack.packb({'type': 'connect', 'name': 'MyBot'}))

while True:
    msg = msgpack.unpackb(ws.recv())
    if msg['type'] == 'action_request':
        # Make decision
        ws.send_binary(msgpack.packb({
            'type': 'action',
            'action': 'call',
            'amount': 0
        }))
```

## Future Considerations

### Phase 1 (MVP)
- Basic game engine
- Simple bot pool
- Fixed stakes/blinds

### Phase 2
- Multiple speed tiers (blitz/rapid/standard)
- Basic statistics tracking
- Hand history export

### Phase 3
- Tournament mode
- Advanced anti-collusion detection
- Performance optimizations (binary protocol, CPU pinning)

## Security Considerations

- No authentication in MVP (add later if needed)
- Rate limiting on connections
- Input validation on all messages
- Secure random number generation for cards

## Testing Strategy

- Unit tests for game logic
- Integration tests with mock bots
- Load testing with 1000+ concurrent bots
- Fuzzing for protocol handling

## Success Metrics

- Hands per second throughput
- P99 decision latency
- Bot timeout rate
- Fair winner distribution (statistical analysis)