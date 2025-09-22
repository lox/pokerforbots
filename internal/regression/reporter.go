package regression

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// Reporter handles output generation for regression test results
type Reporter struct {
	writer io.Writer
	logger zerolog.Logger
	config *Config
}

// NewReporter creates a new reporter instance
func NewReporter(writer io.Writer, logger zerolog.Logger, config *Config) *Reporter {
	if writer == nil {
		writer = os.Stdout
	}
	return &Reporter{
		writer: writer,
		logger: logger,
		config: config,
	}
}

// ReportResult is the main structure for test reports
type ReportResult struct {
	TestID   string           `json:"test_id"`
	Mode     string           `json:"mode"`
	Metadata ReportMetadata   `json:"metadata"`
	Config   ReportConfig     `json:"configuration"`
	Batches  []BatchResult    `json:"batches"`
	Results  ReportStatistics `json:"results"`
}

// ReportMetadata contains test execution metadata
type ReportMetadata struct {
	StartTime        time.Time `json:"start_time"`
	EndTime          time.Time `json:"end_time"`
	DurationSeconds  float64   `json:"duration_seconds"`
	ServerVersion    string    `json:"server_version,omitempty"`
	TestEnvironment  string    `json:"test_environment,omitempty"`
	HandsPerSecond   float64   `json:"hands_per_second"`
	TotalHandsPlayed int       `json:"total_hands_played"`
	BatchesCompleted int       `json:"batches_completed"`
}

// ReportConfig contains test configuration
type ReportConfig struct {
	Challenger        string  `json:"challenger,omitempty"`
	Baseline          string  `json:"baseline,omitempty"`
	BotA              string  `json:"bot_a,omitempty"`
	BotB              string  `json:"bot_b,omitempty"`
	HandsRequested    int     `json:"hands_requested"`
	HandsCompleted    int     `json:"hands_completed"`
	BatchSize         int     `json:"batch_size"`
	TotalBatches      int     `json:"total_batches"`
	Seeds             []int64 `json:"seeds"`
	SignificanceLevel float64 `json:"significance_level"`
	EarlyStopping     bool    `json:"early_stopping,omitempty"`
	InfiniteBankroll  bool    `json:"infinite_bankroll,omitempty"`
	StartingChips     int     `json:"starting_chips"`
}

// ReportStatistics contains aggregated test results
type ReportStatistics struct {
	ChallengerStats *BotStatistics `json:"challenger,omitempty"`
	BaselineStats   *BotStatistics `json:"baseline,omitempty"`
	BotAStats       *BotStatistics `json:"bot_a,omitempty"`
	BotBStats       *BotStatistics `json:"bot_b,omitempty"`

	// Statistical analysis
	EffectSize     float64 `json:"effect_size"`
	PValue         float64 `json:"p_value"`
	AdjustedPValue float64 `json:"adjusted_p_value"`
	IsSignificant  bool    `json:"is_significant"`

	// Sample size analysis
	SampleSizeWarning string `json:"sample_size_warning,omitempty"`
}

// BotStatistics contains per-bot performance metrics
type BotStatistics struct {
	BB100    float64 `json:"bb_per_100"`
	CI95Low  float64 `json:"ci_95_low"`
	CI95High float64 `json:"ci_95_high"`
	VPIP     float64 `json:"vpip"`
	PFR      float64 `json:"pfr"`
	Hands    int     `json:"hands"`
	Timeouts float64 `json:"timeouts"`
	Busts    float64 `json:"busts"`
}

