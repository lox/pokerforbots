# BotPool Statistics Refactoring Plan (Simplified)

## Current Status: COMPLETE ✅

All phases of the pool statistics refactoring have been successfully completed:

- **Monitor System**: Created clean `HandMonitor` and `StatsProvider` interfaces
- **Unified Statistics**: Consolidated into single `StatsMonitor` with optional detailed tracking
- **Error Tracking**: Added timeout, invalid action, disconnect, and bust counters
- **Protocol Updates**: Extended `GameCompletedPlayer` with error metrics
- **Code Reduction**: Removed ~250 lines from pool.go, achieving simplification goal
- **Tests**: Added comprehensive monitor tests, all passing
- **Documentation**: Updated design docs with statistics configuration details

## Problem Statement

The current BotPool implementation has accumulated significant complexity from managing three separate statistics systems:

1. **Basic botStats** (`map[string]*botStats` with `statsMu` mutex) - Lines 46-49, 534-591 in pool.go
2. **StatsCollector interface** (optional `DetailedStatsCollector`) - Lines 51, 96-108, 606-611
3. **HandMonitor interface** (recently added for regression testing) - Lines 54, 138-144, 614-617

This redundancy creates:
- **Performance overhead**: Three separate mutex operations per hand
- **Code complexity**: ~300 lines of statistics code in pool.go
- **Maintenance burden**: Multiple data structures tracking similar information
- **Interface confusion**: Unclear responsibilities between different systems

## Goals

1. **Simplify BotPool** to focus solely on game orchestration
2. **Consolidate statistics** while preserving all functionality
3. **Improve performance** without regressing the hot path
4. **Support multiple monitors** (progress tracking + statistics)
5. **Enable proper error tracking** (timeouts, invalid actions, disconnects)

## Design Principles (Simplified)

1. **Start simple** - Avoid over-engineering with complex patterns
2. **Two clear roles** - Progress monitoring vs statistics collection
3. **Incremental migration** - Can run old and new systems side by side
4. **Performance first** - Only build expensive data when needed

## Proposed Architecture (Simplified)

### Core Interfaces

```go
// internal/server/monitor.go

// HandMonitor receives game event notifications
type HandMonitor interface {
    OnGameStart(handLimit uint64)
    OnGameComplete(handsCompleted uint64, reason string)
    OnHandComplete(outcome HandOutcome)
}

// MultiHandMonitor fans out events to several monitors (progress + stats, etc.)
func NewMultiHandMonitor(monitors ...HandMonitor) HandMonitor

// HandOutcome contains both simple and detailed data
type HandOutcome struct {
    // Always provided (cheap)
    HandID         string
    HandsCompleted uint64
    HandLimit      uint64

    // Optional detailed data (expensive, only if monitor needs it)
    Detail *HandOutcomeDetail
}

// HandOutcomeDetail contains full statistics data (existing structure)
type HandOutcomeDetail struct {
    HandID         string
    ButtonPosition int
    StreetReached  string
    Board          []string
    BotOutcomes    []BotHandOutcome
}

// BotHandOutcome includes error tracking
type BotHandOutcome struct {
    Bot            *Bot
    Position       int
    ButtonDistance int
    HoleCards      []string
    NetChips       int
    WentToShowdown bool
    WonAtShowdown  bool
    Actions        map[string]string
    TimedOut       bool
    InvalidActions int  // Count of invalid actions attempted
    Disconnected   bool // Bot disconnected during hand
    WentBroke      bool
}

// GameCompletedPlayer now carries error counters for downstream consumers
type GameCompletedPlayer struct {
    TimeoutCount    int `msg:"timeouts"`
    InvalidActions  int `msg:"invalid_actions"`
    DisconnectCount int `msg:"disconnects"`
    BustCount       int `msg:"busts"`
    // ... existing fields remain
}

// StatsProvider is for querying statistics (separate from monitoring)
type StatsProvider interface {
    GetPlayerStats() []PlayerStats
    GetDetailedStats(botID string) *protocol.PlayerDetailedStats
}
```

### Single Unified StatsMonitor

