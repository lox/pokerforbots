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
1. Server accepts WebSocket connections from multiple bots ✅
2. Automatically matches available bots into hands ✅
3. Deals cards and manages betting rounds correctly ✅
4. Handles timeouts gracefully (auto-fold) ✅
5. Determines winners and distributes pots ✅
6. Returns bots to pool for next hand ✅
7. Sustains 350+ hands per second with 6 bots ✅ (Achieved!)

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
- [x] Add HTTP `/games` endpoint for runtime game inspection/discovery
- [x] Add `POST/DELETE /admin/games` HTTP endpoints for runtime table management (authentication TODO)
- [ ] Add authentication/authorization for `/admin/*` endpoints
- [x] Retire `cmd/spawn-bots` once in-process NPC bots cover test/demo use-cases

### 14. Deterministic Testing Tools (TODO)
- [x] Add `--seed` flag and fixed-hand runs (`--hands`) to `cmd/server`; propagate to `server.Config`
- [x] Allow admin-created games to specify `seed`/`hands` and surface `/admin/games/{id}/stats` aggregates
- [x] Emit `game_completed` websocket messages (with per-bot stats) when a hand limit completes
- [ ] Add `--mirror` flag to `cmd/server`; propagate to `server.Config`
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

### 16. Optional Server-Side Statistics ✅
- [x] Create StatsCollector interface with Null and Full implementations
- [x] Extend game configuration with `EnableStats` and `StatsDepth` flags
- [x] Move statistics package to server with memory management controls
- [x] Enhance HandRunner to collect detailed stats when enabled
- [x] Wire StatsCollector integration with pool.RecordHandOutcome
- [x] Update GameCompleted protocol with optional detailed stats
- [x] Add memory management (circular buffer with automatic reset)
- [x] **Tests:** comprehensive unit tests with benchmarks showing zero overhead when disabled

### 17. Optional Auth & Identity Enhancements (Future)
- [ ] Extend `connect` to accept `auth_token`; wire minimal HMAC validator (behind flag)
- [ ] Track bot identity metadata without persistent bankroll (ephemeral stacks per hand remain default)
- [ ] Gate simulation control behind auth checks
- [ ] **Tests:** auth handshake, failure cases, backwards compatibility with unauthenticated bots

## Complex Bot: Winner vs NPCs (Refined)

- Goal: upgrade the "complex" bot into a consistent winner versus built‑in NPCs by tightening preflop ranges, adding fold discipline, standardized bet sizing, simple postflop heuristics, opponent exploitation, and SPR awareness — all localized to `sdk/examples/complex/main.go`.

### Current Status
- SDK plumbing fixed: chip payouts applied on hand_result; `StartingChips` tracked in state.
- Patch 1 implemented: preflop ranges/sizing, fold thresholds, SPR guards, standardized bet sizes.
- Patch 2 foundation implemented: coarse postflop `classifyPostflop()` integrated into decisions; min-raise sizing respected (uses `MinRaise` and `MinBet`).
- Patch 3 basics implemented: track per-opponent VPIP and AF; preflop exploit (no bluff 3-bet vs nits, occasional pressure vs loose openers IP); postflop exploit vs passive (overfold big bets) and aggressive (apply pressure vs small bets) villains.
- Validation (1k hands, infinite bankroll): +433.01 BB/100, Showdown WR 36.0%, Non-showdown BB +467.5. Quick sanity check only; requires larger batches.
- Prior batch (5×10k, no infinite bankroll): per-run BB/100 ≈ [63.3, 45.5, 37.6, 38.9, 56.8], mean ≈ +48.4 BB/100. CO still slightly negative; non-showdown slightly negative (pre-Patch 2 refinement).

### Remaining Issues
- Postflop strength still heuristic; board texture handling is coarse.
- Opponent tags not yet fully integrated into all lines (e.g., check-raise frequencies, river thin value).

### Success Criteria
- Primary: BB/100 > 0 over 50k hands vs default 3‑NPC mix (target +20 to +80 BB/100).
- Showdown Win Rate ~55% with substantially lower showdown frequency (<25%).
- BTN/CO clearly positive; early positions tighter and near breakeven.
- 95% CI lower bound on BB/100 after 5×10k runs > −20.

### Implementation Notes (single file)
- Modify `sdk/examples/complex/main.go` only.
- Add helpers:
  - `betSize(req, pct float64) int`
  - `shouldFold(req protocol.ActionRequest, equity float64) bool`
  - `classifyPostflop() (class string, equity float64)`
  - `deriveVillainTag(seat int) string`
  - `calcSPR(req protocol.ActionRequest) float64`
- In `makeStrategicDecision`:
  - Route preflop → `preflopDecision(req)` with the ranges and sizes above.
  - Postflop → use `classifyPostflop`, `shouldFold`, sizing table, and SPR rules.
- Keep `evaluateHandStrength` as a thin wrapper over `classifyPostflop` for now.

### Validation Loop
- Quick sanity: `go run ./cmd/server --npc-bots=5 --bot-cmd "go run ./sdk/examples/complex" --hands=1000 --infinite-bankroll --enable-stats --print-stats-on-exit`
- Run 5×10k hands with stats enabled:
  - `task server -- --infinite-bankroll --hands 10000 --timeout-ms 20 --npc-bots 3 --enable-stats --stats-depth=full`
  - `go run ./sdk/examples/complex`
- Pass if:
  - Mean BB/100 > 0 and 95% CI lower bound > −20.
  - Showdown frequency falls below 35% after Patch 1; to ≤25% after Patch 2.
- Scenario checks (single 20k hand runs):
  - 3 calling stations → should be clearly profitable (value heavy).
  - 3 aggro bots → around breakeven or better (fold discipline working).
  - 2 random + 1 tight → BTN steals show strong positive result.

### Benchmarks / Targets
- Short‑term: eliminate catastrophic leaks (BB/100 > −50), showdown rate < 40%.
- Mid‑term: BB/100 > 0 in 50k hands; BTN/CO strongly positive; showdown ≤ 25%.
- Long‑term: BB/100 +20 to +80 vs default NPC mix; stable across seeds.

### Nice‑to‑Have (later)
- Board texture classifier (A‑high dry vs low/connected/wet) to refine c‑betting.
- Mixed frequencies by RNG for balance.
- Simple range heatmaps for self‑checks in logs.

## Mirror Deals and Self-Play (Deterministic Testing Tools)

- Add `--mirror` and `--mirror-count` flags to `cmd/server`, propagate to `server.Config`.
- Deck injection:
  - Add `NewDeckFromCards` and basic JSON deck format in `internal/game/cards.go` for dev mode.
  - HandRunner to reuse the same deck across N mirrors and rotate seat-to-card mapping per mirror.
- Metadata (optional but useful): add `hand_group_id` and `mirror_index` to protocol messages for logging.
- Tests:
  - Deterministic hand replay across mirrors with identical boards and rotated hole cards.
  - CLI coverage for `--mirror`/`--mirror-count`.

## Snapshot & Self-Play Benchmarking

- Snapshot current bot: tag repo `complex-bot-snapshot-YYYYMMDD`.
- Self-play harness:
  - Run mirrors with multiple instances of the complex bot (mirror mode on) to reduce variance.
  - Record BB/100 and CI over 5×50k–100k hands; disable NPC respawns for controlled matchups.

