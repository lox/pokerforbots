# WebSocket Protocol for PokerForBots

## Overview

Binary protocol using msgpack for efficient bot-to-server communication. Optimized for ultra-low latency with sub-100ms decision timeouts.

## Transport

- WebSocket with binary frames
- msgpack encoding (libraries available for all languages)
- No authentication required (bots identify by name only)
- No reconnection support (simplicity over resilience)

## Message Format

All messages are msgpack-encoded with minimal fields for speed.

## Client → Server Messages

### Connect
Sent immediately after WebSocket connection established.
```
{
  "type": "connect",
  "name": "BotName"        // Bot identifier (max 32 chars)
}
```

### Action
Response to action_request from server. Must be sent within timeout window.
```
{
  "type": "action",
  "action": "fold|call|check|raise|allin",
  "amount": 0              // Only required for raise (0 for other actions)
}
```

## Server → Client Messages

### Hand Start
Sent when a new hand begins. Bot receives hole cards and game setup.
```
{
  "type": "hand_start",
  "hand_id": 12345,         // Unique hand identifier
  "hole_cards": ["As", "Kh"], // Your two cards (string format)
  "seat": 2,                // Your seat position (0-8)
  "button": 0,               // Button seat position
  "players": [               // All players in hand
    {"seat": 0, "name": "Bot1", "chips": 1000},
    {"seat": 2, "name": "YourBot", "chips": 1000},
    {"seat": 4, "name": "Bot3", "chips": 1000}
  ],
  "small_blind": 5,          // Small blind amount
  "big_blind": 10            // Big blind amount
}
```

### Action Request
Server requests action from bot. Must respond within timeout_ms.
```
{
  "type": "action_request",
  "timeout_ms": 100,         // Milliseconds to respond
  "valid_actions": ["fold", "call", "raise"],  // Legal actions
  "to_call": 20,             // Amount to call (0 if can check)
  "min_raise": 40,           // Minimum raise amount
  "max_raise": 990,          // Maximum raise (your stack)
  "pot": 35,                 // Current pot size
  "board": ["Ah", "Kd", "7c"], // Community cards (0-5 cards)
  "current_bet": 20          // Current bet to match
}
```

### Game Update
Broadcast when any player acts. Lets bots track game state.
```
{
  "type": "game_update",
  "seat": 0,                 // Acting player's seat
  "action": "raise",         // Action taken
  "amount": 50,              // Amount (0 for fold/check)
  "pot": 85,                 // New pot size
  "street": "flop"           // Current street
}
```

### Street Change
Sent when moving to next betting round.
```
{
  "type": "street_change",
  "street": "flop",          // New street: flop|turn|river
  "board": ["Ah", "Kd", "7c"], // All community cards
  "pot": 100                 // Current pot
}
```

### Hand Result
Sent at hand completion with winner(s) and final state.
```
{
  "type": "hand_result",
  "winners": [
    {
      "seat": 2,
      "amount": 200,         // Amount won
      "hand_rank": "Two Pair", // Hand description
      "hole_cards": ["As", "Kh"] // Winner's cards (if showdown)
    }
  ],
  "board": ["Ah", "Kd", "7c", "2s", "9h"], // Final board
  "pot": 200,                // Final pot size
  "showdown": true           // Whether cards were shown
}
```

### Error
Sent when bot sends invalid message or action.
```
{
  "type": "error",
  "code": "invalid_action",
  "message": "Cannot raise less than minimum"
}
```

## Error Codes

- `invalid_action`: Action not in valid_actions list
- `action_timeout`: Failed to respond within timeout_ms
- `insufficient_chips`: Not enough chips for requested action
- `invalid_message`: Malformed msgpack or missing fields
- `not_your_turn`: Sent action when not requested

## Timeout Handling

- Bots must respond to `action_request` within `timeout_ms`
- On timeout, server automatically folds the bot's hand
- No warnings or grace period - optimize for speed
- Chronically slow bots may be disconnected

## Card Representation

Cards use string format: rank + suit
- Ranks: 2, 3, 4, 5, 6, 7, 8, 9, T, J, Q, K, A
- Suits: s (spades), h (hearts), d (diamonds), c (clubs)
- Examples: "As" (ace of spades), "Th" (ten of hearts)

## Game Rules

Follows standard No-Limit Texas Hold'em rules as specified in [poker-rules.md](poker-rules.md):
- Fixed blinds per hand (configured on server)
- All players start with same chip count
- Each hand is independent (no chip carryover)
- 2-9 players per hand (typically 2-6)
