# Solver Framework

---
- Status: draft
- Updated: 2025-09-28
- Tags: solver, research, infra
- Scope: sdk/solver, cmd/solver, sdk/examples/complex, docs
- Risk: high
- Effort: L
---

## TL;DR

- Problem: We lack an in-house poker solver to generate equilibrium-grade strategies for our bots.
- Approach: Build a Go-native CFR solver library with training/evaluation tooling and a runtime loader before wiring re-solving into gameplay clients.
- Expected outcome: Consumable blueprint strategy packs, metrics proving solver strength, and a bot-ready policy API.

## Prereads

- docs/design.md – overall architecture, solver integration points, and runtime expectations.
- docs/solver.md – current solver workflows, environment wiring, and blueprint lifecycle notes.
- cmd/solver/main.go – CLI surface for training/evaluation and configuration flow.
- sdk/solver/ (blueprint.go, bucket.go, config.go, regret.go, trainer.go) – core abstractions, MCCFR primitives, and serialization logic.
- sdk/examples/complex/main.go – reference bot blueprint loading path and runtime policy usage.

## Goals

- Produce a baseline blueprint for 6-max NLHE with ≤ 30 mbb/100 exploitability proxy within 30M MCCFR iterations.
- Deliver training + evaluation tooling capable of running ≥ 8 parallel self-play tables while streaming convergence metrics with <5% overhead.

## Non-Goals

- Implementing real-time re-solving in the live bot (covered by plans/pluribus-bot.md).
- Bankroll, authentication, or production deployment workflows.

## Plan

- Iterative strategy: favor small on-device training cycles (≤10k iterations) to validate plumbing, policy loading, and metrics before scaling to distributed runs. Use local telemetry to guide abstraction/raise tweaks and only introduce parallel tables/large iterations after checkpoint + evaluation harnesses are proven.

- Step 1: Stand up `sdk/solver` with abstractions, regret data structures, and hand/equity adapters backed by the `poker` package.
- Step 2: Implement distributed-capable MCCFR/Linear CFR engine with snapshot serialization under `sdk/solver/trainer`.
- Step 3: Add `cmd/solver train` subcommand to orchestrate self-play runs via the PokerForBots engine and persist blueprint packs.
- Step 4: Add `cmd/solver eval` subcommand for mirror-mode matches, exploitability probes, and BB/100 reporting.
- Step 5: Ship a runtime policy loader (e.g., `sdk/solver/runtime`) and update the complex bot to support pure-blueprint play via the new API.
- Step 6: Document workflows (`docs/solver.md`) and wire automated convergence dashboards plus regression gates into CI.

## Progress (as of 2025-09-28)

- Checkpoint/resume support stores trainer state to JSON snapshots; CLI now exposes `--checkpoint-path`, `--checkpoint-every`, and `--resume-from` for iterative local runs (sdk/solver/checkpoint.go:1, cmd/solver/main.go:74).
- Phase 1 foundations scaffolded: abstraction config, bucket mapper, and regret table primitives are implemented with fixture coverage and concurrency tests now in place (sdk/solver/config.go:1, sdk/solver/bucket.go:1, sdk/solver/regret.go:1, sdk/solver/regret_test.go:8).
- Trainer plumbing, blueprint serialization, and CLI stubs now drive a fold/call MCCFR traversal over real hand simulations with node instrumentation emitted via progress hooks (sdk/solver/trainer.go:1, sdk/solver/traversal.go:1, sdk/solver/blueprint.go:1, cmd/solver/main.go:1).
- Initial MCCFR traversal integrated with `internal/game`; trainer iterations update real regrets, record traversal stats, and emit blueprint outputs with bet-sizing raises (sdk/solver/trainer.go:33, sdk/solver/traversal.go:1).
- Runtime policy loader and complex bot integration support optional blueprint-driven decisions behind an environment flag, now backed by targeted runtime tests (sdk/solver/runtime/policy.go:1, sdk/solver/runtime/policy_test.go:1, sdk/examples/complex/main.go:352).
- Solver documentation drafted to capture current workflows and status; blueprint load failures (version/metadata/corruption) are covered via unit tests (docs/solver.md:1, sdk/solver/blueprint_test.go:1).
- CFR toggles added: `cmd/solver train` now exposes `--cfr-plus` and `--sampling` to switch between positive-regret/linear averaging and full traversal debugging paths, and `cmd/solver eval --mirror` runs seat-rotated matches via the spawner for lower-variance BB/100 estimates (cmd/solver/main.go:101, cmd/solver/eval_runner.go:1).

