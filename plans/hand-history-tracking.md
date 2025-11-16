# Hand History Tracking

- Owner: @lox
- Status: complete
- Updated: 2025-11-16
- Tags: feature, server, observability
- Scope: internal/phh, internal/server, cmd/pokerforbots
- Risk: low
- Effort: M

## TL;DR

- Problem: No persistent record of hands for post-game analysis and bot debugging
- Approach: Implement PHH (Poker Hand History) format with buffered per-game recording
- Expected outcome: Optional hand history recording with ~2MB memory overhead and minimal performance impact

## Current Status

**Implementation**: ✅ Complete (all phases 1-6)
**Testing**: ✅ All acceptance criteria met

### Implementation Summary

All critical bugs resolved and verified:
- ✅ Board cards emit in chronological order (flop=3, turn=1, river=1)
- ✅ All-in actions show correct cumulative bet amounts
- ✅ Player rotation (SB-first) per PHH spec
- ✅ 100% PHH spec compliance (no custom extensions)
- ✅ UTC timestamp normalization
- ✅ Accurate showdown tracking via actual card reveals
- ✅ Bulk `.phhs` format with section headers
- ✅ Hole cards masked as `????` by default
- ✅ Cumulative bet amounts on all raises
- ✅ Blind posts correctly skipped
- ✅ Valid TOML structure
- ✅ Buffer concurrency (no races detected)
- ✅ Flush failure state machine (3 strikes → disable)
- ✅ Manager lifecycle (register/unregister)
- ✅ Flushing flag properly cleared via defer

## Goals

- Record complete hand histories in PHH (Poker Hand History) standard format
- Support multi-game server architecture with per-game file isolation
- Minimize performance impact via in-memory buffering and periodic flushing
- Enable post-game analysis, debugging, and statistical validation
- Maintain backward compatibility (disabled by default)

## Non-Goals

- Real-time hand history streaming or live analysis
- Hand replay/visualization tools (future extension)
- PokerStars or other proprietary format support
- Hand history compression or rotation (future extension)
- Remote storage or database persistence

## Background

Currently, pokerforbots logs hand details at Debug level but provides no structured, persistent record. Users testing bots have no way to:

1. Review specific hands where their bot made questionable decisions
2. Validate statistical results against actual hand data
3. Build training datasets for ML-based bots
4. Share reproducible examples with others

### Why PHH Format?

The Poker Hand History (PHH) format is an academic standard published in 2024 (IEEE CoG):

- **Human-readable**: TOML-based, easy to inspect
- **Machine-parseable**: Standard TOML parsers, Python PokerKit library
- **Concise**: 70% smaller than PokerStars format
- **Purpose-built**: Designed for AI research and bot development
- **Multi-variant**: Supports 11+ poker variants (future-proof)

Example PHH file:
```toml
variant = "NT"
antes = [0, 0, 0]
blinds_or_straddles = [1, 2, 0]
min_bet = 2
starting_stacks = [200, 200, 200]
actions = [
  "d dh p1 AhKh",
  "d dh p2 7c2d",
  "d dh p3 QsJs",
  "p1 cbr 6",
  "p2 f",
  "p3 cc",
  "d db Jc3d5c",
  "p1 cbr 10",
  "p3 cc",
  "d db 4h",
  "p1 sm AhKh",
  "p3 sm QsJs",
]
players = ["alice-bot", "bob-bot", "charlie-bot"]
hand_id = "hand-00042"
timestamp = "2025-11-14T15:22:00Z"
```

## Architecture

### Component Structure

```
Server
├── GameManager
│   ├── GameInstance (game-id: "default")
│   │   └── BotPool
│   │       └── HandHistoryMonitor (buffers hands)
│   └── GameInstance (game-id: "custom")
│       └── BotPool
│           └── HandHistoryMonitor (buffers hands)
└── HandHistoryManager
    ├── Periodic flush ticker (every 10s)
    └── Graceful shutdown coordinator
```

