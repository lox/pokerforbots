# Benchmarking

This document describes how to run and interpret performance benchmarks for PokerForBots, focusing on the hand evaluator in `internal/game`.

## Evaluator Benchmarks (hands/sec)

The evaluator benchmarks report “hands/sec” for evaluating 7-card hands. Two benchmarks exist:

- Sequential: evaluates a large pre-generated pool of random hands.
- Parallel: uses Go’s `RunParallel` to stress multi-core throughput.

Hands are pre-generated with a deterministic RNG and re-used during the benchmark hot loop to avoid measuring deck shuffling or allocation overhead.

### Run via Task

- Run both benchmarks (sequential and parallel):

```bash
task bench:evaluator
```

- Pass custom arguments (e.g., longer benchtime, specific CPUs):

```bash
task bench:evaluator -- -benchtime=5s -cpu=1,4,8
```

### Run directly with `go test`

- Sequential only:

```bash
go test -run ^$ -bench BenchmarkEvaluate7Cards_LargeSample$ -benchmem ./internal/game
```

- Parallel only:

```bash
go test -run ^$ -bench BenchmarkEvaluate7Cards_LargeSample_Parallel$ -benchmem ./internal/game
```

### Interpreting results

- `hands/sec`: derived metric added by the benchmark to directly show throughput.
- `ns/op`: lower is better; roughly the inverse of `hands/sec`.
- `B/op` and `allocs/op` should remain near zero (the evaluator is allocation-free).

Example output (Apple M1):

```
BenchmarkEvaluate7Cards_LargeSample-8              199.5 ns/op   5,011,290 hands/sec   0 B/op   0 allocs/op
BenchmarkEvaluate7Cards_LargeSample_Parallel-8     67.14 ns/op  14,894,684 hands/sec   0 B/op   0 allocs/op
```

### Notes and tips

- Control timing and repetitions:
  - `-benchtime=3s` or `-benchtime=100x`
  - `-count=5` to average multiple runs
- Control CPU parallelism:
  - `-cpu=1,2,4,8` to explore scaling
  - Set `GOMAXPROCS` env var to constrain cores
- Keep comparisons apples-to-apples: close other workloads and pin power profile if on a laptop.

## Advanced: profiling with uniprof (optional)

If you prefer flamegraphs, [uniprof](https://github.com/indragiek/uniprof) can record and visualize profiles. On macOS native binaries, use host mode and include the `--` separator:

```bash
# Build test binary
go test -c -o dist/evaluator.test ./internal/game

# Record and visualize sequential benchmark (10s)
uniprof record --mode host -o dist/eval-seq.json -- ./dist/evaluator.test -test.run=^$ -test.bench=BenchmarkEvaluate7Cards_LargeSample -test.benchtime=10s -test.count=1
uniprof visualize dist/eval-seq.json
```

Note: Instruments may prompt for permissions and can be slower to launch; use the pprof shortcut below for quick top-10 summaries.

### Tips

- Increase `-test.benchtime` (e.g., 10–15s) for stable sampling.
- Control cores with `-test.cpu` and/or `GOMAXPROCS` to compare single-core vs multi-core.
- Keep environment stable (close other apps) for comparable profiles.

## Quick top 10 with Go pprof (no Instruments)

For a fast, non-interactive summary of hotspots (top 10), use Go’s built‑in pprof on the non‑parallel benchmark:

- Via Task:

```bash
task profile:evaluator:top -- -test.benchtime=5s
```

- Manually:

```bash
go test -c -o dist/evaluator.test ./internal/game
./dist/evaluator.test -test.run=^$ -test.bench=BenchmarkEvaluate7Cards_LargeSample -test.count=1 -test.benchtime=5s -test.cpuprofile=dist/eval-seq.pprof
go tool pprof -top -nodecount=10 ./dist/evaluator.test dist/eval-seq.pprof
```

This avoids Instruments and prints the top CPU consumers directly in the terminal.

## Troubleshooting

- If `go test ./...` fails due to unrelated packages (e.g., SDK examples), scope to the evaluator package as shown above or use `task bench:evaluator`, `task profile:evaluator`, or `task profile:evaluator:top` which target `./internal/game` only.

## Implementation references

- Benchmarks are defined in `internal/game/evaluator_test.go`:
  - Sequential large-sample: `BenchmarkEvaluate7Cards_LargeSample`
  - Parallel large-sample: `BenchmarkEvaluate7Cards_LargeSample_Parallel`

The sample generator uses a deterministic RNG and reuses pre-generated 7-card hands to ensure the hot loop measures only evaluator performance.
