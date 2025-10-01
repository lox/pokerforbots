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
  "name": "BotName",          // Bot identifier (max 32 chars)
  "game": "default",          // Preferred game/table identifier (optional, defaults to server's default game)
  "auth_token": "...",        // (optional/TODO) Authentication credential
  "protocol_version": "2"     // Protocol version: "1" (legacy) or "2" (simplified, default)
}
```

If `game` is omitted the server will place the bot in the default game (until the lobby/listing flow ships). `auth_token` is ignored today but reserved for future authentication.

**Protocol Version**: The server supports two protocol versions for backwards compatibility:
- **Version 2** (default, recommended): Simplified 4-action protocol (`fold`, `call`, `raise`, `allin`). The server handles context-dependent normalization (e.g., `call` with `to_call=0` becomes `check` internally).
- **Version 1** (legacy): Full 6-action protocol (`fold`, `check`, `call`, `bet`, `raise`, `allin`). Deprecated but still supported for backward compatibility.

If `protocol_version` is omitted, the server defaults to version 2.

### Game Discovery & Selection *(TODO)*
Planned additions to allow bots to list, join, and leave named game instances:

- `list_games` (client → server): request current catalog.
- `game_list` (server → client): respond with an array of `{id, blinds, seats, description}`.
- `join_game` (client → server): ask to enter a specific game.
- `game_joined` (server → client): confirmation with final configuration.
- `leave_game` / `game_left`: explicit detachment back to lobby.

Until these messages are implemented, bots are matched in the single default game as described elsewhere in this document.

### Action
Response to action_request from server. Must be sent within timeout window.

**Protocol v2 (recommended)**:
```
{
  "type": "action",
  "action": "fold|call|raise|allin",
  "amount": 0              // For "raise" this is the *total* bet size (raise-to value). 0 for other actions.
}
```

**Protocol v1 (legacy)**:
```
{
  "type": "action",
  "action": "fold|check|call|bet|raise|allin",
  "amount": 0              // For "bet" or "raise" this is the *total* bet size. 0 for other actions.
}
```

Notes:
- **Protocol v2**: Simplified to 4 actions. Use `call` when you want to match the current bet (even when `to_call=0`). Use `raise` when you want to increase the bet (even when there's no prior bet).
- **Protocol v1**: Full 6 actions with explicit `check` (when `to_call=0`) and `bet` (when no prior bet exists).
- When sending `"raise"` or `"bet"`, set `amount` to the final total bet (call amount + raise increment). This mirrors the server's `player_bet` field.
- For `"allin"` the `amount` field is ignored; the server deduces the wager from the stack size.

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
Server asks the acting bot to choose an action.

```
{
  "type": "action_request",
  "hand_id": "hand-42",
  "time_remaining": 100,            // Milliseconds left before timeout
  "valid_actions": ["fold", "call", "raise"],
  "to_call": 20,                    // Chips required to match the current wager (0 if checking is allowed)
  "min_bet": 40,                    // Smallest legal total bet/raise size
  "min_raise": 20,                  // Minimum incremental chips beyond the call when raising
  "pot": 35                         // Pot size before acting
}
```

Field semantics:

- `to_call` – amount that must be invested to call. When `0`, checking is legal.
- `min_bet` – the smallest total bet the player may declare if they choose to bet or raise. When no bet exists this equals the big blind; otherwise it is the current highest bet plus the minimum raise increment.
- `min_raise` – the minimum *additional* chips that must be added beyond the call to make a legal raise. When `to_call == 0`, this matches the opening bet size.
- `valid_actions` – subset of legal actions based on protocol version:
  - **Protocol v2**: `fold`, `call`, `raise`, `allin` (simplified vocabulary)
  - **Protocol v1**: `fold`, `check`, `call`, `bet`, `raise`, `allin` (semantic vocabulary)
- `time_remaining` – deadline in milliseconds. Missing it causes the server to fold the hand automatically.

### Player Action
Broadcast immediately after every player action (including blind posts and auto-folds) so all bots can mirror wagering state.

```
{
  "type": "player_action",
  "hand_id": "hand-42",
  "street": "preflop",
  "seat": 3,
  "player_name": "Bot3",
  "action": "raise",                 // fold | check | call | bet | raise | allin | post_small_blind | post_big_blind | timeout_fold
  "amount_paid": 20,                  // Chips added during this action only
  "player_bet": 70,                   // Player's total committed bet after acting
  "player_chips": 930,                // Stack remaining
  "pot": 120                          // Pot size after acting
}
```

Action vocabulary:

- `bet` – first voluntary wager on the street. `player_bet` equals the bet size.
- `raise` – increase after a wager already exists. `player_bet` shows the new “to” amount.
- `allin` – the player’s entire stack went in. Treat it as a bet or raise based on whether a wager existed; short all-ins that do not meet the minimum raise still use `action = "allin"` and do **not** reopen betting.
- `post_small_blind`, `post_big_blind` – forced blinds at hand start.
- `timeout_fold` – server auto-folded the player due to timeout or disconnect.

### Game Update
Sent periodically to snapshot the full table state (e.g., after each action).

```
{
  "type": "game_update",
  "hand_id": "hand-42",
  "pot": 120,
  "players": [
    {"name": "Bot1", "chips": 930, "bet": 70, "folded": false, "all_in": false},
    {"name": "Bot2", "chips": 995, "bet": 10, "folded": false, "all_in": false},
    {"name": "Bot3", "chips": 1000, "bet": 0, "folded": true,  "all_in": false}
  ]
}
```

`bet` reflects the total chips each player has committed on the current street, `chips` is their remaining stack, and the boolean flags indicate folded/all-in status.

### Street Change
Sent when moving to next betting round.
```
{
  "type": "street_change",
  "hand_id": "hand-42",
  "street": "flop",          // New street: preflop|flop|turn|river
  "board": ["Ah", "Kd", "7c"] // All community cards dealt so far
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

### Game Completed
Broadcast exactly once when a game instance stops creating new hands (for example, when a configured hand limit is reached). Bots can treat this as the end of a simulation run and disconnect or request a fresh game.
```
{
  "type": "game_completed",
  "game_id": "sandbox",
  "hands_completed": 500,
  "hand_limit": 500,
  "reason": "hand_limit_reached",
  "seed": 1337,
  "players": [
    {
      "bot_id": "complex-3298",
      "display_name": "complex",
      "hands": 500,
      "net_chips": 12850,
      "avg_per_hand": 25.7,
      "total_won": 94210,
      "total_lost": 81360,
      "last_delta": 180,
      "detailed_stats": {  // Optional: only when server has --enable-stats
        "bb_100": 257.0,
        "mean": 25.7,
        "std_dev": 68.2,
        "win_rate": 22.4,
        "showdown_win_rate": 55.8,
        "position_stats": {
          "Button": {"hands": 125, "net_bb": 450.5, "bb_per_hand": 3.6},
          "Cutoff": {"hands": 125, "net_bb": 320.2, "bb_per_hand": 2.56},
          // ... other positions
        },
        "street_stats": {
          "preflop": {"hands_ended": 150, "net_bb": -45.0, "bb_per_hand": -0.3},
          "river": {"hands_ended": 200, "net_bb": 1330.0, "bb_per_hand": 6.65}
        },
        "hand_category_stats": {  // Only when stats_depth=full
          "Premium": {"hands": 25, "net_bb": 750.0, "bb_per_hand": 30.0},
          "Strong": {"hands": 45, "net_bb": 400.0, "bb_per_hand": 8.89},
          "Weak": {"hands": 280, "net_bb": -365.0, "bb_per_hand": -1.3}
        }
      }
    }
  ]
}
```

`reason` currently uses `hand_limit_reached`; additional values may appear as new shutdown triggers are implemented.

**DetailedStats fields** (when server runs with `--enable-stats`):
- `bb_100`: Big blinds won/lost per 100 hands
- `mean`: Average BB won/lost per hand
- `std_dev`: Standard deviation of results
- `win_rate`: Percentage of hands won
- `showdown_win_rate`: Win rate when reaching showdown
- `position_stats`: Performance breakdown by table position (BTN, CO, etc.)
- `street_stats`: Analysis of where hands end (preflop, flop, turn, river, showdown)
- `hand_category_stats`: Performance by hand strength (Premium, Strong, Medium, Weak)

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

## Connection Lifecycle

1. Bot establishes WebSocket connection
2. Server assigns internal bot ID
3. Bot sends Connect message with display name
4. Bot enters available pool
5. Bot plays hands until disconnection
6. Disconnection immediately folds the bot from any active hand and removes it from all queues
7. Reconnection requires a brand-new WebSocket session and `Connect` message; no in-hand recovery is attempted

Notes:
- The server does not support mid-hand reconnection. Every hand remains independent.
- Future work will add bot authentication so a reconnecting bot can reclaim its bankroll balance when rejoining the idle pool, but it still starts fresh for upcoming hands.

## Simulation Control *(TODO)*

To support deterministic testing without restarting the process, a privileged control channel will be added:

- `simulate` (client → server, auth required): describe a simulation session (`game_id`, `deck_seed`, `mirror_count`, `hands`, `bot_ids`).
- `simulation_update` (server → client): stream progress for each generated hand, including mirror index and aggregated chip deltas.
- `simulation_complete`: emit final statistics when the batch finishes.

These messages are currently unimplemented; the existing protocol is sufficient for single-game bot play.

## Card Representation

Cards use string format: rank + suit
- Ranks: 2, 3, 4, 5, 6, 7, 8, 9, T, J, Q, K, A
- Suits: s (spades), h (hearts), d (diamonds), c (clubs)
- Examples: "As" (ace of spades), "Th" (ten of hearts)

## Migration Guide: Protocol v1 → v2

**Why migrate?** Protocol v2 simplifies bot development by eliminating context-dependent action selection. Bots no longer need to track whether to send `check` vs `call` or `bet` vs `raise`.

**Breaking changes:**
- `valid_actions` now returns `call` instead of `check` (even when `to_call=0`)
- `valid_actions` now returns `raise` instead of `bet` (even when there's no prior bet)
- Server rejects `check` and `bet` actions from v2 bots (will auto-fold)

**Migration steps:**

1. **Update Connect message**: Add `"protocol_version": "2"` to your connect message
   ```diff
   {
     "type": "connect",
     "name": "MyBot",
   + "protocol_version": "2"
   }
   ```

2. **Simplify action logic**: Replace context-aware selection with simple mapping
   ```diff
   - action = "check" if to_call == 0 else "call"
   + action = "call"

   - action = "bet" if current_bet == 0 else "raise"
   + action = "raise"
   ```

3. **Update valid_actions parsing**: Expect simplified vocabulary
   ```diff
   - if "check" in valid_actions or "call" in valid_actions:
   + if "call" in valid_actions:

   - if "bet" in valid_actions or "raise" in valid_actions:
   + if "raise" in valid_actions:
   ```

**Backward compatibility:** The server still supports v1 bots (omit `protocol_version` or send `"1"`), but v2 is recommended for all new implementations.

## Game Rules

Follows standard No-Limit Texas Hold'em rules as specified in [poker-rules.md](poker-rules.md):
- Fixed blinds per hand (configured on server)
- All players start with same chip count
- Each hand is independent (no chip carryover)
- 2-9 players per hand (typically 2-6)
