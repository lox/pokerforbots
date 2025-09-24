# Evaluator Optimization Plan

This document tracks planned improvements to the 7-card evaluator in `internal/game` to reduce latency per evaluation while preserving correctness. Changes are incremental, test-driven, and should not alter public APIs or hand ranking semantics.

## Baseline (today)

- Benchmarks (Apple M1, sequential): ~200 ns/op; ~5.0M hands/sec; 0 B/op, 0 allocs/op
  - See the large-sample benches in [`evaluator_test.go`](./evaluator_test.go).
- Profiling (pprof top 10 on sequential bench):
  - `countRanks` ~26%
  - `findOrderedKickers` ~10% (+ hash overhead: `runtime.mapaccess1`, `aeshashbody`)
  - `checkStraight` ~3%
  - `Evaluate7Cards` cum ~67%

References:
- [`countRanks`](./evaluator.go#L148-L161)
- [`findOrderedKickers`](./evaluator.go#L208-L226)
- [`checkStraight`](./evaluator.go#L235-L253)

## Goals

- Reduce evaluator ns/op by 20–30% with local, safe changes.
- Remove map/hash overhead from kicker selection.
- Keep zero allocations per evaluation.
- Maintain all existing tests; add coverage for new edge cases (esp. straights, SF).

## Phase 1 — Quick wins (no tables/MPHF)

1) Branchless straight detection with bitwise cascade
- Replace scan-in-a-loop with a 5-shift bitwise cascade (and an explicit wheel check), returning the high card index.
- Reuse the same primitive for straight-flush detection by applying it to a single suit mask.
- Touch points:
  - Replace [`checkStraight`](./evaluator.go#L235-L253) with branchless impl.
  - Use the new primitive directly when evaluating suited 5-card runs (SF).

2) O(cards) rank counting (7 iterations, not 52)
- Implement `countRanksFast` that iterates set bits only:
  - Loop `x &= x-1`, compute `pos := bits.TrailingZeros64(x)`, map to `rank := pos % 13`.
- Wire it into `Evaluate7Cards` instead of nested suit/rank loops in [`countRanks`](./evaluator.go#L148-L161).

3) Eliminate maps in kicker selection
- Replace `map[uint8]bool` with a `uint16` bitmask of used ranks.
- Implement helpers using bit scans:
  - `topKickers(rankMask uint16, used uint16, n int) []uint8` — repeatedly take MSB, clear bit.
  - `findKicker` becomes a thin wrapper over `topKickers(..., 1)`.
- Update callers in pair, two-pair, trips paths.
- Drops `runtime.mapaccess1`/`aeshashbody` from profiles and the tiny GC noise.

4) Precompute suit and rank masks once per evaluation
- Extract per-suit masks (`cm, dm, hm, sm`) and a `rankMask := cm|dm|hm|sm` up-front and reuse across steps.
- Avoid recomputing `GetRankMask()` multiple times.

Acceptance for Phase 1:
- Sequential bench improves from ~200 ns/op to < 160 ns/op (target) on the same machine.
- `go tool pprof -top` shows no `mapaccess`/`aeshash` in top 10; `countRanks` contribution significantly reduced.
- All tests green.

## Phase 2 — Bitset-only pair/trips/quads detection

- Compute masks across suits to avoid `[13]uint8` counts entirely:
  - Let `s0,s1,s2,s3` be suit masks (u16) for clubs, diamonds, hearts, spades.
  - `quads := s0 & s1 & s2 & s3`
  - `atLeast3 := (s0&s1&s2)|(s0&s1&s3)|(s0&s2&s3)|(s1&s2&s3)`
  - `atLeast2 := (s0&s1)|(s0&s2)|(s0&s3)|(s1&s2)|(s1&s3)|(s2&s3)`
  - `tripsMask := atLeast3 &^ quads`
  - `pairsMask := atLeast2 &^ atLeast3`
- Pick highest ranks by scanning these masks (MSB first). This removes N scans and simplifies `findNOfAKind*`.
- Ensure full house logic accepts trips+trips and trips+pair (already fixed in Go code, preserve that behavior).

Acceptance for Phase 2:
- Further reduction in ns/op and simpler code paths; correctness maintained.

## Phase 3 — Micro-optimizations & cleanup

- Replace `getTopCardsOrdered` for flush with a direct “top 5 bits” selection; prefer using SF detection first.
- Inline small helpers (prefer `//go:nosplit` only if justified — likely unnecessary).
- Keep return shapes consistent (e.g., fixed-size arrays internally, slice adaptors at call sites) to help the compiler.
- Reconfirm Ace-low straight handling with new bitwise method across all suits and flush paths.

## Benchmarks, Profiling, and Verification

- Benchmarks:
  - `task bench:evaluator -- -benchtime=3s`
- Quick top-10 (sequential only):
  - `task profile:evaluator:top -- -test.benchtime=5s`
- Success criteria (Phase 1):
  - `< 160 ns/op` sequential on same machine (document CPU & Go version in output).
  - No allocations per op.
  - No `mapaccess`/`aeshash` in top 10.

## Testing

- Existing tests in [`evaluator_test.go`](./evaluator_test.go) must remain green.
- Add/ensure coverage for:
  - Wheel straights and high straights via new detection.
  - Straight flush tie-break correctness (high card).
  - Trips over trips ⇒ full house.
  - Pair/two-pair rank ordering with kickers after mask refactor.

## Not in scope (for now)

- MPHF/CHD tables or precomputed lookup tables.
- SIMD batch evaluation.
- Cross-language parity with Zig evaluator internals.

## Tasks Checklist

- [x] Implement `straightHigh` (bitwise cascade) and replace `checkStraight`
- [x] Reuse `straightHigh` for SF detection (single-suit mask)
- [x] Implement `countRanksFast` and integrate
- [x] Replace map-based used-rank with `uint16` mask in kicker helpers
- [x] Precompute suit masks and a single `rankMask` per eval and reuse
- [x] Verify tests; add missing edge-case tests (straights/SF)
- [x] Measure: benches + pprof top; record before/after
- [x] Phase 2: mask algebra for pairs/trips/quads; remove `[13]uint8` counts
- [x] Re-run tests and benches; document results here
- [x] Add batch evaluation surface (non-SIMD) to amortize call overhead

## Experiment Log

- 2025-09-24 — Baseline Apple M1, Go `task bench:evaluator -- -benchtime=3s`: `BenchmarkEvaluate7Cards_LargeSample` 199.6 ns/op (≈5.01M hands/sec), parallel 69.57 ns/op (≈14.37M hands/sec); 0 B/op, 0 allocs/op.
- 2025-09-24 — Branchless straight detection + bitmask kickers/counting: sequential 49.70 ns/op (≈20.12M hands/sec), parallel 53.87 ns/op (≈18.56M hands/sec); `go test ./poker` passes.
- 2025-09-24 — Suit mask reuse + full mask algebra for pairs/trips/quads: sequential 22.95 ns/op (≈43.58M hands/sec), parallel 57.01 ns/op (≈17.54M hands/sec); `go test ./poker` passes.
- 2025-09-24 — Batch API prototype (32 at a time, scalar): `BenchmarkEvaluate7CardsBatch32` 555.9 ns/op total (≈17.4 ns per hand); sequential bench unchanged (22.95 ns/op) while parallel settles around 57 ns/op due to framework overhead; `go test ./poker` passes.
- 2025-09-25 — 16-bit hand-strength encoding experiment: sequential 31.17 ns/op (≈32.08M hands/sec), parallel 51.79 ns/op (≈19.31M hands/sec); combinatorial ranking dominated CPU. Reverted to 32-bit layout until lookup tables are ready (current sequential 23.19 ns/op, parallel 57.03 ns/op); `bin/gotestsum -race ./...` back to green.

### Notes 2025-09-24

- Suit masks cached at eval entry; straight-high cascade now feeds both flush and board checks directly.
- Count arrays removed in favour of suit-intersection masks (`quadsMask`, `tripsMask`, `pairsMask`); kickers sourced from shared rank mask.
- Next profiling pass (`task profile:evaluator:top`) scheduled to confirm new hotspots before further micro-tuning.
- Batched Go API mirrors Zig structure-of-arrays approach (no SIMD yet) and gives ~1.5× effective speedup when processing 32 hands per call; parallel harness now dominated by scheduler/cache overhead (~57 ns/op).
- 16-bit strength encoding without lookup tables regressed sequential perf (~+8 ns/op) because mask→rank combinatorics (`highestRank`, `rankOrdinalAsc`, `adjustFiveCardIndex`) dominate. Parked the idea until CHD/flush tables are in place.

## Notes & References

- Zig evaluator patterns we can mirror:
  - Branchless bit ops for straight detection (see `getTop5Ranks` and straight patterns).
  - Favor bit masks over arrays/maps for selection of top ranks.
- We intentionally defer MPHF/table-driven ranking.