### Package Structure

```
internal/
├── phh/                          # NEW: PHH format encoder
│   ├── encoder.go                # TOML encoding
│   ├── encoder_test.go
│   ├── types.go                  # HandHistory struct
│   └── cards.go                  # Card notation (Ah → Ah, 10c → Tc)
│
└── server/
    ├── monitor.go                # HandMonitor interface (existing)
    ├── hand_history/             # NEW: Hand history subpackage
    │   ├── monitor.go            # Buffered monitor (implements server.HandMonitor)
    │   ├── monitor_test.go
    │   ├── manager.go            # Server-wide coordinator
    │   └── manager_test.go
    ├── server.go                 # Add manager initialization
    ├── pool.go                   # Wire monitor into pool
    └── config.go                 # Add EnableHandHistory, etc.
```

### Data Flow

```
Hand completes → HandMonitor.OnHandComplete()
                      ↓
                 Build PHH struct
                      ↓
                 Append to buffer ([]HandHistory)
                      ↓
            Signal flush needed (non-blocking)
                      ↓
       HandHistoryManager ticker goroutine
                      ↓
            Check all monitors for flush
                      ↓
         Async write to disk (off critical path)
                      ↓
              hands/game-{id}/session.phhs
```

**Important**: Flushing happens asynchronously on the manager's ticker goroutine. `OnHandComplete` never blocks on disk I/O - it only adds to the in-memory buffer and signals the flush channel (non-blocking).

### Action Mapping to PHH Format

The server emits these action strings to `OnPlayerAction(handID, seat, action, amountPaid, stackAfter)`:
- `fold` - Player folds voluntarily
- `check` - Player checks
- `call` - Player calls (`amountPaid` = chips contributed this action)
- `raise` - Player raises (`amountPaid` = chips contributed this action)
- `allin` - Player goes all-in (`amountPaid` = chips contributed this action)
- `timeout_fold` - Player timed out or disconnected (forced fold)
- `post_small_blind` - Small blind posted (`amountPaid` = blind amount)
- `post_big_blind` - Big blind posted (`amountPaid` = blind amount)

**Critical issue**: `OnPlayerAction` receives `amountPaid` (chips contributed this step), but PHH requires the **total amount bet this round** for `cbr` actions. The monitor therefore maintains a `seatContributions` slice sized to `len(players)` at `OnHandStart`, resetting it on every `OnStreetChange`. `OnPlayerAction` adds the incremental amount, derives the running total, and feeds that value into the encoder. Empty strings returned by the mapper (e.g. for blind posts) are ignored so the TOML array never contains blank entries.

**PHH mapping summary**:
- `fold`/`timeout_fold` → `f`
- `check`/`call` → `cc`
- `raise`/`allin` → `cbr <totalContributionThisStreet>`
- `post_small_blind`/`post_big_blind` → skipped entirely (info already present in `blinds_or_straddles`)
- Unknown actions → prefixed comment entry for visibility

Timeout metadata (e.g. distinguishing `timeout_fold`) is stored in optional PHH fields for later analysis without leaking private cards.

**Per-game buffering**:
- Each `BotPool` gets its own `HandHistoryMonitor`
- Monitor buffers hands in memory (default: 100 hands)
- Separate output files per game: `hands/game-{gameID}/session.phhs`

**Flush triggers** (whichever comes first):
1. Time-based: Every 10 seconds (configurable)
2. Count-based: Every 100 hands (configurable)
3. Game completion: When BotPool stops
4. Graceful shutdown: Server shutdown flushes all buffers

**Memory overhead**:
- ~2KB per buffered hand (typical)
- 100 hand buffer = ~200KB per game
- 10 concurrent games = ~2MB total (negligible)

### Buffer Concurrency Strategy

The monitor's buffer is accessed from two goroutines:
1. **HandRunner goroutine**: Calls `OnHandComplete()` to append hands
2. **Manager ticker goroutine**: Calls `Flush()` to write hands

