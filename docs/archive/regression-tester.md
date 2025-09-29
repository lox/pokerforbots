# Regression Tester Documentation

The regression tester is the primary tool for validating bot improvements and detecting performance regressions. It provides statistically rigorous comparisons between bot versions with multiple test modes and confidence intervals.

## Quick Start

The fastest way to test bot changes:

```bash
# Test your changes against the previous version (default 1000 hands)
task regression:heads-up

# More hands for higher confidence
task regression:heads-up HANDS=5000

# Compare specific versions
task regression:heads-up \
  CHALLENGER="go run ./sdk/examples/complex" \
  BASELINE="go run ./sdk/examples/calling-station" \
  HANDS=10000
```

## Understanding Results

### Sample Output
```
Regression Test Report
======================
Challenger: complex
Baseline: complex (previous)
Mode: heads-up
Hands: 5000
Duration: 6.3s

Results
-------
Challenger: +12.3 BB/100 [95% CI: +8.1 to +16.5]
  VPIP: 15.2%, PFR: 12.8%, Busts: 0.0%
  Latency: p95 108.0 ms (avg 64.2 ms, max 182.0 ms, samples 5000, timeouts 3)
Baseline: -12.3 BB/100 [95% CI: -16.5 to -8.1]
  VPIP: 14.8%, PFR: 11.9%, Busts: 0.0%
  Latency: p95 96.0 ms (avg 58.7 ms, max 140.0 ms, samples 5000, timeouts 1)
Effect Size: 0.45 (medium)
P-Value: 0.002
Hands/sec: 793

Verdict: IMPROVEMENT (95% confidence)
```

### Key Metrics Explained

- **BB/100**: Big blinds won/lost per 100 hands - the primary performance metric
- **Confidence Interval**: 95% CI shows the likely range of true performance
- **VPIP/PFR**: Voluntarily put money in pot / Pre-flop raise percentages
- **Effect Size**: Magnitude of difference (>0.2 = small, >0.5 = medium, >0.8 = large)
- **P-Value**: Statistical significance (<0.05 = significant)
- **Latency**: Response-time metrics highlight whether bots are approaching the
  100 ms decision budget. The tester emits a warning when the p95 latency
  exceeds the `--latency-warn-ms` threshold (default 100 ms).

### Interpreting Verdicts

- **IMPROVEMENT**: Challenger significantly outperforms baseline (p < 0.05)
- **REGRESSION**: Challenger significantly underperforms baseline (p < 0.05)
- **NO SIGNIFICANT DIFFERENCE**: Changes too small to be statistically meaningful
- **INCONCLUSIVE**: More hands needed for reliable conclusion

## Test Modes

### 1. Heads-Up Mode (Recommended First Test)

Direct 1v1 comparison isolating performance difference:

```bash
task regression:heads-up HANDS=5000
```

**Use for**: Quick validation of changes, isolating bot-vs-bot performance
**Sample size**: 5,000+ hands for reliable results
**Expected variance**: ±5-10 BB/100 is normal

### 2. Population Mode

Tests multiple bots at the same table:

```bash
task regression:population HANDS=10000
```

**Use for**: Testing multi-way pot handling, field dynamics
**Configuration**: 2 challenger vs 4 baseline bots (default)
**Sample size**: 10,000+ hands recommended

### 3. NPC Benchmark Mode

Compare against fixed-strategy opponents:

```bash
task regression:npc HANDS=10000
```

**Performance Targets**:
- vs Calling Stations: +25 to +35 BB/100
- vs Random: +15 to +25 BB/100
- vs Aggressive: +5 to +15 BB/100

**Use for**: Detecting regressions in exploitative play

### 4. Self-Play Mode

Baseline variance measurement:

```bash
task regression:self-play HANDS=2500
```

**Expected result**: ~0 BB/100 (confirms statistical validity)
**Use for**: Sanity checking the test framework

### 5. All Modes

Comprehensive test suite with multiple comparison correction:

```bash
task regression:all HANDS=5000
```

Runs all applicable modes and adjusts p-values for multiple testing.

## Sample Size Guidelines

| Confidence Level | Minimum Hands | Use Case |
|-----------------|---------------|----------|
| Low (quick check) | 1,000 | Development iteration |
| Medium | 5,000 | Pre-commit validation |
| High | 10,000+ | Major changes |
| Very High | 50,000+ | Release validation |

The tester automatically warns when sample sizes are insufficient:
- **< 5,000 hands**: ⚠️ Small sample warning
- **5,000-10,000**: May need more for small effects
- **10,000+**: Generally sufficient

## Advanced Usage

### Custom Bot Commands

Test any bot implementation:

```bash
go run ./cmd/regression-tester \
  --mode heads-up \
  --challenger "./my-improved-bot" \
  --baseline "./my-previous-bot" \
  --hands 50000
```

### Deterministic Testing

Use seeds for reproducible results:

```bash
go run ./cmd/regression-tester \
  --mode population \
  --challenger "go run ./sdk/examples/complex" \
  --baseline "go run ./sdk/examples/complex" \
  --seeds "42,123,456,789,999" \
  --hands 10000
```

### Batching Strategy

Handle variance with smaller batches:

