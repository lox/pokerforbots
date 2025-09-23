# Development Workflow Guide

Quick guide for testing and improving poker bots.

For detailed spawner documentation and API reference, see [spawner.md](spawner.md).

## SDK Overview

PokerForBots provides an SDK with reusable components for bot development:

- **`sdk/spawner`** - Bot process management (public API)
- **`sdk/config`** - Environment variable configuration
- **`sdk/client`** - WebSocket client for connections
- **`sdk/analysis`** - Hand analysis utilities
- **`sdk/classification`** - Card classification

Example bots in `sdk/examples/` demonstrate SDK usage.

## Prerequisites

Uses CashApp's Hermit: `source bin/activate-hermit` or call things directly from bin.

## Quick Start

### Using the Spawner (Recommended)

The spawner manages bot processes and includes an embedded server:

```bash
# Quick demo with default bots (6 calling stations)
go run ./cmd/spawner

# Test your bot against NPC strategies
go run ./cmd/spawner --spec "calling-station:3,random:2" \
  --bot-cmd "go run ./sdk/examples/complex" \
  --hand-limit 1000 --print-stats

# Deterministic testing with seed
go run ./cmd/spawner --seed 42 --hand-limit 1000 \
  --bot-cmd "go run ./sdk/examples/complex" \
  --spec "aggressive:3" \
  --write-stats stats.json
```

### Legacy Server Mode

**Note**: Direct server NPC flags are deprecated. Use the spawner instead.

```bash
# Basic test run (will show deprecation warning)
task server -- --hands 1000 --npc-bots 3 --bot-cmd "go run ./sdk/examples/complex" \
  --collect-detailed-stats --print-stats-on-exit
```

The spawner will:
1. Start an embedded server
2. Spawn and manage bot processes
3. Run the specified number of hands
4. Write/print statistics when complete
5. Clean up all processes on exit

### Monitor Real-time Stats

While the server is running, check progress:

```bash
# Human-readable stats
curl -s http://localhost:8080/stats

# Detailed JSON stats
curl -s http://localhost:8080/admin/games/default/stats | jq .

# Watch stats update live
watch -n 2 'curl -s http://localhost:8080/stats'
```

## Common Configurations

### Test Against Specific Strategies

```bash
# Simple opponents (calling stations)
go run ./cmd/spawner --spec "calling-station:5" \
  --bot-cmd "go run ./sdk/examples/complex" \
  --hand-limit 1000

# Mixed opponents (calling, random, aggressive)
go run ./cmd/spawner --spec "calling-station:2,random:2,aggressive:2" \
  --bot-cmd "go run ./sdk/examples/complex" \
  --hand-limit 1000

# Aggressive opponents only
go run ./cmd/spawner --spec "aggressive:4" \
  --bot-cmd "go run ./sdk/examples/complex" \
  --hand-limit 1000
```

### Multiple Bot Commands

You can spawn multiple different bot commands:

```bash
# Mix custom bots with built-in strategies
go run ./cmd/spawner \
  --bot-cmd "go run ./my-bot" --count 2 \
  --bot-cmd "./another-bot" --count 1 \
  --spec "calling-station:3" \
  --hand-limit 1000
```

### Custom Stakes

```bash
go run ./cmd/spawner --spec "calling-station:2,aggressive:3" \
  --bot-cmd "go run ./sdk/examples/complex" \
  --small-blind 25 --big-blind 50 --start-chips 5000 \
  --hand-limit 1000 --print-stats
```

## Automated Testing Loop

```bash
#!/bin/bash
# test-loop.sh - Run multiple test sessions

for i in {1..5}; do
  echo "Test $i of 5"
  go run ./cmd/spawner --seed $i --hand-limit 1000 \
    --spec "calling-station:2,random:2,aggressive:1" \
    --bot-cmd "go run ./sdk/examples/complex" \
    --write-stats results/test-$i.json
done

# Extract BB/100 from all runs
for f in results/test-*.json; do
  echo "$f: $(jq '.players[0].bb_per_100' $f)"
done
```

## Analyzing Results

### Quick Stats Check

After a run completes, the server prints a summary table. For more detail:

```bash
# Save last run's stats
curl -s http://localhost:8080/admin/games/default/stats > last-run.json

# Player performance
jq '.players[] | select(.role == "player") | {
  name: .display_name,
  bb_per_100: .detailed_stats.bb_per_100,
  win_rate: .detailed_stats.win_rate,
  showdown_rate: .detailed_stats.showdown_rate
}' last-run.json

# Position breakdown
jq '.players[] | select(.role == "player") | .detailed_stats.position_stats' last-run.json
```

## Key Metrics

- **BB/100**: Big Blinds per 100 hands (target: positive)
- **Win Rate**: % of hands won (target: 15-25% in 6-max)
- **Showdown Win Rate**: % won at showdown (target: 50-60%)
- **Position Stats**: Button should be most profitable
- **Street Stats**: Track where hands end (avoid playing every hand to showdown)

## Creating Custom Bots

### Using the SDK

Create a new bot using the SDK packages:

```go
package main

import (
    "github.com/lox/pokerforbots/sdk/client"
    "github.com/lox/pokerforbots/sdk/config"
    "github.com/lox/pokerforbots/protocol"
)

type MyBot struct{}

func (MyBot) OnActionRequest(state *client.GameState, req protocol.ActionRequest) (string, int, error) {
    // Your strategy logic here
    return "call", 0, nil
}

func main() {
    // Parse environment configuration
    cfg, _ := config.FromEnv()

    // Create and run bot
    bot := client.New("my-bot", MyBot{}, logger)
    bot.Connect(cfg.ServerURL)
    bot.Run(context.Background())
}
```