**Locking strategy**: A single mutex protects both the buffer and the failure-state fields. `OnHandStart`, `OnStreetChange`, `OnPlayerAction`, and `OnHandComplete` all early-return when the monitor is disabled, ensuring we stop work immediately after a fatal flush failure. `OnHandComplete` holds the lock only long enough to append the finished hand. `Flush` grabs the lock, swaps the buffer with a fresh slice (preserving capacity), releases the lock, and then performs I/O so disk writes never block the hot path. `IsDisabled` is a tiny helper used by the manager when deciding whether to keep invoking a monitor.

### Flush Failure State Machine

**States**: `active` (normal), `degraded` (1–2 consecutive failures), `disabled` (3 failures, recording stops).

**Manager workflow**:
1. Snapshot the registry so the ticker goroutine never holds the lock across I/O.
2. Call `Flush` on each monitor unless it already reports `IsDisabled()==true`.
3. Each monitor updates its own `consecutiveFailures` counter inside the mutex via `recordFlushResult` and tells the manager when it has transitioned to `disabled`.
4. Disabled monitors clear their buffers, log exactly how many hands were dropped, and are unregistered immediately so the pool can rebuild its `MultiHandMonitor` chain without dangling references.

**Data-loss policy**: Up to two failures simply stay buffered for the next attempt. The third failure drops the buffered hands, emits a high-priority log, short-circuits all future callbacks, and forces the pool to detach the monitor so GC can reclaim it. Recovery requires restarting the game with a fresh monitor.

## Plan

### Phase 1 — PHH Encoder Package

Create `internal/phh` package with core encoding logic:

1. Define `HandHistory` struct with all PHH fields (required + optional)
2. Implement TOML encoder using `github.com/BurntSushi/toml`
3. Add card notation helpers:
   - Convert `"10h"` → `"Th"` (PHH uses T for ten)
   - Support `"??"` for unknown cards
4. Add action notation helpers:
   - Map `("raise", 100)` → `"p1 cbr 100"`
   - Map `("fold", 0)` → `"p2 f"`
   - Map `("call", 0)` → `"p3 cc"`
5. Write comprehensive unit tests for encoding

**Files**: `internal/phh/{encoder,types,cards}.go`

### Phase 2 — Buffered HandHistoryMonitor

Implement monitor that buffers hands and writes periodically:

1. Create `HandHistoryMonitor` struct with:
   - Buffer with mutex protection
   - `includeHoleCards` flag
   - `seatContributions []int` for tracking cumulative bets per street
   - `consecutiveFailures` and `disabled` **protected by same mutex**
2. Implement `HandMonitor` interface methods:
   - `OnHandStart`: Initialize new PHH struct, add masked hole cards (`????` unless `includeHoleCards=true`), reset seat contributions
   - `OnStreetChange`: Append board cards, **reset seat contributions to zero**
   - `OnPlayerAction`: Update seat contributions, calculate total bet, append PHH action (skip empty strings for blinds)
   - `OnHandComplete`: Mutex-lock, **short-circuit if disabled**, append to buffer, unlock
3. Add `Flush()` method:
   - Mutex-lock, **check disabled flag**, snapshot buffer, swap with fresh buffer, unlock
   - Write snapshot to disk (no lock held during I/O)
   - Return error for failure tracking
4. Add `recordFlushResult(err, logger, gameID)` method:
   - Mutex-lock, update `consecutiveFailures` and `disabled` **atomically**
   - Clear buffer and log on 3rd failure
   - Return true if should unregister
5. Add `IsDisabled()` method: Thread-safe disabled check
6. Add `Close()` method: final flush on shutdown
7. Write integration tests:
   - Simulated hands with hole card masking
   - Seat contribution tracking across streets
   - Buffer concurrency (race detector)
   - Disabled monitor short-circuits OnHandComplete