## Acceptance Criteria

- `go test ./sdk/solver/...` passes with coverage on abstraction mapping, regret updates, serialization, and runtime loading.
- Training dry run: `cmd/solver train --iterations=100000 --seed=1 --out=out/blueprint-demo.pb` completes in <15 minutes on an 8-core box.
- Evaluation run: `cmd/solver eval --blueprint=out/blueprint-demo.pb --hands=10000 --mirror` reports BB/100 ±5 with 95% CI.
- Complex bot smoke test: `go test ./sdk/examples/complex -run TestBlueprintIntegration` confirms the bot can load a blueprint pack and sample actions ≤100 ms.
- Snapshot compatibility check prevents loading mismatched abstraction versions.

## Risks & Mitigations

- Risk: Solver math bugs silently skew strategies — Mitigation: add deterministic unit tests comparing toy games to closed-form solutions.
- Risk: Training throughput too slow — Mitigation: support batched traversal, multiple concurrent tables, and resumable checkpoints.
- Risk: Memory blow-up from regret tables — Mitigation: start with sparse maps + compression and introduce pruning heuristics.
- Risk: Evaluation noise hides regressions — Mitigation: integrate mirror mode and variance-reduction (AIVAT/ACE) to tighten confidence intervals.
- Risk: Runtime integration stalls — Mitigation: keep runtime loader minimal and add feature flags to fall back to existing heuristic bot.

## Tasks

### Phase 1 – Abstraction & Regret Foundations

- [x] Finalise `AbstractionConfig` surfaces and fixtures that cover bucket counts, bet sizes, and versioning semantics for backward compatibility.
- [x] Implement and test bucket mappers (preflop/postflop) plus helper utilities so we can deterministically translate poker states into abstraction IDs.
- [x] Build the regret-table primitives with thread-safe updates, strategy averaging, and golden tests that mirror closed-form toy games.
- Exit criteria: `go test ./sdk/solver/...` passes with new fixtures verifying bucket assignments/regret averaging, and a sample blueprint round-trip (`cmd/solver train --iterations=10 --out=out/phase1-smoke.json`) reproduces the expected uniform policy snapshot.

### Phase 2 – MCCFR Traversal Engine

- [x] Replace the placeholder iteration loop with MCCFR traversal wired to the poker engine, currently supporting fold/call decisions with reach tracking (extend bet sizing/raises next).
- [x] Add instrumentation (node counts, regret growth, convergence metrics) and deterministic seeding so training telemetry is reproducible.
- [x] Implement snapshot/restore of trainer state to support longer runs and checkpoint-based recovery.
- Exit criteria: `cmd/solver train --iterations=5000 --seed=7 --out=out/phase2-blueprint.pb` traverses full-hand simulations (non-zero infosets per street, stable regret deltas) and a toy-game harness asserts exploitability below the analytic baseline.

#### Action Sampling Roadmap

- **Phase 1 – Deterministic Prune:** Keep fold/check/call and the smallest useful raise sizes (min-raise, pot, shove) per pot/to-call bucket. Gate via `MaxRaisesPerBucket` in `AbstractionConfig` so smoke runs stay tiny without touching long-run abstractions.
- **Phase 2 – Stochastic Sampling:** (✅) Converted traversal to external-sampling MCCFR. Smoke preset now runs ~1K nodes/iteration (down from ~530K) and completes 10-smoke iterations in ~0.13s.
- **Phase 3 – Adaptive Expansion:** Periodically rescan infosets for large pending regrets and reintroduce suppressed raises, ensuring long training restores the full abstraction without overwhelming local tests.
- Exit criteria: `cmd/solver train --iterations=50 --progress-every=1 --disable-raises=false` completes under 2 minutes on a MacBook Air while emitting progress logs each ≤5 seconds, and full-raise training (5k iterations) shows parity against a fold/call-only baseline in evaluation harness smoke tests.

#### Blueprint Smoke Loop

- Run a small local training (e.g., `go run ./cmd/solver train --smoke --iterations=5000 --players=2 --out=dist/h2h.json --checkpoint-path=dist/h2h.ckpt --checkpoint-every=1000`).
- Export `POKERFORBOTS_BLUEPRINT=dist/h2h.json` (optionally `POKERFORBOTS_BLUEPRINT_FAIL_HARD=1`) when launching the complex bot.
- Play the solver bot against a baseline such as `sdk/examples/calling-station` via the demo harness; confirm solver-driven actions and <100 ms responses in logs.
- Inspect progress telemetry (nodes, iteration time) and sampled strategy weights to ensure policies aren’t uniform or skewed unexpectedly.
- Iterate with larger iteration counts only after the bot behaves sensibly and instrumentation looks healthy.
- Expect early blueprints (≤5k iterations) to be competitive but not consistently stronger than calling-station; real gains require longer training plus evaluation metrics.


