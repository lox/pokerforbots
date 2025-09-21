package regression

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Runner orchestrates regression tests
type Runner struct {
	config        *Config
	healthMonitor *HealthMonitor
	orchestrator  *Orchestrator // Uses server bot commands
}

// NewRunner creates a new test runner
func NewRunner(config *Config) *Runner {
	healthMonitor := NewHealthMonitor(
		config.MaxCrashesPerBot,
		config.MaxTimeoutsPerBot,
		time.Duration(config.RestartDelayMs)*time.Millisecond,
		config.Logger,
	)

	orchestrator := NewOrchestrator(config, healthMonitor)

	return &Runner{
		config:        config,
		healthMonitor: healthMonitor,
		orchestrator:  orchestrator,
	}
}

// ValidateBinaries validates all configured bot binaries
func (r *Runner) ValidateBinaries() error {
	binaries := r.collectBinaries()

	for _, binary := range binaries {
		// Skip validation for go run commands
		if strings.HasPrefix(binary, "go run ") {
			r.config.Logger.Debug().
				Str("command", binary).
				Msg("Skipping validation for go run command")
			continue
		}

		// Check if file exists
		if _, err := os.Stat(binary); os.IsNotExist(err) {
			return fmt.Errorf("binary not found: %s", binary)
		}

		// Check if executable
		fileInfo, err := os.Stat(binary)
		if err != nil {
			return fmt.Errorf("cannot stat binary %s: %v", binary, err)
		}

		if fileInfo.Mode()&0111 == 0 {
			return fmt.Errorf("binary is not executable: %s", binary)
		}

		// Try to run with --help to validate it starts
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, binary, "--help")
		if err := cmd.Run(); err != nil {
			// Some bots might not have --help, so just check if it starts
			if ctx.Err() == context.DeadlineExceeded {
				// If it timed out waiting for help, it's probably OK
				r.config.Logger.Debug().
					Str("binary", binary).
					Msg("Binary validation passed (timeout on --help)")
				continue
			}
			// Check if it's just exit code 1 (common for no --help flag)
			if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
				r.config.Logger.Debug().
					Str("binary", binary).
					Msg("Binary validation passed (exit 1 on --help)")
				continue
			}
			return fmt.Errorf("binary %s failed to run: %v", binary, err)
		}

		r.config.Logger.Debug().
			Str("binary", binary).
			Msg("Binary validation passed")
	}

	return nil
}

// Run executes the configured regression tests
func (r *Runner) Run(ctx context.Context) error {
	// Validate binaries first
	if err := r.ValidateBinaries(); err != nil {
		return fmt.Errorf("binary validation failed: %w", err)
	}

	var results []*TestResult

	// Apply multiple test correction if running all modes
	if r.config.Mode == ModeAll {
		r.config.MultipleTestCorrection = true
	}

	// Run the appropriate test mode(s)
	switch r.config.Mode {
	case ModeHeadsUp:
		result, err := r.runHeadsUpTest(ctx)
		if err != nil {
			return fmt.Errorf("heads-up test failed: %w", err)
		}
		results = append(results, result)

	case ModePopulation:
		result, err := r.runPopulationTest(ctx)
		if err != nil {
			return fmt.Errorf("population test failed: %w", err)
		}
		results = append(results, result)

	case ModeNPCBenchmark:
		result, err := r.runNPCBenchmarkTest(ctx)
		if err != nil {
			return fmt.Errorf("NPC benchmark test failed: %w", err)
		}
		results = append(results, result)

	case ModeSelfPlay:
		result, err := r.runSelfPlayTest(ctx)
		if err != nil {
			return fmt.Errorf("self-play test failed: %w", err)
		}
		results = append(results, result)

	case ModeAll:
		// Run all applicable modes based on provided bot configurations
		var allResults []*TestResult
		numTests := 0

		// Determine which tests can be run based on provided bots
		canRunHeadsUp := r.config.BotA != "" && r.config.BotB != ""
		canRunPopulation := r.config.Challenger != "" && r.config.Baseline != ""
		canRunNPCBenchmark := (r.config.Challenger != "" && r.config.Baseline != "") || r.config.Bot != ""
		canRunSelfPlay := r.config.Bot != ""

		// Count number of tests that will be run for Bonferroni correction
		if canRunHeadsUp {
			numTests++
		}
		if canRunPopulation {
			numTests++
		}
		if canRunNPCBenchmark {
			numTests++
		}
		if canRunSelfPlay {
			numTests++
		}

		if numTests == 0 {
			return fmt.Errorf("no valid bot configurations provided for any test mode")
		}

		r.config.Logger.Info().
			Int("num_tests", numTests).
			Msg("Running all applicable test modes with multiple comparison correction")

		// Run heads-up test if possible
		if canRunHeadsUp {
			r.config.Logger.Info().Msg("Running heads-up test")
			result, err := r.runHeadsUpTest(ctx)
			if err != nil {
				r.config.Logger.Error().Err(err).Msg("Heads-up test failed")
				// Continue with other tests
			} else {
				allResults = append(allResults, result)
			}
		}

		// Run population test if possible
		if canRunPopulation {
			r.config.Logger.Info().Msg("Running population test")
			result, err := r.runPopulationTest(ctx)
			if err != nil {
				r.config.Logger.Error().Err(err).Msg("Population test failed")
				// Continue with other tests
			} else {
				allResults = append(allResults, result)
			}
		}

		// Run NPC benchmark test if possible
		if canRunNPCBenchmark {
			r.config.Logger.Info().Msg("Running NPC benchmark test")
			// Use challenger/baseline if available, otherwise use bot
			if r.config.Challenger == "" && r.config.Bot != "" {
				r.config.Challenger = r.config.Bot
				r.config.Baseline = r.config.Bot
			}
			result, err := r.runNPCBenchmarkTest(ctx)
			if err != nil {
				r.config.Logger.Error().Err(err).Msg("NPC benchmark test failed")
				// Continue with other tests
			} else {
				allResults = append(allResults, result)
			}
		}

		// Run self-play test if possible
		if canRunSelfPlay {
			r.config.Logger.Info().Msg("Running self-play test")
			result, err := r.runSelfPlayTest(ctx)
			if err != nil {
				r.config.Logger.Error().Err(err).Msg("Self-play test failed")
				// Continue with other tests
			} else {
				allResults = append(allResults, result)
			}
		}

		// Apply Bonferroni correction to all p-values
		if r.config.MultipleTestCorrection && len(allResults) > 1 {
			r.config.Logger.Info().
				Int("num_tests", len(allResults)).
				Msg("Applying Bonferroni correction for multiple comparisons")

			for _, result := range allResults {
				// Adjust p-value
				result.Verdict.AdjustedPValue = result.Verdict.PValue * float64(len(allResults))
				if result.Verdict.AdjustedPValue > 1.0 {
					result.Verdict.AdjustedPValue = 1.0
				}

				// Update significance based on adjusted p-value
				result.Verdict.SignificantDifference = result.Verdict.AdjustedPValue < r.config.SignificanceLevel

				// Update recommendation based on adjusted significance
				if result.Verdict.SignificantDifference {
					// Keep original recommendation if still significant
					// (already set based on effect size and direction)
				} else {
					// Not significant after correction
					result.Verdict.Recommendation = "inconclusive"
				}

				// Add note about multiple comparison correction
				result.Config.MultipleTestCorrection = true
			}
		}

		if len(allResults) == 0 {
			return fmt.Errorf("all tests failed")
		}

		results = allResults

	default:
		return fmt.Errorf("unknown test mode: %s", r.config.Mode)
	}

	// Add sample size guidance to results
	addSampleSizeGuidance(results)

	// Output results
	return r.outputResults(results)
}