**Files**: `internal/server/hand_history/monitor.go`, `internal/server/hand_history/monitor_test.go`

### Phase 3 — HandHistoryManager Coordinator

Create server-wide coordinator for all monitors with proper lifecycle management:

1. Implement `HandHistoryManager` with:
   - Mutex-protected registry of `HandHistoryMonitor` instances (keyed by gameID)
   - Single ticker goroutine for periodic flush (avoids N timers for N games)
   - Graceful shutdown coordination via WaitGroup
   - Flush failure tracking per monitor (disable after 3 consecutive failures)
2. Add `RegisterMonitor(gameID, monitor)`:
   - Lock registry
   - Add monitor to map
   - Unlock
3. Add `UnregisterMonitor(gameID)`:
   - Lock registry
   - Get monitor
   - Call monitor.Close() to flush and cleanup
   - Remove from map
   - Unlock
4. Add `flushAll()` method called by ticker goroutine:
   - Lock registry (read), snapshot monitor map, unlock
   - For each monitor:
     - Skip if `IsDisabled()` returns true
     - Call `Flush()` and get error
     - Call `monitor.recordFlushResult(err, logger, gameID)` to update failure state **atomically**
     - If returns true (3rd failure), add to unregister list
   - After loop, call `UnregisterMonitor()` for each disabled monitor (removes from system)
   - Never panic on nil monitors (race-safe)
5. Wire into `Server.Shutdown()` for final flush:
   - Stop ticker
   - Wait for goroutine to exit (WaitGroup)
   - Flush all remaining buffers
6. Write tests for:
   - Multi-game concurrent flushing
   - Monitor registration/unregistration races
   - Flush failure recovery
   - Graceful shutdown waits for flush

**Files**: `internal/server/hand_history/manager.go`, `internal/server/hand_history/manager_test.go`

**Lifecycle guarantees**:
- RegisterMonitor/UnregisterMonitor are mutex-protected (no races)
- UnregisterMonitor always flushes before removing (no lost data)
- Ticker snapshots monitor list before flushing (no nil dereference)
- Shutdown waits for ticker goroutine and final flush (no truncated writes)

### Phase 4 — Server Integration

Wire hand history into server lifecycle:

1. Add config fields to `Config` struct:
   - `EnableHandHistory bool`
   - `HandHistoryDir string` (default: "hands")
   - `HandHistoryFlushSecs int` (default: 10)
   - `HandHistoryFlushHands int` (default: 100)
   - `HandHistoryIncludeHoleCards bool` (default: false)
2. Initialize `HandHistoryManager` in `NewServer()` if enabled
3. Create `HandHistoryMonitor` when creating `GameInstance`
4. Register monitor with manager
5. Add monitor to `MultiHandMonitor` chain in pool
6. Unregister and flush on game deletion
7. Update `Server.Shutdown()` to flush all monitors

**Files**: `internal/server/{server,pool,config}.go`

### Phase 5 — CLI Integration

Add CLI flags to spawn and server commands:

1. Add flags to `SpawnCmd`:
   ```go
   HandHistory          bool   `help:"Enable hand history recording"`
   HandHistoryDir       string `default:"hands"`
   HandHistoryFlushSecs int    `default:"10"`
   HandHistoryFlushHands int   `default:"100"`
   HandHistoryHoleCards bool   `help:"Include hole cards (for debugging)"`
   ```
2. Add same flags to `ServerCmd`
3. Pass config through to server initialization
4. Add log messages when hand history is enabled
5. Update help text and examples

**Files**: `cmd/pokerforbots/{spawn,server}.go`

### Phase 6 — Testing & Documentation

Comprehensive testing and user-facing docs:

1. Unit tests:
   - PHH encoding (valid TOML, correct action notation)
   - Card notation conversion
   - Monitor buffering and flushing
   - Manager coordination
