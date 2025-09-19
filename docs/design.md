# PokerForBots Design Document

## Overview

PokerForBots is a high-performance Texas Hold'em poker server optimized for bot-vs-bot play. The design prioritizes speed, fairness, and simplicity while minimizing opportunities for collusion.

### Core Principles

1. **Speed First**: 350+ hands/second throughput with 10ms decision timeouts
2. **Stateless Hands**: Each hand is independent - no chip carryover, no persistent state
3. **Continuous Play**: Bots immediately re-enter the pool after each hand
4. **Simple Protocol**: msgpack binary protocol for minimal latency
5. **Fair Matching**: Random bot selection for each hand

## Architecture

### System Components

```
Bot → WebSocket → Game Manager → Game Instance → Bot Pool → Hand Runner → Game Engine
```

- **WebSocket Server**: Manages bot connections
- **Game Manager**: Mediates lobby/game selection and routes bots to the appropriate game instance
- **Game Instance**: Encapsulates per-table configuration and lifecycle state
- **Bot Pool**: Queue of available bots waiting for hands inside a game instance
- **Hand Runner**: Executes individual poker hands in parallel
- **Game Engine**: Core poker rules and hand evaluation

### Hand Lifecycle

1. Bot connects and lands in the lobby
2. Bot requests to join a specific game configuration
3. Game instance adds bot to its local pool
4. When the game has enough ready bots, it starts a hand
5. Bots receive hole cards and play through streets
6. Hand completes (fold or showdown)
7. Bots return to the same game pool or detach back to the lobby

## Protocol

### Transport

- WebSocket with binary msgpack messages
- Zero-allocation marshaling via code generation (msgp)
- Message size: 20-100 bytes typical

### Message Types

**Client → Server:**
- `Connect`: Join server with bot name, desired role (player/npc), and target game
- `Action`: Send poker decision (fold/call/raise)

**Server → Client:**
- `HandStart`: Deal hole cards, announce players
- `ActionRequest`: Request decision with timeout
- `GameUpdate`: Broadcast other player actions
- `HandResult`: Announce winner(s)

### Timeout Handling & Disconnects

- Configurable timeout per action (default 100ms)
- Auto-fold on timeout
- Immediate fold on WebSocket disconnect; the seat is removed from the hand with no replacement
- Reconnecting bots must initiate a fresh session (new connection + Connect message)
- Future roadmap: authenticate reconnecting bots and restore their bankroll when rejoining the pool, but never resume an in-flight hand
- No mid-hand reconnection support – simplicity over resilience remains the guiding principle

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

### Performance Achieved

- ✅ **350+ hands/second** with 6-bot tables at 10ms timeout
- ✅ **2000+ hands/second** aggregate with parallel games
- ✅ **5ms decision timeout** capability demonstrated
- ✅ **<1ms message processing** via binary msgpack protocol
- Scales to 1000+ concurrent connections

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
  require_player: true   # Hand will only start if at least one player-role bot is seated
  hand_limit: 0          # 0 = unlimited, otherwise stop spawning hands after N

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

## Architectural Decisions

### Randomization Strategy
The server uses dependency injection for all random number generation. A single *rand.Rand instance is passed through constructors, enabling deterministic testing while preventing race conditions through proper synchronization.

### Security Model
- Actions are internally wrapped in ActionEnvelope with bot ID verification
- Timeouts are enforced server-side (100ms) with automatic folding
- No client can affect another client's actions
- Input validation on all messages
- Secure random number generation for cards

### Stateless Design
Each hand is completely independent:
- Random selection of 2-9 bots from the available pool (respecting per-game requirements such as "must include a player")
- Button assigned to the first seat in that shuffled order (no rotation carried between hands), so the seating shuffle alone defines blinds/position
- Fresh game state with no carryover

### Game Manager & Simulation Roadmap
- **Multiple Game Instances**: The server will expose multiple named "games", each with their own `Config` (blinds, timeouts, min/max seats) and bot pool. Bots join and leave games explicitly through the protocol.
- **Bot Roles**: Connecting bots declare a role (`player` or `npc`). Game configs (default included) can require at least one `player` before starting a hand, keeping background sparring bots idle until a focus bot attaches.
- **Built-in NPC Bots**: Games may spawn NPC opponents (calling station, aggressive, random). The server runs these strategies in-process when configured via the admin API.
- **Deterministic Runs**: CLI flags (`--seed`, `--hands`) and matching admin payload fields let you spin up reproducible games that halt after N hands, while `/admin/games/{id}/stats` exposes per-bot performance metrics for rapid tuning.
- **Session Completion Signal**: When a game exhausts its configured hand budget it emits a `game_completed` message (with per-bot stats) so tooling can wrap up gracefully without tearing down connections.
- **Mirrored Hands** *(planned)*: Game instances can optionally replay each shuffled deck across every seat rotation to reduce variance during testing.
- **Scenario Scripts** *(planned)*: A game may accept scripted decks or seed lists for deterministic simulation runs without restarting the server.
- **Simulation Control Channel** *(planned)*: Trusted tooling can request ad-hoc simulation sessions (e.g., `N` mirrored hands with specified bots) routed through dedicated game instances, leaving other tables unaffected.

HTTP endpoints (`GET /games`, `GET /stats`, `GET /admin/games/{id}/stats`) expose discovery and monitoring data while the WebSocket protocol stays focused on gameplay messages.

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