```bash
go run ./cmd/regression-tester \
  --mode heads-up \
  --batch-size 1000 \
  --hands 10000 \
  --starting-chips 1000
```

Each batch starts fresh with new stacks, reducing impact of early losses.

### Output Options

Control output format and destination:

```bash
# JSON output for CI/CD
go run ./cmd/regression-tester \
  --output json \
  --output-file results.json

# Quiet mode (progress dots only)
go run ./cmd/regression-tester \
  --output quiet

# Both console and file
go run ./cmd/regression-tester \
  --output both \
  --output-file results.json

# Adjust latency warning threshold (default 100 ms)
go run ./cmd/regression-tester --latency-warn-ms 80
```

## Troubleshooting Unexpected Results

When regression tests show unexpected results, use these debugging strategies:

### 1. Verify with Direct Spawner Testing

Run the spawner with debug logging to examine individual hands:

```bash
# Enable debug logging to see all decisions
go run ./cmd/spawner --log-level debug \
  --spec "calling-station:5" \
  --bot-cmd "go run ./sdk/examples/complex" \
  --hand-limit 100 \
  --print-stats
```

Debug output shows:
- Every betting decision with reasoning
- Hand evaluations and strengths
- Pot odds calculations
- Position considerations

### 2. Analyze Specific Scenarios

Test against known opponent types:

```bash
# Should crush passive players
go run ./cmd/spawner --log-level debug \
  --spec "calling-station:5" \
  --bot-cmd "go run ./sdk/examples/complex" \
  --hand-limit 100 \
  --print-stats \
  --seed 42  # Deterministic for debugging
```

### 3. Check Statistical Patterns

Look for strategic issues in the stats:

```bash
# Write detailed stats for analysis
go run ./cmd/spawner \
  --spec "aggressive:3,calling-station:2" \
  --bot-cmd "go run ./sdk/examples/complex" \
  --hand-limit 1000 \
  --write-stats debug-stats.json

# Analyze the results
jq '.players[] | select(.display_name == "complex") | {
  name: .display_name,
  bb_per_100: .detailed_stats.bb_per_100,
  vpip: .detailed_stats.vpip,
  pfr: .detailed_stats.pfr,
  aggression: .detailed_stats.aggression_factor,
  position_stats: .detailed_stats.position_stats
}' debug-stats.json
```

Warning signs to look for:
- VPIP too high (>30%) or too low (<10%)
- PFR much lower than VPIP (playing too passive)
- Button not most profitable position
- Negative BB/100 vs calling stations

### 4. Hand History Review

For deep debugging, examine specific problem hands:

```bash
# Run with very verbose logging
go run ./cmd/spawner --log-level trace \
  --spec "calling-station:1" \
  --bot-cmd "go run ./sdk/examples/complex" \
  --hand-limit 10 \
  --seed 12345 | tee hands.log

# Search for specific patterns
grep "all-in" hands.log
grep "showdown" hands.log
grep "fold.*raise" hands.log  # Folding to raises
```

### 5. Common Issues and Solutions

| Symptom | Likely Cause | Solution |
|---------|-------------|----------|
| Losing to calling stations | Not value betting enough | Increase aggression with strong hands |
| VPIP too high | Playing too many hands | Tighten preflop ranges |
| Button not profitable | Not stealing blinds | Add position-aware stealing |
| High variance | Too much bluffing | Reduce bluff frequency |
| Timeouts | Bot too slow | Profile and optimize decision logic |

## Continuous Integration

### GitHub Actions Example

```yaml
name: Bot Regression Tests

on: [push, pull_request]

jobs:
  regression:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Quick regression check
        run: task regression:heads-up HANDS=5000

      - name: NPC benchmark
        run: task regression:npc HANDS=10000

      - name: Save results
        if: always()
        uses: actions/upload-artifact@v3
        with:
          name: regression-results
          path: regression-*.json
```

### Pre-commit Hook

```bash
#!/bin/bash
# .git/hooks/pre-commit

echo "Running regression tests..."
task regression:heads-up HANDS=1000

if [ $? -ne 0 ]; then
    echo "Regression detected! Run 'task regression:heads-up HANDS=5000' for confirmation"
    exit 1
fi
```

## Performance Benchmarks

Expected performance on modern hardware:

- **Heads-up**: 500-1,000 hands/second
- **Population (6 bots)**: 200-500 hands/second
- **NPC benchmark**: 300-600 hands/second

Factors affecting speed:
- Bot complexity (complex bot is slower than calling station)
- Number of players at table
- Hardware (CPU cores, memory)
- Logging level (debug is slower)

## Best Practices

1. **Start with heads-up tests** - Fastest and most direct comparison
2. **Use consistent hardware** - CPU throttling can affect results
3. **Run multiple seeds** - Reduces variance from lucky/unlucky runs
4. **Test incrementally** - Small changes are easier to validate
5. **Document bot changes** - Track what changed between versions
6. **Save regression results** - Build performance history over time
7. **Set CI thresholds** - Fail builds on significant regressions

## See Also

- [Development Workflow](development-workflow.md) - Overall development process
- [Spawner Documentation](spawner.md) - Direct bot testing tool
- [Plans: Regression Tester](../plans/regression-tester.md) - Implementation details
