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
   selected `game`.

## `GET /stats`

Existing plaintext endpoint that surfaces aggregate server statistics (hands
completed, timeouts, etc.). Use this for quick health checks.

## Admin Endpoints

Mutating and inspection operations live under `/admin/*`:

- `POST /admin/games` – create a new game. Payload mirrors the `GET /games` fields.
- `GET /admin/games/{id}/stats` – JSON aggregate statistics for a specific game (hands played, per-bot performance, timeouts, etc.).
- `GET /admin/games/{id}/stats.txt` – human-readable plaintext summary per player (pretty format).
- `GET /admin/games/{id}/stats.md` – Markdown summary including game overview, leaderboard, aggregate position/street analysis, and per-player sections.
- `DELETE /admin/games/{id}` – remove an existing game (current hands are allowed to finish before the pool stops).

When detailed stats are enabled (`--collect-detailed-stats`), per-player objects in both `game_completed` and admin JSON include `detailed_stats` with BB/100, position, street and category breakdowns.

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
  "infinite_bankroll": false,
  "hands": 500,
  "seed": 1337
}
```

- `infinite_bankroll` (optional) when true, players never bust out and always have chips to continue playing.
- `hands` (optional) caps how many hands the game will run before idling.
- `seed` (optional) seeds the game-specific RNG so shuffles and seatings are reproducible.

Note: NPC spawning has been moved out of the server. To add bots to a game, use the `pokerforbots spawn` command or connect bots separately using the `pokerforbots bots` commands.

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
