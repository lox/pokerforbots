# Plan: Consolidate Commands into Single Pokerforbots CLI

## Overview

Consolidate four runtime commands in `./cmd/` into a single `pokerforbots` CLI with kong sub-commands, while keeping `gen-preflop` as a separate development utility. This improves usability, discoverability, and maintains a single point of entry for all runtime functionality.

## Current State

Five separate binaries:
- `cmd/server/` - Main poker server
- `cmd/client/` - Interactive human CLI client
- `cmd/spawner/` - Server + bot spawner for testing/demos
- `cmd/regression-tester/` - Regression testing framework
- `cmd/gen-preflop/` - Preflop table generator utility (development tool)

Each currently uses different CLI patterns:
- `server`, `client`, `spawner`, `regression-tester` use kong
- `gen-preflop` uses standard `flag` package

## Proposed Structure

Single `pokerforbots` binary with sub-commands for runtime tools:

```
pokerforbots
├── server        # Run the poker server
├── client        # Connect as interactive client
├── spawn         # Spawn server with bots (renamed from spawner)
└── regression    # Run regression tests (renamed from regression-tester)

gen-preflop       # Separate binary (development utility)
```

## Implementation Plan

### Phase 1: Create Main Command Structure

- [x] Create `cmd/pokerforbots/main.go` with kong command structure:
   ```go
   type CLI struct {
       Server     ServerCmd     `cmd:"" help:"Run poker server"`
       Client     ClientCmd     `cmd:"" help:"Interactive CLI client"`
       Spawn      SpawnCmd      `cmd:"" help:"Spawn server with bots"`
       Regression RegressionCmd `cmd:"" help:"Run regression tests"`
   }
   ```

- [x] Move command logic from individual `main.go` files into command structs

### Phase 2: Migrate Commands

#### Server Command
- [x] Move `cmd/server/main.go` logic to `ServerCmd.Run()`
- [x] Preserve all existing flags and functionality
- [x] Keep current config structure

#### Client Command
- [x] Move `cmd/client/main.go` logic to `ClientCmd.Run()`
- [x] Minimal changes, just structural

#### Spawn Command (renamed from spawner)
- [x] Move `cmd/spawner/main.go` logic to `SpawnCmd.Run()`
- [x] Rename for clarity and consistency

#### Regression Command (renamed from regression-tester)
- [x] Move `cmd/regression-tester/main.go` logic to `RegressionCmd.Run()`
- [x] Preserve complex configuration options

#### Gen-Preflop Command (Remains Separate)
- [x] Keep as standalone `cmd/gen-preflop/` binary
- [x] No changes needed, continues using `flag` package
- [x] Development utility, not part of main runtime CLI

### Phase 3: Update Build and Documentation

- [x] Update `Taskfile.yml`:
   - [x] Change build targets to single `pokerforbots` binary
   - [x] Update any references to old binaries

- [x] Update documentation:
   - [x] README.md with new command structure
   - [x] Development workflow docs
   - [x] Any scripts that reference old commands

- [x] Extract shared utilities:
   - [x] Common logging setup (zerolog configuration)
   - [x] Signal handling (graceful shutdown)
   - [x] HTTP server utilities (for server and spawn commands)

### Phase 4: Testing and Validation

- [x] Verify all functionality works identically
- [x] Test help text and command discovery
- [x] Ensure no regression in features

### Phase 5: Bot Execution Strategy

- [x] Implement bot resolver with execution mode detection
- [x] Add `pokerforbots bots` sub-commands for embedded bots
- [x] Support both `go run` and binary execution modes
- [x] Test calling-station, random, aggressive, and complex bots

## Benefits

1. **Single Binary**: Easier distribution and installation for runtime tools
2. **Discoverability**: `pokerforbots --help` shows all available commands
3. **Consistency**: Unified CLI experience with kong throughout
4. **Maintainability**: Shared code and utilities more easily
5. **Versioning**: Single version for all runtime tools

## No Backwards Compatibility Required

- Clean break from old command structure
- Update all documentation and scripts directly
- No wrapper scripts or symlinks needed

## File Structure After Consolidation

```
cmd/
├── pokerforbots/
│   ├── main.go           # Main entry point with CLI struct
│   ├── server.go         # ServerCmd implementation
│   ├── client.go         # ClientCmd implementation
│   ├── spawn.go          # SpawnCmd implementation
│   ├── regression.go     # RegressionCmd implementation
│   └── shared/           # Shared utilities
│       ├── logging.go    # Zerolog setup
│       └── signals.go    # Graceful shutdown
└── gen-preflop/          # Remains separate
    └── main.go
```

Old `cmd/{server,client,spawner,regression-tester}/` directories will be removed after migration.

## Risks and Mitigations

1. **Breaking Changes**: Acceptable - no backwards compatibility required
2. **Build Complexity**: Single binary will be larger, but Go's static linking makes this manageable
3. **Testing**: Ensure comprehensive testing of all sub-commands

## Decisions Made

1. ✅ **No Backwards Compatibility**: Clean break accepted
2. ✅ **Command Names**: `spawner` → `spawn` and `regression-tester` → `regression`
3. ✅ **Shared Utilities**: Yes, extract common logging and signal handling
4. ✅ **Gen-Preflop**: Remains separate as development utility