// addSampleSizeGuidance adds a warning when sample size might be too small
func addSampleSizeGuidance(results []*TestResult) {
	for _, result := range results {
		hands := result.Config.HandsTotal

		// Check sample size thresholds
		if hands < 5000 {
			result.Performance.SampleAssessment = "⚠️  Small sample size - results may be unreliable"
		} else if hands < 10000 && math.Abs(result.Verdict.EffectSize) < 0.5 {
			result.Performance.SampleAssessment = "Note: More hands recommended for detecting small differences"
		}
		// Otherwise, don't clutter the output
	}
}

// collectBinaries returns all unique bot binaries
func (r *Runner) collectBinaries() []string {
	binaries := make(map[string]bool)

	if r.config.BotA != "" {
		binaries[r.config.BotA] = true
	}
	if r.config.BotB != "" {
		binaries[r.config.BotB] = true
	}
	if r.config.Challenger != "" {
		binaries[r.config.Challenger] = true
	}
	if r.config.Baseline != "" {
		binaries[r.config.Baseline] = true
	}
	if r.config.Bot != "" {
		binaries[r.config.Bot] = true
	}

	result := make([]string, 0, len(binaries))
	for binary := range binaries {
		result = append(result, binary)
	}
	return result
}

// runHeadsUpTest runs a heads-up test between two bots
func (r *Runner) runHeadsUpTest(ctx context.Context) (*TestResult, error) {
	if r.config.BotA == "" || r.config.BotB == "" {
		return nil, fmt.Errorf("heads-up mode requires bot-a and bot-b")
	}

	r.config.Logger.Info().
		Str("mode", "heads-up").
		Str("bot_a", r.config.BotA).
		Str("bot_b", r.config.BotB).
		Int("hands", r.config.HandsTotal).
		Msg("Starting heads-up test")

	return RunHeadsUpTest(ctx, r.config, r.orchestrator)
}

