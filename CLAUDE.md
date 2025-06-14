# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a **client/server Texas Hold'em poker platform** built in Go, designed for bot development and testing. The system features WebSocket-based multiplayer gameplay, interactive CLI clients, automated AI bots, HCL configuration, and Docker deployment.

## Development Commands

### Essential Commands
- **Test**: `./bin/task test` (gotestsum with testname format)
- **Lint**: `./bin/task lint` (golangci-lint)
- **Format**: `./bin/task fmt` (go fmt + goimports)
- **Build**: `./bin/task build` (builds server, client, poker-odds binaries)
- **Full check**: `./bin/task check` (test + lint + build)
- **Development workflow**: `./bin/task dev` (fmt + test + lint)

### Test Execution
- All tests: `./bin/task test`
- Verbose: `./bin/task test-verbose`
- Single test: `go test -run TestName ./path/to/package`
- With coverage: `go test -cover ./...`

### Application Commands
- **Server**: `./bin/holdem-server [--config holdem-server.hcl]`
- **Client**: `./bin/holdem-client join <table-id> [--config holdem-client.hcl]`
- **List tables**: `./bin/holdem-client list-tables`
- **Add bots**: `./bin/holdem-client add-bots <table-id> --count=3`
- **Poker odds**: `go run cmd/poker-odds/main.go --help`

### Docker Workflow
- **Complete test**: `./scripts/docker-test.sh test`
- **Full system**: `./scripts/docker-test.sh full`
- **Interactive client**: `./scripts/docker-test.sh client PlayerName`

## Architecture

### Core Components
- **cmd/holdem-server**: WebSocket game server with table management
- **cmd/holdem-client**: Interactive TUI client (Bubble Tea)
- **cmd/poker-odds**: Monte Carlo hand equity calculator
- **internal/game**: Game engine, state management, betting rounds
- **internal/tui**: Terminal UI with two-pane design (log + actions)
- **internal/bot**: AI strategies (chart-based, TAG, maniac, calling station)
- **internal/server**: WebSocket protocol and message handling
- **internal/evaluator**: Hand strength evaluation and equity calculation

### Key Patterns
- **Clean Architecture**: Game logic separated from networking and presentation
- **Event-driven**: Publisher/subscriber pattern for game events
- **WebSocket Protocol**: JSON messages with request/response correlation
- **HCL Configuration**: Flexible server/client configuration system
- **Deterministic Testing**: Fixed seeds for reproducible game scenarios

### Technology Stack
- **TUI**: charmbracelet/bubbletea, lipgloss, bubbles
- **WebSocket**: gorilla/websocket
- **CLI**: alecthomas/kong
- **Config**: hashicorp/hcl/v2
- **Testing**: stretchr/testify + gotestsum
- **Build**: Taskfile (Task) + Hermit for tools

## Development Guidelines

### Code Requirements
- **Go version**: 1.24
- **Module**: github.com/lox/pokerforbots
- **Formatting**: Use `go fmt` and `goimports` (automated via task fmt)
- **Linting**: Must pass `golangci-lint run` (automated via task lint)
- **Testing**: All changes must include tests and pass existing test suite

### Commit Standards
- **Format**: Conventional commits ONLY
- **Prefixes**: `feat:`, `fix:`, `chore:` or area-specific like `fix(server):`, `feat(client):`
- **Pre-commit hooks**: Automatically format, lint, and validate commit messages

### Testing Approach
- **Unit Tests**: `*_test.go` files throughout packages
- **Integration Tests**: `internal/testing/` directory with end-to-end scenarios
- **Deterministic**: Use fixed seeds for reproducible game testing
- **Test Infrastructure**: Custom test server and client setup utilities

### Key Implementation Details
- **Game IDs**: UUIDv7 with base32 encoding (26 chars, TypeID-compatible)
- **Poker Rules**: 6-max $1/$2 No Limit Hold'em implementation
- **TUI Design**: Two-pane layout with game log and action interface
- **Bot Architecture**: Pluggable AI strategies with range-based decisions
- **WebSocket Messages**: JSON format with request/response correlation IDs

## Configuration Files
- **holdem-server.hcl**: Server settings, tables, bot configuration
- **holdem-client.hcl**: Client preferences and UI settings
- **Taskfile.yml**: Build automation and development tasks
- **Docker**: Complete containerized setup in scripts/

## Verification Workflow

Always run the complete verification cycle after making changes:
1. Format: `./bin/task fmt`
2. Test: `./bin/task test`
3. Lint: `./bin/task lint`
4. Build: `./bin/task build`

Or use the combined command: `./bin/task check`
