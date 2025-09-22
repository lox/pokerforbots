# Bot Spawner Extraction Plan

## Problem Statement

The server has accumulated significant orchestration complexity beyond its core responsibility:

### Bot Management (~300 lines)
- 4 different bot spawning mechanisms (internal NPCs, external processes, admin API, CLI configs)
- Complex CLI flag parsing (`--bot-cmd`, `--npc-bot-cmd`, `--npcs`, `--npc-calling`, etc.)
- Process lifecycle management intertwined with server startup/shutdown

### Game Orchestration Logic
- Auto-exit when default game hits hand limit
- StopOnInsufficientBots behavior
- PrintStatsOnExit and WriteStatsOnExit
- Complex shutdown coordination

These responsibilities belong in external orchestration, not the core server.

## Proposed Solution

1. **Extract bot spawning** into a reusable `internal/spawner` package
2. **Simplify server** to only handle core poker game logic
3. **Move orchestration logic** to external tools that use the server's HTTP/WebSocket APIs

### Core Design

```go
// internal/spawner/spawner.go
type BotSpawner struct {
    serverURL string
    processes map[string]*Process  // All bots are external processes
    logger    zerolog.Logger
}

type BotSpec struct {
    Command  string            // Command to execute (e.g. "go")
    Args     []string          // Arguments (e.g. ["run", "./sdk/examples/calling-station"])
    Count    int               // Number to spawn
    GameID   string            // Target game (default: "default")
    Env      map[string]string // Additional environment variables
}

// Simple, focused API
func New(serverURL string, logger zerolog.Logger) *BotSpawner
func (s *BotSpawner) Spawn(spec BotSpec) error
func (s *BotSpawner) SpawnMany(specs []BotSpec) error
func (s *BotSpawner) StopAll() error
func (s *BotSpawner) Wait() error
```

### Strategy Bots (External)

All strategies are now standalone bots in `sdk/examples/`:
- `sdk/examples/calling-station` - Always checks/calls
- `sdk/examples/aggressive` - Raises 70% of the time
- `sdk/examples/random` - Random valid actions
- `sdk/examples/complex` - Advanced strategy

No built-in strategies needed - they're all just external processes!

## Implementation Phases

### Phase 1: Create Package Structure
- [ ] Create `internal/spawner/spawner.go` with core types
- [ ] Create `internal/spawner/process.go` for process management
- [ ] Add comprehensive tests

### Phase 2: Port External Process Management
- [ ] Move subprocess spawning from `cmd/server/subprocess.go`
- [ ] Move environment variable injection logic
- [ ] Move output prefixing and logging
- [ ] Add proper process cleanup and signal handling

### Phase 3: Simplify Server Core
- [ ] Remove ALL NPC code from `internal/server/npc.go` (delete file)
- [ ] Remove hand limit auto-exit logic (use external monitoring instead)
- [ ] Remove StopOnInsufficientBots (external orchestrator handles this)
- [ ] Remove PrintStatsOnExit/WriteStatsOnExit (use HTTP API `/games/{id}/stats`)
- [ ] Remove bot management from `internal/server/server.go`
- [ ] Simplify CLI to just `--addr`, `--small-blind`, `--big-blind`, `--timeout-ms`

### Phase 4: Integrate with Server Main
- [ ] Update `cmd/server/main.go` to optionally use BotSpawner
- [ ] Move all orchestration logic to external runner
- [ ] Server just starts and waits for shutdown signal
- [ ] Update tests to use spawner

### Phase 5: Update and Consolidate Tools
- [ ] Update regression tester to use spawner
- [ ] Migrate benchmark functionality into spawner/regression-tester
- [ ] Deprecate and remove `cmd/benchmark` (redundant with spawner)
- [ ] Update demo scripts to use spawner
- [ ] Update documentation with new workflows

## Benefits

1. **Simplicity**: Server focuses only on poker game logic
2. **Reusability**: Spawner can be used by any tool (server, regression tester, demos)
3. **Testability**: Each component can be tested in isolation
4. **Maintainability**: Clear separation of concerns
5. **Flexibility**: Easy to add new bot types or strategies

## Critical Requirements (from Codex Review)

