# Operations Guide

## Running the Server

### Quick Start: Spawn Command (Recommended)

For most use cases, use the `spawn` command which manages both server and bots:

```bash
# Quick demo with bots
pokerforbots spawn --spec "calling-station:3,random:3"

# Test your bot
pokerforbots spawn --bot-cmd "./my-bot" --spec "aggressive:5"

# Deterministic testing
pokerforbots spawn --seed 42 --hand-limit 1000 --spec "random:6"
```

### Standalone Server

For production deployments or custom integrations:

```bash
# Basic server on default port
pokerforbots server

# Custom configuration
pokerforbots server --addr :9090 \
  --small-blind 25 --big-blind 50 \
  --start-chips 5000

# With statistics collection
pokerforbots server --enable-stats --max-stats-hands 50000

# Deterministic testing mode
pokerforbots server --seed 1337 --hand-limit 500

# For human players (longer timeout)
pokerforbots server --timeout-ms 10000
```

Note: The standalone server no longer includes built-in NPC bots. Use the `spawn` command for testing with bots, or connect bots separately using the `bot` command.

### Multi-Game Setup

Create additional games via the admin API:

```bash
# Start the server
pokerforbots server

# Create a custom game
curl -X POST http://localhost:8080/admin/games \
     -H "Content-Type: application/json" \
     -d '{
           "id": "high-stakes",
           "small_blind": 50,
           "big_blind": 100,
           "start_chips": 10000,
           "timeout_ms": 100,
           "min_players": 2,
           "max_players": 6
         }'

# Connect bots to the custom game
pokerforbots bot random ws://localhost:8080/ws --game high-stakes
pokerforbots bot aggressive ws://localhost:8080/ws --game high-stakes

# Or connect your custom bot
./my-bot --server ws://localhost:8080/ws --game high-stakes
```

## Hand History Recording

Set `--hand-history` (on either `spawn` or `server`) to persist every hand in PHH format. Example (spawn mode):

```bash
pokerforbots spawn --spec "complex:3,random:3" --hand-limit 200 \
  --hand-history --hand-history-dir ./hands \
  --hand-history-flush-secs 5 --hand-history-flush-hands 50
```

The standalone server exposes the same flags. Hand histories are written to `<dir>/game-<id>/session.phhs`. See [docs/hand-history.md](hand-history.md) for details on the PHH format, configuration options, and parsing.

## Monitoring

The server exposes HTTP endpoints for monitoring and discovery:

- `GET /health` - Health check endpoint
- `GET /stats` - Basic aggregate statistics (connected bots, hands completed)
- `GET /games` - JSON list of configured games with blinds, seat limits, and player requirements
- `GET /admin/games/{id}/stats` - Detailed per-game stats including bot win/loss deltas and remaining hand budget
- `POST /admin/games` / `DELETE /admin/games/{id}` - create or remove tables (authentication TBD; restrict to trusted environments)
- Bots connected over WebSocket receive a `game_completed` message (with the per-bot stats snapshot) whenever a game exhausts its configured hand budget.

## Architecture Notes

### Connection Management
- WebSocket connections with binary msgpack protocol
- Automatic reconnection not supported (bots must reconnect)
- Ping/pong keepalive at 54-second intervals

### Hand Execution
- Concurrent hand execution in separate goroutines
- 100ms timeout for all decisions (hardcoded)
- Automatic folding on timeout or disconnection

### Bot Pool
- Channel-based queue for available bots
- Automatic matching when 2+ bots available
- Random selection for fairness
