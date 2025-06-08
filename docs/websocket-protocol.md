# WebSocket Protocol for Holdem Client/Server

## Message Format

All messages are JSON with this structure:
```json
{
  "type": "message_type",
  "data": { /* message-specific payload */ },
  "timestamp": "2025-01-06T10:30:00Z",
  "requestId": "optional-correlation-id"
}
```

## Client → Server Messages

### Authentication
```json
{
  "type": "auth",
  "data": {
    "playerName": "string",
    "token": "optional-auth-token"
  }
}
```

### Table Operations
```json
{
  "type": "join_table",
  "data": {
    "tableId": "string",
    "seatNumber": "optional-number",
    "buyIn": "number"
  }
}

{
  "type": "leave_table",
  "data": {
    "tableId": "string"
  }
}

{
  "type": "list_tables",
  "data": {}
}
```

### Game Actions
```json
{
  "type": "player_decision",
  "data": {
    "tableId": "string",
    "action": "fold|call|check|raise|allin",
    "amount": "number-for-raises",
    "reasoning": "string"
  }
}
```

## Server → Client Messages

### Connection Status
```json
{
  "type": "auth_response",
  "data": {
    "success": true,
    "playerId": "string",
    "error": "optional-error-message"
  }
}

{
  "type": "error",
  "data": {
    "code": "error_code",
    "message": "human-readable-error"
  }
}
```

### Table Information
```json
{
  "type": "table_list",
  "data": {
    "tables": [
      {
        "id": "string",
        "name": "string",
        "playerCount": "number",
        "maxPlayers": "number",
        "stakes": "1/2",
        "status": "waiting|active"
      }
    ]
  }
}

{
  "type": "table_joined",
  "data": {
    "tableId": "string",
    "seatNumber": "number",
    "players": [ /* player list */ ]
  }
}
```

### Game Events (Mirror existing EventBus events)
```json
{
  "type": "hand_start",
  "data": {
    "handId": "string",
    "players": [
      {
        "name": "string",
        "chips": "number",
        "position": "string",
        "seatNumber": "number",
        "holeCards": ["As", "Kh"] // Only for acting player
      }
    ],
    "smallBlind": "number",
    "bigBlind": "number",
    "initialPot": "number"
  }
}

{
  "type": "player_action",
  "data": {
    "player": "string",
    "action": "fold|call|check|raise|allin",
    "amount": "number",
    "potAfter": "number",
    "round": "preflop|flop|turn|river",
    "reasoning": "string"
  }
}

{
  "type": "street_change",
  "data": {
    "round": "flop|turn|river|showdown",
    "communityCards": ["As", "Kh", "Qd"],
    "currentBet": "number"
  }
}

{
  "type": "hand_end",
  "data": {
    "handId": "string",
    "winners": [
      {
        "playerName": "string",
        "amount": "number",
        "handRank": "string",
        "holeCards": ["As", "Kh"]
      }
    ],
    "potSize": "number",
    "showdownType": "fold|showdown",
    "finalBoard": ["As", "Kh", "Qd", "Jc", "Ts"],
    "summary": "formatted-hand-summary"
  }
}

{
  "type": "player_timeout",
  "data": {
    "tableId": "string",
    "playerName": "string",
    "timeoutSeconds": "number",
    "action": "fold|check"
  }
}

{
  "type": "player_left",
  "data": {
    "tableId": "string",
    "playerName": "string",
    "reason": "disconnected|quit"
  }
}

{
  "type": "table_left",
  "data": {
    "tableId": "string"
  }
}

{
  "type": "bot_added",
  "data": {
    "tableId": "string",
    "botName": "string",
    "seatNumber": "number"
  }
}

{
  "type": "bot_kicked",
  "data": {
    "tableId": "string",
    "botName": "string"
  }
}
```

### Action Requests
```json
{
  "type": "action_required",
  "data": {
    "tableId": "string",
    "playerName": "string",
    "validActions": [
      {
        "action": "call",
        "minAmount": "number",
        "maxAmount": "number"
      }
    ],
    "tableState": {
      "currentBet": "number",
      "pot": "number",
      "currentRound": "preflop|flop|turn|river",
      "communityCards": ["As", "Kh"],
      "players": [ /* full player state */ ]
    },
    "timeoutSeconds": "number"
  }
}
```

## Error Handling

### Connection Errors
- `connection_lost`: Server will buffer events during reconnection
- `invalid_session`: Client must re-authenticate
- `table_full`: Join request rejected

### Game Errors
- `invalid_action`: Action not in validActions list
- `action_timeout`: Player took too long to act
- `insufficient_chips`: Not enough chips for action

## Security Considerations

1. **Authentication**: JWT tokens for player identity
2. **Action Validation**: Server validates all actions against game rules
3. **Rate Limiting**: Prevent message spam
4. **Reconnection**: Handle dropped connections gracefully