// GenerateReport creates a comprehensive test report
func (r *Reporter) GenerateReport(mode TestMode, batches []BatchResult, startTime, endTime time.Time) (*ReportResult, error) {
	duration := endTime.Sub(startTime)

	// Calculate total hands actually played
	totalHands := 0
	for _, batch := range batches {
		// Use actual hands from results if available
		if actualHands, ok := batch.Results["actual_hands"]; ok {
			totalHands += int(actualHands)
		} else {
			totalHands += batch.Hands
		}
	}

	handsPerSecond := float64(totalHands) / duration.Seconds()

	// Generate test ID
	testID := fmt.Sprintf("regression-%s-%s", string(mode), startTime.Format("20060102-150405"))

	// Build metadata
	metadata := ReportMetadata{
		StartTime:        startTime,
		EndTime:          endTime,
		DurationSeconds:  duration.Seconds(),
		HandsPerSecond:   handsPerSecond,
		TotalHandsPlayed: totalHands,
		BatchesCompleted: len(batches),
	}

	// Build configuration
	config := ReportConfig{
		HandsRequested:    r.config.HandsTotal,
		HandsCompleted:    totalHands,
		BatchSize:         r.config.BatchSize,
		TotalBatches:      len(batches),
		Seeds:             r.config.Seeds,
		SignificanceLevel: r.config.SignificanceLevel,
		EarlyStopping:     r.config.EarlyStopping,
		InfiniteBankroll:  r.config.InfiniteBankroll,
		StartingChips:     r.config.StartingChips,
	}

	// Mode-specific configuration
	switch mode {
	case ModeHeadsUp:
		config.BotA = r.config.BotA
		config.BotB = r.config.BotB
	case ModePopulation, ModeNPCBenchmark:
		config.Challenger = r.config.Challenger
		config.Baseline = r.config.Baseline
	case ModeSelfPlay:
		config.BotA = r.config.BotA
	}

	// Aggregate statistics based on mode
	stats := r.aggregateStatistics(mode, batches)

	// Add sample size warning if needed
	if totalHands < 5000 {
		stats.SampleSizeWarning = "⚠️ Small sample size - results may be unreliable"
	} else if totalHands < 10000 && math.Abs(stats.EffectSize) < 0.5 {
		stats.SampleSizeWarning = "Note: More hands needed for small effect sizes"
	}

	return &ReportResult{
		TestID:   testID,
		Mode:     string(mode),
		Metadata: metadata,
		Config:   config,
		Batches:  batches,
		Results:  stats,
	}, nil
}

