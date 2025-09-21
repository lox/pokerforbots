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

	// Use BatchSize for consistent batch sizing across all modes
	handsPerBatch := config.BatchSize
	remainingHands := config.HandsTotal

	// Generate seeds if needed (same as NPC mode)
	seeds := config.Seeds
	if len(seeds) == 0 {
		seeds = []int64{42} // Default seed
	}

	config.Logger.Info().
		Int("hands_total", config.HandsTotal).
		Int("batch_size", handsPerBatch).
		Msg("Running heads-up test batches")

	// Run batches
	var batches []BatchResult
	batchNum := 0
	for remainingHands > 0 {
		batchHands := min(handsPerBatch, remainingHands)

		// Get or generate seed for this batch
		var seed int64
		if batchNum < len(seeds) {
			seed = seeds[batchNum]
		} else {
			// Generate additional seed based on last available seed
			seed = seeds[len(seeds)-1] + int64(batchNum-len(seeds)+1)*1000
		}

		config.Logger.Info().
			Int("batch", batchNum+1).
			Int64("seed", seed).
			Int("hands", batchHands).
			Msg("Running batch")

		batch, err := orchestrator.RunHeadsUpBatch(ctx, config.BotA, config.BotB, seed, batchHands)
		if err != nil {
			return nil, fmt.Errorf("batch %d failed: %w", batchNum+1, err)
		}

		batches = append(batches, *batch)
		remainingHands -= batchHands
		batchNum++

		// Check for early stopping
		if config.EarlyStopping && batchNum*handsPerBatch >= config.MinHands {
			// TODO: Check if significance reached
			break
		}
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
			BatchSize:              handsPerBatch,
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
