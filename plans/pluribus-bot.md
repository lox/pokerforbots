# Pluribus-Style Bot Implementation

- Owner: @ai-platform
- Status: draft
- Updated: 2025-09-25
- Tags: bot, solver, research
- Scope: sdk/pluribus, sdk/examples/complex, cmd/solver, docs
- Risk: high
- Effort: L

## TL;DR

- Problem: The current complex bot relies on static heuristics and will be crushed by modern solver-backed opponents.
- Approach: Build a Go-native Pluribus-style stack with offline MCCFR blueprint training plus real-time subgame solving.
- Expected outcome: A deployable bot that plays near-GTO poker within PokerForBots' 100 ms decision window.

## Goals

- Deliver a blueprint strategy whose exploitability proxy is ≤ 20 mbb/100 against best-response probes over 100k mirrored hands.
- Keep live decision latency ≤ 80 ms P95 while running blueprint lookup + depth-limited search in production mode.

## Non-Goals

- Real-money bankroll management or wager settlement.
- Persistent opponent modelling across hands or tables.

## Plan

- Step 1: Establish Go solver primitives (card abstraction, action abstraction, regret data structures) and hook them into a fast hand evaluator.
- Step 2: Implement a self-play training harness that runs Linear/MCCFR to produce weighted blueprint strategies and serialize them.
- Step 3: Build policy loading + sampling APIs in the SDK and refactor `sdk/examples/complex` to consume blueprint policies for baseline play.
- Step 4: Add depth-limited real-time re-solving with continuation policies and bounded MCCFR iterations per decision.
- Step 5: Integrate telemetry, benchmarking, and mirror-mode evaluation workflows to gate blueprint and runtime releases.

## Acceptance Criteria

- Blueprint training run completes 10M iterations with convergence monitored via regret/EV dashboards; serialized pack size ≤ 8 GB compressed.
- Integration test: `go test ./sdk/...` passes with new solver packages.
- Mirror-mode evaluation: `cmd/tools/pluribus-eval --hands=20000 --mirror --seed=42` reports ≥ break-even BB/100 versus the existing complex bot.
- Runtime profiling: `bin/task bench:pluribus -- -timeout=120s` shows ≤ 80 ms P95 decision latency and zero deadline misses in 100k sampled actions.

## Risks & Mitigations

- Risk: MCCFR training cost explodes — Mitigation: start with coarse abstractions, support distributed training checkpoints.
- Risk: Runtime solver exceeds 100 ms — Mitigation: add strict time budgets with fallback to blueprint action.
- Risk: Strategy regression hard to detect — Mitigation: automate mirror-mode regression suite with statistical significance checks (AIVAT/AER).
- Risk: Data model drift between training and runtime — Mitigation: version abstractions and enforce compatibility checks during load.

## Tasks

- [ ] Draft abstraction configs (bet sizing ladders, bucket definitions) and unit tests.
- [ ] Implement Go MCCFR solver with serialization and deterministic seeds.
- [ ] Create training CLI + infra docs for running distributed self-play.
- [ ] Refactor complex bot to load blueprint packs and sample actions.
- [ ] Add depth-limited search module with continuation policies.
- [ ] Build evaluation harness (mirror-mode sims, mbb/100 tracker, latency profiler).
- [ ] Update docs (`docs/design.md`, `docs/development-workflow.md`) with solver workflow.

## References

- docs/design.md
- sdk/examples/complex/main.go
- TODO.md (Phase 14)
- https://www.science.org/doi/10.1126/science.aay2400
- https://github.com/conorarmstrong/pluribus

## Verify

```bash
bin/task lint
bin/task test
bin/task build
bin/task bench:evaluator -- -benchtime=3s
cmd/tools/pluribus-train --iterations=100000 --seed=1
cmd/tools/pluribus-eval --hands=5000 --mirror --seed=2
```