// aggregateStatistics combines batch results into final statistics
func (r *Reporter) aggregateStatistics(mode TestMode, batches []BatchResult) ReportStatistics {
	stats := ReportStatistics{}

	switch mode {
	case ModeHeadsUp:
		// Aggregate bot A stats
		botACombined := CombineBatches(batches, "bot_a")
		stats.BotAStats = &BotStatistics{
			BB100:    botACombined.BB100,
			CI95Low:  botACombined.BB100 - 1.96*10, // Placeholder CI
			CI95High: botACombined.BB100 + 1.96*10,
			VPIP:     botACombined.VPIP,
			PFR:      botACombined.PFR,
			Hands:    botACombined.TotalHands,
			Timeouts: botACombined.Timeouts,
			Busts:    botACombined.Busts,
		}

		// Aggregate bot B stats
		botBCombined := CombineBatches(batches, "bot_b")
		stats.BotBStats = &BotStatistics{
			BB100:    botBCombined.BB100,
			CI95Low:  botBCombined.BB100 - 1.96*10, // Placeholder CI
			CI95High: botBCombined.BB100 + 1.96*10,
			VPIP:     botBCombined.VPIP,
			PFR:      botBCombined.PFR,
			Hands:    botBCombined.TotalHands,
			Timeouts: botBCombined.Timeouts,
			Busts:    botBCombined.Busts,
		}

		// Calculate effect size (placeholder Cohen's d)
		stats.EffectSize = (botACombined.BB100 - botBCombined.BB100) / 20.0

	case ModePopulation, ModeNPCBenchmark:
		// Aggregate challenger stats
		challengerCombined := CombineBatches(batches, "challenger")
		stats.ChallengerStats = &BotStatistics{
			BB100:    challengerCombined.BB100,
			CI95Low:  challengerCombined.BB100 - 1.96*10,
			CI95High: challengerCombined.BB100 + 1.96*10,
			VPIP:     challengerCombined.VPIP,
			PFR:      challengerCombined.PFR,
			Hands:    challengerCombined.TotalHands,
			Timeouts: challengerCombined.Timeouts,
			Busts:    challengerCombined.Busts,
		}

		// Aggregate baseline stats
		baselineCombined := CombineBatches(batches, "baseline")
		stats.BaselineStats = &BotStatistics{
			BB100:    baselineCombined.BB100,
			CI95Low:  baselineCombined.BB100 - 1.96*10,
			CI95High: baselineCombined.BB100 + 1.96*10,
			VPIP:     baselineCombined.VPIP,
			PFR:      baselineCombined.PFR,
			Hands:    baselineCombined.TotalHands,
			Timeouts: baselineCombined.Timeouts,
			Busts:    baselineCombined.Busts,
		}

		// Calculate effect size
		stats.EffectSize = (challengerCombined.BB100 - baselineCombined.BB100) / 20.0

	case ModeSelfPlay:
		// For self-play, we expect near-zero BB/100
		combined := CombineBatches(batches, "bot")
		stats.BotAStats = &BotStatistics{
			BB100:    combined.BB100,
			CI95Low:  combined.BB100 - 1.96*10,
			CI95High: combined.BB100 + 1.96*10,
			VPIP:     combined.VPIP,
			PFR:      combined.PFR,
			Hands:    combined.TotalHands,
			Timeouts: combined.Timeouts,
			Busts:    combined.Busts,
		}

		// Effect size is BB/100 divided by expected variance
		stats.EffectSize = combined.BB100 / 20.0
	}

	// Placeholder p-value calculation
	stats.PValue = 2.0 * (1.0 - normalCDF(math.Abs(stats.EffectSize)))
	stats.AdjustedPValue = stats.PValue // No correction for now
	stats.IsSignificant = stats.AdjustedPValue < r.config.SignificanceLevel

	return stats
}

