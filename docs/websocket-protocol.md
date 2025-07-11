# WebSocket Protocol for Holdem Client/Server

## JSON Field Naming Convention

**Standard: camelCase**

All JSON fields use camelCase naming (e.g., `playerName`, `tableId`, `buyIn`, `validActions`).

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

### Game Events
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
        "holeCards": ["As", "Kh"]
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
```

## Schema Validation

### ValidAction Schema
```json
{
  "type": "object",
  "properties": {
    "action": {
      "type": "string",
      "enum": ["fold", "call", "check", "raise", "allin"]
    },
    "minAmount": {
      "type": "integer",
      "minimum": 0
    },
    "maxAmount": {
      "type": "integer",
      "minimum": 0
    }
  },
  "required": ["action"]
}
```

### TableState Schema
```json
{
  "type": "object",
  "properties": {
    "currentBet": {
      "type": "integer",
      "minimum": 0
    },
    "pot": {
      "type": "integer",
      "minimum": 0
    },
    "currentRound": {
      "type": "string",
      "enum": ["preflop", "flop", "turn", "river", "showdown"]
    },
    "communityCards": {
      "type": "array",
      "items": {
        "type": "string",
        "pattern": "^[2-9TJQKA][shdc]$"
      }
    },
    "players": {
      "type": "array",
      "items": {
        "$ref": "#/definitions/PlayerState"
      }
    }
  },
  "required": ["currentBet", "pot", "currentRound", "players"]
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

## Implementation Notes

1. **Field Naming**: All JSON fields use camelCase consistently
2. **Action Validation**: Server validates all actions against game rules
3. **Type Safety**: Use proper type conversion between string actions and enum types
4. **Error Handling**: Always validate JSON structure before processing
5. **Reconnection**: Handle dropped connections gracefully

## Schema Validation

### Available Schema Files

The system includes JSON Schema files for runtime validation:

- `sdk/schemas/message.json` - Base message format
- `sdk/schemas/auth.json` - Authentication messages
- `sdk/schemas/table.json` - Table operations
- `sdk/schemas/game.json` - Game actions and state
- `sdk/schemas/error.json` - Error messages

### Usage in Go

```go
import "github.com/lox/pokerforbots/sdk"

// Create validator
validator, err := sdk.NewValidator()
if err != nil {
    log.Fatal("Failed to create validator:", err)
}

// Validate a message
messageJSON := []byte(`{"type": "auth", "data": {"playerName": "TestBot"}}`)
if err := validator.ValidateMessage(messageJSON); err != nil {
    log.Printf("Invalid message: %v", err)
}
```

### Automatic Validation

Both the SDK client and server automatically validate messages:

- **SDK Client**: Validates incoming messages from server
- **Server**: Validates incoming messages from clients
- **Error Handling**: Invalid messages are logged and rejected with descriptive errors

### Validation Features

- **Type Checking**: Ensures correct data types (string, integer, boolean)
- **Required Fields**: Validates all required fields are present
- **Value Constraints**: Checks string patterns, number ranges, array lengths
- **Enum Validation**: Validates action types, rounds, error codes
- **Cross-Field Validation**: Conditional requirements (e.g., auth success requires playerId)

### Testing Validation

```bash
# Run validation tests
go test ./sdk

# Test SDK validation functionality
go test ./sdk -run TestValidator
```
