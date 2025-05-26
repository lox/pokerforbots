# Agent Instructions

## Project Overview
This is a **Texas Hold'em CLI poker game** with AI opponents, built in Go. The game features:
- Professional CLI interface with dynamic prompts
- Position-aware AI players
- Complete poker hand progression (pre-flop → flop → turn → river → showdown)
- Clean architecture with separated presentation logic
- Test mode for automated gameplay

## Project Commands
- Run: `go run cmd/holdem/main.go [--players N]` or `./bin/task run`
- Tests: `go test ./...` or `./bin/task test`
- Test single file: `go test -run TestName ./path/to/package`
- Lint: `golangci-lint run` or `./bin/task lint`
- Format: `go fmt ./...` or `./bin/task fmt`
- Clean: `./bin/task clean`
- All checks: `./bin/task check` (test + lint + build)

## Code Style Guidelines
- Language: Go (1.24.3)
- Formatting: Use `go fmt` and `goimports`
- Imports: Group std, external, local; use `goimports`
- Naming: CamelCase for exported, camelCase for unexported
- Types: Use interfaces, prefer composition over inheritance
- Error handling: Return errors explicitly, wrap with context

## Project Structure
- `cmd/holdem/` - Main application entry point
- `internal/game/` - Core game logic and game state
- `internal/deck/` - Card and deck implementation
- `internal/evaluator/` - Hand strength evaluation
- `internal/display/` - TUI presentation layer (Bubble Tea interface)
- `internal/gameid/` - UUIDv7-based game ID generation with TypeID encoding

## Architecture Notes
- **Display Logic**: All presentation code is in `internal/display/`
- **Game Flow**: Main game loop in `internal/game/` with clean separation of concerns
- **TUI Interface**: Uses Bubble Tea for two-pane design (game log + action interface)
- **CLI Interface**: Kong for argument parsing, Bubble Tea for interactive TUI
- **Styling**: Lipgloss for terminal styling within TUI components
- **Clean Architecture**: Presentation logic completely separated from game logic

## Development Notes
- Use Hermit for dependency management  
- Go module: github.com/lox/holdem-cli
- **Game IDs**: Random UUIDv7 IDs with base32 encoding (26 chars, TypeID-compatible)
- **Deterministic Testing**: Use `RandSource` interface for reproducible game ID generation in tests