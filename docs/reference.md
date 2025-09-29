# Command Reference

Complete reference for all `pokerforbots` commands and options.

## Global Usage

```bash
pokerforbots <command> [options]
```

Available commands:
- `spawn` - Start server with bots for testing
- `regression` - Run statistical bot comparison
- `server` - Run standalone poker server
- `client` - Connect as interactive human client
- `bots` - Run built-in example bots

## spawn Command

Start an embedded server with bot processes for testing and demos.

### Synopsis

```bash
pokerforbots spawn [options]
```

### Options

| Option | Default | Description |
|--------|---------|-------------|
| `--addr` | `localhost:0` | Server address (0 = random port) |
| `--spec` | `calling-station:6` | Bot specification string |
| `--bot-cmd` | - | External bot command (repeatable) |
| `--count` | `1` | Number of each --bot-cmd to spawn |
| `--hand-limit` | `0` | Stop after N hands (0 = unlimited) |
| `--seed` | `0` | RNG seed (0 = random) |
| `--small-blind` | `5` | Small blind amount |
| `--big-blind` | `10` | Big blind amount |
| `--start-chips` | `1000` | Starting chip stack |
| `--timeout-ms` | `100` | Bot decision timeout (ms) |
| `--min-players` | `0` | Min players to start (0 = auto) |
| `--max-players` | `9` | Maximum players at table |
| `--print-stats` | `false` | Print statistics on exit |
| `--write-stats` | - | Write stats to file on exit |
| `--pretty` | `false` | Pretty-print hand output |
| `--log-level` | - | Log level (debug/info/warn/error) |
| `--latency-tracking` | `false` | Enable latency metrics collection |

### Bot Specification Format

The `--spec` option uses the format `strategy:count,strategy:count,...`

Available strategies:
- `calling-station` (aliases: `calling`, `cs`) - Always calls/checks
- `random` (aliases: `rnd`) - Random valid actions
- `aggressive` (aliases: `aggro`) - Raises frequently
- `complex` - Advanced strategy bot

### Examples

```bash
# Basic usage with built-in bots
pokerforbots spawn --spec "calling-station:3,random:3"

# Test custom bot
pokerforbots spawn --bot-cmd "./my-bot" --spec "aggressive:5"

# Multiple custom bots
pokerforbots spawn \
  --bot-cmd "./bot-v1" --count 2 \
  --bot-cmd "./bot-v2" --count 2 \
  --spec "random:2"

# Deterministic testing
pokerforbots spawn --seed 42 --hand-limit 1000 \
  --spec "calling-station:6" --write-stats results.json

# Custom stakes
pokerforbots spawn --small-blind 25 --big-blind 50 \
  --start-chips 5000 --spec "random:6"

# Pretty output mode
pokerforbots spawn --pretty --spec "aggressive:4" --hand-limit 100
```

## regression Command

Run statistical comparison between bot versions.

### Synopsis

```bash
pokerforbots regression [options]
```

### Options

| Option | Default | Description |
|--------|---------|-------------|
| `--mode` | `heads-up` | Test mode (see below) |
| `--challenger` | - | Challenger bot command |
| `--baseline` | - | Baseline bot command |
| `--hands` | `10000` | Total hands to run |
| `--batch-size` | `10000` | Hands per batch |
| `--seeds` | `42` | Comma-separated seeds |
| `--starting-chips` | `1000` | Starting chips |
| `--timeout-ms` | `100` | Bot decision timeout (ms) |
| `--challenger-seats` | `2` | Challenger seats (population mode) |
| `--baseline-seats` | `4` | Baseline seats (population mode) |
| `--npcs` | - | NPC configuration (e.g., `aggressive:2,calling:1`) |
| `--significance-level` | `0.05` | P-value threshold |
| `--latency-warn-ms` | `100` | Latency warning threshold |
| `--output` | `both` | Output format (see below) |
| `--output-file` | - | Output file for results |

### Test Modes

- `heads-up` - Direct 1v1 comparison
- `population` - Mixed table with configurable seats
- `npc-benchmark` - Test against fixed strategies
- `self-play` - Variance baseline test
- `all` - Run all modes with correction

### Output Formats

- `json` - JSON format for parsing
- `summary` - Human-readable summary
- `both` - Both JSON and summary

### Examples

```bash
# Basic heads-up test
pokerforbots regression --mode heads-up --hands 5000

# Compare specific bots
pokerforbots regression \
  --challenger "./bot-new" \
  --baseline "./bot-old" \
  --hands 10000

# Population test with custom mix
pokerforbots regression \
  --mode population \
  --challenger-seats 3 \
  --baseline-seats 3 \
  --hands 10000

# Deterministic with seeds
pokerforbots regression \
  --seeds "42,123,456" \
  --hands 15000

# CI/CD with JSON output
pokerforbots regression \
  --mode all \
  --hands 50000 \
  --output json \
  --output-file results.json

# Custom latency threshold
pokerforbots regression \
  --latency-warn-ms 80 \
  --hands 5000
```