### Phase 3 – Training Tooling & Persistence

#### Snapshot Design Notes

- Introduce a minimal smoke-test action profile (fold/call plus a single raise tier/all-in) with a CLI flag to toggle between smoke mode and full abstraction so short runs finish quickly while we optimise traversal.

- Checkpoint pack will be msgpack/JSON with version tag, iteration count, RNG seed, abstraction hash, and serialized regret table (key -> regrets, strategy sums).
- Store bucket mapper config and traversal stats headers to validate resume compatibility.
- Trainer resume flow: load snapshot, rebuild mapper, restore RNG state via recorded seed/offset, and merge pending stats.
- CLI flags: `--resume-from` (path), `--checkpoint-path`, `--checkpoint-interval` (minutes/iterations).
- Ensure checkpoint save is atomic (temp file + rename) to avoid partial writes.

- [ ] Flesh out `cmd/solver train` with parallel table orchestration, configurable batching, and periodic checkpoint emission gated behind `CheckpointEvery`.
- [ ] Emit structured metrics/logging for progress hooks and surface them to dashboards or CLI summaries.
- [ ] Harden blueprint serialization: include abstraction hashes, metadata validation, and unit tests around corrupted/incompatible saves.
- Exit criteria: Running `cmd/solver train --iterations=20000 --parallel=4 --checkpoint-mins=1 --out=out/phase3-blueprint.pb` produces rolling checkpoints, resumes cleanly after interruption (`--resume` or equivalent flag), and blueprint metadata validation rejects tampered files in unit tests.

### Phase 4 – Evaluation Harness & Metrics

- [x] Implement `cmd/solver eval` mirror-mode matches via embedded spawner, reporting BB/100 and perf metrics for solver vs baseline.
- [ ] Integrate variance-reduction techniques (AIVAT/ACE) and configurable sample sizes so short runs still produce actionable signals.
- [ ] Produce regression fixtures that compare current blueprints against baselines and fail fast on significant performance drifts.
- Exit criteria: `cmd/solver eval --blueprint=out/phase2-blueprint.pb --hands=10000 --mirror` emits BB/100 ± 95% CI plus exploitability proxy metrics, and CI regression tests flag deviations beyond configured thresholds.

### Phase 5 – Runtime Integration & Automation

- [ ] Ship the runtime policy loader API, wire blueprint-driven play into the complex bot behind feature flags, and add latency profiling to enforce the 100 ms bound.
- [ ] Expand `docs/solver.md` with end-to-end workflows (training, evaluation, runtime consumption), troubleshooting, and abstraction change procedures.
- [ ] Extend CI with lint/test, solver smoke training (`--iterations=1000`), evaluation regression runs, and blueprint integration tests so every change exercises the full loop.
- Exit criteria: `go test ./sdk/examples/complex -run TestBlueprintIntegration` loads a fresh blueprint, takes solver-driven actions in under 100 ms, and the CI pipeline runs lint/test/train/eval jobs automatically on each PR.

## References

- docs/design.md
- plans/pluribus-bot.md
- sdk/examples/complex/main.go
- TODO.md (Phase 14 deterministic tooling)
- https://www.science.org/doi/10.1126/science.aay2400

## Verify

```bash
bin/task lint
bin/task test
bin/task build
cmd/solver train --iterations=1000 --seed=7 --out=out/blueprint-smoke.pb
cmd/solver eval --blueprint=out/blueprint-smoke.pb --hands=2000 --mirror --seed=7
```

### Upcoming Experiments

- [ ] CFR+ 10M smoke baseline (EPYC 8c):
  - `task solver -- train --smoke --iterations=10000000 --parallel=8 --cfr-plus --sampling=external --checkpoint-path=out/cfrp-smoke.ckpt --checkpoint-every=500000 --progress-every=50000 --seed=42 --out=out/cfrp-smoke.json`
  - After completion run `task solver -- eval --blueprint=out/cfrp-smoke.json --hands=20000 --mirror --seed=42` and log BB/100 vs calling-station.
  - Record results in docs/solver.md and TODO.md once the job finishes.
