package regression

import (
	"context"
	"fmt"
	"os"
	"time"
)

// RunHeadsUpTest runs a complete heads-up regression test
func RunHeadsUpTest(ctx context.Context, config *Config, orchestrator *Orchestrator) (*TestResult, error) {
	startTime := time.Now()
	testID := fmt.Sprintf("heads-up-%d", startTime.Unix())

	// Create heads-up strategy
	strategy := &HeadsUpStrategy{
		Challenger: config.Challenger,
		Baseline:   config.Baseline,
		Config:     config,
	}

	// Use common batch executor
	batches, err := orchestrator.ExecuteBatches(ctx, strategy, config.HandsTotal)
	if err != nil {
		return nil, fmt.Errorf("failed to execute batches: %w", err)
	}

	// Calculate proper statistics
	challengerStats := CalculateStatistics(batches, "challenger_bb_per_100", "challenger_hands")
	baselineStats := CalculateStatistics(batches, "baseline_bb_per_100", "baseline_hands")

	// Perform statistical comparison
	comparison := CompareStatistics(challengerStats, baselineStats)

	// Aggregate results
	aggregate := aggregateHeadsUpResults(batches, challengerStats, baselineStats, comparison)

	// Calculate verdict
	verdict := calculateVerdict(comparison, config)

	// Get error summary
	crashes, timeouts, recovered := orchestrator.healthMonitor.GetErrorSummary()

	// Build result
	result := &TestResult{
		TestID: testID,
		Mode:   ModeHeadsUp,
		Metadata: TestMetadata{
			StartTime:       startTime,
			DurationSeconds: time.Since(startTime).Seconds(),
			ServerVersion:   "1.0.0", // TODO: Get actual version
			TestEnvironment: getTestEnvironment(),
		},
		Config: TestConfigSummary{
			Challenger:             config.Challenger,
			Baseline:               config.Baseline,
			HandsTotal:             config.HandsTotal,
			Batches:                len(batches),
			BatchSize:              config.BatchSize,
			SignificanceLevel:      config.SignificanceLevel,
			MultipleTestCorrection: config.MultipleTestCorrection,
		},
		Batches:   batches,
		Aggregate: aggregate,
		Performance: PerformanceMetrics{
			HandsPerSecond: float64(config.HandsTotal) / time.Since(startTime).Seconds(),
		},
		Errors: ErrorSummary{
			BotCrashes:       crashes,
			Timeouts:         timeouts,
			RecoveredCrashes: recovered,
		},
		Verdict: verdict,
	}

	return result, nil
}

// aggregateHeadsUpResults aggregates batch results for heads-up mode
func aggregateHeadsUpResults(batches []BatchResult, challengerStats, baselineStats StatisticalResult, comparison StatisticalComparison) AggregateResults {
	// Get aggregated VPIP/PFR from batches
	challengerCombined := CombineBatches(batches, "challenger")
	baselineCombined := CombineBatches(batches, "baseline")

	return AggregateResults{
		Challenger: &BotResults{
			BBPer100:            challengerStats.Mean,
			CI95Low:             challengerStats.CI95Low,
			CI95High:            challengerStats.CI95High,
			VPIP:                challengerCombined.VPIP,
			PFR:                 challengerCombined.PFR,
			EffectSize:          comparison.EffectSize,
			AvgResponseMs:       challengerCombined.AvgResponseMs,
			P95ResponseMs:       challengerCombined.P95ResponseMs,
			MaxResponseMs:       challengerCombined.MaxResponseMs,
			MinResponseMs:       challengerCombined.MinResponseMs,
			ResponseStdMs:       challengerCombined.ResponseStdMs,
			ResponsesTracked:    challengerCombined.ResponsesTracked,
			ResponseTimeouts:    challengerCombined.ResponseTimeouts,
			ResponseDisconnects: challengerCombined.ResponseDisconnects,
		},
		Baseline: &BotResults{
			BBPer100:            baselineStats.Mean,
			CI95Low:             baselineStats.CI95Low,
			CI95High:            baselineStats.CI95High,
			VPIP:                baselineCombined.VPIP,
			PFR:                 baselineCombined.PFR,
			AvgResponseMs:       baselineCombined.AvgResponseMs,
			P95ResponseMs:       baselineCombined.P95ResponseMs,
			MaxResponseMs:       baselineCombined.MaxResponseMs,
			MinResponseMs:       baselineCombined.MinResponseMs,
			ResponseStdMs:       baselineCombined.ResponseStdMs,
			ResponsesTracked:    baselineCombined.ResponsesTracked,
			ResponseTimeouts:    baselineCombined.ResponseTimeouts,
			ResponseDisconnects: baselineCombined.ResponseDisconnects,
		},
	}
}

// calculateVerdict calculates the test verdict
func calculateVerdict(comparison StatisticalComparison, config *Config) TestVerdict {
	// comparison already contains all the statistical results we need

	// Determine direction
	direction := "no_change"
	if comparison.Difference > 0 {
		direction = "improvement"
	} else if comparison.Difference < 0 {
		direction = "regression"
	}

	// Determine recommendation based on significance and effect size
	recommendation := "inconclusive"
	if comparison.PValue < config.SignificanceLevel {
		effectMagnitude := InterpretEffectSize(comparison.EffectSize)
		switch direction {
		case "improvement":
			if effectMagnitude == "large" || effectMagnitude == "medium" {
				recommendation = "accept"
			} else {
				recommendation = "marginal"
			}
		case "regression":
			if effectMagnitude == "large" || effectMagnitude == "medium" {
				recommendation = "reject"
			} else {
				recommendation = "marginal"
			}
		}
	}

	return TestVerdict{
		SignificantDifference: comparison.PValue < config.SignificanceLevel,
		PValue:                comparison.PValue,
		AdjustedPValue:        comparison.PValue, // No correction in single test
		EffectSize:            comparison.EffectSize,
		Direction:             direction,
		Confidence:            0.95,
		Recommendation:        recommendation,
	}
}

// getTestEnvironment returns the test environment
func getTestEnvironment() string {
	if ci := os.Getenv("CI"); ci != "" {
		return "CI"
	}
	return "local"
}
