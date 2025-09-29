# Solver Performance Rewrite

- Owner: TBD
- Status: draft
- Updated: 2025-09-29
- Tags: solver, performance, optimization
- Scope: sdk/solver, cmd/pokerforbots, docs
- Risk: high
- Effort: L

## TL;DR

- Problem: Go MCCFR traversal runs ~1,000× slower than the Zig baseline because it rebuilds full hand state and allocates heavily.
- Approach: Replace general game state with solver-specific structs, reuse memory aggressively, and optimize sampling to mirror the Zig implementation.
- Expected outcome: Achieve ≥9M nodes/sec in smoke configuration with linear scaling across parallel workers.

## Goals

- Reach ≥9M nodes/sec on the smoke benchmark (`--smoke --iterations=500 --parallel=1`).
- Maintain parity with existing blueprints (no regression in evaluation vs calling-station after optimization).

## Non-Goals

- Introducing neural function approximation or RL-based solvers.
- Changing existing abstractions (bucket counts, raise grid) beyond what is needed for benchmarking.

## Plan

- Step 1: Implement lightweight `SolverHand` state (bit-packed cards, fixed arrays) and swap traversal to use it.
- Step 2: Pool and reuse per-iteration buffers (paths, reach probabilities, raise lists) to eliminate GC pressure.
- Step 3: Replace regret table map with packed-key hash table and add deterministic deck permutations to match Zig throughput.
- Step 4: Add benchmarking/metrics (nodes/sec, allocations) and tune parallel scaling and logging.

## Acceptance Criteria

- `go test ./...` passes after each phase.
- Smoke benchmark (`task solver -- train --smoke --iterations=500 --parallel=1`) reports ≥9M nodes/sec on local hardware.
- Evaluation (`task solver -- eval --blueprint=<latest> --hands=20000 --mirror --seed=42`) matches current blueprint performance within ±5 BB/100.

## Risks & Mitigations

- Risk: Regression in solver correctness — Mitigation: keep parity tests and frequent evaluations after each major change.
- Risk: Large refactor diff introducing bugs — Mitigation: land work in incremental phases with focused tests.
- Risk: Over-optimization tied to current abstraction — Mitigation: keep solver state modular so abstractions remain pluggable.

## Tasks

- [ ] Phase 1: Implement `SolverHand` (bit-packed cards, pot, betting) and update traversal/tests.
- [ ] Phase 2: Introduce arenas/pools for traversal buffers; rewrite path handling to avoid allocations.
- [ ] Phase 3: Implement packed-key regret table and deterministic deck permutations.
- [ ] Phase 4: Add benchmarking harness, log nodes/sec, and tune parallel scaling.
- [ ] Phase 5: Update docs/workflow, remove legacy `internal/game` dependencies, and capture before/after metrics.

## References

- plans/template.md
- sdk/solver/traversal.go
- docs/solver.md
- Zig reference: `../poker-bot-project/src/poker/card.zig`

## Verify

```bash
bin/task lint
bin/task test
bin/task build
# Smoke benchmark (post-optimization target)
cmd/pokerforbots solver train --smoke --iterations=500 --parallel=1 --progress-every=0
```