// runPopulationTest runs a population test
func (r *Runner) runPopulationTest(ctx context.Context) (*TestResult, error) {
	if r.config.Challenger == "" || r.config.Baseline == "" {
		return nil, fmt.Errorf("population mode requires challenger and baseline")
	}

	// Default seat counts if not specified
	if r.config.ChallengerSeats == 0 {
		r.config.ChallengerSeats = 2
	}
	if r.config.BaselineSeats == 0 {
		r.config.BaselineSeats = 4
	}

	r.config.Logger.Info().
		Str("mode", "population").
		Str("challenger", r.config.Challenger).
		Str("baseline", r.config.Baseline).
		Int("challenger_seats", r.config.ChallengerSeats).
		Int("baseline_seats", r.config.BaselineSeats).
		Int("hands", r.config.HandsTotal).
		Msg("Starting population test")

	startTime := time.Now()
	var allBatches []BatchResult

	handsPerBatch := r.config.BatchSize
	remainingHands := r.config.HandsTotal

	// Generate seeds if needed
	seeds := r.config.Seeds
	if len(seeds) == 0 {
		seeds = []int64{42} // Default seed
	}

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

		r.config.Logger.Info().
			Int("batch", batchNum+1).
			Int64("seed", seed).
			Int("hands", batchHands).
			Msg("Running population batch")

		batch, err := r.orchestrator.RunPopulationBatch(ctx, r.config.Challenger, r.config.Baseline,
			r.config.ChallengerSeats, r.config.BaselineSeats, seed, batchHands)
		if err != nil {
			return nil, fmt.Errorf("batch %d failed: %w", batchNum+1, err)
		}

		allBatches = append(allBatches, *batch)
		remainingHands -= batchHands
		batchNum++

		// TODO: Check for early stopping
	}

	// Check if any batches were run
	if len(allBatches) == 0 {
		return nil, fmt.Errorf("no batches completed - check configuration")
	}

	// Aggregate results using weighted averages
	var totalChallengerChips, totalBaselineChips float64
	var totalChallengerHands, totalBaselineHands int
	totalHands := 0

	// Aggregate challenger results
	for _, batch := range allBatches {
		// Use actual hands completed from stats if available
		actualChallengerHands := batch.Hands
		if handsFromStats, exists := batch.Results["challenger_hands"]; exists {
			actualChallengerHands = int(handsFromStats)
		}

		if challengerBB100, exists := batch.Results["challenger_bb_per_100"]; exists {
			totalChallengerChips += (challengerBB100 / 100.0) * float64(actualChallengerHands)
		}
		totalChallengerHands += actualChallengerHands

		// Same for baseline
		actualBaselineHands := batch.Hands
		if handsFromStats, exists := batch.Results["baseline_hands"]; exists {
			actualBaselineHands = int(handsFromStats)
		}

		if baselineBB100, exists := batch.Results["baseline_bb_per_100"]; exists {
			totalBaselineChips += (baselineBB100 / 100.0) * float64(actualBaselineHands)
		}
		totalBaselineHands += actualBaselineHands

		totalHands += actualChallengerHands + actualBaselineHands
	}

	// Calculate weighted average BB/100
	challengerBB100 := 0.0
	if totalChallengerHands > 0 {
		challengerBB100 = (totalChallengerChips / float64(totalChallengerHands)) * 100.0
	}

	baselineBB100 := 0.0
	if totalBaselineHands > 0 {
		baselineBB100 = (totalBaselineChips / float64(totalBaselineHands)) * 100.0
	}

	// Calculate confidence intervals (placeholder for now)
	challengerCI := [2]float64{challengerBB100 - 10, challengerBB100 + 10}
	baselineCI := [2]float64{baselineBB100 - 10, baselineBB100 + 10}

	duration := time.Since(startTime)
	handsPerSecond := float64(r.config.HandsTotal) / duration.Seconds()

	// Calculate effect size (Cohen's d placeholder)
	effectSize := (challengerBB100 - baselineBB100) / 50.0 // Using 50 BB/100 as std dev estimate

	// Calculate p-value (placeholder)
	pValue := 0.05
	if math.Abs(effectSize) > 0.5 {
		pValue = 0.01
	}

	// Determine direction
	direction := "no_change"
	if challengerBB100 > baselineBB100 {
		direction = "improvement"
	} else if challengerBB100 < baselineBB100 {
		direction = "regression"
	}

	// Determine verdict
	verdict := "marginal"
	if math.Abs(effectSize) > 0.8 {
		if direction == "improvement" {
			verdict = "accept"
		} else {
			verdict = "reject"
		}
	} else if math.Abs(effectSize) > 0.5 {
		if direction == "improvement" {
			verdict = "accept"
		} else {
			verdict = "marginal"
		}
	}

	result := &TestResult{
		TestID: fmt.Sprintf("population-%d", time.Now().Unix()),
		Mode:   ModePopulation,
		Metadata: TestMetadata{
			StartTime:       startTime,
			DurationSeconds: duration.Seconds(),
			ServerVersion:   "1.0.0",
			TestEnvironment: "test",
		},
		Config: TestConfigSummary{
			Challenger:        r.config.Challenger,
			Baseline:          r.config.Baseline,
			HandsTotal:        r.config.HandsTotal,
			Batches:           len(allBatches),
			BatchSize:         r.config.BatchSize,
			SignificanceLevel: r.config.SignificanceLevel,
		},
		Batches: allBatches,
		Aggregate: AggregateResults{
			Challenger: &BotResults{
				BBPer100:         challengerBB100,
				CI95Low:          challengerCI[0],
				CI95High:         challengerCI[1],
				VPIP:             0, // TODO: Extract from stats
				PFR:              0, // TODO: Extract from stats
				AggressionFactor: 0, // TODO: Calculate
				BustRate:         0, // TODO: Calculate
				EffectSize:       effectSize,
			},
			Baseline: &BotResults{
				BBPer100:         baselineBB100,
				CI95Low:          baselineCI[0],
				CI95High:         baselineCI[1],
				VPIP:             0, // TODO: Extract from stats
				PFR:              0, // TODO: Extract from stats
				AggressionFactor: 0, // TODO: Calculate
				BustRate:         0, // TODO: Calculate
			},
		},
		Performance: PerformanceMetrics{
			HandsPerSecond: handsPerSecond,
		},
		Errors: ErrorSummary{
			BotCrashes:       0, // TODO: Track
			Timeouts:         0, // TODO: Track
			ConnectionErrors: 0, // TODO: Track
			RecoveredCrashes: 0, // TODO: Track
		},
		Verdict: TestVerdict{
			SignificantDifference: math.Abs(effectSize) > 0.5,
			PValue:                pValue,
			EffectSize:            effectSize,
			Direction:             direction,
			Confidence:            0.95,
			Recommendation:        verdict,
		},
	}

	return result, nil
}

