# Solver Toolkit

The solver toolkit provides plumbing for building Pluribus-style strategies in Go. It is early-stage and focuses on wiring together the key pieces (abstractions, regret tracking, blueprint persistence, CLI commands) so we can iterate toward a production-quality Monte Carlo CFR stack.

## Package Layout

- `sdk/solver`: core types, abstractions, MCCFR trainer, blueprint serialization.
- `sdk/solver/runtime`: read-only runtime loader for consuming blueprint packs inside bots.
- `cmd/pokerforbots`: consolidated command-line entry point with `solver` subcommands for training and evaluation.

## Quick Start

```bash
# Run a smoke training run with the preset (short stack + pruned raises)
cmd/pokerforbots solver train --smoke --iterations=5000 --out=out/blueprint-smoke.json --progress-every=100

# Verify the preset on a tiny loop
cmd/pokerforbots solver train --smoke --iterations=10 --progress-every=1 --out=/tmp/smoke.json

# Inspect metadata using the eval command (full evaluation TBD)
cmd/pokerforbots solver eval --blueprint=out/blueprint-smoke.json
```

- Add `--cfr-plus` to `cmd/pokerforbots solver train` to enable CFR+ (positive regret matching with linear strategy averaging).
- Use `--sampling=full` to disable external sampling when debugging traversal math; the default `external` keeps Monte Carlo variance low.
- Pass `--mirror` to `cmd/pokerforbots solver eval` to run a seat-rotated evaluation (two back-to-back matches with bots swapping seats) before aggregating BB/100.

Long-run CFR+ smoke baseline (EPYC 8-core preset):

```bash
task solver -- train \
  --smoke \
  --iterations=10000000 \
  --parallel=8 \
  --cfr-plus \
  --sampling=external \
  --checkpoint-path=out/cfrp-smoke.ckpt \
  --checkpoint-every=500000 \
  --progress-every=50000 \
  --seed=42 \
  --out=out/cfrp-smoke.json

task solver -- eval \
  --blueprint=out/cfrp-smoke.json \
  --hands=20000 \
  --mirror \
  --seed=42
```

## Environment Integration

Bots can load a blueprint by setting `POKERFORBOTS_BLUEPRINT` to the generated pack path. The complex example bot will automatically fall back to heuristic logic if the blueprint fails to load.

## Status & Next Steps

- Checkpoints: use `--checkpoint-path` with `--checkpoint-every` for periodic saves and `--resume-from` to continue a run on the same machine.
- Pruned raises: use `--smoke` (stack=50, blinds=1/2, max raises=2) for quick loops, or pass `--max-raises=N` / `--disable-raises` to tune manually. Enable `--adaptive-raise-visits=X` to let high-traffic nodes reintroduce larger raise sets once they have at least X visits (default 500).
- Parallel tables: pass `--parallel=N` (default 1) to run N independent traversals per iteration and better utilize multi-core CPUs.

- MCCFR traversal currently samples full hands via `internal/game`, updates regrets over fold/check/call/raise actions, and emits traversal metrics (nodes, depth, iteration time) for monitoring. External-sampling MCCFR reduces smoke-run node counts from ~530K to ~1K per iteration (10x+ faster).
- Real poker state integration, distributed training, and evaluation harnesses are tracked in `plans/solver.md`.
- Runtime policies expose averaged strategies; real-time re-solving will build on top of this interface.

Always regenerate or validate blueprint packs after modifying abstraction parameters—loads will fail if the runtime abstraction validation does not match the saved metadata.