// WriteJSON outputs the report as JSON
func (r *Reporter) WriteJSON(report *ReportResult) error {
	encoder := json.NewEncoder(r.writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

// WriteSummary outputs a human-readable summary
func (r *Reporter) WriteSummary(report *ReportResult) error {
	var sb strings.Builder

	// Header
	sb.WriteString("\nRegression Test Report\n")
	sb.WriteString("======================\n")

	// Test configuration
	switch TestMode(report.Mode) {
	case ModeHeadsUp:
		sb.WriteString(fmt.Sprintf("Bot A: %s\n", report.Config.BotA))
		sb.WriteString(fmt.Sprintf("Bot B: %s\n", report.Config.BotB))
	case ModePopulation, ModeNPCBenchmark:
		sb.WriteString(fmt.Sprintf("Challenger: %s\n", report.Config.Challenger))
		sb.WriteString(fmt.Sprintf("Baseline: %s\n", report.Config.Baseline))
	case ModeSelfPlay:
		sb.WriteString(fmt.Sprintf("Bot: %s\n", report.Config.BotA))
	}

	sb.WriteString(fmt.Sprintf("Mode: %s\n", report.Mode))
	sb.WriteString(fmt.Sprintf("Hands: %d\n", report.Metadata.TotalHandsPlayed))
	sb.WriteString(fmt.Sprintf("Duration: %.1fs\n", report.Metadata.DurationSeconds))
	sb.WriteString("\n")

	// Results section
	sb.WriteString("Results\n")
	sb.WriteString("-------\n")

	// Mode-specific results
	switch TestMode(report.Mode) {
	case ModeHeadsUp:
		if report.Results.BotAStats != nil {
			sb.WriteString(fmt.Sprintf("Bot A BB/100: %.2f (VPIP: %.1f%%, PFR: %.1f%%)\n",
				report.Results.BotAStats.BB100,
				report.Results.BotAStats.VPIP*100,
				report.Results.BotAStats.PFR*100))
		}
		if report.Results.BotBStats != nil {
			sb.WriteString(fmt.Sprintf("Bot B BB/100: %.2f (VPIP: %.1f%%, PFR: %.1f%%)\n",
				report.Results.BotBStats.BB100,
				report.Results.BotBStats.VPIP*100,
				report.Results.BotBStats.PFR*100))
		}

	case ModePopulation, ModeNPCBenchmark:
		if report.Results.ChallengerStats != nil {
			sb.WriteString(fmt.Sprintf("Challenger BB/100: %.2f (VPIP: %.1f%%, PFR: %.1f%%)\n",
				report.Results.ChallengerStats.BB100,
				report.Results.ChallengerStats.VPIP*100,
				report.Results.ChallengerStats.PFR*100))
		}
		if report.Results.BaselineStats != nil {
			sb.WriteString(fmt.Sprintf("Baseline BB/100: %.2f (VPIP: %.1f%%, PFR: %.1f%%)\n",
				report.Results.BaselineStats.BB100,
				report.Results.BaselineStats.VPIP*100,
				report.Results.BaselineStats.PFR*100))
		}

	case ModeSelfPlay:
		if report.Results.BotAStats != nil {
			sb.WriteString(fmt.Sprintf("Average BB/100: %.2f (expected ~0)\n", report.Results.BotAStats.BB100))
			sb.WriteString(fmt.Sprintf("VPIP: %.1f%%, PFR: %.1f%%\n",
				report.Results.BotAStats.VPIP*100,
				report.Results.BotAStats.PFR*100))
		}
	}

	// Statistical analysis
	sb.WriteString(fmt.Sprintf("Effect Size: %.2f (%s)\n",
		report.Results.EffectSize,
		interpretEffectSize(report.Results.EffectSize)))
	sb.WriteString(fmt.Sprintf("P-Value: %.3f (adjusted: %.3f)\n",
		report.Results.PValue,
		report.Results.AdjustedPValue))
	sb.WriteString(fmt.Sprintf("Hands/sec: %.0f\n", report.Metadata.HandsPerSecond))

	// Sample size warning if present
	if report.Results.SampleSizeWarning != "" {
		sb.WriteString(fmt.Sprintf("\n%s\n", report.Results.SampleSizeWarning))
	}

	// Verdict
	sb.WriteString("\n")
	if report.Results.IsSignificant {
		sb.WriteString(fmt.Sprintf("Verdict: REJECT (%.0f%% confidence)\n",
			(1-r.config.SignificanceLevel)*100))
	} else {
		sb.WriteString("Verdict: NO SIGNIFICANT DIFFERENCE\n")
	}

	_, err := fmt.Fprint(r.writer, sb.String())
	return err
}

// interpretEffectSize provides a human-readable interpretation
func interpretEffectSize(d float64) string {
	absD := math.Abs(d)
	switch {
	case absD < 0.2:
		return "negligible"
	case absD < 0.5:
		return "small"
	case absD < 0.8:
		return "medium"
	default:
		return "large"
	}
}

// normalCDF approximates the cumulative distribution function of standard normal
func normalCDF(z float64) float64 {
	// Using approximation from Abramowitz and Stegun
	const (
		a1 = 0.254829592
		a2 = -0.284496736
		a3 = 1.421413741
		a4 = -1.453152027
		a5 = 1.061405429
		p  = 0.3275911
	)

	sign := 1.0
	if z < 0 {
		sign = -1.0
	}
	z = math.Abs(z) / math.Sqrt(2.0)

	t := 1.0 / (1.0 + p*z)
	t2 := t * t
	t3 := t2 * t
	t4 := t3 * t
	t5 := t4 * t

	y := 1.0 - (((((a5*t5+a4)*t4+a3)*t3+a2)*t2+a1)*t)*math.Exp(-z*z)

	return 0.5 * (1.0 + sign*y)
}
