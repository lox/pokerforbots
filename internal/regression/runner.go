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

// RunPowerAnalysis calculates required sample size
func (r *Runner) RunPowerAnalysis(printf func(string, ...any)) {
	// Using approximation formula for sample size
	// n = 2 * ((z_alpha + z_beta) / d)^2

	zAlpha := 1.96 // for 0.05 significance level, two-tailed
	zBeta := 0.84  // for 0.80 power

	switch r.config.Power {
	case 0.9:
		zBeta = 1.28
	case 0.95:
		zBeta = 1.64
	}

	ratio := (zAlpha + zBeta) / r.config.EffectSize
	n := 2.0 * ratio * ratio

	// Convert to hands
	handsRequired := int(math.Ceil(n))

	printf("Power Analysis Results\n")
	printf("======================\n")
	printf("Effect Size: %.2f\n", r.config.EffectSize)
	printf("Significance Level: %.2f\n", r.config.SignificanceLevel)
	printf("Desired Power: %.2f\n", r.config.Power)
	printf("\n")
	printf("Required Sample Size: %d hands per bot\n", handsRequired)
	printf("Total Hands Needed: %d\n", handsRequired*2)
	printf("\n")
	printf("Note: This is a simplified calculation.\n")
	printf("Actual requirements may vary based on variance.\n")
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
		// Run all modes
		// TODO: Implement running all modes
		return fmt.Errorf("mode 'all' not yet implemented")

	default:
		return fmt.Errorf("unknown test mode: %s", r.config.Mode)
	}

	// Output results
	return r.outputResults(results)
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

	r.config.Logger.Info().
		Str("mode", "population").
		Str("challenger", r.config.Challenger).
		Str("baseline", r.config.Baseline).
		Int("challenger_seats", r.config.ChallengerSeats).
		Int("baseline_seats", r.config.BaselineSeats).
		Int("hands", r.config.HandsTotal).
		Msg("Starting population test")

	// TODO: Implement population test logic
	return nil, fmt.Errorf("population test not yet implemented")
}

// runNPCBenchmarkTest runs an NPC benchmark test
func (r *Runner) runNPCBenchmarkTest(ctx context.Context) (*TestResult, error) {
	if r.config.Bot == "" {
		return nil, fmt.Errorf("NPC benchmark mode requires bot")
	}

	r.config.Logger.Info().
		Str("mode", "npc-benchmark").
		Str("bot", r.config.Bot).
		Int("bot_seats", r.config.BotSeats).
		Interface("npcs", r.config.NPCs).
		Int("hands", r.config.HandsTotal).
		Msg("Starting NPC benchmark test")

	// TODO: Implement NPC benchmark test logic
	return nil, fmt.Errorf("NPC benchmark test not yet implemented")
}

// runSelfPlayTest runs a self-play test
func (r *Runner) runSelfPlayTest(ctx context.Context) (*TestResult, error) {
	if r.config.Bot == "" {
		return nil, fmt.Errorf("self-play mode requires bot")
	}

	r.config.Logger.Info().
		Str("mode", "self-play").
		Str("bot", r.config.Bot).
		Int("hands", r.config.HandsTotal).
		Msg("Starting self-play test")

	// TODO: Implement self-play test logic
	return nil, fmt.Errorf("self-play test not yet implemented")
}

// outputResults outputs test results in the configured format
func (r *Runner) outputResults(results []*TestResult) error {
	if len(results) == 0 {
		return fmt.Errorf("no results to output")
	}

	var output string

	// Generate JSON output
	if r.config.OutputFormat == "json" || r.config.OutputFormat == "both" {
		jsonBytes, err := json.MarshalIndent(results[0], "", "  ")
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
		output += generateSummary(results[0])
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
	sb.WriteString("\nPerformance\n")
	sb.WriteString("-----------\n")
	if result.Errors.BotCrashes > 0 {
		sb.WriteString(fmt.Sprintf("Bot Crashes: %d", result.Errors.BotCrashes))
		if result.Errors.RecoveredCrashes > 0 {
			sb.WriteString(" (recovered)")
		}
		sb.WriteString("\n")
	}
	if result.Errors.Timeouts > 0 {
		sb.WriteString(fmt.Sprintf("Timeouts: %d\n", result.Errors.Timeouts))
	}
	if result.Performance.HandsPerSecond > 0 {
		sb.WriteString(fmt.Sprintf("Hands/sec: %s\n", formatNumber(int(result.Performance.HandsPerSecond))))
	}

	// Verdict
	sb.WriteString(fmt.Sprintf("\nVerdict: %s", strings.ToUpper(result.Verdict.Recommendation)))
	if result.Verdict.SignificantDifference {
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
