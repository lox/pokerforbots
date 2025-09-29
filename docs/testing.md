# Testing Guide

This guide covers regression testing, statistical validation, and bot improvement workflows using the `pokerforbots regression` command.

## Quick Start

The fastest way to validate bot changes:

```bash
# Test your changes against the previous version (default 10000 hands)
pokerforbots regression --mode heads-up

# More hands for higher confidence
pokerforbots regression --mode heads-up --hands 50000

# Compare specific versions
pokerforbots regression --mode heads-up \
  --challenger "./my-new-bot" \
  --baseline "./my-old-bot" \
  --hands 10000
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
- **Latency**: Response time metrics (p95 should be under 100ms)

### Interpreting Verdicts

- **IMPROVEMENT**: Challenger significantly outperforms baseline (p < 0.05)
- **REGRESSION**: Challenger significantly underperforms baseline (p < 0.05)
- **NO SIGNIFICANT DIFFERENCE**: Changes too small to be statistically meaningful
- **INCONCLUSIVE**: More hands needed for reliable conclusion

## Test Modes

### 1. Heads-Up Mode (Recommended First Test)

Direct 1v1 comparison isolating performance difference:

```bash
pokerforbots regression --mode heads-up --hands 5000
```

- **Use for**: Quick validation of changes, isolating bot-vs-bot performance
- **Sample size**: 5,000+ hands for reliable results
- **Expected variance**: ±5-10 BB/100 is normal

### 2. Population Mode

Tests multiple bots at the same table:

```bash
pokerforbots regression --mode population --hands 10000
```

- **Use for**: Testing multi-way pot handling, field dynamics
- **Configuration**: 2 challenger vs 4 baseline bots (default)
- **Sample size**: 10,000+ hands recommended

### 3. NPC Benchmark Mode

Compare against fixed-strategy opponents:

```bash
pokerforbots regression --mode npc-benchmark --hands 10000
```

**Performance Targets**:
- vs Calling Stations: +25 to +35 BB/100
- vs Random: +15 to +25 BB/100
- vs Aggressive: +5 to +15 BB/100

- **Use for**: Detecting regressions in exploitative play

### 4. Self-Play Mode

Baseline variance measurement:

```bash
pokerforbots regression --mode self-play --hands 2500
```

- **Expected result**: ~0 BB/100 (confirms statistical validity)
- **Use for**: Sanity checking the test framework

### 5. All Modes

Comprehensive test suite with multiple comparison correction:

```bash
pokerforbots regression --mode all --hands 5000
```

Runs all applicable modes and adjusts p-values for multiple testing.

## Sample Size Guidelines

| Confidence Level | Minimum Hands | Use Case |
|-----------------|---------------|----------|
| Low (quick check) | 5,000 | Development iteration |
| Medium | 10,000 | Pre-commit validation (default) |
| High | 25,000+ | Major changes |
| Very High | 50,000+ | Release validation |

The regression tester automatically warns when sample sizes are insufficient:
- **< 5,000 hands**: ⚠️ Small sample warning
- **5,000-10,000**: May need more for small effects
- **10,000+**: Generally sufficient

## Advanced Usage

### Custom Bot Commands

Test any bot implementation:

```bash
pokerforbots regression \
  --mode heads-up \
  --challenger "./my-improved-bot" \
  --baseline "./my-previous-bot" \
  --hands 50000
```

### Deterministic Testing

Use seeds for reproducible results:

```bash
pokerforbots regression \
  --mode population \
  --challenger "./my-bot" \
  --baseline "./baseline-bot" \
  --seeds "42,123,456,789,999" \
  --hands 10000
```

### Batching Strategy

Handle variance with smaller batches:

```bash
pokerforbots regression \
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
pokerforbots regression \
  --output json \
  --output-file results.json

# Summary output only
pokerforbots regression \
  --output summary

# Both JSON and summary (default)
pokerforbots regression \
  --output both \
  --output-file results.json
```

### Latency Warnings

Set custom latency thresholds:

```bash
# Warn when p95 latency exceeds 80ms (default is 100ms)
pokerforbots regression --latency-warn-ms 80
```

## Bot Development Workflow

### 1. Test-First Development

The recommended workflow:

```bash
# Step 1: Establish baseline performance
pokerforbots regression --mode heads-up --hands 5000

# Step 2: Make your changes to the bot
vim my-bot/strategy.go

# Step 3: Validate improvements
pokerforbots regression --mode heads-up --hands 10000

# Step 4: If regression detected, debug with spawn
pokerforbots spawn --log-level debug \
  --spec "calling-station:5" \
  --bot-cmd "./my-bot" \
  --hand-limit 100 --print-stats
