# Agent Instructions

## Project Overview
This is a **client/server Texas Hold'em poker platform** designed for bot development and testing, built in Go. The system features:
- WebSocket-based multiplayer server supporting multiple concurrent tables
- Interactive CLI clients for human players
- Automated bot clients with configurable AI strategies
- Real-time gameplay with proper poker rules implementation
- HCL configuration system for flexible server and client setup
- Docker support for easy deployment and testing

## Project Commands

### Server Commands
- Run server: `./bin/holdem-server [--config holdem-server.hcl]`
- Server with flags: `./bin/holdem-server --addr=0.0.0.0:8080 --log-level=debug`

### Client Commands
- Interactive client: `./bin/holdem-client [--config holdem-client.hcl]`
- Client with flags: `./bin/holdem-client --server=ws://localhost:8080 --player=Alice`

### Docker Commands
- Complete test: `./scripts/docker-test.sh test`
- Start full system: `./scripts/docker-test.sh full`
- Interactive client: `./scripts/docker-test.sh client PlayerName`
- Show logs: `./scripts/docker-test.sh logs [service]`
- Clean up: `./scripts/docker-test.sh clean`

### Development Commands
- Tests: `go test ./...` or `./bin/task test`
- Test single file: `go test -run TestName ./path/to/package`
- Lint: `golangci-lint run` or `./bin/task lint`
- Format: `go fmt ./...` or `./bin/task fmt`
- Clean: `./bin/task clean`
- All checks: `./bin/task check` (test + lint + build)
- Pre-commit hooks: `pre-commit install` (run once), then hooks run automatically on commit
- Demo: `vhs demo.tape` to generate animated GIF demo of the TUI in demo.gif

### Poker Odds Calculator
- Help: `go run cmd/poker-odds/main.go --help`
- Basic usage: `go run cmd/poker-odds/main.go "AcKh" "KdQs"`

## Code Style Guidelines
- Language: Go (1.24.3)
- Formatting: Use `go fmt` and `goimports`
- Imports: Group std, external, local; use `goimports`
- Naming: CamelCase for exported, camelCase for unexported
- Types: Use interfaces, prefer composition over inheritance
- Error handling: Return errors explicitly, wrap with context
- Constants: Use `const` for constants, in CamelCase or camelCase style

## Project Structure
- `cmd/holdem-server/` - WebSocket game server
- `cmd/holdem-client/` - Interactive client application
- `cmd/poker-odds/` - Poker odds calculator tool
- `internal/game/` - Core game logic and game state
- `internal/deck/` - Card and deck implementation
- `internal/evaluator/` - Hand strength evaluation
- `internal/tui/` - TUI presentation layer (Bubble Tea interface)
- `internal/bot/` - AI agents
- `internal/server/` - WebSocket server and protocol
- `internal/client/` - Client networking and game interface

## Architecture Notes
- **TUI Logic**: All presentation code is in `internal/tui/`
- **Game Flow**: Main game loop in `internal/game/` with clean separation of concerns
- **TUI Interface**: Uses Bubble Tea for two-pane design (game log + action interface)
- **CLI Interface**: Kong for argument parsing, Bubble Tea for interactive TUI
- **Styling**: Lipgloss for terminal styling within TUI components
- **Clean Architecture**: Presentation logic completely separated from game logic

## Development Notes
- Use CashApp's Hermit for dependency management
- Go module: github.com/lox/pokerforbots
- **Game IDs**: Random UUIDv7 IDs with base32 encoding (26 chars, TypeID-compatible)
- **Deterministic Testing**: Use `rand.New` with fixed seed for reproducible game ID generation in tests
- **Commits**: Use conventional commits with prefixes `chore`, `feat`, `fix`, or area-specific like `fix(server)`, `feat(client)`, `chore(docs)`
- **Git History**: ~78% of commits now follow conventional format (57/73). Remaining historical commits use older formats but all new commits should be conventional

## Collaboration Guidelines

### Communication Style
- **Never start responses with effusive praise**: Don't use phrases like "You're absolutely right!", "Excellent idea!", "Great suggestion!", or "Perfect!" unless genuinely warranted
- **Provide analytical feedback**: When user suggests changes, analyze tradeoffs, potential issues, and alternatives rather than automatic agreement. Remember that users sometimes make errors themselves
- **State hypotheses, not conclusions**: When debugging, say "I have a hypothesis that..." or "This suggests..." rather than "I found the bug!" or "The issue is..." until confirmed through testing
- **Skip unnecessary preamble**: Avoid phrases like "Here's what I'll do", "Let me help you", or "Based on your request" - respond directly to the user's needs

### Implementation Process
- **MANDATORY: Seek input on alternatives**: When proposing multiple implementation approaches (e.g., "Here are some options..."), STOP and ask for user preference before writing any code
- **MANDATORY: Complete the verification cycle**: After making changes, ALWAYS run `./bin/task check` (or equivalent tests + lint) before considering work complete. Never declare completion without verification
- **Boy Scout principle**: Proactively suggest how changes can unify with existing architecture patterns and leave code cleaner than found
- **Flag broad impact changes**: When avoiding wide-reaching changes in favor of localized workarounds, STOP and explicitly seek user guidance on approach before implementing
- **Tool usage workflow**: Use tools concurrently when possible, search extensively before making changes, always verify understanding before implementation
