package regression

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/lox/pokerforbots/internal/server"
)

// Runner orchestrates regression tests
type Runner struct {
	config        *Config
	healthMonitor *HealthMonitor
	orchestrator  *Orchestrator // Uses server bot commands
	reporter      *Reporter
}

// RunnerOption configures a Runner
type RunnerOption func(*Runner)

// WithProgressReporter sets a progress reporter for batch progress
func WithProgressReporter(reporter ProgressReporter) RunnerOption {
	return func(r *Runner) {
		r.orchestrator.SetProgressReporter(reporter)
	}
}

// WithHandMonitor sets a hand monitor for real-time server progress
func WithHandMonitor(monitor server.HandMonitor) RunnerOption {
	return func(r *Runner) {
		r.orchestrator.SetHandMonitor(monitor)
	}
}

// NewRunner creates a new test runner with options
func NewRunner(config *Config, opts ...RunnerOption) *Runner {
	healthMonitor := NewHealthMonitor(
		config.MaxCrashesPerBot,
		config.MaxTimeoutsPerBot,
		time.Duration(config.RestartDelayMs)*time.Millisecond,
		config.Logger,
	)

	orchestrator := NewOrchestrator(config, healthMonitor)
	reporter := NewReporter(nil, config.Logger, config)

	runner := &Runner{
		config:        config,
		healthMonitor: healthMonitor,
		orchestrator:  orchestrator,
		reporter:      reporter,
	}

	// Apply options
	for _, opt := range opts {
		opt(runner)
	}

	return runner
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
		if err := r.validateBinaryExecution(binary); err != nil {
			return err
		}

		r.config.Logger.Debug().
			Str("binary", binary).
			Msg("Binary validation passed")
	}

	return nil
}

// validateBinaryExecution tries to run a binary with --help to validate it starts
func (r *Runner) validateBinaryExecution(binary string) error {
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
			return nil
		}
		// Check if it's just exit code 1 (common for no --help flag)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			r.config.Logger.Debug().
				Str("binary", binary).
				Msg("Binary validation passed (exit 1 on --help)")
			return nil
		}
		return fmt.Errorf("binary %s failed to run: %v", binary, err)
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

		// All modes now use challenger and baseline
		canRunHeadsUp := r.config.Challenger != "" && r.config.Baseline != ""
		canRunPopulation := r.config.Challenger != "" && r.config.Baseline != ""
		canRunNPCBenchmark := r.config.Challenger != "" && r.config.Baseline != ""
		canRunSelfPlay := r.config.Challenger != "" // Self-play only needs challenger

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

	// All modes now use challenger and baseline
	if r.config.Challenger != "" {
		binaries[r.config.Challenger] = true
	}
	if r.config.Baseline != "" {
		binaries[r.config.Baseline] = true
	}

	result := make([]string, 0, len(binaries))
	for binary := range binaries {
		result = append(result, binary)
	}
	return result
}

// printTestHeader prints a formatted header for the test
func (r *Runner) printTestHeader(mode string, totalBots int) {
	// We need to use fmt.Printf here for immediate output to the user
	//nolint:forbidigo
	fmt.Printf("\n%s\n", strings.Repeat("=", 60))
	//nolint:forbidigo
	fmt.Printf("Test Mode: %s\n", mode)
	//nolint:forbidigo
	fmt.Printf("Bots at table: %d\n", totalBots)
	//nolint:forbidigo
	fmt.Printf("Total hands: %d\n", r.config.HandsTotal)
	//nolint:forbidigo
	fmt.Printf("Batch size: %d\n", r.config.BatchSize)
	//nolint:forbidigo
	fmt.Printf("Number of batches: %d\n", (r.config.HandsTotal+r.config.BatchSize-1)/r.config.BatchSize)
	//nolint:forbidigo
	fmt.Printf("%s\n\n", strings.Repeat("=", 60))
}

