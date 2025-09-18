# HTTP API Overview

PokerForBots keeps the WebSocket protocol focused on gameplay (`connect` + `action`).
Discovery and operational controls live behind simple HTTP endpoints that bots and
operators can call when needed.

## `GET /games`

Returns the list of configured game instances.

```json
[
  {
    "id": "default",
    "small_blind": 5,
    "big_blind": 10,
    "start_chips": 1000,
    "timeout_ms": 100,
    "min_players": 2,
    "max_players": 9,
    "require_player": true,
    "connected_bots": 0,
    "hands_played": 0
  }
]
```

**Typical flow**
1. Bot makes an HTTP request to `/games`.
2. Chooses the appropriate `id` for the desired table.
3. Opens a WebSocket connection and sends the usual `connect` message with the
   selected `game` and `role` (`player` or `npc`).

## `GET /stats`

Existing plaintext endpoint that surfaces aggregate server statistics (hands
completed, timeouts, etc.). Use this for quick health checks.

## Admin Endpoints

Mutating and inspection operations live under `/admin/*`:

- `POST /admin/games` – create a new game. Payload mirrors the `GET /games` fields.
- `GET /admin/games/{id}/stats` – return aggregate statistics for a specific game (hands played, bot performance, timeouts, etc.).
- `DELETE /admin/games/{id}` – remove an existing game (current hands are allowed to finish before the pool stops).

⚠️ **Authentication:** currently open for convenience; add shared-secret or mTLS before exposing outside trusted environments. (TODO)

Example payload:

```json
{
  "id": "sandbox",
  "small_blind": 5,
  "big_blind": 10,
  "start_chips": 1000,
  "timeout_ms": 100,
  "min_players": 2,
  "max_players": 6,
  "require_player": true,
  "hands": 500,
  "seed": 1337,
  "npcs": [
    {"strategy": "calling", "count": 2},
    {"strategy": "aggressive", "count": 1},
    {"strategy": "random", "count": 2}
  ]
}
```

- `hands` (optional) caps how many hands the game will run before idling.
- `seed` (optional) seeds the game-specific RNG so shuffles and seatings are reproducible.

Strategies supported for NPCs: `calling` (calling-station), `aggressive`, `random`. Count values of `0` are ignored.

### Example Stats Response

```
{
  "id": "sandbox",
  "small_blind": 5,
  "big_blind": 10,
  "start_chips": 1000,
  "timeout_ms": 100,
  "min_players": 2,
  "max_players": 6,
  "require_player": true,
  "seed": 1337,
  "hands_completed": 120,
  "hand_limit": 500,
  "hands_remaining": 380,
  "timeouts": 0,
  "hands_per_second": 1.8,
  "players": [
    {
      "bot_id": "bot-player-1234",
      "display_name": "complex",
      "role": "player",
      "hands": 120,
      "net_chips": 480,
      "avg_per_hand": 4.0,
      "total_won": 2800,
      "total_lost": 2320,
      "last_delta": 35,
      "last_updated": "2025-09-18T08:45:17Z"
    }
  ]
}
```

Keeping the WebSocket contract minimal lets bot authors plug into the system
without tracking additional message types, while operations teams can script or
curl the HTTP surfaces as needed.
