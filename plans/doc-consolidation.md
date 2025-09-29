# Documentation Consolidation Plan

## Overview

This plan outlines the updates needed to simplify and consolidate documentation following the command consolidation into a single `pokerforbots` binary with sub-commands.

## Current State

Multiple documentation files reference the old command structure:
- `cmd/server` → now `pokerforbots server`
- `cmd/client` → now `pokerforbots client`
- `cmd/spawner` → now `pokerforbots spawn`
- `cmd/regression-tester` → now `pokerforbots regression`
- `cmd/testbots` → removed (functionality in `pokerforbots bots`)

## Proposed Changes

### 1. Merge and Simplify Documentation

**Consolidate into fewer, clearer docs:**

- **README.md** - Quick start and overview
- **docs/quickstart.md** - Consolidated quick start guide combining spawner + development workflow
- **docs/testing.md** - Merge regression-tester.md with testing sections from development-workflow.md
- **docs/operations.md** - Keep but update for new commands
- **docs/reference.md** - Command reference for all pokerforbots sub-commands
- Keep technical docs unchanged: design.md, websocket-protocol.md, poker-rules.md, sdk.md, http-api.md, benchmarking.md

**Remove/Archive:**
- `docs/spawner.md` - Content merged into quickstart.md
- `docs/regression-tester.md` - Content merged into testing.md
- `docs/development-workflow.md` - Content split between quickstart.md and testing.md

### 2. Command Updates Throughout

Replace all old command references with new structure:

#### Server Commands
```bash
# Old
go run ./cmd/server --npc-bots 6
task server

# New
pokerforbots server --addr :8080
# Or during development:
go run ./cmd/pokerforbots server --addr :8080
```

#### Spawn Commands (Most Common)
```bash
# Old
go run ./cmd/spawner --spec "calling-station:2,random:2"
./spawner --hand-limit 1000

# New
pokerforbots spawn --spec "calling-station:2,random:2"
# Or during development:
go run ./cmd/pokerforbots spawn --spec "calling-station:2,random:2"
```

#### Client Commands
```bash
# Old
go run ./cmd/client --server ws://localhost:8080/ws

# New
pokerforbots client ws://localhost:8080/ws
# Or during development:
go run ./cmd/pokerforbots client ws://localhost:8080/ws
```

#### Regression Testing
```bash
# Old
go run ./cmd/regression-tester --mode heads-up --hands 5000
task regression:heads-up

# New
pokerforbots regression --mode heads-up --hands 5000
# Or during development:
go run ./cmd/pokerforbots regression --mode heads-up --hands 5000
# Task still works: task regression:heads-up
```

#### Bot Commands (New)
```bash
# New embedded bot execution
pokerforbots bots calling-station ws://localhost:8080/ws
pokerforbots bots random ws://localhost:8080/ws
pokerforbots bots aggressive ws://localhost:8080/ws
pokerforbots bots complex ws://localhost:8080/ws

# Info about bot resolver
pokerforbots bots info
```

### 3. Simplify Installation Instructions

Add a clear installation section to README:

```markdown
## Installation

### Option 1: Install Binary (Recommended)
```bash
# Build and install to ~/bin
go build -o ~/bin/pokerforbots ./cmd/pokerforbots

# Verify installation
pokerforbots --help
```

### Option 2: Run from Source
```bash
# During development, use go run
go run ./cmd/pokerforbots spawn --spec "calling-station:4,random:2"
```
```

### 4. Update Task File References

The Taskfile.yml should be updated to use the new commands, keeping backward compatibility where sensible:

```yaml
tasks:
  server:
    desc: "Run the poker server"
    cmds:
      - go run ./cmd/pokerforbots server {{.CLI_ARGS}}

  spawn:demo:
    desc: "Run a demo with bots"
    cmds:
      - go run ./cmd/pokerforbots spawn --spec "calling-station:2,random:2,aggressive:2" --hand-limit 1000 {{.CLI_ARGS}}

  regression:heads-up:
    desc: "Run heads-up regression test"
    cmds:
      - go run ./cmd/pokerforbots regression --mode heads-up --hands {{.HANDS | default "1000"}} {{.CLI_ARGS}}
```

### 5. Consolidate Examples

Create a unified examples section that shows common workflows:

```markdown
## Common Workflows

### Quick Testing
```bash
# Test your bot against various opponents
pokerforbots spawn --spec "calling-station:3,aggressive:2" \
  --bot-cmd "./my-bot" --hand-limit 1000 --print-stats
```

### Regression Testing
```bash
# Validate bot improvements
pokerforbots regression --mode heads-up --hands 5000 \
  --challenger "./my-new-bot" --baseline "./my-old-bot"
```

### Development Iteration
```bash
# Run from source during development
go run ./cmd/pokerforbots spawn --spec "random:6" --hand-limit 100

# With debugging
go run ./cmd/pokerforbots spawn --spec "calling-station:5" \
  --bot-cmd "go run ./my-bot" --log-level debug
```
```

## Implementation Order

1. **Update README.md** - Add installation instructions, update quick start
2. **Create docs/quickstart.md** - Merge spawner + basic development workflow
3. **Create docs/testing.md** - Merge regression-tester + testing sections
4. **Create docs/reference.md** - Complete command reference
5. **Update docs/operations.md** - Fix command examples
6. **Update Taskfile.yml** - Use new commands
7. **Archive old docs** - Move to docs/archive/ for reference
8. **Update any remaining references** - SDK docs, etc.

## Benefits

1. **Simpler onboarding** - One binary, clear commands
2. **Less documentation** - 3 consolidated docs instead of 5+ scattered ones
3. **Clearer workflows** - Unified examples using consistent commands
4. **Better discoverability** - `pokerforbots --help` shows everything
5. **Easier maintenance** - Less duplication, single source of truth

## Notes

- Keep backward compatibility in Taskfile for common workflows
- Preserve all technical documentation (protocol, design, rules)
- Focus on the most common use cases (spawn, regression testing)
- Make installation prominent in README