```go
// StatsMonitor handles all statistics collection
type StatsMonitor struct {
    mu             sync.RWMutex
    basicStats     map[string]*BasicBotStats   // Chip tracking (always enabled)
    detailedStats  map[string]*BotStatistics   // Variance/CI (optional)
    enableDetailed bool                        // Whether to track detailed stats
    bigBlind       int
    maxHands       int                         // For circular buffer
    currentHands   int
}

type BasicBotStats struct {
    BotID          string
    DisplayName    string
    BotCommand     string  // Original command for metadata
    ConnectOrder   int     // Order of connection for deterministic sorting
    Hands          int
    NetChips       int64
    TotalWon       int64
    TotalLost      int64
    Timeouts       int
    InvalidActions int
    Disconnects    int
    LastDelta      int
    LastUpdated    time.Time
}

// NewStatsMonitor creates a new stats monitor
func NewStatsMonitor(bigBlind int, enableDetailed bool, maxHands int) *StatsMonitor {
    return &StatsMonitor{
        basicStats:     make(map[string]*BasicBotStats),
        detailedStats:  make(map[string]*BotStatistics),
        enableDetailed: enableDetailed,
        bigBlind:       bigBlind,
        maxHands:       maxHands,
    }
}

// Implements HandMonitor
func (s *StatsMonitor) OnGameStart(handLimit uint64) {
    // Initialize if needed
}

func (s *StatsMonitor) OnGameComplete(handsCompleted uint64, reason string) {
    // Final cleanup if needed
}

func (s *StatsMonitor) OnHandComplete(outcome HandOutcome) {
    // Skip if no detail provided
    if outcome.Detail == nil {
        return
    }

    s.mu.Lock()
    defer s.mu.Unlock()

    // Always update basic stats
    for _, botOutcome := range outcome.Detail.BotOutcomes {
        stats := s.basicStats[botOutcome.Bot.ID]
        if stats == nil {
            stats = &BasicBotStats{
                BotID:        botOutcome.Bot.ID,
                DisplayName:  botOutcome.Bot.DisplayName(),
                BotCommand:   botOutcome.Bot.BotCommand(),
                ConnectOrder: len(s.basicStats) + 1,
            }
            s.basicStats[botOutcome.Bot.ID] = stats
        }

        stats.Hands++
        stats.NetChips += int64(botOutcome.NetChips)
        stats.LastDelta = botOutcome.NetChips
        stats.LastUpdated = time.Now()

        if botOutcome.NetChips > 0 {
            stats.TotalWon += int64(botOutcome.NetChips)
        } else {
            stats.TotalLost += int64(-botOutcome.NetChips)
        }

        if botOutcome.TimedOut {
            stats.Timeouts++
        }
        stats.InvalidActions += botOutcome.InvalidActions
        if botOutcome.Disconnected {
            stats.Disconnects++
        }
    }

    // Update detailed stats if enabled
    if s.enableDetailed {
        for _, botOutcome := range outcome.Detail.BotOutcomes {
            if s.detailedStats[botOutcome.Bot.ID] == nil {
                s.detailedStats[botOutcome.Bot.ID] = NewBotStatistics(s.bigBlind)
            }
            netBB := float64(botOutcome.NetChips) / float64(s.bigBlind)
            s.detailedStats[botOutcome.Bot.ID].AddResult(
                netBB,
                botOutcome.WentToShowdown,
                botOutcome.WonAtShowdown,
            )
        }
    }

    // Check circular buffer limit
    s.currentHands++
    if s.maxHands > 0 && s.currentHands > s.maxHands {
        // Reset to avoid unbounded memory growth
        s.basicStats = make(map[string]*BasicBotStats)
        s.detailedStats = make(map[string]*BotStatistics)
        s.currentHands = 1
    }
}

// Implements StatsProvider
func (s *StatsMonitor) GetPlayerStats() []PlayerStats {
    s.mu.RLock()
    defer s.mu.RUnlock()

    // Build PlayerStats from basicStats
    players := make([]PlayerStats, 0, len(s.basicStats))
    for _, stats := range s.basicStats {
        ps := PlayerStats{
            GameCompletedPlayer: protocol.GameCompletedPlayer{
                BotID:       stats.BotID,
                DisplayName: stats.DisplayName,
                Hands:       stats.Hands,
                NetChips:    stats.NetChips,
                TotalWon:    stats.TotalWon,
                TotalLost:   stats.TotalLost,
                LastDelta:   stats.LastDelta,
            },
            LastUpdated: stats.LastUpdated,
        }

        // Add detailed stats if available
        if s.enableDetailed {
            if detailed := s.detailedStats[stats.BotID]; detailed != nil {
                ps.DetailedStats = detailed.ToProtocolStats()
            }
        }

        players = append(players, ps)
    }

    // Sort by ConnectOrder for deterministic output
    sort.Slice(players, func(i, j int) bool {
        return s.basicStats[players[i].BotID].ConnectOrder <
               s.basicStats[players[j].BotID].ConnectOrder
    })

    return players
}

func (s *StatsMonitor) GetDetailedStats(botID string) *protocol.PlayerDetailedStats {
    if !s.enableDetailed {
        return nil
    }

    s.mu.RLock()
    defer s.mu.RUnlock()

    if stats := s.detailedStats[botID]; stats != nil {
        return stats.ToProtocolStats()
    }
    return nil
}
```

