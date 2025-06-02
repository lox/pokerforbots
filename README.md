# Poker for Bots

A command-line Texas Hold'em poker game that simulates a PokerStars-style experience with AI opponents, plus a poker odds calculator tool.

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

### Texas Hold'em Game

```bash
# Normal gameplay
go run cmd/holdem/main.go

# Test mode (automated decisions for testing)
go run cmd/holdem/main.go --test-mode

# Specify table size
go run cmd/holdem/main.go --players 9
```

### Poker Odds Calculator

```bash
# Calculate odds between two hands
go run cmd/poker-odds/main.go "AcKh" "KdQs"

# With community board
go run cmd/poker-odds/main.go "AcKh" "KdQs" --board "Td7s8h"

# Show detailed hand type probabilities
go run cmd/poker-odds/main.go "AcKh" "KdQs" --board "Td7s8h" --possibilities

# Multiple hands
go run cmd/poker-odds/main.go "AcKh" "KdQs" "2h2c" "TsJs"

# High accuracy with more iterations
go run cmd/poker-odds/main.go "AcKh" "KdQs" --iterations 1000000
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

## Poker Odds Calculator

The `poker-odds` tool calculates win probabilities for Texas Hold'em hands using Monte Carlo simulation or exhaustive calculation. It supports:

- **Multiple hands**: Compare any number of hands against each other
- **Board cards**: Specify 0-5 community cards using standard notation (e.g., "Td7s8h")
- **Hand probabilities**: Show detailed breakdown of hand types with `--possibilities`
- **High accuracy**: Configurable iterations with `--iterations N` for precision control
- **Fast calculation**: Optimized Monte Carlo simulation using existing evaluator
- **Reproducible results**: Use `--seed N` for deterministic output

### Card Notation

Cards use standard poker notation:
- **Ranks**: A (Ace), K (King), Q (Queen), J (Jack), T (Ten), 9, 8, 7, 6, 5, 4, 3, 2
- **Suits**: s (spades), h (hearts), d (diamonds), c (clubs)
- **Examples**: "AcKh" (Ace of clubs, King of hearts), "2s2d" (pocket deuces)

### Output

```
hand        win    tie
A♣ K♥   71.4%   1.1%
K♦ Q♠   27.5%   1.1%

100000 iterations in 54ms
```

## Project Structure

```
cmd/holdem/          # Main application entry point
cmd/poker-odds/      # Poker odds calculator tool
internal/game/       # Core game logic and display system
internal/deck/       # Card and deck implementation
internal/evaluator/  # Hand strength evaluation and equity calculation
internal/bot/        # AI agents
internal/tui/        # Terminal UI components
```

## Development

See [TODO.md](TODO.md) for planned features and current development status.

### Commands

- Test: `go test ./...`
- Run: `go run ./cmd/holdem`