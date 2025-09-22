# Regression Tester for Bot Snapshots

## Overview

A regression testing framework for comparing poker bot performance across versions. The tester runs controlled experiments between bot snapshots to detect performance regressions and track strategic changes over time.

## Goals

1. **Detect Performance Regressions**: Identify when changes make bots play worse
2. **Provide Statistical Confidence**: Use sufficient sample sizes and confidence intervals
3. **Support Multiple Test Scenarios**: Heads-up, population, and NPC benchmarks
4. **Enable Reproducible Results**: Deterministic seeding and result archiving
5. **Track Strategic Shifts**: Monitor changes in VPIP, PFR, aggression factors

## Architecture

### Command Location
- `cmd/regression-tester/main.go` - Main entry point
- Leverages server's `--bot-cmd` and `--npc-bot-cmd` for all bot management
- Parses results from server's statistics output

### Core Components

1. **Test Runner**: Coordinates test execution by configuring and launching server
2. **Server Launcher**: Starts server with appropriate `--bot-cmd` flags
3. **Result Parser**: Extracts statistics from server output or JSON files
4. **Report Generator**: Produces JSON and human-readable summaries with effect sizes
5. **Statistical Analyzer**: Calculates confidence intervals, p-values, and effect sizes

### Key Design Decision: Server-Managed Bots

The regression tester delegates all bot management to the server via `--bot-cmd` flags. This provides:
- **Automatic bot output with prefixes**: `[player#1 bot-name]` for clear identification
- **Server-managed lifecycle**: Server spawns bots, monitors health, and ensures clean shutdown
- **Built-in crash handling**: Server already handles bot disconnections gracefully
- **Simplified architecture**: Regression tester focuses only on statistical analysis

Future enhancements to bot management (crash detection, restart policies, health monitoring) should be implemented in the server itself, keeping the regression tester minimal and focused.

## Test Modes

### 1. Heads-Up Mode
Direct 1v1 comparison between two bot versions.
```bash
# Using task command (default 1000 hands)
task regression:heads-up

# With custom hands count
task regression:heads-up HANDS=50000

# Full manual command
task regression -- --mode heads-up \
  --challenger "go run ./sdk/examples/complex" \
  --baseline "go run ./sdk/examples/complex" \
  --hands 50000 \
  --seed 42
```

**Purpose**: Isolate performance difference between two versions
**Table Setup**: 2 seats, challenger vs baseline
**Key Metrics**: BB/100 win rate, confidence interval

### 2. Population Mode
Test challenger bot(s) against a field of baseline bots.
```bash
# Using task command
task regression -- --mode population \
  --challenger "go run ./sdk/examples/complex" \
  --baseline "go run ./sdk/examples/complex" \
  --challenger-seats 2 \
  --baseline-seats 4 \
  --hands 10000
```

**Purpose**: Test performance in multi-way pots
**Table Setup**: 6 seats with configurable mix
**Key Metrics**: BB/100, VPIP, PFR, aggression frequency

### 3. NPC Benchmark Mode
Compare two bots against fixed-strategy NPCs for stable regression detection.
```bash
# Using task command
task regression -- --mode npc-benchmark \
  --challenger "go run ./sdk/examples/complex" \
  --baseline "go run ./sdk/examples/complex" \
  --npcs "aggressive:2,calling:1,random:1" \
  --hands 10000
```

**Purpose**: Detect performance regression against known exploitable strategies
**Table Setup**: Both challenger and baseline bots tested separately against same NPC field
**Implementation Approach**:
- Challenger and baseline are tested in separate batches with different seeds
- This avoids correlation between their results while maintaining fairness
- Each bot plays the same total number of hands against identical NPC configuration
- Statistical comparison focuses on relative performance difference, not absolute BB/100

**Expected Performance Baselines**:
- vs Calling: +25 to +35 BB/100 (should crush passive play)
- vs Random: +15 to +25 BB/100 (should exploit chaos)
- vs Aggressive: +5 to +15 BB/100 (should handle pressure)
**Regression Detection**: Compare challenger vs baseline performance against same NPCs

