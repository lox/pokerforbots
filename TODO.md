# TODO: PokerForBots Implementation

## Current Status: Phase 2 Complete ✅

**Completed:**
- ✅ Core poker server with WebSocket support
- ✅ Bit-packed card representation for performance
- ✅ Complete Texas Hold'em game logic with all betting rounds
- ✅ Hand evaluation and winner determination
- ✅ Side pot management for all-in scenarios
- ✅ Test bot framework with 3 strategies (calling station, random, aggressive)
- ✅ Demo script that runs server with 6 bots
- ✅ All tests passing including race detection

**Recently Completed Infrastructure Improvements:**
- ✅ Dependency injection for *rand.Rand throughout codebase
- ✅ Fix RNG race conditions with proper mutex protection
- ✅ Action routing security with bot ID verification
- ✅ Deterministic bot ID generation for testing
- ✅ Consolidated test suite with better organization
- ✅ Fixed button randomization in stateless design

## Simple Performance Goals
- ✅ Handle 100ms timeouts reliably
- ✅ Run many concurrent hands
- ✅ Use bit-packed cards for speed (like the Zig code)

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
  - [x] Test timeout triggers auto-fold (implemented in hand runner)
  - [x] Test with multiple concurrent bots

### 5. Hand Execution ✅
- [x] Hand runner (internal/server/hand_runner.go)
  - [x] Deal cards
  - [x] Run betting rounds
  - [x] Handle timeouts (100ms auto-fold)
  - [x] Broadcast updates
  - [x] Return bots to pool
- [x] Integration with game logic
  - [x] Action validation
  - [x] State transitions
  - [x] Winner calculation
- [x] **Tests:**
  - [x] Test hand runner broadcasts
  - [x] Test action requests
  - [x] Test hand completion scenarios

### 6. Code Quality & Edge Cases ✅
- [x] Fix Go idiom issues
  - [x] Replace panic/recover in bot.SendMessage with proper channel handling
  - [x] Add error checking in tests
  - [x] Convert to table-driven tests where appropriate
- [x] Address missing edge cases
  - [x] Implement proper heads-up blind posting
  - [x] Add comprehensive split pot testing
  - [x] Test invalid action scenarios thoroughly
  - [x] Fix race conditions in pool tests
  - [x] Improve disconnection handling during hands (graceful handling with error returns)
- [x] **Tests:**
  - [x] Integration tests for complete hand scenarios
  - [x] Stress test with rapid connections/disconnections
  - [x] Verify all edge cases pass

## Phase 2: Demo Setup

### 7. Test Bots
- [x] Create cmd/testbot/main.go
  - [x] Simple bot framework
  - [x] Connect and play loop
- [x] Implement 3 simple bots:
  - [x] Calling station (always calls/checks)
  - [x] Random bot (random valid actions)
  - [x] Aggressive bot (raises often)
- [x] **Tests:**
  - [x] Test bots connect and respond within timeout

### 8. Demo Runner ✅
- [x] Create demo script that:
  - [x] Starts server
  - [x] Launches 4-6 test bots
  - [x] Shows hands completing in terminal
  - [x] Displays basic stats (hands/second)
- [x] **Tests:**
  - [x] Verify demo runs for 1000 hands without errors (579 hands/min achieved)

### 9. Integration Testing ✅
- [x] End-to-end test with real bots
- [x] Test edge cases (all-ins, everyone folds, etc.)
- [x] Basic load test with 20+ bots (created, can be run with -run TestLoadWith20Bots)

## Phase 3: Polish

### 10. Logging & Metrics ✅
- [x] Add basic logging
  - [x] Hand start/end (Info level with duration)
  - [x] Player actions (Info level for all actions)
  - [x] Errors (Error level with context)
- [x] Simple metrics
  - [x] Hands per second counter
  - [x] Timeout counter (tracked and exposed in /stats)

### 11. Configuration ✅
- [x] Add command-line flags for:
  - [x] Server port (-addr flag)
  - [x] Blinds and starting chips (-small-blind, -big-blind, -start-chips)
  - [x] Timeout values (-timeout-ms)
  - [x] Min/max players (-min-players, -max-players)

### 12. Human CLI Client ✅
- [x] Build minimal interactive CLI for human players
  - [x] Connect via WebSocket with msgpack protocol
  - [x] Display table state updates and prompt for actions
  - [x] Ship as `cmd/client`

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

## Phase 4: Multi-Game & Simulation Harness (Planned)

### 13. Game Manager & Lobby ✅
- [x] Introduce a `GameManager` that tracks multiple named game instances with per-table configs
- [x] Update connection flow: bots specify a game during connect (defaulting to `default`)
- [x] Teach `protocol.Connect` new fields (`game`, `role`, `auth_token` placeholder)
- [x] Ensure bots persist metadata (display name, game, role) for logging/hand runner usage
- [x] **Tests:** updated integration helpers to send connect handshake before assertions

TODO follow-up:
- [ ] Implement lobby/list/leave protocol messages so clients can discover games dynamically
- [x] Add HTTP `/games` endpoint for runtime game inspection/discovery
- [x] Add `POST/DELETE /admin/games` HTTP endpoints for runtime table management (authentication TODO)
- [ ] Add authentication/authorization for `/admin/*` endpoints
- [ ] Retire `cmd/spawn-bots` once in-process NPC bots cover test/demo use-cases

### 14. Deterministic Testing Tools (TODO)
- [ ] Add `--seed` and `--mirror` flags to `cmd/server`; propagate to `server.Config`
- [ ] Allow game configs to opt into mirror runs (replay same deck across seat rotations)
- [ ] Define deck/script injection format for dev mode (JSON or seed list)
- [ ] Expose hand group metadata (`hand_group_id`, `mirror_index`) in logs/protocol
- [ ] **Tests:** deterministic hand replay, mirror rotation correctness, CLI flag coverage

### 15. Simulation Control Channel (TODO)
- [ ] Add authenticated control endpoint (WebSocket or HTTP) for scheduling simulations
- [ ] Implement `simulate` / `simulation_update` / `simulation_complete` protocol messages (docs marked TODO)
- [ ] Support reserving specific bots for a simulation run without disrupting other games
- [ ] Emit aggregated results (mean BB/hand, CI) per simulation session
- [ ] **Tests:** integration covering multi-session scheduling and cleanup

### 16. Optional Auth & Identity Enhancements (Future)
- [ ] Extend `connect` to accept `auth_token`; wire minimal HMAC validator (behind flag)
- [ ] Track bot identity metadata without persistent bankroll (ephemeral stacks per hand remain default)
- [ ] Gate simulation control behind auth checks
- [ ] **Tests:** auth handshake, failure cases, backwards compatibility with unauthenticated bots
