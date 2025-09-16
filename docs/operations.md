# Operations Guide

## Running the Server

### Basic Server
```bash
# Default port 8080
task server

# Custom port (set in environment or modify Taskfile.yml)
PORT=9090 task server
```

### Demo with Test Bots
```bash
# Basic demo
task spawn-bots -- -bots 6 -spawn-server

# Reproducible testing with seed
task spawn-bots -- -seed 42 -bots 4 -spawn-server

# Verbose output
task spawn-bots -- -bots 6 -spawn-server -v
```

## Monitoring

The server exposes HTTP endpoints for monitoring:

- `GET /health` - Health check endpoint
- `GET /stats` - Basic statistics (connected bots, hands completed)

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