### 4. Self-Play Mode
Baseline variance measurement with identical bots.
```bash
# Using task command
task regression -- --mode self-play \
  --challenger "go run ./sdk/examples/complex" \
  --hands 5000
```

**Purpose**: Establish variance baseline
**Expected Result**: Near 0 BB/100 (zero-sum)
**Note**: Only uses challenger (plays against itself)

## Batching Strategy

To handle bankroll limitations and sample variance:

```bash
# Using task command with batching
task regression -- --mode population \
  --challenger "go run ./sdk/examples/complex" \
  --baseline "go run ./sdk/examples/complex" \
  --batch-size 1000 \
  --hands 10000 \
  --starting-chips 1000 \
  --seeds "42,123,456,789,1337,2024,3141,4242,5555,9999"
```

Each batch:
1. Starts fresh with 1000BB stacks
2. Runs specified number of hands
3. Records results including bust-outs
4. Uses different seed for deck variation
5. Replaces crashed bots automatically

Alternative for pure performance measurement:
```bash
regression-tester --infinite-bankroll \
  --hands 500000
```

## Statistical Rigor

### Multiple Comparison Correction
When running multiple test modes, apply Bonferroni correction to maintain α = 0.05:
```bash
# Quick test with all modes
task regression:all

# Custom configuration
task regression:all HANDS=5000

# Full command with correction
task regression -- --mode all \
  --challenger "go run ./sdk/examples/complex" \
  --baseline "go run ./sdk/examples/complex" \
  --multiple-test-correction \
  --significance-level 0.05 \
  --hands 10000
```

### Sample Size Warnings
The regression tester automatically shows warnings when sample sizes are too small:
- **< 5,000 hands**: Shows `⚠️ Small sample size - results may be unreliable`
- **5,000-10,000 hands with small effects**: Shows note about needing more hands
- **10,000+ hands**: No warning (sufficient for most testing)

### Early Stopping
For CI efficiency, stop when significance is reached:
```bash
# Using task command
task regression -- --early-stopping \
  --challenger "go run ./sdk/examples/complex" \
  --baseline "go run ./sdk/examples/complex" \
  --min-hands 1000 \
  --max-hands 10000 \
  --check-interval 500
```

## Output Format

### JSON Report Structure
```json
{
  "test_id": "regression-20250922-143022",
  "mode": "population",
  "metadata": {
    "start_time": "2025-09-22T14:30:22Z",
    "duration_seconds": 285,
    "server_version": "v1.2.3",
    "test_environment": "CI"
  },
  "configuration": {
    "challenger": "complex-20250922-abc123",
    "baseline": "complex-20250921-f7f2f70",
    "hands_total": 100000,
    "batches": 10,
    "batch_size": 10000,
    "significance_level": 0.05,
    "multiple_test_correction": true
  },
  "batches": [
    {
      "seed": 42,
      "hands": 10000,
      "results": {
        "challenger_bb_per_100": 12.3,
        "challenger_vpip": 0.285,
        "challenger_pfr": 0.195,
        "challenger_busts": 0,
        "baseline_bb_per_100": -3.1
      }
    }
  ],
  "aggregate": {
    "challenger": {
      "bb_per_100": 9.7,
      "ci_95_low": 5.2,
      "ci_95_high": 14.2,
      "vpip": 0.278,
      "pfr": 0.187,
      "aggression_factor": 2.3,
      "bust_rate": 0.02,
      "effect_size": 0.42
    },
    "baseline": {
      "bb_per_100": -2.4,
      "ci_95_low": -4.8,
      "ci_95_high": -0.1,
      "bust_rate": 0.05
    }
  },
  "performance_metrics": {
    "hands_per_second": 1842
  },
  "error_summary": {
    "bot_crashes": 2,
    "timeouts": 8,
    "connection_errors": 0,
    "recovered_crashes": 2
  },
  "verdict": {
    "significant_difference": true,
    "p_value": 0.003,
    "adjusted_p_value": 0.012,
    "effect_size": 0.42,
    "direction": "improvement",
    "confidence": 0.95,
    "recommendation": "accept"
  }
}
```