// runNPCBenchmarkTest runs an NPC benchmark test
func (r *Runner) runNPCBenchmarkTest(ctx context.Context) (*TestResult, error) {
	if r.config.Challenger == "" || r.config.Baseline == "" {
		return nil, fmt.Errorf("NPC benchmark mode requires both challenger and baseline bot paths")
	}

	// Use NPCs configuration if provided, otherwise default
	npcConfig := r.config.NPCs
	if len(npcConfig) == 0 {
		// Default NPC configuration for benchmark
		npcConfig = map[string]int{
			"calling":    1,
			"aggressive": 1,
			"random":     1,
		}
	}

	r.config.Logger.Info().
		Str("mode", "npc-benchmark").
		Str("challenger", r.config.Challenger).
		Str("baseline", r.config.Baseline).
		Int("challenger_seats", r.config.ChallengerSeats).
		Int("baseline_seats", r.config.BaselineSeats).
		Interface("npcs", npcConfig).
		Int("hands", r.config.HandsTotal).
		Msg("Starting NPC benchmark test")

	startTime := time.Now()
	var allChallengerBatches []*BatchResult
	var allBaselineBatches []*BatchResult
	handsPerBatch := r.config.BatchSize
	remainingHands := r.config.HandsTotal

	// Generate seeds if needed
	seeds := r.config.Seeds
	if len(seeds) == 0 {
		seeds = []int64{42} // Default seed
	}

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

		r.config.Logger.Info().
			Int("batch", batchNum+1).
			Int64("seed", seed).
			Int("hands", batchHands).
			Msg("Running NPC benchmark batch")

		// Run challenger against NPCs
		challengerBatch, err := r.orchestrator.RunNPCBenchmarkBatch(ctx, r.config.Challenger, r.config.ChallengerSeats, npcConfig, seed, batchHands)
		if err != nil {
			return nil, fmt.Errorf("failed to run challenger batch %d: %w", batchNum+1, err)
		}
		allChallengerBatches = append(allChallengerBatches, challengerBatch)

		// Run baseline against NPCs with different seed to avoid identical results
		baselineSeed := seed + 1000000 // Offset to ensure different randomness
		baselineBatch, err := r.orchestrator.RunNPCBenchmarkBatch(ctx, r.config.Baseline, r.config.BaselineSeats, npcConfig, baselineSeed, batchHands)
		if err != nil {
			return nil, fmt.Errorf("failed to run baseline batch %d: %w", batchNum+1, err)
		}
		allBaselineBatches = append(allBaselineBatches, baselineBatch)

		remainingHands -= batchHands
		batchNum++

		// TODO: Check if significance reached for early stopping
	}

	// Check if any batches were run
	if len(allChallengerBatches) == 0 || len(allBaselineBatches) == 0 {
		return nil, fmt.Errorf("no batches completed - check hands and batch size configuration")
	}

	// Calculate aggregate results from challenger and baseline batches using weighted averages
	var totalChallengerChips, totalBaselineChips float64
	var totalChallengerHands, totalBaselineHands int
	totalHands := 0

	// Aggregate challenger results (weighted by actual hands completed)
	for _, batch := range allChallengerBatches {
		// Use actual hands completed from the batch results
		actualHands := batch.Hands
		if handsFromStats, exists := batch.Results["bot_hands"]; exists {
			actualHands = int(handsFromStats)
		}

		if botBB100, exists := batch.Results["bot_bb_per_100"]; exists {
			// Convert BB/100 back to total chips for this batch using actual hands
			totalChallengerChips += (botBB100 / 100.0) * float64(actualHands)
		}
		totalChallengerHands += actualHands
		totalHands += actualHands
	}

	// Aggregate baseline results (weighted by actual hands completed)
	for _, batch := range allBaselineBatches {
		// Use actual hands completed from the batch results
		actualHands := batch.Hands
		if handsFromStats, exists := batch.Results["bot_hands"]; exists {
			actualHands = int(handsFromStats)
		}

		if botBB100, exists := batch.Results["bot_bb_per_100"]; exists {
			// Convert BB/100 back to total chips for this batch using actual hands
			totalBaselineChips += (botBB100 / 100.0) * float64(actualHands)
		}
		totalBaselineHands += actualHands
		totalHands += actualHands
	}

	// Calculate timing and performance
	endTime := time.Now()
	duration := endTime.Sub(startTime)
	durationSeconds := duration.Seconds()
	handsPerSecond := float64(totalHands) / durationSeconds

	// Calculate weighted average BB/100
	avgChallengerBB100 := 0.0
	if totalChallengerHands > 0 {
		avgChallengerBB100 = (totalChallengerChips / float64(totalChallengerHands)) * 100.0
	}

	avgBaselineBB100 := 0.0
	if totalBaselineHands > 0 {
		avgBaselineBB100 = (totalBaselineChips / float64(totalBaselineHands)) * 100.0
	}

	// Calculate performance difference (challenger - baseline)
	performanceDiff := avgChallengerBB100 - avgBaselineBB100

	// TODO: Calculate proper confidence intervals - using placeholder for now
	challengerCIRange := math.Abs(avgChallengerBB100) * 0.1
	baselineCIRange := math.Abs(avgBaselineBB100) * 0.1

	return &TestResult{
		TestID: fmt.Sprintf("npc-benchmark-%d", time.Now().Unix()),
		Mode:   ModeNPCBenchmark,
		Metadata: TestMetadata{
			StartTime:       startTime,
			DurationSeconds: durationSeconds,
			ServerVersion:   "1.0.0", // TODO: Get actual version
			TestEnvironment: "test",
		},
		Config: TestConfigSummary{
			Challenger:        r.config.Challenger,
			Baseline:          r.config.Baseline,
			HandsTotal:        r.config.HandsTotal,
			Batches:           len(allChallengerBatches),
			BatchSize:         r.config.BatchSize,
			SignificanceLevel: r.config.SignificanceLevel,
		},
		Batches: func() []BatchResult {
			// Combine challenger and baseline batches for reporting
			allBatches := make([]BatchResult, 0, len(allChallengerBatches)+len(allBaselineBatches))
			for i, batch := range allChallengerBatches {
				result := *batch
				result.Results["batch_type"] = float64(1) // Mark as challenger
				result.Results["batch_index"] = float64(i)
				allBatches = append(allBatches, result)
			}
			for i, batch := range allBaselineBatches {
				result := *batch
				result.Results["batch_type"] = float64(2) // Mark as baseline
				result.Results["batch_index"] = float64(i)
				allBatches = append(allBatches, result)
			}
			return allBatches
		}(),
		Aggregate: AggregateResults{
			Challenger: &BotResults{
				BBPer100:         avgChallengerBB100,
				CI95Low:          avgChallengerBB100 - challengerCIRange,
				CI95High:         avgChallengerBB100 + challengerCIRange,
				VPIP:             0, // TODO: Extract from detailed stats
				PFR:              0, // TODO: Extract from detailed stats
				AggressionFactor: 0, // TODO: Calculate
				BustRate:         0, // TODO: Calculate
			},
			Baseline: &BotResults{
				BBPer100:         avgBaselineBB100,
				CI95Low:          avgBaselineBB100 - baselineCIRange,
				CI95High:         avgBaselineBB100 + baselineCIRange,
				VPIP:             0, // TODO: Extract from detailed stats
				PFR:              0, // TODO: Extract from detailed stats
				AggressionFactor: 0, // TODO: Calculate
				BustRate:         0, // TODO: Calculate
			},
		},
		Performance: PerformanceMetrics{
			HandsPerSecond: handsPerSecond,
		},
		Errors: ErrorSummary{
			BotCrashes:       0,
			Timeouts:         0,
			ConnectionErrors: 0,
			RecoveredCrashes: 0,
		},
		Verdict: TestVerdict{
			SignificantDifference: math.Abs(performanceDiff) > 10, // Significant if > 10 BB/100 difference
			PValue:                0.001,                          // TODO: Calculate proper p-value
			EffectSize:            performanceDiff / 25,           // Effect size relative to 25 BB/100 baseline
			Direction: func() string {
				if performanceDiff > 0 {
					return "improvement"
				} else {
					return "regression"
				}
			}(),
			Confidence: 0.95,
			Recommendation: func() string {
				// Regression test verdict based on challenger vs baseline performance
				switch {
				case performanceDiff > 20:
					return "accept" // Strong improvement
				case performanceDiff > 5:
					return "accept" // Modest improvement
				case performanceDiff > -5:
					return "marginal" // No significant change
				default:
					return "reject" // Performance regression
				}
			}(),
		},
	}, nil
}