### Must Preserve
1. **Zero-config demo path**: `cmd/spawner` must provide equivalent one-liner to current `--npcs` flags
2. **Deterministic testing**: Maintain seed injection (`POKERFORBOTS_SEED`, `POKERFORBOTS_BOT_ID`)
3. **Admin API compatibility**: Keep or replace `/admin/games` NPC endpoints during transition
4. **CI/test coverage**: Ensure all existing tests work with new architecture

### Implementation Details
1. **Rich Process abstraction**: Include context management, stdout/stderr piping, exit callbacks
2. **Declarative config**: Support YAML/JSON for batch spawning, not just CLI flags
3. **Server as subprocess**: Consider spawning server as child process vs embedding (isolation vs simplicity)

## Migration Strategy

To ensure smooth transition:

1. **Adopt before delete**: New spawner must work before removing old code
2. **Compatibility shim**: Keep old CLI flags working temporarily via adapter
3. **Parallel implementation**: Build spawner alongside existing code
4. **Update docs first**: Document new flow before removing old
5. **Gradual removal**: Delete old code only after all tools migrated

## Example Usage

### Minimal Server (Pure Poker Logic)

```go
// cmd/server/main.go - AFTER simplification
func main() {
    var cli struct {
        Addr       string `default:"localhost:8080"`
        SmallBlind int    `default:"5"`
        BigBlind   int    `default:"10"`
        TimeoutMs  int    `default:"100"`
    }
    kong.Parse(&cli)

    // Just start server and wait
    server := server.New(server.Config{
        Addr:       cli.Addr,
        SmallBlind: cli.SmallBlind,
        BigBlind:   cli.BigBlind,
        TimeoutMs:  cli.TimeoutMs,
    })

    server.Start()
    <-signals
    server.Shutdown()
}
```

### External Runner with Spawner (In-Process Server)

```go
// cmd/spawner/main.go - Bot spawning orchestrator
func main() {
    config := parseConfig()

    // Start server in-process (like benchmark does)
    srv := server.NewServer(logger, rng, server.WithConfig(server.Config{
        SmallBlind: config.SmallBlind,
        BigBlind:   config.BigBlind,
        StartChips: config.StartChips,
        Timeout:    config.Timeout,
    }))

    listener, _ := net.Listen("tcp", config.ServerAddr)
    go srv.Serve(listener)
    defer srv.Shutdown()

    serverURL := fmt.Sprintf("ws://%s/ws", listener.Addr())

    // Spawn bots
    spawner := spawner.New(serverURL, logger)
    spawner.SpawnMany(config.Bots)

    // Monitor game status via HTTP API
    baseURL := strings.Replace(serverURL, "/ws", "", 1)
    for {
        resp, _ := http.Get(baseURL + "/games/default/stats")
        var stats GameStats
        json.NewDecoder(resp.Body).Decode(&stats)

        if stats.HandsCompleted >= config.HandLimit {
            break
        }
        if stats.ActiveBots < config.MinBots {
            log.Warn("Insufficient bots")
            break
        }
        time.Sleep(1 * time.Second)
    }

    // Clean shutdown
    if config.SaveStats {
        resp, _ := http.Get(baseURL + "/games/default/stats")
        // Save to file...
    }

    spawner.StopAll()
    srv.Shutdown()
}
```

### Regression Tester with Spawner

```go
// cmd/regression-tester/runner.go
func runBatch(config BatchConfig) error {
    // Start server
    server := startServer(config)

    // Use spawner for all bots
    spawner := spawner.New(server.URL(), logger)

    // Spawn challenger bots
    spawner.Spawn(BotSpec{
        Command: "go",
        Args:    []string{"run", "./sdk/examples/complex"},
        Count:   2,
    })

    // Spawn baseline bots (using standalone bot)
    spawner.Spawn(BotSpec{
        Command: "go",
        Args:    []string{"run", "./sdk/examples/calling-station"},
        Count:   3,
    })

    // Wait for completion
    server.Wait()
    spawner.StopAll()
}
```

## Success Metrics

- Server main.go reduced from ~500 to ~100 lines
- Server core reduced from ~766 to ~400 lines
- Bot management consolidated in `internal/spawner` (~500 lines)
- All orchestration logic moves to external runners
- Server becomes a pure poker game engine
- Tests pass without modification

## Key Principle

**The server should be a stateless poker game engine that:**
- Accepts WebSocket connections from bots
- Runs poker games according to rules
- Provides HTTP API for game status
- That's it.

**Everything else (bot spawning, hand limits, stats writing, etc.) belongs in external orchestration tools.**