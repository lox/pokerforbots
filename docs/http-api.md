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

Mutating operations live under `/admin/*`:

- `POST /admin/games` – create a new game. Payload mirrors the `GET /games` fields.
- `DELETE /admin/games/{id}` – remove an existing game (current hands are allowed to finish before the pool stops).

⚠️ **Authentication:** currently open for convenience; add shared-secret or mTLS before exposing outside trusted environments. (TODO)

Keeping the WebSocket contract minimal lets bot authors plug into the system
without tracking additional message types, while operations teams can script or
curl the HTTP surfaces as needed.
