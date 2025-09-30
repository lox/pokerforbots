# Tutorial: Build Your First Poker Bot

This tutorial walks you through building a simple poker bot that connects to the PokerForBots server.

## Prerequisites

- Go 1.21 or later
- PokerForBots server running (or use `pokerforbots spawn`)

## Quick Start

The simplest bot just needs to handle action requests:

```go
package main

import (
    "context"
    "github.com/lox/pokerforbots/protocol"
    "github.com/lox/pokerforbots/sdk/client"
    "github.com/rs/zerolog"
    "os"
)

// MyBot handles game events
type MyBot struct{}

// Required: Handle action requests (choose what to do)
func (MyBot) OnActionRequest(state *client.GameState, req protocol.ActionRequest) (string, int, error) {
    // Simple strategy: always check or call
    for _, action := range req.ValidActions {
        if action == "check" {
            return "check", 0, nil
        }
        if action == "call" {
            return "call", 0, nil
        }
    }
    return "fold", 0, nil
}

// Other handlers (can be no-ops)
func (MyBot) OnHandStart(*client.GameState, protocol.HandStart) error         { return nil }
func (MyBot) OnGameUpdate(*client.GameState, protocol.GameUpdate) error       { return nil }
func (MyBot) OnPlayerAction(*client.GameState, protocol.PlayerAction) error   { return nil }
func (MyBot) OnStreetChange(*client.GameState, protocol.StreetChange) error   { return nil }
func (MyBot) OnHandResult(*client.GameState, protocol.HandResult) error       { return nil }
func (MyBot) OnGameCompleted(*client.GameState, protocol.GameCompleted) error { return nil }

func main() {
    logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
    bot := client.New("my-bot", MyBot{}, logger)

    if err := bot.Connect("ws://localhost:8080/ws"); err != nil {
        logger.Fatal().Err(err).Msg("connect failed")
    }

    ctx := context.Background()
    if err := bot.Run(ctx); err != nil {
        logger.Error().Err(err).Msg("run failed")
    }
}
```

Save as `my-bot/main.go`, then:

```bash
go mod init my-bot
go mod tidy
go build

# Test against built-in bots
pokerforbots spawn --spec "calling-station:5" --bot-cmd "./my-bot" --hand-limit 100
```

## Understanding the Interface

### Handler Methods

Your bot must implement these methods:

```go
type Handler interface {
    OnHandStart(*GameState, protocol.HandStart) error
    OnActionRequest(*GameState, protocol.ActionRequest) (string, int, error)
    OnGameUpdate(*GameState, protocol.GameUpdate) error
    OnPlayerAction(*GameState, protocol.PlayerAction) error
    OnStreetChange(*GameState, protocol.StreetChange) error
    OnHandResult(*GameState, protocol.HandResult) error
    OnGameCompleted(*GameState, protocol.GameCompleted) error
}
```

**Most important**: `OnActionRequest` - where you decide what to do.

### Action Requests

When it's your turn, you receive:

```go
type ActionRequest struct {
    ValidActions []string  // e.g., ["check", "fold"] or ["call", "raise", "fold"]
    ToCall       int       // Amount needed to call
    MinBet       int       // Minimum bet amount (if betting available)
    MinRaise     int       // Minimum raise amount (if raising available)
    Pot          int       // Current pot size
}
```

Return: `(action string, amount int, error)`

- `action`: One of `ValidActions`
- `amount`: Bet/raise amount (0 for check/call/fold)

## Adding Strategy

### Access Game State

The `GameState` parameter gives you context:

```go
func (b *MyBot) OnActionRequest(state *client.GameState, req protocol.ActionRequest) (string, int, error) {
    // Your hole cards
    myChips := state.Chips

    // Simple strategy: only play if we have chips
    if myChips < req.ToCall {
        return "fold", 0, nil
    }

    // Call if cheap, fold if expensive
    if req.ToCall < req.Pot / 3 {
        return "call", 0, nil
    }
    return "fold", 0, nil
}
```

### Track Hand Information

Use `OnHandStart` to store hand-specific data:

```go
type MyBot struct {
    holeCards []string
}

func (b *MyBot) OnHandStart(state *client.GameState, start protocol.HandStart) error {
    b.holeCards = start.HoleCards
    return nil
}

func (b *MyBot) OnActionRequest(state *client.GameState, req protocol.ActionRequest) (string, int, error) {
    // Now you can use b.holeCards to make decisions
    // ...
}
```

### See the Board

Use `OnStreetChange`:

```go
type MyBot struct {
    board []string
}

func (b *MyBot) OnStreetChange(state *client.GameState, street protocol.StreetChange) error {
    b.board = street.Board  // Flop/turn/river cards
    return nil
}
```

## Example: Aggressive Bot

```go
type AggressiveBot struct {
    rng *rand.Rand
}

func (b *AggressiveBot) OnActionRequest(state *client.GameState, req protocol.ActionRequest) (string, int, error) {
    // Raise 60% of the time if possible
    if b.rng.Float64() < 0.6 {
        for _, action := range req.ValidActions {
            if action == "raise" {
                return "raise", req.MinRaise, nil
            }
            if action == "bet" {
                return "bet", req.MinBet, nil
            }
        }
    }

    // Otherwise call or check
    for _, action := range req.ValidActions {
        if action == "call" {
            return "call", 0, nil
        }
        if action == "check" {
            return "check", 0, nil
        }
    }

    return "fold", 0, nil
}
```

## Testing Your Bot

### Quick Test

```bash
# Build your bot
go build -o my-bot

# Test with spawn
pokerforbots spawn --spec "calling-station:5" --bot-cmd "./my-bot" --hand-limit 100
```

### Statistical Comparison

Compare two versions:

```bash
pokerforbots regression --mode heads-up --hands 10000 \
  --challenger "./my-bot-v2" \
  --baseline "./my-bot-v1"
```

## Next Steps

### Use the SDK

PokerForBots includes helper packages:

```go
import (
    "github.com/lox/pokerforbots/sdk/analysis"      // Equity calculations
    "github.com/lox/pokerforbots/sdk/classification" // Board texture analysis
)
```

See built-in bots in `sdk/bots/` for examples.

### Study Built-in Bots

- `calling-station` - Always calls/checks (simplest)
- `random` - Random valid actions
- `aggressive` - Raises often
- `complex` - Uses hand ranges and position (most complex)

View source: [sdk/bots/bots.go](../sdk/bots/bots.go)

### Read the Protocol

For advanced features: [WebSocket Protocol](websocket-protocol.md)

### Performance Tips

- Keep decision logic fast (100ms timeout)
- Avoid expensive calculations on every action
- Pre-compute lookup tables
- Use deterministic RNG for testing

## Common Patterns

### Pot Odds Calculation

```go
potOdds := float64(req.Pot + req.ToCall) / float64(req.ToCall)
// If potOdds > handEquity, call
```

### Position Awareness

```go
func (b *MyBot) OnHandStart(state *client.GameState, start protocol.HandStart) error {
    // Calculate position relative to button
    position := (state.Seat - start.Button + len(start.Players)) % len(start.Players)
    // Play tighter in early position
    return nil
}
```

### Opponent Tracking

```go
type MyBot struct {
    opponentActions map[int][]string  // Seat -> actions
}

func (b *MyBot) OnPlayerAction(state *client.GameState, action protocol.PlayerAction) error {
    if action.Seat != state.Seat {
        b.opponentActions[action.Seat] = append(
            b.opponentActions[action.Seat],
            action.Action,
        )
    }
    return nil
}
```

## Troubleshooting

### Bot Disconnects Immediately

Check that you're implementing all required Handler methods.

### Bot Times Out

Your `OnActionRequest` is taking too long. Simplify logic or pre-compute.

### Bot Folds Too Much

You're returning "fold" when check/call would be better. Check `ValidActions` first.

### Can't Parse Messages

Make sure you're using the SDK's `client.New()` - it handles protocol automatically.

## Resources

- [Go SDK Documentation](sdk.md)
- [WebSocket Protocol](websocket-protocol.md)
- [Built-in Bot Source](../sdk/bots/bots.go)
- [Example: Range Bot](../sdk/bots/bots.go) (search for "Range")

---

**Ready to compete?** Build your bot and test it with `pokerforbots spawn`!