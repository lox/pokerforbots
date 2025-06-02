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
- Demo: `vhs demo.tape` to generate animated GIF demo of the TUI in demo.gif

## Code Style Guidelines
- Language: Go (1.24.3)
- Formatting: Use `go fmt` and `goimports`
- Imports: Group std, external, local; use `goimports`
- Naming: CamelCase for exported, camelCase for unexported
- Types: Use interfaces, prefer composition over inheritance
- Error handling: Return errors explicitly, wrap with context
- Constants: Use `const` for constants, in CamelCase or camelCase style

## Project Structure
- `cmd/holdem/` - Main application entry point
- `internal/game/` - Core game logic and game state
- `internal/deck/` - Card and deck implementation
- `internal/evaluator/` - Hand strength evaluation
- `internal/tui/` - TUI presentation layer (Bubble Tea interface)
- `internal/bot/` - AI agents

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
