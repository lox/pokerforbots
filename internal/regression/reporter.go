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
	Challenger        string  `json:"challenger"`
	Baseline          string  `json:"baseline"`
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

	// Statistical analysis
	EffectSize     float64 `json:"effect_size"`
	PValue         float64 `json:"p_value"`
	AdjustedPValue float64 `json:"adjusted_p_value"`
	IsSignificant  bool    `json:"is_significant"`
	Direction      string  `json:"direction"`
	Recommendation string  `json:"recommendation"`

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
func (r *Reporter) GenerateReport(result *TestResult) (*ReportResult, error) {
	batches := result.Batches

	// Calculate total hands actually played
	totalHands := 0
	for _, batch := range batches {
		if actualHands, ok := batch.Results["actual_hands"]; ok {
			totalHands += int(actualHands)
		} else {
			totalHands += batch.Hands
		}
	}

	duration := time.Duration(result.Metadata.DurationSeconds * float64(time.Second))
	if duration < 0 {
		duration = 0
	}

	startTime := result.Metadata.StartTime
	endTime := startTime.Add(duration)

	metadata := ReportMetadata{
		StartTime:        startTime,
		EndTime:          endTime,
		DurationSeconds:  duration.Seconds(),
		ServerVersion:    result.Metadata.ServerVersion,
		TestEnvironment:  result.Metadata.TestEnvironment,
		TotalHandsPlayed: totalHands,
		BatchesCompleted: len(batches),
	}
	if duration.Seconds() > 0 {
		metadata.HandsPerSecond = float64(totalHands) / duration.Seconds()
	}

	config := ReportConfig{
		Challenger:        result.Config.Challenger,
		Baseline:          result.Config.Baseline,
		HandsRequested:    result.Config.HandsTotal,
		HandsCompleted:    totalHands,
		BatchSize:         result.Config.BatchSize,
		TotalBatches:      len(batches),
		Seeds:             r.config.Seeds,
		SignificanceLevel: r.config.SignificanceLevel,
		EarlyStopping:     r.config.EarlyStopping,
		InfiniteBankroll:  r.config.InfiniteBankroll,
		StartingChips:     r.config.StartingChips,
	}

	stats := r.aggregateStatistics(result)

	// Add sample size warning if needed
	if totalHands < 5000 {
		stats.SampleSizeWarning = "⚠️ Small sample size - results may be unreliable"
	} else if totalHands < 10000 && math.Abs(stats.EffectSize) < 0.5 {
		stats.SampleSizeWarning = "Note: More hands needed for small effect sizes"
	}

	testID := result.TestID
	if testID == "" {
		testID = fmt.Sprintf("regression-%s-%s", string(result.Mode), startTime.Format("20060102-150405"))
	}

	return &ReportResult{
		TestID:   testID,
		Mode:     string(result.Mode),
		Metadata: metadata,
		Config:   config,
		Batches:  batches,
		Results:  stats,
	}, nil
}

