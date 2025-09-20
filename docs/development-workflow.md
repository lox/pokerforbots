# Development Workflow Guide

This guide explains how to run a development loop for testing and improving poker bots using tmux, the PokerForBots server, and performance metrics.

## Prerequisites

Uses CashApp's Hermit, `source bin/activate-hermit` or call things directly from bin.

## Quick Start

### 1. Start the Server in tmux

Start a tmux session with the poker server configured for testing:

```bash
# Kill any existing tmux sessions
tmux kill-server 2>/dev/null || true

# Start server with infinite bankroll for long simulations
tmux new-session -d -s poker-server \
  'task server -- --infinite-bankroll --hands 50000 --timeout-ms 20 --npc-bots 3'

# Or with custom configuration
tmux new-session -d -s poker-server \
  'task server -- \
    --infinite-bankroll \
    --hands 100000 \
    --timeout-ms 50 \
    --small-blind 5 \
    --big-blind 10 \
    --start-chips 1000 \
    --npc-calling 1 \
    --npc-random 1 \
    --npc-aggro 1'

# With server-side statistics enabled (for bot development)
tmux new-session -d -s poker-server \
  'task server -- \
    --infinite-bankroll \
    --hands 10000 \
    --timeout-ms 20 \
    --npc-bots 3 \
    --enable-stats \
    --stats-depth=full \
    --max-stats-hands=5000'
```

### One-liner: Run server and complex bot together

This runs a 3k-hand session with 3 NPCs, spawns the complex bot once, prints stats on exit, and terminates automatically:

```bash
task server -- --hands 3000 --npc-bots 3 --bot-cmd 'go run ./sdk/examples/complex' --print-stats-on-exit
```

Tip: add flags like `--timeout-ms 20`, `--enable-stats`, and `--stats-depth=full` as needed.

### 2. Monitor Server Logs

```bash
# View server logs in real-time
tmux attach -t poker-server

# Or capture recent logs
tmux capture-pane -t poker-server -p | tail -20

# Detach from tmux: Press Ctrl+B, then D
```

### 3. Run the Complex Bot

```bash
# Run the complex bot (it will auto-terminate when game completes)
go run ./sdk/examples/complex

# Or run in background
go run ./sdk/examples/complex &

# With debug logging
go run ./sdk/examples/complex --debug
```

### 4. Monitor Performance Metrics

#### Real-time Server Stats
```bash
# Check current game statistics
curl -s http://localhost:8080/stats

# Pretty-print with jq
curl -s http://localhost:8080/stats | jq .

# Watch stats update every 2 seconds
watch -n 2 'curl -s http://localhost:8080/stats'
```

#### Game Configuration
```bash
# View active games
curl -s http://localhost:8080/games | jq .

# Check specific game stats
curl -s http://localhost:8080/admin/games/default/stats | jq .
```

#### Bot Performance Results

The complex bot saves detailed results to JSON when the game completes:

```bash
# Find latest results file
ls -lat complex-bot-results*.json | head -1

# View bot performance summary
cat complex-bot-results-*.json | jq '{
  won,
  hands: .hands_completed,
  my_stats: .all_players[] | select(.Role == "player") | {
    name: .DisplayName,
    net_chips: .NetChips,
    avg_per_hand: .AvgPerHand,
    bb_per_100: ((.AvgPerHand / 10) * 100)
  }
}'

# Compare all players
cat complex-bot-results-*.json | jq '.all_players |
  sort_by(.NetChips) | reverse | .[] | {
    name: .DisplayName,
    role: .Role,
    net_chips: .NetChips,
    bb_per_100: ((.AvgPerHand / 10) * 100),
    win_rate: ((.TotalWon / (.TotalWon + .TotalLost)) * 100)
  }'

# View detailed statistics (when server has --enable-stats)
cat complex-bot-results-*.json | jq '.all_players[] | select(.Role == "player") | .DetailedStats | {
  BB100,
  WinRate,
  ShowdownWinRate,
  Mean,
  StdDev
}'

# Analyze position performance
cat complex-bot-results-*.json | jq '.all_players[] | select(.Role == "player") | .DetailedStats.PositionStats'

# Check street statistics (where hands end)
cat complex-bot-results-*.json | jq '.all_players[] | select(.Role == "player") | .DetailedStats.StreetStats'

# Analyze hand category performance
cat complex-bot-results-*.json | jq '.all_players[] | select(.Role == "player") | .DetailedStats.HandCategoryStats'
```

## Advanced Configurations

### Infinite Bankroll Mode

Enable infinite bankroll to prevent bots from being eliminated:

```bash
# Bots never run out of chips
./server --infinite-bankroll --hands 100000
```

### Deterministic Testing

Use a fixed seed for reproducible games:

```bash
# Same shuffles and outcomes each run
./server --seed 12345 --hands 1000
```

### Custom NPC Mix

Control the types of NPC opponents:

```bash
# Specific NPC distribution
./server \
  --npc-calling 2 \  # 2 calling stations
  --npc-random 1 \   # 1 random bot
  --npc-aggro 3      # 3 aggressive bots

# Or auto-distribution
./server --npc-bots 6  # Automatically distributes strategies
```

### Performance Tuning

Adjust timeout for faster games:

```bash
# Ultra-fast games (20ms timeout)
./server --timeout-ms 20

# Standard games (100ms timeout)
./server --timeout-ms 100
```

### Server-Side Statistics

Enable detailed statistics collection for bot development:

```bash
# Basic statistics (BB/100, win rate only)
./server --enable-stats --stats-depth=basic

# Detailed statistics (+ position and street analysis)
./server --enable-stats --stats-depth=detailed

# Full statistics (+ hand categories and action tracking)
./server --enable-stats --stats-depth=full --max-stats-hands=10000
```

**Note:** Statistics have minimal performance impact (~309ns per hand) but should be disabled in production for maximum throughput.

## Development Loop

### Typical Workflow

1. **Start Server**: Launch server in tmux with desired configuration
2. **Run Bot**: Execute your bot implementation
3. **Monitor Metrics**: Watch real-time stats during execution
4. **Analyze Results**: Review JSON results after completion
5. **Iterate**: Modify bot strategy based on metrics
6. **Repeat**: Run again to test improvements

### Script Example

```bash
#!/bin/bash
# dev-loop.sh - Automated testing loop

# Configuration
HANDS=10000
TIMEOUT_MS=20
RUNS=5

for i in $(seq 1 $RUNS); do
  echo "Run $i of $RUNS"

  # Start server
  tmux new-session -d -s poker-$i \
    "./server --infinite-bankroll --hands $HANDS --timeout-ms $TIMEOUT_MS --npc-bots 3"

  sleep 2

  # Run bot and wait for completion
  go run ./sdk/examples/complex

  # Save results
  mv complex-bot-results-*.json results/run-$i.json

  # Kill server
  tmux kill-session -t poker-$i
done

# Analyze all runs
jq -s 'map(.all_players[] | select(.Role == "player")) |
  {avg_chips: (map(.NetChips) | add / length)}' results/*.json
```

## Metrics to Track

### Key Performance Indicators

1. **BB/100** (Big Blinds per 100 hands)
   - Target: Positive value
   - Current: Complex bot loses ~2569 BB/100 (catastrophic!)

2. **Win Rate** (Percentage of hands won)
   - Target: 15-25% in 6-max
   - Current: Complex bot wins 11.4% (too low)
   - Track via: `.DetailedStats.WinRate`

3. **Showdown Win Rate**
   - Target: 50-60% when reaching showdown
   - Current: Complex bot at 56.4% (good!)
   - Track via: `.DetailedStats.ShowdownWinRate`

4. **Position Performance**
   - Button (BTN): Should be most profitable
   - Cutoff (CO): Second most profitable
   - Early positions: Tighter, less profitable
   - Track via: `.DetailedStats.PositionStats`

5. **Hand Category Analysis**
   - Premium hands: Should be highly profitable
   - Strong hands: Moderately profitable
   - Medium/Weak: Should mostly fold pre-flop
   - Track via: `.DetailedStats.HandCategoryStats`

6. **Street Analysis**
   - Where are hands ending?
   - Current: Complex bot plays 99.9% to showdown (major leak!)
   - Track via: `.DetailedStats.StreetStats`

### Debugging Tips

```bash
# Check if server is running
tmux list-sessions

# View all server logs
tmux capture-pane -t poker-server -p -S -

# Check for timeouts
curl -s http://localhost:8080/stats | grep -i timeout

# Monitor specific bot
tmux capture-pane -t poker-server -p | grep "complex-"
```

## Troubleshooting

### Common Issues

1. **Bot disconnects early**
   - Without infinite bankroll, bot runs out of chips
   - Solution: Use `--infinite-bankroll` flag

2. **Server not responding**
   - Check if tmux session exists: `tmux ls`
   - Check port: `lsof -i :8080`

3. **No results file generated**
   - Bot must receive `game_completed` message
   - Check `--hands` limit is set

4. **Slow performance**
   - Reduce timeout: `--timeout-ms 20`
   - Reduce debug logging
   - Close other applications

### Clean Up

```bash
# Kill all tmux sessions
tmux kill-server

# Remove result files
rm complex-bot-results-*.json

# Kill stuck processes
pkill -f "task server"
pkill -f "sdk/examples/complex"
```

## Example Analysis Session

```bash
# 1. Start server for 20k hands
tmux new-session -d -s poker \
  'task server -- --infinite-bankroll --hands 20000 --timeout-ms 20 --npc-bots 3 --require-player'

# 2. Monitor initial state
curl -s http://localhost:8080/games | jq '.[0]'

# 3. Run bot
go run ./sdk/examples/complex

# 4. Check final stats
cat complex-bot-results-*.json | jq '.all_players |
  map(select(.Role == "player")) | .[0] | {
    hands: .Hands,
    net: .NetChips,
    bb_per_100: ((.AvgPerHand / 10) * 100),
    win_rate: ((.TotalWon / (.TotalWon + .TotalLost)) * 100)
  }'

# 5. Clean up
tmux kill-server
rm complex-bot-results-*.json
```

## Next Steps

1. **Improve Bot Strategy**: Focus on beating aggressive NPCs
2. **Add Custom Metrics**: Track specific scenarios (bluffs, folds, etc.)
3. **Create Benchmarks**: Set performance targets
4. **Automate Testing**: Build scripts for repeated runs
5. **Visualize Results**: Create charts from JSON data
