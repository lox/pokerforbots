# Quick Start Guide

This guide helps you get up and running with PokerForBots quickly, whether you're testing bots, developing new strategies, or just exploring the system.

## Installation

### Install the Binary

```bash
# Build and install to ~/bin
go build -o ~/bin/pokerforbots ./cmd/pokerforbots

# Verify installation
pokerforbots --help
```

### Or Run from Source

```bash
# During development, you can use go run
go run ./cmd/pokerforbots spawn --spec "calling-station:6"
```

## Most Common Command: Spawn

The `spawn` command is your primary tool for testing. It starts an embedded server and manages bot processes automatically.

### Basic Usage

```bash
# Quick demo with default bots
pokerforbots spawn --spec "calling-station:6"

# Mix different strategies
pokerforbots spawn --spec "calling-station:2,random:2,aggressive:2"

# Test your bot against opponents
pokerforbots spawn --bot-cmd "./my-bot" --spec "calling-station:5"

# Combine custom bots with built-in strategies
pokerforbots spawn --bot-cmd "./my-bot" --count 2 \
  --spec "calling-station:2,aggressive:2"
```

### Common Options

- `--spec` - Bot specification string (e.g., `"calling-station:2,random:3"`)
- `--bot-cmd` - External bot command (can be specified multiple times)
- `--count` - Number of instances for each --bot-cmd
- `--hand-limit` - Stop after N hands (0 for unlimited)
- `--seed` - Deterministic seed for reproducible testing
- `--print-stats` - Display statistics on exit
- `--write-stats` - Save statistics to JSON file

### Built-in Bot Strategies

The spawn command supports these built-in strategies:
- `calling-station` - Always calls/checks, never raises
- `random` - Makes random valid actions
- `aggressive` - Raises frequently (70% of the time)
- `complex` - Advanced strategy bot

## Testing Your Bot

### Quick Test Against Known Strategies

```bash
# Test against passive opponents (expect +25 to +35 BB/100)
pokerforbots spawn --spec "calling-station:5" \
  --bot-cmd "./my-bot" --hand-limit 1000 --print-stats

# Test against aggressive opponents (expect +5 to +15 BB/100)
pokerforbots spawn --spec "aggressive:5" \
  --bot-cmd "./my-bot" --hand-limit 1000 --print-stats

# Mixed field test
pokerforbots spawn --spec "calling-station:2,random:1,aggressive:2" \
  --bot-cmd "./my-bot" --hand-limit 1000 --print-stats
```

### Deterministic Testing

Use seeds for reproducible results:

```bash
# Run with fixed seed
pokerforbots spawn --seed 42 --hand-limit 1000 \
  --spec "random:5" --bot-cmd "./my-bot" \
  --write-stats results.json
```

### Custom Stakes

```bash
pokerforbots spawn --spec "calling-station:6" \
  --small-blind 25 --big-blind 50 --start-chips 5000 \
  --hand-limit 1000
```

## Running Built-in Bots

You can run example bots directly:

```bash
# Run individual bots
pokerforbots bot calling-station ws://localhost:8080/ws
pokerforbots bot random ws://localhost:8080/ws
pokerforbots bot aggressive ws://localhost:8080/ws
pokerforbots bot complex ws://localhost:8080/ws

# With custom game
pokerforbots bot random ws://localhost:8080/ws --game high-stakes
```

## Interactive Play

Connect as a human player:

```bash
# Start server with longer timeout for human play
pokerforbots server --timeout-ms 10000

# In another terminal, connect as a player
pokerforbots client --name Alice

# Or connect to a custom server
pokerforbots client --server ws://localhost:9000/ws --name Alice
```

Type `info` during play to see the current table state.

## Monitoring and Statistics

### Real-time Monitoring

While spawn is running, you can check stats via HTTP:

```bash
# Human-readable stats
curl -s http://localhost:8080/stats

# Detailed JSON stats
curl -s http://localhost:8080/admin/games/default/stats | jq .

# Watch stats update live
watch -n 2 'curl -s http://localhost:8080/stats'
```

### Understanding Statistics

