# Docker Setup for Holdem CLI

This Docker setup makes it easy to test the client-server functionality of the Holdem CLI poker game.

## Quick Start

1. **Run the complete test**:
   ```bash
   ./scripts/docker-test.sh test
   ```

2. **Connect an interactive client**:
   ```bash
   ./scripts/docker-test.sh client Alice
   ```

3. **Start everything**:
   ```bash
   ./scripts/docker-test.sh full
   ```

## Available Services

### Server
- **holdem-server**: WebSocket server managing multiple poker tables
- Accessible at `http://localhost:8080`
- Health check endpoint: `http://localhost:8080/health`
- Starts with 2 tables and 1 bot per table by default

### Clients
- **holdem-client-1**: Automated client "Alice"
- **holdem-client-2**: Automated client "Bob" 
- **holdem-client-interactive**: On-demand interactive client

### Local Game (for comparison)
- **holdem-local**: Traditional single-process game (profile: `local-only`)

## Script Commands

Use `./scripts/docker-test.sh [command]`:

| Command | Description |
|---------|-------------|
| `test` | Run complete test flow |
| `build` | Build Docker image |
| `server` | Start server only |
| `full` | Start server + automated clients |
| `client [name]` | Connect interactive client |
| `logs [service]` | Show logs |
| `status` | Show service status |
| `stop` | Stop all services |
| `clean` | Clean up Docker resources |

## Manual Docker Commands

### Start server only:
```bash
docker-compose up -d holdem-server
```

### Start everything:
```bash
docker-compose up -d
```

### Connect interactive client:
```bash
docker-compose run --rm holdem-client-interactive
```

### View logs:
```bash
docker-compose logs -f holdem-server
docker-compose logs -f holdem-client-1
```

### Stop everything:
```bash
docker-compose down
```

## Client Commands

When connected to a client, you can use these commands:

- `/list` - List available tables
- `/join <table_id>` - Join a table with 1000 chip buy-in
- `/leave` - Leave current table
- `/quit` - Quit the game

During gameplay, use standard poker actions:
- `call`, `c` - Call the current bet
- `raise 50`, `r 50` - Raise to 50
- `fold`, `f` - Fold hand
- `check`, `k` - Check (when no bet)
- `allin`, `a` - Go all-in

## Architecture

```
┌─────────────────┐    WebSocket    ┌─────────────────┐
│   Client 1      │◄──────────────► │                 │
│   (Alice)       │                 │                 │
├─────────────────┤                 │  Holdem Server  │
│   Client 2      │◄──────────────► │                 │
│   (Bob)         │                 │  - GameService  │
├─────────────────┤                 │  - Tables       │
│   Client N      │◄──────────────► │  - Bots         │
│   (Interactive) │                 │                 │
└─────────────────┘                 └─────────────────┘
```

## Logs and Data

- Server logs: `./logs/`
- Hand history: `./handhistory/`
- Client logs: Inside containers at `/app/logs/`

## Troubleshooting

### Server not starting
```bash
# Check server logs
docker-compose logs holdem-server

# Verify health
curl http://localhost:8080/health
```

### Client connection issues
```bash
# Check network connectivity
docker-compose exec holdem-client-1 curl http://holdem-server:8080/health

# Check client logs
docker-compose logs holdem-client-1
```

### Port conflicts
If port 8080 is already in use, modify `docker-compose.yml`:
```yaml
ports:
  - "8081:8080"  # Use port 8081 instead
```

### Clean restart
```bash
./scripts/docker-test.sh clean
./scripts/docker-test.sh test
```

## Development

### Rebuilding after code changes
```bash
docker-compose build
docker-compose up -d
```

### Running local game for comparison
```bash
docker-compose --profile local-only up holdem-local
```

### Debugging
```bash
# Run with debug logging
docker-compose run --rm holdem-server ./bin/holdem-server --log-level=debug

# Interactive shell in container
docker-compose exec holdem-server /bin/bash
```
