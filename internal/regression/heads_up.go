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
		BotA:   config.BotA,
		BotB:   config.BotB,
		Config: config,
	}

	// Use common batch executor
	batches, err := orchestrator.ExecuteBatches(ctx, strategy, config.HandsTotal)
	if err != nil {
		return nil, fmt.Errorf("failed to execute batches: %w", err)
	}

	// Aggregate results
	aggregate := aggregateHeadsUpResults(batches)

	// Calculate verdict
	verdict := calculateVerdict(aggregate, config)

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
			BotA:                   config.BotA,
			BotB:                   config.BotB,
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
func aggregateHeadsUpResults(batches []BatchResult) AggregateResults {
	// Calculate means and confidence intervals using weighted averages
	var botASum, botBSum float64
	var botACount, botBCount int

	for _, batch := range batches {
		// Use actual hands completed from stats if available, otherwise use requested
		actualHandsA := batch.Hands
		if handsFromStats, exists := batch.Results["bot_a_hands"]; exists {
			actualHandsA = int(handsFromStats)
		}

		actualHandsB := batch.Hands
		if handsFromStats, exists := batch.Results["bot_b_hands"]; exists {
			actualHandsB = int(handsFromStats)
		}

		if val, ok := batch.Results["bot_a_bb_per_100"]; ok {
			botASum += val * float64(actualHandsA)
			botACount += actualHandsA
		}
		if val, ok := batch.Results["bot_b_bb_per_100"]; ok {
			botBSum += val * float64(actualHandsB)
			botBCount += actualHandsB
		}
	}

	botAMean := botASum / float64(botACount)
	botBMean := botBSum / float64(botBCount)

	// TODO: Calculate proper confidence intervals
	// For now, use simple approximation
	margin := 2.0 // Placeholder

	return AggregateResults{
		BotA: &BotResults{
			BBPer100: botAMean,
			CI95Low:  botAMean - margin,
			CI95High: botAMean + margin,
			VPIP:     0.45, // TODO: Aggregate from batches
			PFR:      0.35,
		},
		BotB: &BotResults{
			BBPer100: botBMean,
			CI95Low:  botBMean - margin,
			CI95High: botBMean + margin,
			VPIP:     0.42,
			PFR:      0.30,
		},
	}
}

// calculateVerdict calculates the test verdict
func calculateVerdict(aggregate AggregateResults, config *Config) TestVerdict {
	// Simple verdict calculation for demonstration
	// TODO: Implement proper statistical tests

	pValue := 0.02    // Placeholder
	effectSize := 0.3 // Placeholder

	// Don't apply correction here - it's done in the runner for "all" mode
	adjustedPValue := pValue

	significant := adjustedPValue < config.SignificanceLevel
	direction := "neutral"
	recommendation := "inconclusive"

	// Determine direction based on which bot is being tested
	// Bot A is typically the baseline/old version
	// Bot B is typically the challenger/new version
	if aggregate.BotA != nil && aggregate.BotB != nil {
		if aggregate.BotB.BBPer100 > aggregate.BotA.BBPer100 {
			// Bot B (new) beats Bot A (old) = improvement
			direction = "improvement"
			if significant {
				recommendation = "accept"
			}
		} else {
			// Bot A (old) beats Bot B (new) = regression
			direction = "regression"
			if significant {
				recommendation = "reject"
			}
		}
	}

	return TestVerdict{
		SignificantDifference: significant,
		PValue:                pValue,
		AdjustedPValue:        adjustedPValue,
		EffectSize:            effectSize,
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
