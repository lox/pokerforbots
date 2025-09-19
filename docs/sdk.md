# PokerForBots Go SDK

This SDK simplifies creating Go bots for the PokerForBots server by providing:

1. **Bot Framework** - Handles WebSocket connections and message routing
2. **Game State Management** - Tracks table state automatically  
3. **Built-in Strategies** - Common bot behaviors ready to use
4. **Clean Handler Interface** - Focus on decision logic, not protocol details

## Quick Start

```go
package main

import (
    "context"
    "github.com/lox/pokerforbots/sdk"
    "github.com/rs/zerolog"
)

func main() {
    logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
    
    // Implement your strategy
    strategy := &MyStrategy{}
    bot := sdk.New("my-bot", strategy, logger)
    
    // Connect and run
    bot.Connect("ws://localhost:8080/ws")
    bot.Run(context.Background())
}
```

## Architecture

### Bot Framework
- `Bot` - Main bot instance that manages connections and state
- `Handler` - Interface for implementing bot decision-making logic
- `GameState` - Current table state (pot, players, cards, etc.)

## Custom Strategy Example

```go
type MyStrategy struct {}

func (s *MyStrategy) OnActionRequest(state *sdk.GameState, req protocol.ActionRequest) (string, int, error) {
    // Your decision logic here
    if len(state.HoleCards) == 2 && state.HoleCards[0][0] == 'A' {
        return "raise", req.MinBet * 2, nil  // Raise with aces
    }
    return "call", 0, nil  // Otherwise call
}

// Implement other Handler methods...
```

## Examples

- `sdk/examples/random/` - Simple random bot using SDK
- `sdk/examples/complex/` - Advanced bot with statistics and opponent modeling

## Benefits over Raw Implementation

**Before SDK (200+ lines):**
```go
// Complex message handling, state tracking, connection management
func (b *bot) handle(data []byte) error {
    if b.tryHandStart(data) || b.tryGameUpdate(data) || /* ... */ {
        return nil
    }
    // ... lots of boilerplate
}
```

**With SDK (30 lines):**
```go
strategy := &MyStrategy{}
bot := sdk.New("my-bot", strategy, logger)
bot.Connect(serverURL)
bot.Run(context.Background())
```

The SDK reduces boilerplate by ~85% while providing better error handling and state management.