### Simplified BotPool

```go
type BotPool struct {
    // Core game state only
    handCounter  uint64
    handLimit    uint64
    bots         map[string]*Bot

    // Two separate monitors (both optional)
    progressMonitor HandMonitor    // For progress tracking (e.g., regression tester)
    statsMonitor    *StatsMonitor  // For statistics collection

    // Pool mechanics (unchanged)
    available    chan *Bot
    register     chan *Bot
    unregister   chan *Bot
    stopCh       chan struct{}

    // Core configuration
    rng          *rand.Rand
    rngMutex     sync.Mutex
    config       Config
    logger       zerolog.Logger
}

// NewBotPool creates a new bot pool
func NewBotPool(logger zerolog.Logger, rng *rand.Rand, config Config) *BotPool {
    pool := &BotPool{
        // ... initialize core fields ...
    }

    // Setup statistics if enabled
    if config.EnableStats {
        pool.statsMonitor = NewStatsMonitor(
            config.BigBlind,
            config.DetailedStats,  // Whether to track variance/CI
            config.MaxStatsHands,
        )
    }

    return pool
}

// SetHandMonitor sets a progress monitor (e.g., for regression testing)
func (p *BotPool) SetHandMonitor(monitor HandMonitor) {
    p.progressMonitor = monitor
    // Notify of game start if fresh
    if monitor != nil && atomic.LoadUint64(&p.handCounter) == 0 {
        monitor.OnGameStart(p.handLimit)
    }
}

// PlayerStats delegates to stats monitor if available
func (p *BotPool) PlayerStats() []PlayerStats {
    if p.statsMonitor != nil {
        return p.statsMonitor.GetPlayerStats()
    }
    return nil
}

// RecordHandOutcome is called by HandRunner
func (p *BotPool) RecordHandOutcome(detail *HandOutcomeDetail) {
    // Build simple outcome (always cheap)
    outcome := HandOutcome{
        HandID:         detail.HandID,
        HandsCompleted: atomic.LoadUint64(&p.handCounter),
        HandLimit:      p.handLimit,
    }

    // Notify progress monitor (cheap, no detail)
    if p.progressMonitor != nil {
        p.progressMonitor.OnHandComplete(outcome)
    }

    // Notify stats monitor (with detail)
    if p.statsMonitor != nil {
        outcome.Detail = detail
        p.statsMonitor.OnHandComplete(outcome)
    }
}

// Helper to check if we need detailed data
func (p *BotPool) NeedsDetailedData() bool {
    return p.statsMonitor != nil
}
```

### HandRunner Integration

```go
// In HandRunner
func (hr *HandRunner) Run() {
    // ... run hand logic ...

    // Only build detailed outcome if needed
    if hr.pool.NeedsDetailedData() {
        detail := hr.buildDetailedOutcome()
        hr.pool.RecordHandOutcome(detail)
    } else {
        // Just increment counter for progress
        hr.pool.RecordHandOutcome(&HandOutcomeDetail{
            HandID: hr.handID,
        })
    }
}

func (hr *HandRunner) buildDetailedOutcome() *HandOutcomeDetail {
    // Build full outcome with all statistics data
    botOutcomes := make([]BotHandOutcome, len(hr.bots))

    for i, bot := range hr.bots {
        player := hr.handState.Players[i]
        botOutcomes[i] = BotHandOutcome{
            Bot:            bot,
            Position:       i,
            ButtonDistance: (i - hr.button + len(hr.bots)) % len(hr.bots),
            HoleCards:      hr.holeCardStrings(i),
            NetChips:       player.TotalWon - player.TotalBet,
            WentToShowdown: hr.wentToShowdown[i],
            WonAtShowdown:  hr.wonAtShowdown[i],
            Actions:        hr.botActions[i],
            TimedOut:       hr.timeouts[bot.ID],
            InvalidActions: hr.invalidActions[bot.ID],
            Disconnected:   hr.disconnects[bot.ID],
            WentBroke:      player.Chips == 0,
        }
    }

    return &HandOutcomeDetail{
        HandID:         hr.handID,
        ButtonPosition: hr.button,
        StreetReached:  hr.furthestStreet,
        Board:          hr.boardStrings(),
        BotOutcomes:    botOutcomes,
    }
}
```

## Error Tracking Implementation

Add tracking to HandRunner:

