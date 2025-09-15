# TODO: PokerForBots Implementation

## Simple Performance Goals
- Handle 100ms timeouts reliably
- Run many concurrent hands
- Use bit-packed cards for speed (like the Zig code)

## Phase 1: Core Infrastructure (MVP)

### 1. Project Setup ✅
- [x] Initialize Go module and dependencies
  - [x] Add gorilla/websocket
  - [x] Add msgpack library (tinylib/msgp)
  - [x] Create Taskfile.yml with generate/build/test tasks
- [x] Create basic project structure
  - [x] cmd/server/main.go
  - [x] internal/protocol/
  - [x] internal/game/
  - [x] internal/server/
- [x] **Tests:**
  - [x] Verify project builds
  - [x] Verify msgp code generation works

### 2. Protocol Layer ✅
- [x] Define msgpack message types in protocol/messages.go
  - [x] Connect, Action (client messages)
  - [x] HandStart, ActionRequest, GameUpdate, StreetChange, HandResult, Error (server messages)
- [x] Setup msgp code generation
- [x] **Tests:**
  - [x] Test message serialization/deserialization
  - [x] Basic benchmark to ensure it's fast enough (~20-50ns per operation)

### 3. Card & Game Logic ✅
- [x] Implement bit-packed card representation (internal/game/cards.go)
  - [x] Card as uint64 (single bit per card, like Zig)
  - [x] Hand as uint64 bitset (like Zig implementation)
  - [x] Simple shuffle function
  - [x] String conversion helpers
- [x] Implement hand evaluator (internal/game/evaluator.go)
  - [x] Basic 7-card evaluation
  - [x] Fast enough for our needs (don't over-optimize)
- [x] Create game state machine (internal/game/hand.go)
  - [x] Hand structure with players, pot, board
  - [x] Betting round logic
  - [x] Pot management (including side pots)
- [x] **Tests:**
  - [x] Test all 52 cards encode/decode correctly
  - [x] Test hand evaluator with all hand types
  - [x] Test basic game flow (some edge cases need work)

### 4. Server Core ✅
- [x] WebSocket server (internal/server/server.go)
  - [x] Accept connections
  - [x] Basic message routing
  - [x] Goroutine per connection
- [x] Bot connection management (internal/server/bot.go)
  - [x] Bot struct with connection and ID
  - [x] Send/receive helpers
  - [x] Ping/pong keepalive
- [x] Bot pool (internal/server/pool.go)
  - [x] Simple channel-based queue
  - [x] Match bots when 2+ available
- [x] **Tests:**
  - [x] Test WebSocket connection/disconnection
  - [ ] Test timeout triggers auto-fold (needs hand runner)
  - [x] Test with multiple concurrent bots

### 5. Hand Execution
- [ ] Hand runner (internal/server/hand_runner.go)
  - [ ] Deal cards
  - [ ] Run betting rounds
  - [ ] Handle timeouts
  - [ ] Broadcast updates
  - [ ] Return bots to pool
- [ ] Integration with game logic
  - [ ] Action validation
  - [ ] State transitions
  - [ ] Winner calculation
- [ ] **Tests:**
  - [ ] Test complete hand flow
  - [ ] Test all-in and side pot scenarios
  - [ ] Test timeout during each street

## Phase 2: Demo Setup

### 6. Test Bots
- [ ] Create cmd/testbot/main.go
  - [ ] Simple bot framework
  - [ ] Connect and play loop
- [ ] Implement 3 simple bots:
  - [ ] Calling station (always calls/checks)
  - [ ] Random bot (random valid actions)
  - [ ] Aggressive bot (raises often)
- [ ] **Tests:**
  - [ ] Test bots connect and respond within timeout

### 7. Demo Runner
- [ ] Create demo script that:
  - [ ] Starts server
  - [ ] Launches 4-6 test bots
  - [ ] Shows hands completing in terminal
  - [ ] Displays basic stats (hands/second)
- [ ] **Tests:**
  - [ ] Verify demo runs for 1000 hands without errors

### 8. Integration Testing
- [ ] End-to-end test with real bots
- [ ] Test edge cases (all-ins, everyone folds, etc.)
- [ ] Basic load test with 20+ bots

## Phase 3: Polish

### 9. Logging & Metrics
- [ ] Add basic logging
  - [ ] Hand start/end
  - [ ] Player actions
  - [ ] Errors
- [ ] Simple metrics
  - [ ] Hands per second counter
  - [ ] Timeout counter

### 10. Configuration
- [ ] Add config file or env vars for:
  - [ ] Server port
  - [ ] Blinds and starting chips
  - [ ] Timeout values
  - [ ] Min/max players

## Completion Criteria

A successful demo should:
1. Server accepts WebSocket connections from multiple bots
2. Automatically matches available bots into hands
3. Deals cards and manages betting rounds correctly
4. Handles timeouts gracefully (auto-fold)
5. Determines winners and distributes pots
6. Returns bots to pool for next hand
7. Sustains 100+ hands per minute with 6 bots

## Next Steps

1. Start with project setup and protocol definition
2. Build game logic in isolation with tests
3. Implement server and bot pool
4. Create simple test bots
5. Run demo and iterate on performance
