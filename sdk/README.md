# PokerForBots SDK

The PokerForBots SDK provides a simple and powerful interface for developing poker bots that can connect to the PokerForBots server.

## Quick Start

```go
package main

import (
    "context"
    "log/slog"
    "os"

    "github.com/lox/pokerforbots/sdk"
    "github.com/lox/pokerforbots/sdk/examples/simple"
)

func main() {
    logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

    // Create a simple bot
    bot := simple.NewCallBot("MyBot")

    // Create a client
    client := sdk.NewBotClient("ws://localhost:8080/ws", "MyBot", bot, logger)

    // Connect and authenticate
    ctx := context.Background()
    if err := client.Connect(ctx, "MyBot"); err != nil {
        logger.Error("Failed to connect", "error", err)
        return
    }
    defer client.Disconnect()

    // Join a table
    if err := client.JoinTable("table-123", 200); err != nil {
        logger.Error("Failed to join table", "error", err)
        return
    }

    // Bot will automatically handle decisions
    select {} // Keep running
}
```

## Core Concepts

### Agent Interface

All bots must implement the `Agent` interface:

```go
type Agent interface {
    MakeDecision(tableState TableState, validActions []ValidAction) Decision
}
```

Your bot receives:
- **TableState**: Complete game state including players, cards, pot, betting round
- **ValidActions**: Legal actions your bot can take with min/max amounts

Your bot returns:
- **Decision**: The action to take (fold, call, check, raise, allin) with optional amount and reasoning

### Game State

The `TableState` provides all visible information:

```go
type TableState struct {
    CurrentBet      int           // Amount needed to call
    Pot             int           // Total pot size
    CurrentRound    string        // preflop, flop, turn, river
    CommunityCards  []Card        // Visible board cards
    Players         []PlayerState // All players and their states
    ActingPlayerIdx int           // Who needs to act
}
```

Key methods:
- `GetBotPlayer()` - Your bot's player state (includes hole cards)
- `GetActingPlayer()` - Player who needs to make a decision
- `GetActivePlayers()` - Players still in the hand
- `IsPreflop()`, `IsFlop()`, etc. - Check current betting round

### Hand Evaluation

The SDK includes poker hand evaluation utilities:

```go
evaluator := sdk.NewEvaluator()

// Evaluate hand strength
strength := evaluator.EvaluateHand(holeCards, communityCards)
fmt.Printf("Hand rank: %d, Class: %s, Percentile: %.1f%%\n",
    strength.Rank, strength.Class, strength.Percentile)

// Calculate win probability
equity := evaluator.CalculateEquity(holeCards, communityCards, 3, 10000)
fmt.Printf("Equity vs 3 opponents: %.1f%%\n", equity*100)

// Preflop hand classification
if evaluator.IsPremiumPreflop(holeCards) {
    fmt.Println("Premium starting hand!")
}
```

## Example Bots

### Simple Bots

Located in `examples/simple/`:

- **CallBot**: Always calls when possible, otherwise checks/folds
- **FoldBot**: Always folds unless it can check for free
- **RandomBot**: Makes random decisions from valid actions

```go
import "github.com/lox/pokerforbots/sdk/examples/simple"

bot := simple.NewCallBot("CallBot_1")
bot := simple.NewFoldBot("FoldBot_1")
bot := simple.NewRandomBot("RandomBot_1")
```

### Chart-Based Bot

Located in `examples/chart/`:

- **PreflopChartBot**: Uses preflop hand charts and post-flop hand strength

```go
import "github.com/lox/pokerforbots/sdk/examples/chart"

bot := chart.NewPreflopChartBot("ChartBot_1")
```

## Building Your Own Bot

### 1. Implement the Agent Interface

```go
type MyBot struct {
    name      string
    evaluator *sdk.Evaluator
}

func NewMyBot(name string) *MyBot {
    return &MyBot{
        name:      name,
        evaluator: sdk.NewEvaluator(),
    }
}

func (mb *MyBot) MakeDecision(tableState sdk.TableState, validActions []sdk.ValidAction) sdk.Decision {
    // Get your cards and position
    botPlayer := tableState.GetBotPlayer()
    if botPlayer == nil {
        return sdk.NewFoldDecision("Cannot find bot player")
    }

    // Analyze hand strength
    if tableState.IsPreflop() {
        if mb.evaluator.IsPremiumPreflop(botPlayer.HoleCards) {
            // Try to raise with premium hands
            for _, action := range validActions {
                if action.Action == sdk.ActionRaise {
                    return sdk.NewRaiseDecision(action.MinAmount, "Premium hand")
                }
            }
        }
    } else {
        // Post-flop: evaluate hand strength
        strength := mb.evaluator.EvaluateHand(botPlayer.HoleCards, tableState.CommunityCards)
        if strength.Percentile >= 80 {
            // Strong hand - bet for value
            for _, action := range validActions {
                if action.Action == sdk.ActionRaise {
                    betSize := tableState.Pot / 2 // Half pot bet
                    betSize = max(betSize, action.MinAmount)
                    betSize = min(betSize, action.MaxAmount)
                    return sdk.NewRaiseDecision(betSize, "Value bet")
                }
            }
        }
    }

    // Default: call or check
    for _, action := range validActions {
        if action.Action == sdk.ActionCall {
            return sdk.NewCallDecision("Default call")
        }
        if action.Action == sdk.ActionCheck {
            return sdk.NewCheckDecision("Default check")
        }
    }

    return sdk.NewFoldDecision("No good options")
}
```