```

### 2. Strategy Development Process

#### Initial Development
1. **Start with position play**: Button should be most profitable
2. **Implement preflop ranges**: VPIP 15-25% for 6-max
3. **Add postflop logic**: C-bet frequency, pot odds calculation
4. **Tune aggression**: 3-bet ranges, bluff frequencies

#### Testing Against Known Strategies
```bash
# Exploit passive players (should achieve +25 to +35 BB/100)
pokerforbots spawn --spec "calling-station:5" \
  --bot-cmd "./my-bot" \
  --hand-limit 1000 --print-stats

# Handle aggressive players (should achieve +5 to +15 BB/100)
pokerforbots spawn --spec "aggressive:5" \
  --bot-cmd "./my-bot" \
  --hand-limit 1000 --print-stats

# Mixed field test
pokerforbots spawn --spec "calling-station:2,random:1,aggressive:2" \
  --bot-cmd "./my-bot" \
  --hand-limit 1000 --print-stats
```

#### Performance Benchmarks
- vs Calling Stations: +25 to +35 BB/100
- vs Random: +15 to +25 BB/100
- vs Aggressive: +5 to +15 BB/100
- vs Complex (self): ~0 BB/100

### 3. Statistical Validation

For significant changes, use larger samples:

```bash
# High-confidence test (50k+ hands)
pokerforbots regression \
  --mode heads-up \
  --challenger "./my-bot-v2" \
  --baseline "./my-bot-v1" \
  --hands 50000 \
  --output-file validation.json

# Analyze results
jq '.verdict, .effect_size, .confidence_interval' validation.json
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Bot Regression Test
on: [push, pull_request]

jobs:
  regression:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Build bots
        run: |
          go build -o bot-new ./my-bot
          go build -o bot-stable ./my-bot@stable

      - name: Run regression test
        run: |
          pokerforbots regression \
            --mode npc-benchmark \
            --challenger "./bot-new" \
            --baseline "./bot-stable" \
            --hands 50000 \
            --output json \
            --output-file results.json

      - name: Check for regression
        run: |
          if jq -e '.verdict == "REGRESSION"' results.json; then
            echo "Performance regression detected!"
            exit 1
          fi
```

### Snapshot Testing Workflow

```bash
#!/bin/bash
# Create snapshot of current bot
VERSION=$(git rev-parse --short HEAD)
go build -o snapshots/bot-$VERSION ./my-bot

# Test against previous snapshot
pokerforbots regression \
  --mode heads-up \
  --challenger "snapshots/bot-$VERSION" \
  --baseline "snapshots/bot-previous" \
  --hands 50000

# If improvement confirmed, update latest
ln -sf bot-$VERSION snapshots/bot-latest
```

## Troubleshooting Unexpected Results

### When Regression Tests Show Unexpected Results

Use spawn for debugging:

```bash
# Enable debug logging to see decision-making
pokerforbots spawn --log-level debug \
  --spec "calling-station:5" \
  --bot-cmd "./my-bot" \
  --hand-limit 100 \
  --seed 42 \
  --print-stats
```

### Common Issues

| Problem | Solution |
|---------|----------|
| High variance between runs | Increase hand count or use deterministic seeds |
| Bot losing to calling stations | Check VPIP/PFR stats, ensure value betting |
| Timeouts affecting results | Profile with `--log-level trace`, optimize hot paths |
| Inconsistent test results | Use fixed seeds and ensure deterministic bot logic |

### Debug Workflow

1. **Run regression test** to detect issues
2. **Use spawn with debug logging** to understand behavior
3. **Analyze stats output** for strategic problems
4. **Review specific hands** with trace logging
5. **Fix and re-test** with regression command

## Task Integration

The regression testing integrates with task runners:

```bash
# Using task (if configured in Taskfile.yml)
task regression:heads-up HANDS=5000
task regression:all HANDS=10000

# Or use pokerforbots directly
pokerforbots regression --mode heads-up --hands 5000
```

## Best Practices

1. **Always test with sufficient hands** - 5,000 minimum for meaningful results
2. **Use deterministic seeds** during development for reproducibility
3. **Test against multiple opponent types** to ensure robustness
4. **Monitor latency metrics** to ensure bots meet timing requirements
5. **Validate major changes** with 50,000+ hands before deployment
6. **Keep baseline bots** as snapshots for consistent comparison
7. **Use CI/CD integration** to catch regressions automatically

## Next Steps

- See [Quick Start Guide](quickstart.md) for basic bot testing with spawn
- See [Command Reference](reference.md) for all regression command options
- See [SDK Documentation](sdk.md) for building advanced bot strategies