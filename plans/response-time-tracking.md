# Response Time Tracking Plan

This document defines the work required to measure and surface bot response latency throughout the regression-testing pipeline without regressing hand throughput.

## Goals

- Measure bot decision latency during regression runs without impacting the existing 100 ms timeout guarantee.
- Surface latency statistics (average, p95, max, standard deviation) in server stats, spawner output, and regression reports.
- Keep instrumentation lightweight and optional, preserving current performance characteristics.

## Baseline (today)

- No per-action latency metrics are collected; only timeout counts are available.
- `StatsMonitor` aggregates chip outcomes but has no awareness of response times.
- Regression tester reports BB/100, VPIP/PFR, etc., but cannot flag slow bots.
- Spawner stats JSON omits any timing details.

## Phase 1 — Instrumentation & Data Capture

1) Action timing in hand runner
   - Timestamp each `ActionRequest` send in `internal/server/hand_runner.go`.
   - Capture elapsed time when a valid response arrives; treat timeouts separately (record timeout duration but flag as timeout).
   - Handle disconnects gracefully (do not count toward latency averages but track incidence).

2) Stats monitor aggregation
   - Extend `StatsMonitor` / `BotStatistics` to accumulate response metrics: count, min, max, sum, sum of squares, and a bounded reservoir for percentile computation (target 128 samples per bot).
   - Update timeout and disconnect counters to accompany the latency dataset.

3) Protocol extensions
   - Add optional latency fields to `protocol.PlayerDetailedStats`: `responses_tracked`, `avg_response_ms`, `p95_response_ms`, `max_response_ms`, `response_std_ms`, `timeout_count`, `disconnect_count`.
   - Ensure zero values when no data is present for backward compatibility.

## Phase 2 — Reporting & Tooling

1) Server stats exposure
   - Include new latency fields in the admin stats JSON produced by `internal/server/pool.go` (`GameStats` → per-player details).
   - Confirm spawner stats output (`cmd/spawner`) renders latency metrics in console summaries.

2) Regression aggregation
   - Update `internal/regression/stats.go` to ingest latency fields and emit `challenger_avg_response_ms`, `baseline_p95_response_ms`, etc.
   - Teach `cmd/regression-tester` to display latency in both summary and JSON output; warn when p95 exceeds configurable SLA (default 100 ms).

3) Configuration knobs (optional)
   - Introduce CLI/Config options for regression tester to set latency warning thresholds and reservoir size if needed.

## Phase 3 — Tests, Docs, Polish

1) Testing
   - Extend `internal/server/stats_test.go` to validate latency aggregation (mean, stddev, percentile) and timeout accounting.
   - Update `internal/regression/orchestrator_test.go` fixtures to include latency fields; verify parsing.
   - Add regression runner test that reads a synthetic stats file with latency samples and checks reported metrics.

2) Documentation
   - Add a "Latency Tracking" section to `docs/spawner.md` and `docs/regression-tester.md` covering collected metrics and interpretation.
   - Mention default SLA (100 ms) and how to adjust warning thresholds.

3) Cleanup & safeguards
   - Ensure instrumentation minimally affects hot paths (avoid allocations, reuse buffers).
   - Provide feature flag or config to disable latency tracking if necessary.

## Not in Scope

- Live alerting or automated remediation based on latency.
- Persisting per-action traces or histories beyond bounded reservoirs for statistics.
- Changes to bot protocols beyond optional stat fields.

## Tasks Checklist

- [ ] Instrument `ActionRequest` send/receive in `internal/server/hand_runner.go` to measure latency.
- [ ] Introduce latency recording hooks into `StatsMonitor` / `BotStatistics` with reservoir sampling.
- [ ] Extend `protocol.PlayerDetailedStats` and related JSON structs with latency fields.
- [ ] Update admin stats serialization to emit latency metrics per player.
- [ ] Wire latency values into `internal/regression/stats.go` aggregates and outputs.
- [ ] Enhance `cmd/regression-tester` summary/JSON with latency figures and warnings.
- [ ] Update `cmd/spawner` stats printing to include latency metrics when present.
- [ ] Add unit tests for stats aggregation and regression parsing of latency data.
- [ ] Document latency tracking in `docs/spawner.md` and `docs/regression-tester.md`.
- [ ] Consider configuration flags for latency warnings/reservoir sizing; implement if needed.

## Open Questions

- Should timeouts be excluded from average latency but counted separately? (Current assumption: yes.)
- Is a fixed-size reservoir sufficient for accurate percentile estimation under large hand counts?
- Do we need CLI options to disable latency tracking for performance-sensitive runs?