### Human-Readable Summary
```
Regression Test Report
======================
Challenger: complex-20250922-abc123
Baseline:   complex-20250921-f7f2f70
Mode:       Population (2 vs 4)
Hands:      100,000
Duration:   4m 45s

Results
-------
Challenger: +9.7 BB/100 [95% CI: +5.2 to +14.2]
Baseline:   -2.4 BB/100 [95% CI: -4.8 to -0.1]
Effect Size: 0.42 (medium)
P-Value: 0.003 (adjusted: 0.012)

Strategic Changes
-----------------
VPIP:  27.8% (was 26.2%)
PFR:   18.7% (was 17.3%)
3-Bet: 8.2%  (was 7.8%)

Reliability
-----------
Bot Crashes: 2 (recovered)
Timeouts: 8
Hands/sec: 1,842

Verdict: IMPROVEMENT (95% confidence)
```

## Bot Management

### Binary Validation
The regression tester validates bot binaries before starting:
```bash
# Using task command
task regression -- --validate-binaries \
  --challenger "go run ./sdk/examples/complex" \
  --baseline "go run ./sdk/examples/complex"
```

### Server-Managed Bot Lifecycle

All bot lifecycle management is delegated to the server via `--bot-cmd` flags:

1. **Bot Spawning**: Server launches bot processes with proper environment variables
2. **Output Management**: Server prefixes bot output with `[player#N bot-name]` or `[npc#N bot-name]`
3. **Crash Handling**: Server detects and reports bot disconnections
4. **Clean Shutdown**: Server ensures all bots exit gracefully when hands complete

#### Server Bot Environment Variables
```bash
POKERFORBOTS_SERVER=ws://localhost:8080/ws
POKERFORBOTS_GAME=default
POKERFORBOTS_ROLE=player|npc  # Optional, for NPC bots
```

### Future Enhancements

Bot health monitoring features (crash detection, restart policies, timeout tracking) should be added to the server itself rather than the regression tester. This keeps the architecture clean and benefits all server use cases.

## Configuration Structure

```go
type TestConfig struct {
    // Core settings
    Mode                string
    HandsTotal          int
    BatchSize           int
    Seeds               []int64

    // Bankroll
    StartingChips       int
    InfiniteBankroll    bool

    // Statistical
    SignificanceLevel   float64
    EffectSizeThreshold float64
    MultipleTestCorrection bool
    EarlyStopping       bool

    // Performance
    TimeoutMs           int
    MaxConcurrentTables int

    // Health
    MaxCrashesPerBot    int
    MaxTimeoutsPerBot   int
    RestartDelayMs      int
}
```

## Implementation Status

### Phase 1: Core Framework ✅ COMPLETE
- [x] Create `cmd/regression-tester/main.go` with Kong CLI parsing
- [x] Implement server launcher using `--bot-cmd` flags with `--server-cmd` configuration
- [x] Binary validation for bot executables
- [x] Basic heads-up mode with result collection
- [x] Automatic sample size warnings when needed
- [x] Leverage server's bot management and output

### Phase 2: Test Modes ✅ COMPLETE
- [x] Heads-up mode with batching support and weighted averaging
- [x] Population mode with configurable seat distribution
- [x] NPC benchmark mode using server's built-in NPCs with full statistical fixes
- [x] Self-play variance baseline mode
- [x] Batching support with automatic seed generation
- [x] Multiple comparison correction implementation with Bonferroni adjustment
- [x] "All" mode that runs applicable tests with automatic correction

### Phase 3: Refactoring & Code Quality ✅ COMPLETE
- [x] **Strategy pattern implementation** - Eliminated 300+ lines of duplicate batch execution
- [x] **Extract pure statistics functions** - Created standalone `stats.go` for aggregation
- [x] **Fix critical bugs** - Min/max BB/100 initialization, hands count for weighting
- [x] **Wire health policies** - Strategy-specific health monitoring now active
- [x] **Remove dead code** - Removed 139 lines of unused batch methods from orchestrator
- [x] **Enhance stats.go** - Added higher-level helpers (WeightedBB100, WeightedAverage, CombineBatches, etc.)
- [x] **Extract reporter** - Moved reporting logic to dedicated file with dependency injection
- [x] **Consolidate server methods** - Created single StartServer with ServerConfig struct
- [x] **Unify all modes** - All test modes now use challenger/baseline naming consistently
- [ ] **Split runner.go** - Separate validation, execution, and coordination concerns
- [ ] **Go idioms cleanup** - Consistent error wrapping, extract magic numbers

