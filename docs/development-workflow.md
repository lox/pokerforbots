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

## Troubleshooting

```bash
# Kill stuck processes
pkill -f "task server"
pkill -f "sdk/examples/complex"

# Check if server is running
lsof -i :8080
```
