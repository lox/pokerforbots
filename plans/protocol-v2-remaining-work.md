# Protocol v2 - Remaining Implementation Work

## Completed ✅
1. Added `protocol_version` field to Connect message
2. Updated Bot struct to store ProtocolVersion
3. Server now reads and validates protocol version on connect
4. Defaults to v2 if not specified

## TODO - Dual Normalization Paths

### 1. Update normalizeAction() to be version-aware

Current location: `internal/server/hand_runner.go:518`

Need to create two functions:
```go
// For protocol v2 (current implementation)
func normalizeActionV2(clientAction string, clientAmount int, player *game.Player, betting *game.BettingRound) (game.Action, int)

// For protocol v1 (legacy support)
func normalizeActionV1(clientAction string, clientAmount int, player *game.Player, betting *game.BettingRound) (game.Action, int)

// Wrapper that dispatches based on bot version
func (hr *HandRunner) normalizeAction(action protocol.Action, bot *Bot) (game.Action, int)
```

### 2. Update sendActionRequest() to convert valid_actions based on protocol version

Current location: `internal/server/hand_runner.go:374`

Need helper function:
```go
func convertActionsToV1(actions []game.Action) []string {
    // Convert Call -> Check when it appears for checking
    // Keep other actions the same
    var result []string
    for _, action := range actions {
        switch action {
        case game.Call:
            // In v1, this becomes either "check" or "call" depending on context
            // For valid_actions, include both to be safe
            result = append(result, "call")
        case game.Raise:
            result = append(result, "raise")
        case game.Fold:
            result = append(result, "fold")
        case game.AllIn:
            result = append(result, "allin")
        case game.Check:
            result = append(result, "check")
        }
    }
    return result
}
```

Actually, this is tricky because GetValidActions now only returns v2 actions. We need to either:
- Keep the old GetValidActions logic for v1
- Or convert v2 actions back to v1 format when sending

**Simpler approach**: Convert at send time based on to_call:
```go
func convertActionsForProtocol(actions []game.Action, toCall int, version string) []string {
    if version == "2" {
        // Return as-is (fold, call, raise, allin)
        return actionsToStrings(actions)
    }

    // Version 1: convert call to check/call based on context
    var result []string
    for _, action := range actions {
        switch action {
        case game.Call:
            if toCall == 0 {
                result = append(result, "check")
            } else {
                result = append(result, "call")
            }
        default:
            result = append(result, action.String())
        }
    }
    return result
}
```

### 3. Update convertAction() to use bot's protocol version

Current location: `internal/server/hand_runner.go:559`

```go
func (hr *HandRunner) convertAction(action protocol.Action) (game.Action, int) {
    seat := hr.handState.ActivePlayer
    player := hr.handState.Players[seat]
    bot := hr.bots[seat]

    if bot.ProtocolVersion == "1" {
        return normalizeActionV1(action.Action, action.Amount, player, hr.handState.Betting)
    }
    return normalizeActionV2(action.Action, action.Amount, player, hr.handState.Betting)
}
```

### 4. Implement normalizeActionV1

This should accept the old action vocabulary:
- "check" → game.Check
- "bet" → game.Raise (with amount)
- "call" → game.Call
- "raise" → game.Raise (with amount)
- "fold" → game.Fold
- "allin" → game.AllIn

No normalization needed - just direct mapping.

### 5. Add tests for version negotiation

Test cases:
- Bot connecting with `protocol_version: "1"` gets v1 behavior
- Bot connecting with `protocol_version: "2"` gets v2 behavior
- Bot connecting with no version gets v2 behavior (default)
- Bot connecting with invalid version gets v2 behavior (fallback)
- Mixed game: v1 and v2 bots in same hand work correctly

### 6. Update all internal bots to send protocol_version: "2"

Files to update:
- sdk/client/client.go - Update Connect message
- All bots already use v2 actions, just need to declare version

### 7. Update CLI client

- Send `protocol_version: "2"` in connect
- Update keyboard shortcuts if needed

### 8. Documentation

Update `docs/websocket-protocol.md`:
- Document `protocol_version` field in Connect message
- Explain version negotiation
- Add migration guide showing differences between v1 and v2
- Document deprecation timeline for v1

## Testing Plan

1. Unit tests for version negotiation
2. Integration test with v1 bot
3. Integration test with v2 bot
4. Integration test with mixed v1/v2 bots in same game
5. Regression test: 10k hands with v2 bots
6. Regression test: 10k hands with v1 bots (if we want to verify compatibility)

## Migration Timeline

Suggested approach:
- v1.1 (current): Add version negotiation, support both v1 and v2
- v1.2 (2-4 weeks): Log warnings for v1 bots
- v1.3 (2-3 months): Remove v1 support, v2 only

For now, since we control all bots and Aragorn is aligned:
- Just default to v2
- Keep v1 support for safety during transition
- Can remove v1 in next release if not needed