### Phase 4: Statistics & Reporting
- [x] **Real statistical aggregation** - VPIP/PFR tracking implemented in server
- [x] **Parse server's `--write-stats-on-exit` JSON output** - Working with real statistics
- [x] **JSON report generation structure** - Fully functional with real data
- [x] **Human-readable summary output** - Working with real metrics
- [x] **Weighted averaging** - Properly uses actual completed hands for statistics
- [x] **Multi-seat aggregation** - Correctly combines stats from multiple bot instances
- [x] Automatic sample size warnings when needed
- [ ] Proper confidence interval calculations (using placeholder 95% CI)
- [ ] Effect size calculations (using placeholder Cohen's d)
- [ ] Early stopping for CI efficiency
- [ ] Result archiving in `snapshots/regression-*.json`

### Phase 5: Advanced Features (Future)
- [x] **Add `--write-stats-on-exit` to server for JSON statistics** - COMPLETE
- [ ] Mirror mode support (when server implements it)
- [ ] Parallel table execution for faster results
- [ ] Comparison with historical baselines
- [ ] Automated regression detection in CI
- [ ] Trend analysis across multiple versions

### Next Steps

**Immediate (Phase 4 - Statistics):**
1. **Statistical Rigor**: Replace placeholder confidence intervals and effect size calculations with proper statistics
2. **Early Stopping**: Implement significance-based early termination for CI efficiency
3. **Result Archiving**: Save results to `snapshots/regression-*.json` for trend analysis

**Future (Phase 5):**
1. **Split runner.go**: Separate concerns into focused files for maintainability
2. **Go idioms cleanup**: Consistent error wrapping, extract magic numbers

**Then (Phase 4 - Statistics):**
1. **Statistical Rigor**: Replace placeholder confidence intervals and effect size calculations with proper statistics
2. **Early Stopping**: Implement significance-based early termination for CI efficiency
3. **Result Archiving**: Save results to `snapshots/regression-*.json` for trend analysis

## Current Implementation

### Working Features
- **All test modes implemented**: Heads-up, population, NPC benchmark, and self-play modes all functional
- **Heads-up mode**: Fully functional with proper batching and weighted averaging
- **Population mode**: Tests multiple challenger bots vs multiple baseline bots at same table
- **NPC benchmark mode**: Complete with challenger vs baseline comparison against NPCs
- **Self-play mode**: Measures variance baseline with identical bots (expects ~0 BB/100)
- **Real statistics**: VPIP/PFR tracking and parsing from server JSON output
- **Server command configuration**: `--server-cmd` flag with smart defaults (`go run ./cmd/server`)
- **Binary validation**: Checks bot executables before running
- **Sample size warnings**: Automatic warnings when sample too small for reliable results
- **Bot output**: Shows real-time bot decisions with clear prefixes
- **Clean shutdown**: Server manages hand completion and exit
- **JSON and summary reports**: Both working with real statistical data
- **Automatic seed generation**: Creates additional seeds when more batches needed
- **Weighted averaging**: Uses actual completed hands for accurate statistics
- **Multi-seat aggregation**: Properly combines stats from multiple bot instances

### Known Limitations
- **Statistical calculations**: Using placeholder confidence intervals and effect sizes
- **Result archiving**: Not yet saving results to snapshots/ directory
- **Multiple comparison correction**: Not implemented for running multiple test modes
- **Early stopping**: Not implemented for CI efficiency

### Example Output
```
[player#1 go] complex improved bot connected
[player#2 go] complex improved bot connected
[completed 1000 hands in 0.8s at 1267 hands/sec]

Regression Test Report
======================
Bot A: go run ./sdk/examples/complex/main.go
Bot B: go run ./sdk/examples/complex/main.go
Mode: heads-up
Hands: 1000
Duration: 0.8s

Results
-------
Effect Size: 0.30 (medium)
P-Value: 0.020 (adjusted: 0.020)
Hands/sec: 1267

Verdict: REJECT (95% confidence)
```

### Enhanced Summary Output
The regression tester now provides detailed summary reports:
```
Regression Test Report
======================
Bot A: go run ./sdk/examples/complex/main.go
Bot B: go run ./sdk/examples/complex/main.go
Mode: heads-up
Hands: 300
Duration: 0.4s

Results
-------
Bot A: -60.2 BB/100 [95% CI: -62.2 to -58.2]
  VPIP: 45.0%, PFR: 35.0%, Busts: 0.0%
Bot B: +60.2 BB/100 [95% CI: +58.2 to +62.2]
  VPIP: 42.0%, PFR: 30.0%, Busts: 0.0%
Effect Size: 0.30 (medium)
P-Value: 0.020 (adjusted: 0.020)
Hands/sec: 787

Verdict: ACCEPT (95% confidence)
```

### Current Real Statistics
The regression tester extracts real statistics from server:
- **VPIP/PFR**: Real values parsed from server (12-16% / 11-15% for complex bot)
- **BB/100**: Actual measured win rates from game results
- **Timeouts/Busts**: Real error tracking
- **Performance**: Actual hands per second measurements
- **Confidence intervals**: Placeholder calculations (real CI math pending)

## Usage Examples

### Quick Regression Check
```bash
# Fastest - using preset task (1000 hands)
task regression:heads-up

# Medium - with more hands
task regression:heads-up HANDS=5000

# Full control
task regression -- --mode heads-up \
  --challenger "go run ./sdk/examples/complex" \
  --baseline "go run ./sdk/examples/complex" \
  --hands 10000 \
  --output summary
```

### Full Test Suite
```bash
# Comprehensive testing script
#!/bin/bash
CHALLENGER="snapshots/complex-20250922-abc123"
BASELINE="snapshots/complex-20250921-f7f2f70"

# Quick test all modes
task regression:all HANDS=5000

# Or run individual modes
CHALLENGER="go run ./sdk/examples/complex"
BASELINE="go run ./sdk/examples/complex"

task regression -- --mode heads-up --challenger $CHALLENGER --baseline $BASELINE --hands 5000
task regression -- --mode population --challenger $CHALLENGER --baseline $BASELINE --hands 10000
task regression -- --mode npc-benchmark --challenger $CHALLENGER --baseline $BASELINE --hands 10000
task regression -- --mode self-play --challenger $CHALLENGER --hands 2500

# Or simply run all modes at once:
task regression -- --mode all --challenger $CHALLENGER --baseline $BASELINE --hands 5000

# Aggregate results
regression-tester --aggregate results/*.json --output final-report.json
```

### CI Integration
```yaml
# .github/workflows/regression.yml
- name: Run regression tests
  run: |
    ./regression-tester \
      --mode npc-benchmark \
      --bot dist/complex-${{ github.sha }} \
      --hands 50000 \
      --fail-on-regression
```

## Success Criteria

1. **Statistical Validity**: 95% confidence intervals on all metrics
2. **Performance**: Can run 100k hands in under 5 minutes
3. **Reproducibility**: Same seed produces identical results
4. **Actionable Output**: Clear pass/fail with specific metrics and sample size warnings
5. **Low False Positives**: <5% false regression alerts

## Open Questions

1. Should we support custom table sizes (4-max, 8-max)?
2. ~~How to handle tests when bots crash or disconnect?~~ → Addressed with health monitoring
3. Should we archive all raw hand histories or just statistics?
4. Integration with existing server stats vs separate collection?
5. Threshold for "significant" regression (effect size > 0.2?)

## References

- Server statistics: `internal/server/statistics/`
- NPC implementations: `internal/server/npc/`
- Protocol messages: `protocol/messages.go`
- Existing snapshots: `snapshots/`