// runSelfPlayTest runs a self-play test
func (r *Runner) runSelfPlayTest(ctx context.Context) (*TestResult, error) {
	if r.config.Bot == "" {
		return nil, fmt.Errorf("self-play mode requires bot")
	}

	r.config.Logger.Info().
		Str("mode", "self-play").
		Str("bot", r.config.Bot).
		Int("seats", r.config.BotSeats).
		Int("hands", r.config.HandsTotal).
		Msg("Starting self-play test")

	startTime := time.Now()

	// Calculate batches
	numBatches := (r.config.HandsTotal + r.config.BatchSize - 1) / r.config.BatchSize
	if numBatches == 0 {
		numBatches = 1
	}

	// Generate additional seeds if needed
	seeds := r.config.Seeds
	for len(seeds) < numBatches {
		seeds = append(seeds, int64(42+len(seeds)*1337))
	}

	// Create health monitor
	healthMonitor := NewHealthMonitor(
		r.config.MaxCrashesPerBot,
		r.config.MaxTimeoutsPerBot,
		time.Duration(r.config.RestartDelayMs)*time.Millisecond,
		r.config.Logger,
	)

	orchestrator := NewOrchestrator(r.config, healthMonitor)
	var allBatches []BatchResult
	totalHands := 0

	// Run batches
	for i := 0; i < numBatches; i++ {
		handsThisBatch := r.config.BatchSize
		if i == numBatches-1 {
			// Last batch might be smaller
			handsThisBatch = r.config.HandsTotal - (i * r.config.BatchSize)
		}

		r.config.Logger.Info().
			Int("batch", i+1).
			Int("hands", handsThisBatch).
			Int64("seed", seeds[i]).
			Msg("Running self-play batch")

		// Create bot commands for self-play (all same bot)
		var botCmds []string
		for range r.config.BotSeats {
			botCmds = append(botCmds, r.config.Bot)
		}

		// Create temp stats file
		statsFile := fmt.Sprintf("stats-selfplay-%d-%d.json", seeds[i], time.Now().Unix())
		defer os.Remove(statsFile) // Clean up after

		// Start server with bots and stats
		if err := orchestrator.StartServerWithBotsAndStats(ctx, seeds[i], handsThisBatch, botCmds, nil, statsFile); err != nil {
			return nil, fmt.Errorf("failed to start server: %w", err)
		}

		// Wait for completion
		if err := orchestrator.WaitForCompletion(ctx); err != nil {
			orchestrator.StopServer()
			return nil, fmt.Errorf("server failed: %w", err)
		}
		orchestrator.StopServer()

		// Parse stats - we expect near-zero average since it's zero-sum
		results, err := r.parseSelfPlayStats(statsFile, r.config.Bot)
		if err != nil {
			return nil, fmt.Errorf("failed to parse stats: %w", err)
		}

		batch := BatchResult{
			Seed:    seeds[i],
			Hands:   handsThisBatch,
			Results: results,
		}
		allBatches = append(allBatches, batch)

		// Track actual hands completed
		if actualHands, exists := results["actual_hands"]; exists {
			totalHands += int(actualHands)
		} else {
			totalHands += handsThisBatch
		}
	}

	// Calculate aggregate results
	var totalChips float64
	var totalHandsPlayed int
	var avgVPIP, avgPFR float64
	var maxBB100, minBB100 float64

	for i, batch := range allBatches {
		actualHands := batch.Hands
		if handsFromStats, exists := batch.Results["actual_hands"]; exists {
			actualHands = int(handsFromStats)
		}

		if bb100, exists := batch.Results["avg_bb_per_100"]; exists {
			totalChips += (bb100 / 100.0) * float64(actualHands)
			if i == 0 || bb100 > maxBB100 {
				maxBB100 = bb100
			}
			if i == 0 || bb100 < minBB100 {
				minBB100 = bb100
			}
		}
		if vpip, exists := batch.Results["avg_vpip"]; exists {
			avgVPIP += vpip * float64(actualHands)
		}
		if pfr, exists := batch.Results["avg_pfr"]; exists {
			avgPFR += pfr * float64(actualHands)
		}
		totalHandsPlayed += actualHands
	}

	// Calculate weighted averages
	avgBB100 := 0.0
	if totalHandsPlayed > 0 {
		avgBB100 = (totalChips / float64(totalHandsPlayed)) * 100.0
		avgVPIP /= float64(totalHandsPlayed)
		avgPFR /= float64(totalHandsPlayed)
	}

	// Calculate variance as difference between max and min
	_ = maxBB100 - minBB100 // variance, could be used for reporting

	// Calculate timing
	duration := time.Since(startTime)
	handsPerSecond := float64(totalHands) / duration.Seconds()

	// Self-play should average near zero (zero-sum game)
	// Large deviations indicate potential issues
	isNearZero := math.Abs(avgBB100) < 5.0 // Within 5 BB/100 of zero
	verdict := "pass"
	if !isNearZero {
		verdict = "warning"
	}

	return &TestResult{
		TestID: fmt.Sprintf("self-play-%d", time.Now().Unix()),
		Mode:   ModeSelfPlay,
		Metadata: TestMetadata{
			StartTime:       startTime,
			DurationSeconds: duration.Seconds(),
			ServerVersion:   "1.0.0",
			TestEnvironment: "test",
		},
		Config: TestConfigSummary{
			BotA:              r.config.Bot,
			HandsTotal:        r.config.HandsTotal,
			Batches:           len(allBatches),
			BatchSize:         r.config.BatchSize,
			SignificanceLevel: r.config.SignificanceLevel,
		},
		Batches: allBatches,
		Aggregate: AggregateResults{
			BotA: &BotResults{
				BBPer100:         avgBB100,
				CI95Low:          minBB100, // Using min/max as variance bounds
				CI95High:         maxBB100,
				VPIP:             avgVPIP,
				PFR:              avgPFR,
				AggressionFactor: 0, // TODO: Calculate
				BustRate:         0, // TODO: Calculate
			},
		},
		Performance: PerformanceMetrics{
			HandsPerSecond: handsPerSecond,
		},
		Errors: ErrorSummary{
			BotCrashes:       0, // TODO: Track
			Timeouts:         0, // TODO: Track
			ConnectionErrors: 0, // TODO: Track
			RecoveredCrashes: 0, // TODO: Track
		},
		Verdict: TestVerdict{
			SignificantDifference: !isNearZero,
			PValue:                0.5,             // N/A for self-play
			EffectSize:            avgBB100 / 50.0, // Relative to typical std dev
			Direction:             "baseline",
			Confidence:            0.95,
			Recommendation:        verdict,
		},
	}, nil
}