### 2. Connect to Server

```go
func main() {
    logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
    bot := NewMyBot("MyBot")

    client := sdk.NewBotClient("ws://localhost:8080/ws", "MyBot", bot, logger)
    defer client.Disconnect()

    ctx := context.Background()
    if err := client.Connect(ctx, "MyBot"); err != nil {
        log.Fatal(err)
    }

    if err := client.JoinTable("table-123", 200); err != nil {
        log.Fatal(err)
    }

    // Keep running
    select {}
}
```

## Advanced Features

### Position Awareness

```go
botPlayer := tableState.GetBotPlayer()
switch botPlayer.Position {
case sdk.PositionButton:
    // Play wider range in position
case sdk.PositionSmallBlind, sdk.PositionBigBlind:
    // Tight play out of position
default:
    // Regular position play
}
```

### Opponent Modeling

```go
// Track opponent actions
for _, player := range tableState.Players {
    if player.LastAction == sdk.ActionRaise {
        // This player likes to raise
    }

    vpip := float64(player.TotalBet) / float64(player.Chips + player.TotalBet)
    if vpip > 0.3 {
        // Loose player
    }
}
```

### Pot Odds Calculation

```go
callAmount := tableState.CurrentBet - botPlayer.BetThisRound
potOdds := float64(callAmount) / float64(tableState.Pot + callAmount)

// Calculate hand equity
equity := evaluator.CalculateEquity(botPlayer.HoleCards, tableState.CommunityCards, numOpponents, 1000)

if equity > potOdds {
    // Profitable call
    return sdk.NewCallDecision("Positive expected value")
}
```

## Testing Your Bot

### Local Testing

```bash
# Start the server
./bin/holdem-server

# In another terminal, join your bot
go run your-bot/main.go
```

### Bot vs Bot Testing

```go
// Create multiple bots for testing
bots := []sdk.Agent{
    simple.NewCallBot("CallBot"),
    simple.NewRandomBot("RandomBot"),
    chart.NewPreflopChartBot("ChartBot"),
    NewMyBot("MyBot"),
}

// Connect each bot to the same table
for i, bot := range bots {
    client := sdk.NewBotClient(serverURL, fmt.Sprintf("Bot_%d", i), bot, logger)
    // ... connect and join table
}
```

## Constants and Utilities

### Actions
- `sdk.ActionFold`, `sdk.ActionCall`, `sdk.ActionCheck`
- `sdk.ActionRaise`, `sdk.ActionAllIn`

### Betting Rounds
- `sdk.RoundPreflop`, `sdk.RoundFlop`, `sdk.RoundTurn`, `sdk.RoundRiver`

### Card Ranks
- `sdk.RankAce`, `sdk.RankKing`, `sdk.RankQueen`, etc.

### Card Suits
- `sdk.SuitSpades`, `sdk.SuitHearts`, `sdk.SuitDiamonds`, `sdk.SuitClubs`

### Positions
- `sdk.PositionButton`, `sdk.PositionSmallBlind`, `sdk.PositionBigBlind`
- `sdk.PositionUTG`, `sdk.PositionCutoff`, etc.

## Error Handling

The SDK handles most connection and protocol errors automatically. Your bot should focus on decision-making logic and handle edge cases gracefully:

```go
func (bot *MyBot) MakeDecision(tableState sdk.TableState, validActions []sdk.ValidAction) sdk.Decision {
    // Always check for valid state
    botPlayer := tableState.GetBotPlayer()
    if botPlayer == nil {
        return sdk.NewFoldDecision("Invalid game state")
    }

    if len(validActions) == 0 {
        return sdk.NewFoldDecision("No valid actions")
    }

    // Your decision logic here...

    // Always have a fallback
    return sdk.NewFoldDecision("Fallback decision")
}
```

## Next Steps

1. Study the example bots to understand different strategies
2. Implement basic hand evaluation and position awareness
3. Add opponent modeling and betting pattern recognition
4. Test against other bots to refine your strategy
5. Consider advanced concepts like GTO (Game Theory Optimal) play

For more examples and advanced techniques, see the `examples/` directory and the main PokerForBots documentation.
