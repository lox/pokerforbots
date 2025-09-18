# Operations Guide

## Running the Server

### Basic Server
```bash
# Default port 8080
task server

# Custom port (set in environment or modify Taskfile.yml)
PORT=9090 task server

# Spawn NPC opponents in the default game (auto distribute strategies)
go run ./cmd/server --npc-bots 6

# Explicit NPC mix
go run ./cmd/server --npc-calling 2 --npc-random 3 --npc-aggro 1
```

### Demo with Test Bots
Create a sandbox game with NPC opponents via the admin API, then connect your bot:

```bash
curl -X POST http://localhost:8080/admin/games \
     -H "Content-Type: application/json" \
     -d '{
           "id": "sandbox",
           "small_blind": 5,
           "big_blind": 10,
           "start_chips": 1000,
           "timeout_ms": 100,
           "min_players": 2,
           "max_players": 6,
           "require_player": true,
           "npcs": [
             {"strategy": "calling", "count": 2},
             {"strategy": "random", "count": 3}
           ]
         }'

# Run your development bot (connecting as role=player)
go run ./examples/random --server ws://localhost:8080/ws --game sandbox

# OR launch the complex bot skeleton (for custom logic)
go run ./examples/complex --server ws://localhost:8080/ws --game sandbox --debug
```

## Monitoring

The server exposes HTTP endpoints for monitoring and discovery:

- `GET /health` - Health check endpoint
- `GET /stats` - Basic aggregate statistics (connected bots, hands completed)
- `GET /games` - JSON list of configured games with blinds, seat limits, and player requirements
- `POST /admin/games` / `DELETE /admin/games/{id}` - create or remove tables (authentication TBD; restrict to trusted environments)
  - Payload may include an `npcs` array to automatically spawn built-in opponents (strategies: `calling`, `aggressive`, `random`).

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