### Environment Variables

The spawner automatically sets these for your bot:

- `POKERFORBOTS_SERVER` - WebSocket URL
- `POKERFORBOTS_SEED` - Random seed for deterministic behavior
- `POKERFORBOTS_BOT_ID` - Unique identifier
- `POKERFORBOTS_GAME` - Target game (default: "default")

## Regression Testing

The regression tester compares bot performance across versions to detect improvements or regressions.

### Quick Start

```bash
# Fastest check - heads-up test with 1000 hands (default)
task regression:heads-up

# With more hands for higher confidence
task regression:heads-up HANDS=5000

# Compare two specific bots
task regression:heads-up \
  CHALLENGER="go run ./sdk/examples/complex" \
  BASELINE="go run ./sdk/examples/calling-station" \
  HANDS=10000
```

### Test Modes

#### Heads-Up Mode
Direct 1v1 comparison between two bot versions:
```bash
task regression:heads-up HANDS=5000
```
- **Purpose**: Isolate performance difference
- **Setup**: 2 seats, challenger vs baseline
- **Key Metric**: BB/100 win rate difference

#### Population Mode
Test challenger bots against a field of baseline bots:
```bash
task regression:population HANDS=10000
```
- **Purpose**: Test performance in multi-way pots
- **Setup**: 6 seats with configurable mix
- **Metrics**: BB/100, VPIP, PFR, aggression

#### NPC Benchmark Mode
Compare bots against fixed-strategy NPCs:
```bash
task regression:npc HANDS=10000
```
- **Purpose**: Detect regression against exploitable strategies
- **Expected Performance**:
  - vs Calling: +25 to +35 BB/100
  - vs Random: +15 to +25 BB/100
  - vs Aggressive: +5 to +15 BB/100

#### Self-Play Mode
Baseline variance measurement with identical bots:
```bash
task regression:self-play HANDS=2500
```
- **Purpose**: Establish variance baseline
- **Expected**: Near 0 BB/100 (zero-sum)

#### Run All Modes
Comprehensive testing with multiple comparison correction:
```bash
# Quick test all modes (5000 hands each)
task regression:all HANDS=5000

# Or with custom bots
task regression:all \
  CHALLENGER="./my-bot" \
  BASELINE="go run ./sdk/examples/complex" \
  HANDS=10000
```

### Advanced Usage

#### Custom Regression Test
```bash
go run ./cmd/regression-tester \
  --mode heads-up \
  --challenger "./my-improved-bot" \
  --baseline "./my-old-bot" \
  --hands 50000 \
  --batch-size 10000 \
  --seeds "42,123,456,789,999" \
  --significance-level 0.05 \
  --output both \
  --output-file results.json
```

#### Batching for Variance Control
```bash
go run ./cmd/regression-tester \
  --mode population \
  --challenger "go run ./sdk/examples/complex" \
  --baseline "go run ./sdk/examples/aggressive" \
  --batch-size 1000 \
  --hands 10000 \
  --starting-chips 1000 \
  --seeds "42,123,456,789,1337"
```

### Understanding Results

#### Sample Output
```
Regression Test Report
======================
Challenger: complex-v2
Baseline: complex-v1
Mode: heads-up
Hands: 10000
Duration: 15.2s

Results
-------
Challenger: +9.7 BB/100 [95% CI: +5.2 to +14.2]
  VPIP: 15.2%, PFR: 12.8%, Busts: 0.0%
Baseline: -9.7 BB/100 [95% CI: -14.2 to -5.2]
  VPIP: 14.8%, PFR: 11.9%, Busts: 0.0%
Effect Size: 0.42 (medium)
P-Value: 0.003
Hands/sec: 657

Verdict: IMPROVEMENT (95% confidence)
```

#### Key Metrics
- **BB/100**: Big blinds won/lost per 100 hands
- **VPIP**: Voluntarily put money in pot %
- **PFR**: Pre-flop raise %
- **Effect Size**: Magnitude of difference (>0.2 = meaningful)
- **P-Value**: Statistical significance (<0.05 = significant)

#### Sample Size Warnings
- **< 5,000 hands**: ⚠️ Results may be unreliable
- **5,000-10,000 hands**: May need more for small effects
- **10,000+ hands**: Generally sufficient

### CI/CD Integration

```yaml
# .github/workflows/regression.yml
- name: Run regression tests
  run: |
    task regression:npc \
      CHALLENGER="./dist/bot-${{ github.sha }}" \
      BASELINE="./dist/bot-stable" \
      HANDS=50000
```

### Snapshot Testing Workflow

```bash
#!/bin/bash
# Create snapshot of current bot
VERSION=$(git rev-parse --short HEAD)
go build -o snapshots/complex-$VERSION ./sdk/examples/complex

# Test against previous snapshot
task regression:heads-up \
  CHALLENGER="snapshots/complex-$VERSION" \
  BASELINE="snapshots/complex-previous" \
  HANDS=50000

# If improvement confirmed, update latest
ln -sf complex-$VERSION snapshots/complex-latest
```

## Troubleshooting

```bash
# Kill stuck processes
pkill -f "task server"
pkill -f "sdk/examples/complex"
pkill -f "regression-tester"

# Check if server is running
lsof -i :8080

# Clean up temporary stats files
rm stats-*.json
```
