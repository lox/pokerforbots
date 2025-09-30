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

- [x] Update plan to reflect v2 protocol breaking change and address Codex feedback
- [ ] Write failing tests for GetValidActions() returning simplified vocabulary
- [ ] Update GetValidActions() in internal/game/betting.go to pass tests
- [ ] Write failing tests for normalizeAction() covering all edge cases
- [ ] Implement normalizeAction() in internal/server/hand_runner.go to pass tests
- [ ] Update convertAction() to use normalization and handle semantic actions
- [ ] Update sdk/bots/callingstation to use call/raise
- [ ] Update sdk/bots/random to use call/raise
- [ ] Update sdk/bots/aggressive to use call/raise
- [ ] Update sdk/bots/complex to use call/raise
- [ ] Update CLI client (internal/client/client.go) validation and shortcuts
- [ ] Update docs/websocket-protocol.md with v2 protocol and migration guide
- [ ] Run full test suite: task test
- [ ] Run regression test: pokerforbots spawn --hand-limit 10000

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