2. Integration tests:
   - Full hand lifecycle → PHH file
   - Multi-game isolation
   - Graceful shutdown flush
   - Buffer overflow handling
3. Documentation:
   - Add `docs/hand-history.md` with PHH format guide
   - Update `docs/operations.md` with hand history section
   - Add usage examples to README
   - Create `examples/analyze-phh.py` for parsing demo

**Files**: `*_test.go`, `docs/hand-history.md`, `docs/operations.md`

## Configuration

```go
type Config struct {
    // Hand history recording
    EnableHandHistory         bool          // Enable recording (default: false)
    HandHistoryDir            string        // Output directory (default: "hands")
    HandHistoryFlushSecs      int           // Flush interval (default: 10)
    HandHistoryFlushHands     int           // Flush after N hands (default: 100)
    HandHistoryIncludeHoleCards bool        // Include all hole cards (default: false)
}
```

## File Organization

```
hands/
├── game-default/
│   └── session.phhs          # All hands for default game (append-only)
├── game-custom-abc123/
│   └── session.phhs          # All hands for custom game
└── game-heads-up/
    └── session.phhs
```

Each `session.phhs` contains multiple hands with section headers:

```toml
[1]
variant = "NT"
hand = "hand-1"
# ... hand 1 data ...

[2]
variant = "NT"
hand = "hand-2"
# ... hand 2 data ...
```

### File Size Expectations

**Growth rate** (6-player game):
- ~2KB per hand (typical, without hole cards)
- 350 hands/sec × 2KB = 700KB/sec = ~2.5GB/hour
- 100 hand limit test = ~200KB file
- 10,000 hand session = ~20MB file

**Retention strategy** (v1):
- No automatic rotation or cleanup
- Users responsible for managing `hands/` directory
- Recommend periodic cleanup or archival for long-running servers
- Consider adding file size warnings in future (e.g., warn at 100MB, alert at 1GB)

**Future extensions**:
- Automatic rotation at size threshold (e.g., `session-001.phh`, `session-002.phh`)
- Gzip compression (could reduce size by 80%+)
- Retention policies (auto-delete files older than N days)
- Remote upload (S3, GCS) with local cleanup

## Acceptance Criteria

- [x] PHH files are valid TOML and parseable by standard TOML libraries
- [x] Hand histories include all required PHH fields (variant, stacks, actions)
- [x] Buffering reduces disk I/O to 1 write per 100 hands or 10 seconds
- [x] Graceful shutdown flushes all buffered hands (0% data loss)
- [x] Memory overhead < 5MB per game (measured at 100 hand buffer)
- [x] No measurable performance regression in spawn throughput (350+ hands/sec maintained)
- [x] Multi-game server correctly isolates hand histories by game ID
- [x] Hole cards are excluded by default, included only with `--hand-history-hole-cards`
- [x] All tests pass including race detection: `go test -race ./...`
- [x] 100% PHH spec compliance (no custom extensions)
- [x] Player rotation follows PHH spec (SB-first ordering)
- [x] UTC timestamps for consistent time handling

## Risks & Mitigations

- **Risk**: Buffering causes data loss on crash — **Mitigation**: Periodic flush limits loss to 10s window; document trade-off
- **Risk**: Large buffers consume memory — **Mitigation**: Default 100 hands (~200KB), configurable limit
- **Risk**: Disk I/O blocks hand execution — **Mitigation**: Async flush on manager goroutine, never blocks OnHandComplete
- **Risk**: Flush failures (disk full, permissions) lose data — **Mitigation**: Log errors clearly, continue buffering for retry; after 3 consecutive failures, disable recording for that game and alert
- **Risk**: PHH format changes — **Mitigation**: Version field in HandHistory; favor stable v1 spec
- **Risk**: File permissions errors — **Mitigation**: Create dirs with 0755, files with 0644; fail-fast on first write with clear error
- **Risk**: Monitor lifecycle races (game churn) — **Mitigation**: Mutex-protected registration map, always flush before unregister, ticker checks monitor existence before flush