## server Command

Run a standalone poker server.

### Synopsis

```bash
pokerforbots server [options]
```

### Options

| Option | Default | Description |
|--------|---------|-------------|
| `--addr` | `:8080` | Server address |
| `--small-blind` | `5` | Small blind amount |
| `--big-blind` | `10` | Big blind amount |
| `--start-chips` | `1000` | Starting chip stack |
| `--timeout-ms` | `100` | Action timeout (ms) |
| `--min-players` | `2` | Min players to start |
| `--max-players` | `9` | Max players at table |
| `--seed` | `0` | RNG seed (0 = random) |
| `--hand-limit` | `0` | Stop after N hands |
| `--enable-stats` | `false` | Enable statistics collection |
| `--max-stats-hands` | `10000` | Max hands to track in stats |
| `--latency-tracking` | `false` | Enable latency metrics |

### Examples

```bash
# Basic server
pokerforbots server

# Custom port and stakes
pokerforbots server --addr :9000 \
  --small-blind 25 --big-blind 50

# With statistics
pokerforbots server --enable-stats --max-stats-hands 50000

# Deterministic testing
pokerforbots server --seed 42 --hand-limit 1000

# For human play (longer timeout)
pokerforbots server --timeout-ms 10000
```

## client Command

Connect as an interactive human player.

### Synopsis

```bash
pokerforbots client [options]
```

### Options

| Option | Default | Description |
|--------|---------|-------------|
| `--server` | `ws://localhost:8080/ws` | WebSocket server URL |
| `--name` | `$USER` or `Player` | Player name |
| `--game` | `default` | Game ID to join |

### Interactive Commands

During play, you can type:
- `info` - Display current table state
- `fold`, `check`, `call` - Make actions
- `raise <amount>` - Raise to amount
- `bet <amount>` - Bet amount
- `allin` - Go all-in

### Examples

```bash
# Connect to local server (uses default)
pokerforbots client --name Alice

# Connect to custom server
pokerforbots client --server ws://localhost:9000/ws --name Alice

# Join specific game
pokerforbots client --server ws://localhost:8080/ws \
  --name Bob --game high-stakes

# Connect to remote server
pokerforbots client --server ws://poker.example.com/ws
```

## bots Command

Run built-in example bots.

### Synopsis

```bash
pokerforbots bots <strategy> <server-url> [options]
pokerforbots bots info
```

### Available Strategies

- `calling-station` - Always calls/checks, never raises
- `random` - Makes random valid actions
- `aggressive` - Raises frequently (70% of the time)
- `complex` - Advanced strategy with position awareness

### Options

| Option | Default | Description |
|--------|---------|-------------|
| `--game` | `default` | Game ID to join |
| `--name` | auto | Bot name override |

### Examples

```bash
# Run calling station bot
pokerforbots bots calling-station ws://localhost:8080/ws

# Run with custom game
pokerforbots bots random ws://localhost:8080/ws --game test

# Check bot resolver information
pokerforbots bots info

# Run complex bot
pokerforbots bots complex ws://localhost:8080/ws
```

## Environment Variables

These environment variables affect bot behavior:

| Variable | Description |
|----------|-------------|
| `POKERFORBOTS_SERVER` | Server URL (overrides command line) |
| `POKERFORBOTS_SEED` | Random seed for deterministic behavior |
| `POKERFORBOTS_BOT_ID` | Unique bot identifier |
| `POKERFORBOTS_GAME` | Target game ID |

## HTTP API Endpoints

When server or spawn is running:

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Server health check |
| `GET /stats` | Human-readable statistics |
| `GET /games` | List active games |
| `GET /admin/games/{id}/stats` | Detailed game statistics (JSON) |
| `POST /admin/games` | Create new game |
| `DELETE /admin/games/{id}` | Remove game |

### Example API Usage

```bash
# Check server health
curl http://localhost:8080/health

# Get statistics
curl http://localhost:8080/stats

# Get detailed JSON stats
curl http://localhost:8080/admin/games/default/stats | jq .

# Monitor stats
watch -n 2 'curl -s http://localhost:8080/stats'
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Invalid arguments |
| 130 | Interrupted (Ctrl+C) |

## Logging

All commands support these log levels via `--log-level`:
- `debug` - Detailed debugging information
- `info` - Normal informational messages (default)
- `warn` - Warnings only
- `error` - Errors only
- `trace` - Extremely verbose (where supported)

## Performance Notes

- Bot decisions must complete within timeout (default 100ms)
- The spawn command uses embedded server for better performance
- Use `--latency-tracking` to monitor response times
- Deterministic seeds (`--seed`) ensure reproducible testing
- The p95 latency metric is key for performance validation

## See Also

- [Quick Start Guide](quickstart.md) - Getting started quickly
- [Testing Guide](testing.md) - Regression testing workflows
- [SDK Documentation](sdk.md) - Building custom bots
- [Operations Guide](operations.md) - Production deployment