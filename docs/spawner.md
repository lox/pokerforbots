# Bot Spawner Documentation

The bot spawner is a library and tool for managing bot processes in the PokerForBots ecosystem. It provides a clean separation between the poker server (game logic) and bot orchestration (process management).

## Architecture

The spawner consists of:

1. **Library Package** (`internal/spawner`) - Reusable process management functions
2. **Spawner Tool** (`cmd/spawner`) - Standalone orchestrator with embedded server
3. **Integration Points** - Used by regression tester and other tools

## Using the Spawner Tool

### Basic Usage

```bash
# Run with default configuration (6 calling station bots)
go run ./cmd/spawner

# Specify bot types and counts using --spec
go run ./cmd/spawner --spec "calling-station:2,random:2,aggressive:2"

# Run specific bot commands
go run ./cmd/spawner --bot-cmd "go run ./sdk/examples/complex" --count 4

# Combine specifications and custom bots
go run ./cmd/spawner --spec "calling-station:2" --bot-cmd "./my-bot" --count 2
```

### Configuration Options

- `--addr` - Server address (default: `localhost:0` for random port)
- `--spec` - Bot specification string (e.g., `"calling-station:2,aggressive:3"`)
- `--bot-cmd` - External bot command (can be specified multiple times)
- `--count` - Number of instances for each --bot-cmd
- `--hand-limit` - Stop after N hands (0 for unlimited)
- `--seed` - Seed for deterministic testing
- `--write-stats` - Write statistics to file on exit
- `--print-stats` - Print statistics to console on exit

### Bot Specifications

The `--spec` format supports these built-in bot types:

- `calling-station` - Always calls/checks, never raises
- `random` - Makes random valid actions
- `aggressive` - Raises 70% of the time
- `complex` - Advanced strategy bot

Example specifications:
```bash
--spec "calling-station:3"              # 3 calling station bots
--spec "random:2,aggressive:1"          # 2 random, 1 aggressive
--spec "calling-station:2,complex:2"    # Mixed strategy game
```

## Using the Spawner Library

### In Your Own Tools

```go
import "github.com/lox/pokerforbots/internal/spawner"

// Create a spawner instance
sp := spawner.New("ws://localhost:8080/ws", logger)

// Define bot specifications
specs := []spawner.BotSpec{
    {
        Command: "go",
        Args:    []string{"run", "./sdk/examples/complex"},
        Count:   2,
        Env: map[string]string{
            "POKERFORBOTS_SEED": "42",
        },
    },
    {
        Command: "./my-custom-bot",
        Count:   3,
    },
}

// Spawn all bots
if err := sp.SpawnMany(specs); err != nil {
    log.Fatal(err)
}

// Later: stop all bots
sp.StopAll()
```

### Embedded Server Mode

The regression tester uses an embedded server with the spawner library:

```go
// Create embedded server
srv := server.NewServer(logger, rng, server.WithConfig(config))
listener, _ := net.Listen("tcp", "localhost:0")
go srv.Serve(listener)

// Create spawner for bot management
serverURL := fmt.Sprintf("ws://%s/ws", listener.Addr())
botSpawner := spawner.New(serverURL, logger)

// Spawn bots
botSpawner.SpawnMany(specs)

// Wait for game completion
<-srv.DefaultGameDone()

// Clean up
botSpawner.StopAll()
srv.Shutdown(ctx)
```

## Environment Variables

The spawner sets these environment variables for spawned bots:

- `POKERFORBOTS_SERVER` - WebSocket server URL
- `POKERFORBOTS_SEED` - Seed for deterministic RNG
- `POKERFORBOTS_BOT_ID` - Unique bot identifier
- `POKERFORBOTS_GAME` - Target game ID (default: "default")

## Process Management

The spawner handles:

- **Lifecycle** - Start, monitor, and stop bot processes
- **Output** - Prefix bot output with process ID for debugging
- **Cleanup** - Graceful shutdown with SIGTERM, forceful after timeout
- **Error Handling** - Log process exits and errors

## Migration from Server Flags

The server previously managed bots directly via flags like `--npcs` and `--bot-cmd`. This functionality has moved to the spawner:

### Old Way (Deprecated)
```bash
# Server managed bots directly
./server --npcs 6 --npc-calling 3 --npc-aggressive 3
```

### New Way
```bash
# Start server (pure game logic)
./server --addr localhost:8080

# In another terminal, use spawner for bots
./spawner --server ws://localhost:8080/ws --spec "calling-station:3,aggressive:3"
```

Or use the spawner with embedded server:
```bash
# Spawner starts server internally and manages bots
./spawner --spec "calling-station:3,aggressive:3" --hand-limit 1000
```

## Integration Examples

### Regression Testing

The regression tester uses the spawner library for bot management:

```go
// Start embedded server with spawner
orchestrator := regression.NewOrchestrator(config, healthMonitor)
orchestrator.StartServer(ctx, serverConfig) // Uses embedded server + spawner

// The orchestrator internally:
// 1. Creates embedded server
// 2. Uses spawner to launch bot processes
// 3. Waits for hand limit
// 4. Collects statistics
// 5. Cleans up everything
```

### Benchmarking

```bash
# Quick benchmark with spawner tool
time ./spawner --spec "calling-station:6" --hand-limit 1000 --seed 42

# With statistics output
./spawner --spec "complex:6" --hand-limit 10000 --write-stats bench.json
```

### Development Workflow

```bash
# Test your bot against various strategies
./spawner --bot-cmd "go run ./my-bot" --spec "aggressive:3,calling-station:2"

# Deterministic testing with seed
./spawner --seed 12345 --bot-cmd "./my-bot" --spec "complex:5" --hand-limit 100
```

## Best Practices

1. **Use embedded server mode** for testing - faster and more reliable than subprocess
2. **Set seeds** for deterministic testing and reproduction
3. **Monitor statistics** to track bot performance over time
4. **Clean shutdown** - always call StopAll() to terminate bot processes
5. **Resource limits** - be mindful of system resources when spawning many bots

## Troubleshooting

### Bots not connecting
- Check the server URL is correct
- Ensure bots support the POKERFORBOTS_SERVER environment variable
- Verify network connectivity

### Process cleanup issues
- The spawner sends SIGTERM for graceful shutdown
- After 5 seconds, it sends SIGKILL
- Check for zombie processes if bots don't terminate

### Performance considerations
- Each bot is a separate OS process
- Consider system limits when spawning many bots
- Use batch sizes appropriate for your hardware