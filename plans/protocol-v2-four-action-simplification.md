# Protocol v2: Simplified 4-Action System

- Owner: @lox
- Status: in-progress
- Updated: 2025-09-30
- Tags: protocol, breaking-change, dx, bot-framework
- Scope: internal/server/hand_runner.go, internal/game/betting.go, internal/client/client.go, sdk/bots/*, docs/websocket-protocol.md
- Risk: medium
- Effort: M

## TL;DR

- Problem: Bots must reimplement context-dependent action name mapping (check vs call, bet vs raise, allin detection) causing catastrophic performance loss when wrong.
- Approach: Simplify protocol to 4 actions (fold, call, raise, allin); server interprets based on game state and logs semantically (check/bet/call/raise).
- Expected outcome: Bot developers focus on strategy rather than protocol bookkeeping; cleaner API with one-time breaking change during v1.0 window.

## Goals

- Simplify protocol to 4 actions: fold, call, raise, allin (remove check, bet from wire protocol)
- Update valid_actions to only send simplified vocabulary (never send check/bet)
- Server normalizes client actions to semantic names (check/bet/call/raise) for internal processing and logging
- Maintain semantic precision in logs, hand histories, and player_action broadcasts for analysis
- Update all internal bots and CLI client to use simplified protocol
- Update documentation to reflect v2 protocol
- Clean breaking change during v1.0 adoption window

## Non-Goals

- Modifying hand history analysis or logging format (logs continue showing semantic check/bet/call/raise)
- Adding protocol versioning negotiation (clean break, document migration)
- Changing other server-to-client messages (hand_start, player_action, etc. unchanged)

## Plan

- Step 1: Write failing tests for GetValidActions() returning simplified vocabulary
- Step 2: Update GetValidActions() in betting.go to emit simplified action vocabulary (call/raise instead of check/bet)
- Step 3: Write failing tests for normalizeAction() covering all edge cases
- Step 4: Implement normalizeAction() in hand_runner.go that maps client actions to semantic names
- Step 5: Update convertAction() to use normalizeAction() and handle semantic action names
- Step 6: Update all 4 internal bots to use simplified protocol (call/raise instead of check/bet)
- Step 7: Update CLI client validation and shortcuts to use simplified protocol
- Step 8: Update docs/websocket-protocol.md with v2 protocol and migration guide
- Step 9: Run full test suite and regression tests to verify correctness

## Acceptance Criteria

- GetValidActions() returns only simplified vocabulary: fold, call, raise, allin (never check or bet)
- normalizeAction() correctly maps: call+to_call=0→check, raise+to_call=0→bet, raise+amount>=stack→allin
- player_action broadcasts continue using semantic names (check/bet shown in logs and to other bots)
- All unit tests pass after updates
- All integration tests pass after updates
- CLI client can send simplified actions and validates against new valid_actions
- Regression test: pokerforbots spawn --hand-limit 10000 shows zero invalid actions
- Old action names ("check", "bet") explicitly rejected with clear error message
- Documentation includes migration guide for external bots (Aragorn)

## Risks & Mitigations

- Risk: Wire-level breaking change affects all external bots — Mitigation: Aragorn team already aligned; simple migration (find-replace check→call, bet→raise)
- Risk: valid_actions change breaks existing client-side validation — Mitigation: Update CLI client in same commit; comprehensive testing before merge
- Risk: Server complexity increase — Mitigation: Clean separation between wire protocol (4 actions) and game semantics (6 actions); Red→Green→Refactor with tests first

## Tasks

### ✅ Phase 1: Core Protocol Implementation (Completed)
- [x] Update plan to reflect v2 protocol breaking change and address Codex feedback
- [x] Write failing tests for GetValidActions() returning simplified vocabulary
- [x] Update GetValidActions() in internal/game/betting.go to pass tests
- [x] Write failing tests for normalizeAction() covering all edge cases
- [x] Implement normalizeAction() in internal/server/hand_runner.go to pass tests
- [x] Update convertAction() to use normalization and handle semantic actions
- [x] Update sdk/bots/callingstation to use call/raise
- [x] Update sdk/bots/random to use call/raise
- [x] Update sdk/bots/aggressive to use call/raise
- [x] Update sdk/bots/complex to use call/raise

### ✅ Phase 2: Version Negotiation Infrastructure (Completed)
- [x] Add protocol_version field to Connect message (protocol/messages.go)
- [x] Update Bot struct to store ProtocolVersion (internal/server/bot.go)
- [x] Server validates and stores protocol version on connect (internal/server/server.go)
- [x] Default to v2 if protocol_version omitted

### ✅ Phase 3: Dual Protocol Support (Completed)
- [x] Implement normalizeActionV1() for legacy bots
  - Accept old vocabulary: check, bet, call, raise, fold, allin
  - Direct 1:1 mapping to game.Action
  - Location: internal/server/hand_runner.go

- [x] Implement normalizeActionV2() (rename current normalizeAction)
  - Renamed normalizeAction to normalizeActionV2
  - Handles call→check, raise→bet normalization

- [x] Update convertAction() to dispatch based on bot.ProtocolVersion
  - Implemented version-based dispatch
  - v1 bots use normalizeActionV1
  - v2 bots use normalizeActionV2

- [x] Update sendActionRequest() to convert valid_actions by protocol version
  - Implemented convertActionsForProtocol helper
  - v2 bots receive simplified vocabulary (call/raise)
  - v1 bots receive semantic vocabulary (check/bet/call/raise)

- [x] Add tests for dual normalization
  - TestNormalizeActionProtocolV1 - tests v1 action handling
  - TestNormalizeActionProtocolV2 - tests v2 action handling
  - TestConvertActionsForProtocol - tests valid_actions conversion

### ✅ Phase 4: Documentation & Client Updates (Completed)
- [x] Update SDK client to send protocol_version: "2" (sdk/client/bot.go)
- [x] Update CLI client to use protocol v2 (internal/client/client.go)
  - Send protocol_version in connect
  - Updated keyboard shortcuts (k→call, b→raise for v2 compatibility)
- [x] Update docs/websocket-protocol.md
  - Documented protocol_version field
  - Explained version negotiation
  - Added migration guide (v1→v2 differences)
  - Clarified v1 as legacy, v2 as recommended

### ✅ Phase 5: Testing & Validation (Completed)
- [x] Run full test suite: All 471 tests pass with race detection
- [x] Run regression test: 10,000 hands @ 430 hands/second, zero errors
- [x] Run smoke test: 5 hands with mixed bot types, working correctly

## References

- [Protocol Specification](../docs/websocket-protocol.md)
- [Aragorn Investigation Results](https://github.com/lox/pokerforbots/pull/16) - Documents impact of incorrect action mapping (458 invalid actions, -213.6 BB/100 performance loss)
- [Hand Runner Implementation](../internal/server/hand_runner.go)
- [Action Validation Logic](../internal/game/betting.go#L54-L87)

## Verify

```bash
task test
task lint

# Verify built-in bots work
pokerforbots spawn --spec "calling-station:2,random:2,aggressive:2" --hand-limit 1000

# Regression test with complex bot
pokerforbots spawn --spec "complex:6" --hand-limit 10000 --seed 42

# Integration test with external bot (manual)
# Update test bot to send generic "call" and "raise" actions, verify normalization logs
```