// runHeadsUpTest runs a heads-up test between two bots
func (r *Runner) runHeadsUpTest(ctx context.Context) (*TestResult, error) {
	if r.config.Challenger == "" || r.config.Baseline == "" {
		return nil, fmt.Errorf("heads-up mode requires challenger and baseline")
	}

	// Print test header
	r.printTestHeader("Heads-Up", 2)

	r.config.Logger.Info().
		Str("mode", "heads-up").
		Str("challenger", r.config.Challenger).
		Str("baseline", r.config.Baseline).
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

	// Print test header
	totalBots := r.config.ChallengerSeats + r.config.BaselineSeats
	r.printTestHeader(fmt.Sprintf("Population (%d challenger vs %d baseline)", r.config.ChallengerSeats, r.config.BaselineSeats), totalBots)

	r.config.Logger.Info().
		Str("mode", "population").
		Str("challenger", r.config.Challenger).
		Str("baseline", r.config.Baseline).
		Int("challenger_seats", r.config.ChallengerSeats).
		Int("baseline_seats", r.config.BaselineSeats).
		Int("hands", r.config.HandsTotal).
		Msg("Starting population test")

	startTime := time.Now()

	// Create population strategy
	strategy := &PopulationStrategy{
		Challenger:      r.config.Challenger,
		Baseline:        r.config.Baseline,
		ChallengerSeats: r.config.ChallengerSeats,
		BaselineSeats:   r.config.BaselineSeats,
		Config:          r.config,
	}

	// Use common batch executor
	allBatches, err := r.orchestrator.ExecuteBatches(ctx, strategy, r.config.HandsTotal)
	if err != nil {
		return nil, fmt.Errorf("failed to execute population batches: %w", err)
	}

	// Aggregate results using weighted averages
	challengerStats := CombineBatches(allBatches, "challenger")
	baselineStats := CombineBatches(allBatches, "baseline")

	// Calculate proper statistics
	challengerStatResult := CalculateStatistics(allBatches, "challenger_bb_per_100", "challenger_hands")
	baselineStatResult := CalculateStatistics(allBatches, "baseline_bb_per_100", "baseline_hands")

	// Perform statistical comparison
	comparison := CompareStatistics(challengerStatResult, baselineStatResult)

	duration := time.Since(startTime)
	handsPerSecond := float64(r.config.HandsTotal) / duration.Seconds()

	// Get health metrics
	crashes, timeouts, recovered := r.healthMonitor.GetErrorSummary()

	// Determine direction
	direction := "no_change"
	if comparison.Difference > 0 {
		direction = "improvement"
	} else if comparison.Difference < 0 {
		direction = "regression"
	}

	// Determine verdict based on p-value and effect size
	verdict := "inconclusive"
	if comparison.PValue < r.config.SignificanceLevel {
		// Statistically significant
		effectMagnitude := InterpretEffectSize(comparison.EffectSize)
		if direction == "improvement" {
			if effectMagnitude == "large" || effectMagnitude == "medium" {
				verdict = "accept"
			} else {
				verdict = "marginal"
			}
		} else { // regression
			if effectMagnitude == "large" || effectMagnitude == "medium" {
				verdict = "reject"
			} else {
				verdict = "marginal"
			}
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
				BBPer100:         challengerStatResult.Mean,
				CI95Low:          challengerStatResult.CI95Low,
				CI95High:         challengerStatResult.CI95High,
				VPIP:             challengerStats.VPIP,
				PFR:              challengerStats.PFR,
				AggressionFactor: 0, // TODO: Calculate
				BustRate:         challengerStats.Busts,
				EffectSize:       comparison.EffectSize,
			},
			Baseline: &BotResults{
				BBPer100:         baselineStatResult.Mean,
				CI95Low:          baselineStatResult.CI95Low,
				CI95High:         baselineStatResult.CI95High,
				VPIP:             baselineStats.VPIP,
				PFR:              baselineStats.PFR,
				AggressionFactor: 0, // TODO: Calculate
				BustRate:         baselineStats.Busts,
			},
		},
		Performance: PerformanceMetrics{
			HandsPerSecond: handsPerSecond,
		},
		Errors: ErrorSummary{
			BotCrashes:       crashes,
			Timeouts:         timeouts,
			ConnectionErrors: 0, // Connection errors tracked separately if needed
			RecoveredCrashes: recovered,
		},
		Verdict: TestVerdict{
			SignificantDifference: comparison.PValue < r.config.SignificanceLevel,
			PValue:                comparison.PValue,
			EffectSize:            comparison.EffectSize,
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

	// Count total NPCs
	totalNPCs := 0
	for _, count := range npcConfig {
		totalNPCs += count
	}

	// Default bot seats if not specified
	botSeats := r.config.ChallengerSeats
	if botSeats == 0 {
		botSeats = 1 // Default to 1 bot vs NPCs
	}

	// Print test header
	totalBots := botSeats + totalNPCs
	npcStr := ""
	for npcType, count := range npcConfig {
		if npcStr != "" {
			npcStr += ", "
		}
		npcStr += fmt.Sprintf("%d %s", count, npcType)
	}
	r.printTestHeader(fmt.Sprintf("NPC Benchmark (%d bot vs %s NPCs)", botSeats, npcStr), totalBots)

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

	// Create strategies for challenger and baseline
	// Default seat counts if not specified
	challengerSeats := r.config.ChallengerSeats
	if challengerSeats == 0 {
		challengerSeats = 1 // Default to 1 seat for NPC benchmark
	}
	baselineSeats := r.config.BaselineSeats
	if baselineSeats == 0 {
		baselineSeats = 1 // Default to 1 seat for NPC benchmark
	}

	challengerStrategy := &NPCBenchmarkStrategy{
		Challenger:      r.config.Challenger,
		Baseline:        "", // Not used in challenger run
		ChallengerSeats: challengerSeats,
		BaselineSeats:   0, // No baseline bots in challenger run
		NPCs:            npcConfig,
		Config:          r.config,
	}

	baselineStrategy := &NPCBenchmarkStrategy{
		Challenger:      "", // Not used in baseline run
		Baseline:        r.config.Baseline,
		ChallengerSeats: 0, // No challenger bots in baseline run
		BaselineSeats:   baselineSeats,
		NPCs:            npcConfig,
		Config:          r.config,
	}

	// Run challenger batches
	r.config.Logger.Info().
		Str("bot", r.config.Challenger).
		Msg("Running challenger vs NPCs batches")
	allChallengerBatches, err := r.orchestrator.ExecuteBatches(ctx, challengerStrategy, r.config.HandsTotal)
	if err != nil {
		return nil, fmt.Errorf("failed to run challenger batches: %w", err)
	}

	// Run baseline batches with different seeds
	r.config.Logger.Info().
		Str("bot", r.config.Baseline).
		Msg("Running baseline vs NPCs batches")

	// Offset seeds for baseline to ensure different randomness
	originalSeeds := r.config.Seeds
	baselineSeeds := make([]int64, len(originalSeeds))
	for i, seed := range originalSeeds {
		baselineSeeds[i] = seed + 1000000
	}
	r.config.Seeds = baselineSeeds
	allBaselineBatches, err := r.orchestrator.ExecuteBatches(ctx, baselineStrategy, r.config.HandsTotal)
	r.config.Seeds = originalSeeds // Restore original seeds
	if err != nil {
		return nil, fmt.Errorf("failed to run baseline batches: %w", err)
	}

	// Calculate aggregate results from challenger and baseline batches using weighted averages
	challengerStats := CombineBatches(allChallengerBatches, "bot")
	baselineStats := CombineBatches(allBaselineBatches, "bot")

	totalChallengerHands := challengerStats.TotalHands
	totalBaselineHands := baselineStats.TotalHands
	totalHands := totalChallengerHands + totalBaselineHands

	// Calculate timing and performance
	endTime := time.Now()
	duration := endTime.Sub(startTime)
	durationSeconds := duration.Seconds()
	handsPerSecond := float64(totalHands) / durationSeconds

	// Calculate proper statistics
	challengerStatResult := CalculateStatistics(allChallengerBatches, "bot_bb_per_100", "actual_hands")
	baselineStatResult := CalculateStatistics(allBaselineBatches, "bot_bb_per_100", "actual_hands")

	// Perform statistical comparison
	comparison := CompareStatistics(challengerStatResult, baselineStatResult)

	// Get health metrics
	crashes, timeouts, recovered := r.healthMonitor.GetErrorSummary()

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
				result := batch
				result.Results["batch_type"] = float64(1) // Mark as challenger
				result.Results["batch_index"] = float64(i)
				allBatches = append(allBatches, result)
			}
			for i, batch := range allBaselineBatches {
				result := batch
				result.Results["batch_type"] = float64(2) // Mark as baseline
				result.Results["batch_index"] = float64(i)
				allBatches = append(allBatches, result)
			}
			return allBatches
		}(),
		Aggregate: AggregateResults{
			Challenger: &BotResults{
				BBPer100:         challengerStatResult.Mean,
				CI95Low:          challengerStatResult.CI95Low,
				CI95High:         challengerStatResult.CI95High,
				VPIP:             challengerStats.VPIP,
				PFR:              challengerStats.PFR,
				AggressionFactor: 0, // TODO: Calculate
				BustRate:         challengerStats.Busts,
				EffectSize:       comparison.EffectSize,
			},
			Baseline: &BotResults{
				BBPer100:         baselineStatResult.Mean,
				CI95Low:          baselineStatResult.CI95Low,
				CI95High:         baselineStatResult.CI95High,
				VPIP:             baselineStats.VPIP,
				PFR:              baselineStats.PFR,
				AggressionFactor: 0, // TODO: Calculate
				BustRate:         baselineStats.Busts,
			},
		},
		Performance: PerformanceMetrics{
			HandsPerSecond: handsPerSecond,
		},
		Errors: ErrorSummary{
			BotCrashes:       crashes,
			Timeouts:         timeouts,
			ConnectionErrors: 0,
			RecoveredCrashes: recovered,
		},
		Verdict: TestVerdict{
			SignificantDifference: comparison.PValue < r.config.SignificanceLevel,
			PValue:                comparison.PValue,
			EffectSize:            comparison.EffectSize,
			Direction: func() string {
				switch {
				case comparison.Difference > 0:
					return "improvement"
				case comparison.Difference < 0:
					return "regression"
				default:
					return "no_change"
				}
			}(),
			Confidence: 0.95,
			Recommendation: func() string {
				// Verdict based on statistical significance and effect size
				if comparison.PValue >= r.config.SignificanceLevel {
					return "inconclusive"
				}
				effectMagnitude := InterpretEffectSize(comparison.EffectSize)
				switch {
				case comparison.Difference > 0:
					// Improvement
					if effectMagnitude == "large" || effectMagnitude == "medium" {
						return "accept"
					}
					return "marginal"
				default:
					// Regression
					if effectMagnitude == "large" || effectMagnitude == "medium" {
						return "reject"
					}
					return "marginal"
				}
			}(),
		},
	}, nil
}