// aggregateStatistics combines batch results into final statistics
func (r *Reporter) aggregateStatistics(result *TestResult) ReportStatistics {
	stats := ReportStatistics{}
	batches := result.Batches

	switch result.Mode {
	case ModeHeadsUp, ModePopulation, ModeNPCBenchmark:
		challengerCombined := CombineBatches(batches, "challenger")
		baselineCombined := CombineBatches(batches, "baseline")

		challengerStats := CalculateStatistics(batches, "challenger_bb_per_100", "challenger_hands")
		baselineStats := CalculateStatistics(batches, "baseline_bb_per_100", "baseline_hands")

		challengerHands := challengerStats.SampleSize
		if challengerHands == 0 {
			challengerHands = challengerCombined.TotalHands
		}
		baselineHands := baselineStats.SampleSize
		if baselineHands == 0 {
			baselineHands = baselineCombined.TotalHands
		}

		stats.ChallengerStats = &BotStatistics{
			BB100:    challengerStats.Mean,
			CI95Low:  challengerStats.CI95Low,
			CI95High: challengerStats.CI95High,
			VPIP:     challengerCombined.VPIP,
			PFR:      challengerCombined.PFR,
			Hands:    challengerHands,
			Timeouts: challengerCombined.Timeouts * float64(challengerHands),
			Busts:    challengerCombined.Busts * float64(challengerHands),
		}
		stats.BaselineStats = &BotStatistics{
			BB100:    baselineStats.Mean,
			CI95Low:  baselineStats.CI95Low,
			CI95High: baselineStats.CI95High,
			VPIP:     baselineCombined.VPIP,
			PFR:      baselineCombined.PFR,
			Hands:    baselineHands,
			Timeouts: baselineCombined.Timeouts * float64(baselineHands),
			Busts:    baselineCombined.Busts * float64(baselineHands),
		}

	case ModeSelfPlay:
		selfStats := CalculateStatistics(batches, "avg_bb_per_100", "actual_hands")
		totalHands := selfStats.SampleSize
		if totalHands == 0 {
			totalHands = CalculateTotalHands(batches, "actual_hands")
		}

		avgVPIP := WeightedAverage(batches, "avg_vpip", "actual_hands")
		avgPFR := WeightedAverage(batches, "avg_pfr", "actual_hands")

		stats.ChallengerStats = &BotStatistics{
			BB100:    selfStats.Mean,
			CI95Low:  selfStats.CI95Low,
			CI95High: selfStats.CI95High,
			VPIP:     avgVPIP,
			PFR:      avgPFR,
			Hands:    totalHands,
			Timeouts: 0,
			Busts:    0,
		}
		stats.BaselineStats = stats.ChallengerStats
	}

	stats.EffectSize = result.Verdict.EffectSize
	stats.PValue = result.Verdict.PValue
	if result.Verdict.AdjustedPValue > 0 {
		stats.AdjustedPValue = math.Min(1.0, result.Verdict.AdjustedPValue)
	} else {
		stats.AdjustedPValue = result.Verdict.PValue
	}
	stats.IsSignificant = result.Verdict.SignificantDifference
	stats.Direction = result.Verdict.Direction
	stats.Recommendation = result.Verdict.Recommendation

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

	// Test configuration - unified for all modes
	sb.WriteString(fmt.Sprintf("Challenger: %s\n", report.Config.Challenger))
	if report.Mode == string(ModeSelfPlay) {
		sb.WriteString("(Self-play mode)\n")
	} else {
		sb.WriteString(fmt.Sprintf("Baseline: %s\n", report.Config.Baseline))
	}

	sb.WriteString(fmt.Sprintf("Mode: %s\n", report.Mode))
	sb.WriteString(fmt.Sprintf("Hands: %d\n", report.Metadata.TotalHandsPlayed))
	sb.WriteString(fmt.Sprintf("Duration: %.1fs\n", report.Metadata.DurationSeconds))
	sb.WriteString("\n")

	// Results section
	sb.WriteString("Results\n")
	sb.WriteString("-------\n")

	// Unified results display for all modes
	if report.Results.ChallengerStats != nil {
		if report.Mode == string(ModeSelfPlay) {
			sb.WriteString(fmt.Sprintf("Average BB/100: %.2f (expected ~0)\n", report.Results.ChallengerStats.BB100))
			sb.WriteString(fmt.Sprintf("VPIP: %.1f%%, PFR: %.1f%%\n",
				report.Results.ChallengerStats.VPIP,
				report.Results.ChallengerStats.PFR))
		} else {
			sb.WriteString(fmt.Sprintf("Challenger BB/100: %.2f (VPIP: %.1f%%, PFR: %.1f%%)\n",
				report.Results.ChallengerStats.BB100,
				report.Results.ChallengerStats.VPIP,
				report.Results.ChallengerStats.PFR))
		}
	}
	if report.Results.BaselineStats != nil && report.Mode != string(ModeSelfPlay) {
		sb.WriteString(fmt.Sprintf("Baseline BB/100: %.2f (VPIP: %.1f%%, PFR: %.1f%%)\n",
			report.Results.BaselineStats.BB100,
			report.Results.BaselineStats.VPIP,
			report.Results.BaselineStats.PFR))
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
	sb.WriteString(renderVerdictLine(report.Results, r.config.SignificanceLevel))

	_, err := fmt.Fprint(r.writer, sb.String())
	return err
}

func renderVerdictLine(stats ReportStatistics, alpha float64) string {
	confidence := (1 - alpha) * 100
	switch strings.ToLower(stats.Recommendation) {
	case "accept":
		return fmt.Sprintf("Verdict: ACCEPT (%.0f%% confidence)\n", confidence)
	case "reject":
		return fmt.Sprintf("Verdict: REJECT (%.0f%% confidence)\n", confidence)
	case "marginal":
		return fmt.Sprintf("Verdict: MARGINAL (%s, %.0f%% confidence)\n", stats.Direction, confidence)
	case "inconclusive":
		return "Verdict: INCONCLUSIVE\n"
	}
	if stats.IsSignificant {
		return fmt.Sprintf("Verdict: SIGNIFICANT (%s, %.0f%% confidence)\n", stats.Direction, confidence)
	}
	return "Verdict: NO SIGNIFICANT DIFFERENCE\n"
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