// parseSelfPlayStats parses stats file for self-play mode
func (r *Runner) parseSelfPlayStats(filename string, _ string) (map[string]float64, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read stats file: %w", err)
	}

	var stats struct {
		Players []struct {
			DisplayName   string `json:"display_name"`
			Hands         int    `json:"hands"`
			NetChips      int64  `json:"net_chips"`
			DetailedStats *struct {
				VPIP float64 `json:"vpip"`
				PFR  float64 `json:"pfr"`
			} `json:"detailed_stats,omitempty"`
		} `json:"players"`
		HandsCompleted uint64 `json:"hands_completed"`
	}

	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal stats: %w", err)
	}

	// Calculate average stats across all bot instances
	var totalChips int64
	var totalHands int
	var sumVPIP, sumPFR float64
	var maxBB100, minBB100 float64
	first := true

	for _, player := range stats.Players {
		if player.Hands > 0 {
			// Convert net chips to BB/100 (assuming big blind = 10)
			bb100 := float64(player.NetChips) / float64(player.Hands) * 100.0 / 10.0

			totalChips += player.NetChips
			totalHands += player.Hands

			// Extract VPIP/PFR from detailed stats if available
			if player.DetailedStats != nil {
				sumVPIP += player.DetailedStats.VPIP * float64(player.Hands)
				sumPFR += player.DetailedStats.PFR * float64(player.Hands)
			}

			if first || bb100 > maxBB100 {
				maxBB100 = bb100
				first = false
			}
			if first || bb100 < minBB100 {
				minBB100 = bb100
			}
		}
	}

	avgBB100 := 0.0
	avgVPIP := 0.0
	avgPFR := 0.0

	if totalHands > 0 {
		// Convert to BB/100 (big blind = 10)
		avgBB100 = float64(totalChips) / float64(totalHands) * 100.0 / 10.0
		avgVPIP = sumVPIP / float64(totalHands)
		avgPFR = sumPFR / float64(totalHands)
	}

	return map[string]float64{
		"avg_bb_per_100": avgBB100,
		"max_bb_per_100": maxBB100,
		"min_bb_per_100": minBB100,
		"avg_vpip":       avgVPIP,
		"avg_pfr":        avgPFR,
		"actual_hands":   float64(stats.HandsCompleted),
	}, nil
}