// runSelfPlayTest runs a self-play test
func (r *Runner) runSelfPlayTest(ctx context.Context) (*TestResult, error) {
	if r.config.Challenger == "" {
		return nil, fmt.Errorf("self-play mode requires challenger")
	}

	// Default bot seats for self-play
	bots := r.config.ChallengerSeats
	if bots == 0 {
		bots = 6 // Default to 6-max
	}

	// Print test header
	r.printTestHeader(fmt.Sprintf("Self-Play (%d identical bots)", bots), bots)

	r.config.Logger.Info().
		Str("mode", "self-play").
		Str("bot", r.config.Challenger).
		Int("seats", bots).
		Int("hands", r.config.HandsTotal).
		Msg("Starting self-play test")

	startTime := time.Now()

	// Create self-play strategy
	strategy := &SelfPlayStrategy{
		Challenger: r.config.Challenger,
		Baseline:   r.config.Challenger, // Same bot for self-play
		BotSeats:   bots,
		Config:     r.config,
	}

	// Use common batch executor
	allBatches, err := r.orchestrator.ExecuteBatches(ctx, strategy, r.config.HandsTotal)
	if err != nil {
		return nil, fmt.Errorf("failed to execute self-play batches: %w", err)
	}

	// Track total hands from batches
	totalHands := CalculateTotalHands(allBatches, "actual_hands")

	// Calculate proper statistics for self-play
	selfPlayStats := CalculateStatistics(allBatches, "avg_bb_per_100", "actual_hands")

	// Calculate aggregate playing stats
	avgVPIP := WeightedAverage(allBatches, "avg_vpip", "actual_hands")
	avgPFR := WeightedAverage(allBatches, "avg_pfr", "actual_hands")

	// Calculate timing
	duration := time.Since(startTime)
	handsPerSecond := float64(totalHands) / duration.Seconds()

	// Get health metrics
	crashes, timeouts, recovered := r.healthMonitor.GetErrorSummary()

	// Self-play should average near zero (zero-sum game)
	// Test if mean is significantly different from zero
	zeroComparison := StatisticalResult{
		Mean:       0.0,
		StdDev:     selfPlayStats.StdDev, // Use same StdDev for null hypothesis
		CI95Low:    -5.0,                 // Acceptable range for zero-sum
		CI95High:   5.0,
		SampleSize: selfPlayStats.SampleSize,
	}
	comparison := CompareStatistics(selfPlayStats, zeroComparison)

	// Determine verdict based on deviation from zero
	verdict := "pass"
	direction := "balanced"
	if math.Abs(selfPlayStats.Mean) > 5.0 {
		verdict = "warning"
		if selfPlayStats.Mean > 5.0 {
			direction = "positive_bias"
		} else {
			direction = "negative_bias"
		}
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
			Challenger:        r.config.Challenger,
			Baseline:          r.config.Challenger, // Same bot for self-play
			HandsTotal:        r.config.HandsTotal,
			Batches:           len(allBatches),
			BatchSize:         r.config.BatchSize,
			SignificanceLevel: r.config.SignificanceLevel,
		},
		Batches: allBatches,
		Aggregate: AggregateResults{
			Challenger: &BotResults{
				BBPer100:         selfPlayStats.Mean,
				CI95Low:          selfPlayStats.CI95Low,
				CI95High:         selfPlayStats.CI95High,
				VPIP:             avgVPIP,
				PFR:              avgPFR,
				AggressionFactor: 0,                     // TODO: Calculate
				BustRate:         0,                     // TODO: Calculate
				EffectSize:       comparison.EffectSize, // Effect size vs zero
			},
			// In self-play, baseline is the same as challenger
			Baseline: &BotResults{
				BBPer100:         selfPlayStats.Mean,
				CI95Low:          selfPlayStats.CI95Low,
				CI95High:         selfPlayStats.CI95High,
				VPIP:             avgVPIP,
				PFR:              avgPFR,
				AggressionFactor: 0,
				BustRate:         0,
			},
		},
		Performance: PerformanceMetrics{
			HandsPerSecond: handsPerSecond,
		},
		Errors: ErrorSummary{
			BotCrashes:       crashes,
			Timeouts:         timeouts,
			ConnectionErrors: 0, // Connection errors tracked separately if needed
			RecoveredCrashes: recovered,
		},
		Verdict: TestVerdict{
			SignificantDifference: comparison.PValue < r.config.SignificanceLevel,
			PValue:                comparison.PValue,
			EffectSize:            comparison.EffectSize,
			Direction:             direction,
			Confidence:            0.95,
			Recommendation:        verdict,
		},
	}, nil
}

// outputResults outputs test results in the configured format
func (r *Runner) outputResults(results []*TestResult) error {
	if len(results) == 0 {
		return fmt.Errorf("no results to output")
	}

	// Convert TestResults to ReportResults for the reporter
	for _, result := range results {
		// Generate report using the reporter
		report, err := r.reporter.GenerateReport(result)
		if err != nil {
			return fmt.Errorf("failed to generate report: %w", err)
		}

		// Output based on format
		if r.config.OutputFormat == "json" || r.config.OutputFormat == "both" {
			if err := r.reporter.WriteJSON(report); err != nil {
				return fmt.Errorf("failed to write JSON report: %w", err)
			}
		}

		if r.config.OutputFormat == "summary" || r.config.OutputFormat == "both" {
			if r.config.OutputFormat == "both" {
				fmt.Println() //nolint:forbidigo
			}
			if err := r.reporter.WriteSummary(report); err != nil {
				return fmt.Errorf("failed to write summary: %w", err)
			}
		}
	}

	return nil
}
