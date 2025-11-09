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

## Message Catalog

**Client → Server**
- `connect`
- `action`

**Server → Client**
- `hand_start`
- `action_request`
- `player_action`
- `game_update`
- `street_change`
- `hand_result`
- `game_completed`
- `error`

> There is no dedicated `game_start` payload. Bots learn that a game is underway when the first `hand_start` arrives and they learn that it is over when `game_completed` is broadcast.

## Client → Server Messages

### Connect
Sent immediately after WebSocket connection established.
```
{
  "type": "connect",
  "name": "BotName",          // Bot identifier (max 32 chars)
  "game": "default",          // Preferred game/table identifier (optional, defaults to server's default game)
  "auth_token": "...",        // (optional/TODO) Authentication credential
  "protocol_version": "2"     // Protocol version: "1" (legacy, default) or "2" (simplified, recommended)
}
```

If `game` is omitted the server will place the bot in the default game (until the lobby/listing flow ships). `auth_token` is ignored today but reserved for future authentication.

**Protocol Version**: The server supports two protocol versions for backwards compatibility:
- **Version 2** (recommended): Simplified 4-action protocol (`fold`, `call`, `raise`, `allin`). The server handles context-dependent normalization (e.g., `call` with `to_call=0` becomes `check` internally).
- **Version 1** (default, legacy): Full 6-action protocol (`fold`, `check`, `call`, `bet`, `raise`, `allin`). Used for backward compatibility with existing bots.

If `protocol_version` is omitted (or any unsupported value is supplied) the server coerces the session to version 1 for backward compatibility. New bots should explicitly set `"protocol_version": "2"` so they can use the simplified action vocabulary.

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
  "hand_id": "hand-42",         // Unique hand identifier (string)
  "hole_cards": ["As", "Kh"],   // Always two cards
  "your_seat": 2,                // Your seat index (0-based)
  "button": 0,                   // Button seat index
  "players": [                   // All seats, including you
    {"seat": 0, "name": "bot-1", "chips": 1000},
    {"seat": 2, "name": "YourBot", "chips": 1000},
    {"seat": 4, "name": "bot-3", "chips": 1000}
  ],
  "small_blind": 5,
  "big_blind": 10
}
```

Fields:
- `players[].bet`, `players[].folded`, and `players[].all_in` are omitted at hand start (zero values) but appear in later updates once action has occurred.
- `name` is rendered from the observer's point of view – opponents appear as `bot-#` while your own seat uses your configured display name (see `internal/server/hand_runner.go` for the `displayName` logic).

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
- `time_remaining` – deadline in milliseconds. The value equals the server's configured timeout (it is not a live countdown). Missing it causes the server to fold the hand automatically.

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

`player_name` is also perspective-aware (self = configured display name, opponents = `bot-#`).

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

As with other broadcasts, the `name` field is rendered from each recipient's viewpoint.

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
  "hand_id": "hand-42",
  "winners": [
    {
      "name": "bot-1",
      "amount": 200,
      "hand_rank": "Two Pair, Aces and Kings",
      "hole_cards": ["As", "Kh"]
    }
  ],
  "board": ["Ah", "Kd", "7c", "2s", "9h"],
  "showdown": [              // Other hands that reached showdown but lost
    {
      "name": "bot-2",
      "hole_cards": ["Qd", "Qs"],
      "hand_rank": "Pair of Queens"
    }
  ]
}
```

`winners[].name` and `showdown[].name` are perspective-aware labels. `showdown` is omitted unless at least one losing player exposed cards at showdown.

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
      "timeouts": 3,
      "invalid_actions": 0,
      "disconnects": 0,
      "busts": 1,
      "detailed_stats": {  // Optional: only when server has --enable-stats
        "hands": 500,
        "net_bb": 2570.0,
        "bb_per_100": 257.0,
        "mean": 25.7,
        "median": 8.0,
        "std_dev": 68.2,
        "ci_95_low": -5.1,
        "ci_95_high": 56.5,
        "winning_hands": 112,
        "win_rate": 22.4,
        "showdown_wins": 62,
        "showdown_win_rate": 55.8,
        "non_showdown_wins": 50,
        "showdown_bb": 960.0,
        "non_showdown_bb": 1610.0,
        "max_pot_bb": 130.0,
        "big_pots": 12,
        "vpip": 34.2,
        "pfr": 21.6,
        "timeouts": 0,
        "busts": 1,
        "responses_tracked": 500,
        "avg_response_ms": 18.5,
        "p95_response_ms": 42.0,
        "position_stats": {
          "Button": {"hands": 125, "net_bb": 450.5, "bb_per_hand": 3.6},
          "Cutoff": {"hands": 125, "net_bb": 320.2, "bb_per_hand": 2.56}
        },
        "street_stats": {
          "preflop": {"hands_ended": 150, "net_bb": -45.0, "bb_per_hand": -0.3},
          "river": {"hands_ended": 200, "net_bb": 1330.0, "bb_per_hand": 6.65}
        },
        "hand_category_stats": {
          "Premium": {"hands": 25, "net_bb": 750.0, "bb_per_hand": 30.0},
          "Weak": {"hands": 280, "net_bb": -365.0, "bb_per_hand": -1.3}
        }
      }
    }
  ]
}
```

Each entry in `players` matches `protocol.GameCompletedPlayer` and summarizes per-bot aggregates (`hands`, `net_chips`, `avg_per_hand`, `total_won`, `total_lost`, `last_delta`, `timeouts`, `invalid_actions`, `disconnects`, `busts`, plus optional `detailed_stats`).

`reason` currently emits `hand_limit_reached`, but other reasons (admin stop, fatal error, etc.) may be added later. The `players` array is populated only when statistics collection is enabled; otherwise the list can be empty.

**DetailedStats fields** mirror `protocol.PlayerDetailedStats` and are grouped as follows when `--enable-stats` is active:
- Summary: `hands`, `net_bb`, `bb_per_100`, `mean`, `median`, `std_dev`, 95% confidence interval bounds.
- Win/Loss split: `winning_hands`, `win_rate`, `showdown_wins`, `non_showdown_wins`, `showdown_win_rate`, `showdown_bb`, `non_showdown_bb`.
- Pot metrics: `max_pot_bb`, `big_pots`.
- Preflop tendencies: `vpip`, `pfr`.
- Error/response tracking: `timeouts`, `busts`, `responses_tracked`, `avg_response_ms`, `p95_response_ms`, `max_response_ms`, `min_response_ms`, `response_std_ms`, `response_timeouts`, `response_disconnects`.
- Optional breakdowns (when stats depth allows): `position_stats`, `street_stats`, `hand_category_stats`.

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