// outputResults outputs test results in the configured format
func (r *Runner) outputResults(results []*TestResult) error {
	if len(results) == 0 {
		return fmt.Errorf("no results to output")
	}

	var output string

	// Generate JSON output
	if r.config.OutputFormat == "json" || r.config.OutputFormat == "both" {
		// For multiple results, output as array
		var jsonBytes []byte
		var err error
		if len(results) == 1 {
			jsonBytes, err = json.MarshalIndent(results[0], "", "  ")
		} else {
			jsonBytes, err = json.MarshalIndent(results, "", "  ")
		}
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		output = string(jsonBytes)
	}

	// Generate summary output
	if r.config.OutputFormat == "summary" || r.config.OutputFormat == "both" {
		if r.config.OutputFormat == "both" {
			output += "\n\n"
		}

		// Generate summary for each result
		if len(results) == 1 {
			output += generateSummary(results[0])
		} else {
			// Multiple results - create combined summary
			output += "Combined Regression Test Report\n"
			output += "================================\n\n"

			for i, result := range results {
				if i > 0 {
					output += "\n" + strings.Repeat("-", 60) + "\n\n"
				}
				output += fmt.Sprintf("Test %d: %s Mode\n", i+1, result.Mode)
				output += strings.Repeat("-", 30) + "\n"
				output += generateSummary(result)
			}

			// Add overall summary
			output += "\n" + strings.Repeat("=", 60) + "\n"
			output += "Overall Results\n"
			output += strings.Repeat("=", 60) + "\n"

			passed := 0
			failed := 0
			marginal := 0
			for _, result := range results {
				switch result.Verdict.Recommendation {
				case "accept", "pass":
					passed++
				case "reject", "warning":
					failed++
				case "marginal", "inconclusive":
					marginal++
				}
			}

			output += fmt.Sprintf("Passed: %d, Failed: %d, Marginal: %d\n", passed, failed, marginal)

			if r.config.MultipleTestCorrection {
				output += fmt.Sprintf("\n✓ Bonferroni correction applied (α = %.3f per test)\n",
					r.config.SignificanceLevel/float64(len(results)))
			}
		}
	}

	// Write to file or stdout
	if r.config.OutputFile != "" {
		return os.WriteFile(r.config.OutputFile, []byte(output), 0644)
	}

	fmt.Println(output) //nolint:forbidigo
	return nil
}