## Performance Considerations

**Disk I/O**:
- Single append-only write per flush
- Buffered I/O via `bufio.Writer` (future optimization)
- No locks held during write (snapshot buffer first)

**Memory**:
- Fixed-size buffer per game
- Slice reuse to avoid allocations
- ~2KB per hand × 100 hands = 200KB per game

**CPU**:
- TOML encoding is fast (~100μs per hand)
- No serialization on hot path (buffered)
- Flush happens off critical path

**Benchmarks** (to validate):
```bash
# Baseline: spawn without hand history
pokerforbots spawn --spec "complex:6" --hand-limit 1000 --output dots

# With hand history enabled
pokerforbots spawn --spec "complex:6" --hand-limit 1000 --output dots --hand-history

# Should see < 5% throughput difference
```

## Tasks

**Phase 1 - PHH Encoder**: ✅ COMPLETE
- [x] Create `internal/phh` package with encoder, types, cards helpers
- [x] Implement card notation conversion (10c → Tc)
- [x] Write PHH encoder unit tests (valid TOML, action notation)

**Phase 2 - Monitor**: ✅ COMPLETE
- [x] Create `internal/server/hand_history/` subpackage
- [x] Implement `Monitor` with mutex-protected buffer
- [x] Add `seatContributions` tracking for cumulative bet calculation
- [x] Implement hole card masking (default `????`)
- [x] Add `consecutiveFailures` and `disabled` **protected by same mutex**
- [x] Implement `OnHandComplete` short-circuit when disabled
- [x] Implement buffer snapshot/swap in `Flush()` with disabled check
- [x] Implement `recordFlushResult()` for atomic failure state updates
- [x] Implement `IsDisabled()` thread-safe accessor
- [x] Write monitor unit tests (buffer concurrency with race detector)
- [x] Write test: seat contributions reset per street
- [x] Write test: hole card masking on/off
- [x] Write test: disabled monitor short-circuits and stops buffering
- [x] Fixed board card ordering via `HandState.BoardCards()` chronological tracking
- [x] Fixed all-in bet amount tracking with proper seat contributions
- [x] Implement PHH spec-compliant fields (no custom `_` extensions)
- [x] Add player rotation (SB-first ordering per spec)
- [x] Implement UTC timestamp normalization
- [x] Add accurate showdown tracking via card reveal actions

**Phase 3 - Manager**: ✅ COMPLETE
- [x] Implement `Manager` with mutex-protected monitor registry
- [x] Add ticker goroutine with WaitGroup lifecycle
- [x] Implement `flushAll()` with snapshot pattern (avoids holding registry lock during I/O)
- [x] Call `monitor.recordFlushResult()` to update failures **without direct field access**
- [x] Unregister disabled monitors after 3rd failure (prevents memory leak)
- [x] Write manager tests (multi-game, registration races)
- [x] Write test: flush failure recovery and disable logic
- [x] Write test: disabled monitor gets unregistered and stops receiving events

**Phase 4 - Server Integration**: ✅ COMPLETE
- [x] Add hand history config fields to `Config` struct
- [x] Wire `Manager` into `Server` lifecycle
- [x] Create `Monitor` when creating games/pools
- [x] Add monitor to `MultiHandMonitor` chain
- [x] Update `Server.Shutdown()` to flush all buffers
- [x] Wire `UnregisterMonitor` on game deletion

**Phase 5 - CLI**: ✅ COMPLETE
- [x] Add CLI flags to `spawn` command
- [x] Add CLI flags to `server` command
- [x] Add log messages when hand history enabled/disabled

