# Development Workflow Guide

Quick guide for testing and improving poker bots.

## Prerequisites

Uses CashApp's Hermit: `source bin/activate-hermit` or call things directly from bin.

## Quick Start

### One Command Testing

Run a complete test session with your bot vs NPCs:

```bash
# Basic test run (1000 hands with stats)
task server -- --hands 1000 --npc-bots 3 --bot-cmd "go run ./sdk/examples/complex" \
  --collect-detailed-stats --print-stats-on-exit

# With deterministic seed for reproducible results
task server -- --seed 42 --hands 1000 --npc-bots 3 --require-player \
  --bot-cmd "go run ./sdk/examples/complex" --collect-detailed-stats --print-stats-on-exit

# Fast testing (20ms timeout)
task server -- --hands 5000 --timeout-ms 20 --npc-bots 5 \
  --bot-cmd "go run ./sdk/examples/complex" --collect-detailed-stats --print-stats-on-exit
```

The server will:
1. Start with specified NPCs
2. Launch your bot automatically
3. Run the specified number of hands
4. Print statistics when complete
5. Exit cleanly

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

### Test Against Specific NPCs

```bash
# Mix of opponent types
task server -- --hands 1000 \
  --npc-calling 2 \  # 2 calling stations
  --npc-random 1 \   # 1 random bot
  --npc-aggro 2 \    # 2 aggressive bots
  --bot-cmd "go run ./sdk/examples/complex" \
  --collect-detailed-stats --print-stats-on-exit
```

### Infinite Bankroll Mode

Prevent eliminations for long-running tests:

```bash
task server -- --infinite-bankroll --hands 10000 --npc-bots 3 \
  --bot-cmd "go run ./sdk/examples/complex" \
  --collect-detailed-stats --print-stats-on-exit
```

### Custom Stakes

```bash
task server -- --hands 1000 --npc-bots 3 \
  --small-blind 25 --big-blind 50 --start-chips 5000 \
  --bot-cmd "go run ./sdk/examples/complex" \
  --collect-detailed-stats --print-stats-on-exit
```

## Automated Testing Loop

```bash
#!/bin/bash
# test-loop.sh - Run multiple test sessions

for i in {1..5}; do
  echo "Test $i of 5"
  task server -- --seed $i --hands 1000 --npc-bots 3 \
    --bot-cmd "go run ./sdk/examples/complex" \
    --collect-detailed-stats --print-stats-on-exit \
    | tee results/test-$i.txt
done

# Extract BB/100 from all runs
grep "BB/100" results/test-*.txt
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