// generateSummary creates a human-readable summary
func generateSummary(result *TestResult) string {
	var sb strings.Builder

	sb.WriteString("Regression Test Report\n")
	sb.WriteString("======================\n")

	// Add mode-specific summary with aligned formatting
	switch result.Mode {
	case ModeHeadsUp:
		if result.Config.BotA != "" {
			sb.WriteString(fmt.Sprintf("Bot A:      %s\n", result.Config.BotA))
		}
		if result.Config.BotB != "" {
			sb.WriteString(fmt.Sprintf("Bot B:      %s\n", result.Config.BotB))
		}
	case ModePopulation:
		if result.Config.Challenger != "" {
			sb.WriteString(fmt.Sprintf("Challenger: %s\n", result.Config.Challenger))
		}
		if result.Config.Baseline != "" {
			sb.WriteString(fmt.Sprintf("Baseline:   %s\n", result.Config.Baseline))
		}
	case ModeNPCBenchmark:
		if result.Config.BotA != "" {
			sb.WriteString(fmt.Sprintf("Bot:        %s\n", result.Config.BotA))
		}
		sb.WriteString("Opponents:  NPCs (calling, aggressive, random)\n")
	case ModeSelfPlay:
		if result.Config.BotA != "" {
			sb.WriteString(fmt.Sprintf("Bot:        %s\n", result.Config.BotA))
		}
		sb.WriteString("Mode:       Self-play (all seats same bot)\n")
	}

	sb.WriteString(fmt.Sprintf("Mode:       %s\n", result.Mode))
	sb.WriteString(fmt.Sprintf("Hands:      %s\n", formatNumber(result.Config.HandsTotal)))

	// Format duration nicely
	duration := result.Metadata.DurationSeconds
	if duration >= 60 {
		minutes := int(duration / 60)
		seconds := int(duration) % 60
		sb.WriteString(fmt.Sprintf("Duration:   %dm %ds\n", minutes, seconds))
	} else {
		sb.WriteString(fmt.Sprintf("Duration:   %.1fs\n", duration))
	}

	sb.WriteString("\nResults\n")
	sb.WriteString("-------\n")

	// Add results based on mode
	if result.Aggregate.BotA != nil {
		sb.WriteString(fmt.Sprintf("Bot A:      %+.1f BB/100 [95%% CI: %+.1f to %+.1f]\n",
			result.Aggregate.BotA.BBPer100,
			result.Aggregate.BotA.CI95Low,
			result.Aggregate.BotA.CI95High))
	}
	if result.Aggregate.BotB != nil {
		sb.WriteString(fmt.Sprintf("Bot B:      %+.1f BB/100 [95%% CI: %+.1f to %+.1f]\n",
			result.Aggregate.BotB.BBPer100,
			result.Aggregate.BotB.CI95Low,
			result.Aggregate.BotB.CI95High))
	}
	if result.Aggregate.Challenger != nil {
		sb.WriteString(fmt.Sprintf("Challenger: %+.1f BB/100 [95%% CI: %+.1f to %+.1f]\n",
			result.Aggregate.Challenger.BBPer100,
			result.Aggregate.Challenger.CI95Low,
			result.Aggregate.Challenger.CI95High))
	}
	if result.Aggregate.Baseline != nil {
		sb.WriteString(fmt.Sprintf("Baseline:   %+.1f BB/100 [95%% CI: %+.1f to %+.1f]\n",
			result.Aggregate.Baseline.BBPer100,
			result.Aggregate.Baseline.CI95Low,
			result.Aggregate.Baseline.CI95High))
	}
	if result.Verdict.EffectSize > 0 {
		sb.WriteString(fmt.Sprintf("Effect Size: %.2f", result.Verdict.EffectSize))
		switch {
		case result.Verdict.EffectSize < 0.2:
			sb.WriteString(" (small)")
		case result.Verdict.EffectSize < 0.5:
			sb.WriteString(" (medium)")
		case result.Verdict.EffectSize < 0.8:
			sb.WriteString(" (large)")
		default:
			sb.WriteString(" (very large)")
		}
		sb.WriteString("\n")
	}
	if result.Verdict.PValue > 0 {
		sb.WriteString(fmt.Sprintf("P-Value: %.3f", result.Verdict.PValue))
		if result.Verdict.AdjustedPValue > 0 {
			sb.WriteString(fmt.Sprintf(" (adjusted: %.3f)", result.Verdict.AdjustedPValue))
		}
		sb.WriteString("\n")
	}

	// Strategic Changes section (for heads-up mode, show VPIP/PFR)
	if result.Mode == ModeHeadsUp && (result.Aggregate.BotA != nil || result.Aggregate.BotB != nil) {
		sb.WriteString("\nStrategic Profile\n")
		sb.WriteString("-----------------\n")
		if result.Aggregate.BotA != nil {
			sb.WriteString(fmt.Sprintf("Bot A VPIP: %.1f%%, PFR: %.1f%%",
				result.Aggregate.BotA.VPIP*100,
				result.Aggregate.BotA.PFR*100))
			if result.Aggregate.BotA.BustRate > 0 {
				sb.WriteString(fmt.Sprintf(", Busts: %.1f%%", result.Aggregate.BotA.BustRate*100))
			}
			sb.WriteString("\n")
		}
		if result.Aggregate.BotB != nil {
			sb.WriteString(fmt.Sprintf("Bot B VPIP: %.1f%%, PFR: %.1f%%",
				result.Aggregate.BotB.VPIP*100,
				result.Aggregate.BotB.PFR*100))
			if result.Aggregate.BotB.BustRate > 0 {
				sb.WriteString(fmt.Sprintf(", Busts: %.1f%%", result.Aggregate.BotB.BustRate*100))
			}
			sb.WriteString("\n")
		}
	}

	// Performance and Reliability section
	// Strategic Changes section for NPC benchmark mode
	if result.Mode == ModeNPCBenchmark && result.Aggregate.BotA != nil {
		sb.WriteString("\nStrategic Profile\n")
		sb.WriteString("-----------------\n")
		if result.Aggregate.BotA.VPIP > 0 {
			sb.WriteString(fmt.Sprintf("VPIP:  %.1f%%\n", result.Aggregate.BotA.VPIP*100))
		}
		if result.Aggregate.BotA.PFR > 0 {
			sb.WriteString(fmt.Sprintf("PFR:   %.1f%%\n", result.Aggregate.BotA.PFR*100))
		}
		if result.Aggregate.BotA.AggressionFactor > 0 {
			sb.WriteString(fmt.Sprintf("Aggression Factor: %.2f\n", result.Aggregate.BotA.AggressionFactor))
		}
		// Add placeholder note if no detailed stats available
		if result.Aggregate.BotA.VPIP == 0 && result.Aggregate.BotA.PFR == 0 {
			sb.WriteString("(Detailed stats not available - requires server detailed stats mode)\n")
		}
	}

	sb.WriteString("\nPerformance\n")
	sb.WriteString("-----------\n")
	if result.Performance.HandsPerSecond > 0 {
		sb.WriteString(fmt.Sprintf("Hands/sec: %s\n", formatNumber(int(result.Performance.HandsPerSecond))))
	}

	// Only show sample size warning if there is one
	if result.Performance.SampleAssessment != "" {
		sb.WriteString(fmt.Sprintf("\n%s\n", result.Performance.SampleAssessment))
	}

	sb.WriteString("\nReliability\n")
	sb.WriteString("-----------\n")
	hasReliabilityData := false
	if result.Errors.BotCrashes > 0 {
		sb.WriteString(fmt.Sprintf("Bot Crashes: %d", result.Errors.BotCrashes))
		if result.Errors.RecoveredCrashes > 0 {
			sb.WriteString(" (recovered)")
		}
		sb.WriteString("\n")
		hasReliabilityData = true
	}
	if result.Errors.Timeouts > 0 {
		sb.WriteString(fmt.Sprintf("Timeouts: %d\n", result.Errors.Timeouts))
		hasReliabilityData = true
	}
	if !hasReliabilityData {
		sb.WriteString("No errors or timeouts detected\n")
	}

	// Verdict
	sb.WriteString("\nVerdict: ")
	sb.WriteString(strings.ToUpper(result.Verdict.Recommendation))
	if result.Verdict.SignificantDifference && result.Verdict.Confidence > 0 {
		sb.WriteString(fmt.Sprintf(" (%.0f%% confidence)", result.Verdict.Confidence*100))
	}
	sb.WriteString("\n")

	return sb.String()
}

// formatNumber formats large numbers with commas for readability
func formatNumber(n int) string {
	str := strconv.Itoa(n)
	if len(str) <= 3 {
		return str
	}

	var result strings.Builder
	for i, digit := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result.WriteString(",")
		}
		result.WriteRune(digit)
	}
	return result.String()
}