```go
type HandRunner struct {
    // ... existing fields ...

    // Error tracking
    timeouts       map[string]bool
    invalidActions map[string]int
    disconnects    map[string]bool
}

// During action processing
func (hr *HandRunner) processAction(bot *Bot) {
    action, err := bot.RequestAction(hr.config.Timeout)

    if err != nil {
        if errors.Is(err, ErrTimeout) {
            hr.timeouts[bot.ID] = true
            // Force fold
        } else if errors.Is(err, ErrDisconnected) {
            hr.disconnects[bot.ID] = true
            // Force fold
        }
        return
    }

    // Validate action
    if !hr.isValidAction(action) {
        hr.invalidActions[bot.ID]++
        // Force fold
        return
    }

    // Process valid action
}
```

## Implementation Plan

### Phase 1: Create New Monitor System ✅
- [x] Create `internal/server/monitor.go` with interfaces
- [x] Implement `StatsMonitor` with both basic and detailed tracking
- [x] Add comprehensive tests for StatsMonitor
- [x] Ensure all existing botStats fields are preserved

### Phase 2: Update HandRunner ✅
- [x] Add error tracking fields and instrumentation
- [x] Create `buildDetailedOutcome()` method
- [x] Add `NeedsDetailedData()` check
- [x] Update to call `pool.RecordHandOutcome()`

### Phase 3: Update BotPool ✅
- [x] Add `progressMonitor` and `statsMonitor` fields
- [x] Add `SetHandMonitor()` method
- [x] Implement new `RecordHandOutcome()` method
- [x] Add `NeedsDetailedData()` helper
- [x] Update `PlayerStats()` to delegate

### Phase 4: Migrate and Test ✅
- [x] Update regression tester to use `SetHandMonitor()`
- [x] Verify HTTP endpoints work with new system
- [x] Run side-by-side with old system to verify correctness
- [x] Performance benchmark

### Phase 5: Remove Legacy Code ✅
- [x] Remove `botStats` map and `statsMu`
- [x] Remove `statsCollector` and `StatsCollector` interface
- [x] Remove old `RecordHandOutcome` methods
- [x] Clean up unused code

**Total: 7-10 hours** (reduced from 10-12 with simpler design)

## Benefits

### Simplicity
- **No complex patterns**: Just two optional monitor fields
- **Clear responsibilities**: Progress vs statistics
- **Easy to understand**: Straightforward delegation

### Performance
- **Conditional detail building**: Only when statsMonitor present
- **Single mutex**: One lock in StatsMonitor instead of three in pool
- **Fast progress path**: Progress monitors get cheap updates

### Flexibility
- **Both monitors optional**: Can have none, one, or both
- **Easy migration**: Can run old and new side-by-side
- **Future extensibility**: Can add composite pattern later if needed

## Migration Strategy

1. **Add new system**: Implement alongside existing code
2. **Dual operation**: Run both systems, compare outputs
3. **Switch consumers**: Update regression tester, HTTP endpoints
4. **Remove old system**: Once verified working

## Success Criteria ✅

- [x] **Functionality preserved**: All existing features work
- [x] **Performance maintained**: No regression in throughput
- [x] **Code simplified**: 200+ lines removed from pool.go
- [x] **Tests pass**: All existing and new tests
- [x] **Error tracking works**: Timeouts and invalid actions tracked
- [x] **Downstream visibility**: Error counters exposed via GameCompleted/admin stats

## Delivered Features

### New Components
- [x] `internal/server/monitor.go` - Clean interface definitions
- [x] `internal/server/monitor_test.go` - Comprehensive monitor tests
- [x] `MultiHandMonitor` - Composite pattern for multiple monitors
- [x] `StatsMonitor` - Unified statistics collector with memory management

### Enhanced Error Tracking
- [x] Timeout counters per bot
- [x] Invalid action counters per bot
- [x] Disconnect tracking per bot
- [x] Bust (went broke) tracking
- [x] Error metrics in `GameCompletedPlayer` protocol messages
- [x] Error metrics exposed via HTTP admin stats endpoint

### Code Improvements
- [x] Removed redundant `botStats` map from BotPool
- [x] Removed complex `StatsCollector` interface hierarchy
- [x] Simplified mutex usage (single mutex in StatsMonitor)
- [x] Conditional detail building for performance
- [x] Deterministic sorting by connection order
- [x] Memory management with circular buffer (10,000 hand default)

## Future Enhancements (If Needed)

If we later need more complex monitoring:
1. Add composite monitor pattern
2. Add observer chains
3. Add async/buffered statistics
4. Add more specialized monitors

But we start simple and only add complexity when proven necessary.