**Phase 6 - Testing & Docs**: ✅ COMPLETE
- [x] Write integration test: full hand → valid PHH file
- [x] Write integration test: multi-game file isolation
- [x] Write integration test: graceful shutdown flush
- [x] Write integration test: board card ordering (`TestHandRunnerEmitsOrderedBoardSlices`)
- [x] Write integration test: PHH monitor captures ordered board (`TestHandHistoryMonitorCapturesOrderedBoard`)
- [x] Add regression test: all-in cumulative bet tracking (`TestMonitorRecordsAllInCumulativeBet`)
- [x] Add regression test: manager failure state machine (`TestManagerDisablesMonitorAfterRepeatedFailures`)
- [x] Add regression test: concurrent CreateMonitor prevention (`TestManagerCreateMonitorPreventsDuplicates`)
- [x] Add regression test: auto-win doesn't mark showdown (`TestHandRunnerAutoWinDoesNotMarkShowdown`)
- [x] Benchmark spawn throughput with/without hand history
- [x] Create `docs/hand-history.md` with format guide
- [x] Update `docs/operations.md` with usage examples
- [x] Update README with hand history feature
- [x] Verify with `--hand-history-hole-cards` for showdown card recording
- [x] Verify PHH spec compliance against official documentation

## Future Extensions

Not in scope for initial implementation, but natural follow-ons:

1. **Replay tool**: `pokerforbots replay hand-00042.phh` (render with PrettyMonitor)
2. **Format conversion**: `pokerforbots convert session.phhs --to json`
3. **Hand search**: `pokerforbots search --player aragorn --bb-won ">10"`
4. **Aggregation**: `pokerforbots analyze hands/` (stats from PHH files)
5. **Compression**: Gzip rotate files > 10MB
6. **File rotation**: `session-001.phh`, `session-002.phh` at size limit
7. **Remote storage**: S3/GCS upload on flush
8. **Web viewer**: HTTP endpoint for browsing hands
9. **JSONL mode**: Alternative to PHH for streaming pipelines

## References

- PHH specification: https://phh.readthedocs.io/en/stable/
- PHH paper (arXiv): https://arxiv.org/abs/2312.11753
- PHH dataset: https://github.com/uoftcprg/phh-dataset
- PokerKit parser: https://github.com/uoftcprg/pokerkit
- [internal/server/monitor.go](../internal/server/monitor.go) — HandMonitor interface
- [internal/server/stats_monitor.go](../internal/server/stats_monitor.go) — Reference monitor implementation
- [docs/design.md](../docs/design.md) — Server architecture

## Verify

```bash
# Run all tests including race detection
go test -race ./...

# Build all commands
go build ./cmd/...

# Test spawn with hand history
./pokerforbots spawn \
  --spec "complex:3,random:3" \
  --hand-limit 100 \
  --output list \
  --hand-history \
  --hand-history-dir /tmp/test-hands

# Verify PHH file created
ls -lh /tmp/test-hands/game-default/session.phhs

# Verify valid TOML
go run -c 'import toml; toml.load("/tmp/test-hands/game-default/session.phhs")'

# Benchmark throughput (should be < 5% slower)
time ./pokerforbots spawn --spec "complex:6" --hand-limit 1000 --output dots
time ./pokerforbots spawn --spec "complex:6" --hand-limit 1000 --output dots --hand-history

# Lint
golangci-lint run

# Format
gofmt -w internal/phh internal/server cmd/pokerforbots
```

## Open Questions

- Should we support JSONL format in addition to PHH? (Easier for big data tools)
- Should hole cards be included by default or opt-in? (Privacy vs. debugging trade-off)
- Should we add file rotation at a certain size threshold? (e.g., 10MB)
- Do we need compression for production use cases? (gzip could save 80%+)
- Should the flush ticker be per-game or server-wide? (Server-wide is simpler, scales better)

**Current decisions**:
- PHH only for v1 (JSONL is future extension)
- Hole cards opt-in via `--hand-history-hole-cards` (default: hidden with `????`)
- No rotation in v1 (append-only, manual cleanup)
- No compression in v1 (future extension)
- Server-wide flush ticker (one timer for all games)
