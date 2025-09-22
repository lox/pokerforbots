# Development Workflow Guide

Quick guide for testing and improving poker bots.

## Prerequisites

Uses CashApp's Hermit: `source bin/activate-hermit` or call things directly from bin.

## Quick Start

### Using the Spawner (Recommended)

The spawner manages bot processes and includes an embedded server:

```bash
# Quick demo with default bots
go run ./cmd/spawner --demo=simple --num-bots=6

# Test your bot against NPC strategies
go run ./cmd/spawner --demo=mixed --num-bots=5 \
  --bot "go run ./sdk/examples/complex" \
  --hand-limit 1000 --print-stats-on-exit

# Deterministic testing with seed
go run ./cmd/spawner --seed 42 --hand-limit 1000 \
  --bot "go run ./sdk/examples/complex" \
  --demo=aggressive --num-bots=3 \
  --write-stats-on-exit stats.json
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
go run ./cmd/spawner --demo=simple --num-bots=5 \
  --bot "go run ./sdk/examples/complex" \
  --hand-limit 1000

# Mixed opponents (calling, random, aggressive)
go run ./cmd/spawner --demo=mixed --num-bots=6 \
  --bot "go run ./sdk/examples/complex" \
  --hand-limit 1000

# Aggressive opponents only
go run ./cmd/spawner --demo=aggressive --num-bots=4 \
  --bot "go run ./sdk/examples/complex" \
  --hand-limit 1000
```

### Configuration File

For complex setups, use a YAML or JSON config:

```yaml
# spawner-config.yaml
server_url: ws://localhost:8080/ws
seed: 42
bots:
  - command: go
    args: [run, ./sdk/examples/complex]
    count: 1
    game_id: default
  - command: go
    args: [run, ./sdk/examples/calling-station]
    count: 3
  - command: go
    args: [run, ./sdk/examples/aggressive]
    count: 2
```

```bash
go run ./cmd/spawner -c spawner-config.yaml --hand-limit 5000
```

### Custom Stakes

```bash
go run ./cmd/spawner --demo=mixed --num-bots=5 \
  --bot "go run ./sdk/examples/complex" \
  --small-blind 25 --big-blind 50 --start-chips 5000 \
  --hand-limit 1000 --print-stats-on-exit
```

## Automated Testing Loop

```bash
#!/bin/bash
# test-loop.sh - Run multiple test sessions

for i in {1..5}; do
  echo "Test $i of 5"
  go run ./cmd/spawner --seed $i --hand-limit 1000 \
    --demo=mixed --num-bots=5 \
    --bot "go run ./sdk/examples/complex" \
    --write-stats-on-exit results/test-$i.json
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

## Troubleshooting

```bash
# Kill stuck processes
pkill -f "task server"
pkill -f "sdk/examples/complex"

# Check if server is running
lsof -i :8080
```
