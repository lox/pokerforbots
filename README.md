# Texas Hold'em CLI

A command-line Texas Hold'em poker game that simulates a PokerStars-style experience with AI opponents.

## Features

- **Professional CLI Interface**: Dynamic prompt showing your hand, position, and chips
- **Complete Hand Narrative**: Full poker hand progression from deal to showdown with clear betting rounds
- **Intelligent AI opponents**: Computer players with position-aware decision-making and hand evaluation
- **Interactive gameplay**: Full command system (call, raise, fold, quit, stats, help, hand, pot, players)
- **Flexible table sizes**: Support for 6-seat and 9-seat tables
- **Beautiful styling**: Lipgloss-powered visual design with colored cards and clear action indicators
- **Complete showdowns**: See all player hands, winners, and hand rankings at the end
- **Authentic poker flow**: Community card reveals, betting round transitions, and pot tracking
- **Test mode**: Automated gameplay for testing with `--test-mode` flag
- **Clean architecture**: Separated presentation logic for maintainable code
- **Fixed stakes**: $1/$2 No Limit Hold'em

## Quick Start

```bash
# Normal gameplay
go run cmd/holdem/main.go

# Test mode (automated decisions for testing)
go run cmd/holdem/main.go --test-mode

# Specify table size
go run cmd/holdem/main.go 9
```

## Gameplay

- Choose table size (6 or 9 seats)
- Players start with $200 stacks  
- Small blind: $1, Big blind: $2
- Use commands like `call`, `raise 50`, `fold`, `quit`, `stats`
- Full command history and tab completion
- Beautiful card visualization with colors
- Complete hand progression: Pre-flop → Flop → Turn → River → Showdown
- See all players' hole cards and hand rankings at showdown
- Clear betting round transitions and pot tracking

## Game Rules

- Standard Texas Hold'em rules
- No Limit betting structure
- Blinds remain static throughout session
- Bankrolls are ephemeral (reset each session)

## Project Structure

```
cmd/holdem/          # Main application entry point
internal/game/       # Core game logic and display system
internal/deck/       # Card and deck implementation
internal/player/     # Player types (human/AI)
internal/evaluator/  # Hand strength evaluation
```

## Development

See [TODO.md](TODO.md) for planned features and current development status.

### Commands

- Build: `go build`
- Test: `go test ./...`
- Run: `go run cmd/holdem/main.go`
- Test mode: `go run cmd/holdem/main.go -t`