Key metrics to watch:
- **BB/100**: Big blinds won/lost per 100 hands (primary performance metric)
- **VPIP**: Voluntarily put money in pot % (15-25% is typical for 6-max)
- **PFR**: Pre-flop raise % (should be 10-20% for most strategies)
- **Win Rate**: Percentage of hands won (15-25% is normal)

### Latency Tracking

When statistics are enabled, response times are tracked:
- `avg_response_ms` - Average decision time
- `p95_response_ms` - 95th percentile (should be under 100ms)
- `response_timeouts` - Count of timeouts

## Environment Variables

Bots receive these environment variables automatically:
- `POKERFORBOTS_SERVER` - WebSocket server URL
- `POKERFORBOTS_SEED` - Random seed for deterministic behavior
- `POKERFORBOTS_BOT_ID` - Unique bot identifier
- `POKERFORBOTS_GAME` - Target game ID (default: "default")

## Creating Your First Bot

### Minimal Bot Example

```go
package main

import (
    "github.com/lox/pokerforbots/sdk/client"
    "github.com/lox/pokerforbots/sdk/config"
    "github.com/lox/pokerforbots/protocol"
)

type MyBot struct{}

func (b MyBot) OnActionRequest(state *client.GameState, req protocol.ActionRequest) (string, int, error) {
    // Simple strategy: always check or call
    if slices.Contains(req.ValidActions, "check") {
        return "check", 0, nil
    }
    if slices.Contains(req.ValidActions, "call") {
        return "call", 0, nil
    }
    return "fold", 0, nil
}

// ... implement other required methods ...

func main() {
    cfg, _ := config.FromEnv()
    bot := client.New("my-bot", MyBot{}, logger)
    bot.Connect(cfg.ServerURL)
    bot.Run(context.Background())
}
```

### Testing Your Bot

```bash
# Quick test
pokerforbots spawn --bot-cmd "go run ./my-bot" \
  --spec "calling-station:5" --hand-limit 100

# If it works, test longer
pokerforbots spawn --bot-cmd "go run ./my-bot" \
  --spec "calling-station:5" --hand-limit 1000 --print-stats
```

## Common Workflows

### Development Iteration

```bash
# 1. Quick test while developing
pokerforbots spawn --bot-cmd "go run ./my-bot" \
  --spec "random:5" --hand-limit 100

# 2. Test with different opponents
pokerforbots spawn --bot-cmd "go run ./my-bot" \
  --spec "aggressive:5" --hand-limit 100

# 3. Longer test once stable
pokerforbots spawn --bot-cmd "go run ./my-bot" \
  --spec "calling-station:2,random:2,aggressive:1" \
  --hand-limit 1000 --print-stats
```

### Automated Testing Loop

```bash
#!/bin/bash
# Run multiple test sessions with different seeds
for i in {1..5}; do
  echo "Test $i of 5"
  pokerforbots spawn --seed $i --hand-limit 1000 \
    --spec "calling-station:2,random:2,aggressive:1" \
    --bot-cmd "./my-bot" \
    --write-stats results/test-$i.json
done
```

### Performance Benchmarking

```bash
# Quick benchmark
time pokerforbots spawn --spec "calling-station:6" \
  --hand-limit 1000 --seed 42

# With statistics output
pokerforbots spawn --spec "complex:6" \
  --hand-limit 10000 --write-stats bench.json
```

## Troubleshooting

### Common Issues

| Problem | Solution |
|---------|----------|
| Bots not connecting | Check server URL, ensure WebSocket endpoint is correct |
| High latency warnings | Profile bot code, optimize decision logic |
| Inconsistent results | Use deterministic seeds for testing |
| Port already in use | Spawn uses random ports by default, or specify `--addr` |

### Debug Mode

Enable debug logging to see what's happening:

```bash
pokerforbots spawn --spec "calling-station:5" \
  --bot-cmd "./my-bot" --hand-limit 10 \
  --log-level debug
```

## Next Steps

- See [Testing Guide](testing.md) for regression testing and statistical validation
- See [Command Reference](reference.md) for complete command documentation
- See [SDK Documentation](sdk.md) for building advanced bots
- See [Operations Guide](operations.md) for running production servers