# Agent Instructions

## Project Overview
This is a **Texas Hold'em CLI poker game** with AI opponents, built in Go. The game features:
- Professional CLI interface with dynamic prompts
- Position-aware AI players
- Complete poker hand progression (pre-flop → flop → turn → river → showdown)
- Clean architecture with separated presentation logic
- Test mode for automated gameplay

For detailed progress and feature status, see [TODO.md](TODO.md).

## Project Commands
- Build: `go build` or `./bin/task build`
- Run: `go run cmd/holdem/main.go [--players N]` or `./bin/task run`
- Run test mode: `go run cmd/holdem/main.go --test-mode` or `./bin/task run-test`
- Test: `go test ./...` or `./bin/task test`
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
- `internal/game/` - Core game logic and display system
- `internal/deck/` - Card and deck implementation
- `internal/evaluator/` - Hand strength evaluation
- `internal/player/` - Player types (human/AI)

## Architecture Notes
- **Display Logic**: All presentation code is in `internal/game/display.go`
- **Game Flow**: Main game loop in `internal/game/` with clean separation of concerns
- **CLI Interface**: Uses Kong for argument parsing, readline for interactive input
- **Styling**: Lipgloss for terminal styling, consolidated in DisplayStyles struct
- **Testing**: Use `--test-mode` flag for automated gameplay testing

## Current Status
The game is **feature-complete** for core poker gameplay. See [TODO.md](TODO.md) for:
- ✅ Completed features (most core functionality done)
- 📋 Remaining enhancements (winner explanations, continuous play)
- 📅 Future features (statistics, tournaments, etc.)

## Development Notes
- Use Hermit for dependency management
- Go module: github.com/lox/holdem-cli