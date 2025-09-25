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
	LatencyWarning    string `json:"latency_warning,omitempty"`
}

// BotStatistics contains per-bot performance metrics
type BotStatistics struct {
	BB100               float64 `json:"bb_per_100"`
	CI95Low             float64 `json:"ci_95_low"`
	CI95High            float64 `json:"ci_95_high"`
	VPIP                float64 `json:"vpip"`
	PFR                 float64 `json:"pfr"`
	Hands               int     `json:"hands"`
	Timeouts            float64 `json:"timeouts"`
	Busts               float64 `json:"busts"`
	AvgResponseMs       float64 `json:"avg_response_ms,omitempty"`
	P95ResponseMs       float64 `json:"p95_response_ms,omitempty"`
	MaxResponseMs       float64 `json:"max_response_ms,omitempty"`
	MinResponseMs       float64 `json:"min_response_ms,omitempty"`
	ResponseStdMs       float64 `json:"response_std_ms,omitempty"`
	ResponsesTracked    float64 `json:"responses_tracked,omitempty"`
	ResponseTimeouts    float64 `json:"response_timeouts,omitempty"`
	ResponseDisconnects float64 `json:"response_disconnects,omitempty"`
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

	latencyThreshold := r.config.LatencyWarningThresholdMs
	if latencyThreshold <= 0 {
		latencyThreshold = 100
	}
	var latencyWarnings []string
	if stats.ChallengerStats != nil && stats.ChallengerStats.P95ResponseMs > latencyThreshold {
		latencyWarnings = append(latencyWarnings,
			fmt.Sprintf("⚠️ Challenger p95 response %.1f ms exceeds %.1f ms threshold",
				stats.ChallengerStats.P95ResponseMs, latencyThreshold))
	}
	if stats.BaselineStats != nil && stats.BaselineStats.P95ResponseMs > latencyThreshold {
		latencyWarnings = append(latencyWarnings,
			fmt.Sprintf("⚠️ Baseline p95 response %.1f ms exceeds %.1f ms threshold",
				stats.BaselineStats.P95ResponseMs, latencyThreshold))
	}
	if len(latencyWarnings) > 0 {
		stats.LatencyWarning = strings.Join(latencyWarnings, "\n")
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
			BB100:               challengerStats.Mean,
			CI95Low:             challengerStats.CI95Low,
			CI95High:            challengerStats.CI95High,
			VPIP:                challengerCombined.VPIP,
			PFR:                 challengerCombined.PFR,
			Hands:               challengerHands,
			Timeouts:            challengerCombined.Timeouts * float64(challengerHands),
			Busts:               challengerCombined.Busts * float64(challengerHands),
			AvgResponseMs:       challengerCombined.AvgResponseMs,
			P95ResponseMs:       challengerCombined.P95ResponseMs,
			MaxResponseMs:       challengerCombined.MaxResponseMs,
			MinResponseMs:       challengerCombined.MinResponseMs,
			ResponseStdMs:       challengerCombined.ResponseStdMs,
			ResponsesTracked:    challengerCombined.ResponsesTracked,
			ResponseTimeouts:    challengerCombined.ResponseTimeouts,
			ResponseDisconnects: challengerCombined.ResponseDisconnects,
		}
		stats.BaselineStats = &BotStatistics{
			BB100:               baselineStats.Mean,
			CI95Low:             baselineStats.CI95Low,
			CI95High:            baselineStats.CI95High,
			VPIP:                baselineCombined.VPIP,
			PFR:                 baselineCombined.PFR,
			Hands:               baselineHands,
			Timeouts:            baselineCombined.Timeouts * float64(baselineHands),
			Busts:               baselineCombined.Busts * float64(baselineHands),
			AvgResponseMs:       baselineCombined.AvgResponseMs,
			P95ResponseMs:       baselineCombined.P95ResponseMs,
			MaxResponseMs:       baselineCombined.MaxResponseMs,
			MinResponseMs:       baselineCombined.MinResponseMs,
			ResponseStdMs:       baselineCombined.ResponseStdMs,
			ResponsesTracked:    baselineCombined.ResponsesTracked,
			ResponseTimeouts:    baselineCombined.ResponseTimeouts,
			ResponseDisconnects: baselineCombined.ResponseDisconnects,
		}

	case ModeSelfPlay:
		selfStats := CalculateStatistics(batches, "avg_bb_per_100", "actual_hands")
		totalHands := selfStats.SampleSize
		if totalHands == 0 {
			totalHands = CalculateTotalHands(batches, "actual_hands")
		}

		avgVPIP := WeightedAverage(batches, "avg_vpip", "actual_hands")
		avgPFR := WeightedAverage(batches, "avg_pfr", "actual_hands")
		avgResponse, totalResponses := WeightedAverageWithWeights(batches, "avg_response_ms", "responses_tracked")
		responseStd := WeightedStdDevWithWeights(batches, "avg_response_ms", "response_std_ms", "responses_tracked", avgResponse)
		maxResponse := MaxMetric(batches, "max_response_ms")
		minResponse := MinMetric(batches, "min_response_ms")
		p95Response := MaxMetric(batches, "p95_response_ms")
		responseTimeouts := SumMetric(batches, "response_timeouts")
		responseDisconnects := SumMetric(batches, "response_disconnects")

		stats.ChallengerStats = &BotStatistics{
			BB100:               selfStats.Mean,
			CI95Low:             selfStats.CI95Low,
			CI95High:            selfStats.CI95High,
			VPIP:                avgVPIP,
			PFR:                 avgPFR,
			Hands:               totalHands,
			Timeouts:            0,
			Busts:               0,
			AvgResponseMs:       avgResponse,
			P95ResponseMs:       p95Response,
			MaxResponseMs:       maxResponse,
			MinResponseMs:       minResponse,
			ResponseStdMs:       responseStd,
			ResponsesTracked:    totalResponses,
			ResponseTimeouts:    responseTimeouts,
			ResponseDisconnects: responseDisconnects,
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

	if report.Results.ChallengerStats != nil && report.Results.ChallengerStats.ResponsesTracked > 0 {
		label := "Challenger"
		if report.Mode == string(ModeSelfPlay) {
			label = "Bot"
		}
		sb.WriteString(fmt.Sprintf("%s latency p95: %.1f ms (avg %.1f, max %.1f, std %.1f, samples %.0f, timeouts %.0f)\n",
			label,
			report.Results.ChallengerStats.P95ResponseMs,
			report.Results.ChallengerStats.AvgResponseMs,
			report.Results.ChallengerStats.MaxResponseMs,
			report.Results.ChallengerStats.ResponseStdMs,
			report.Results.ChallengerStats.ResponsesTracked,
			report.Results.ChallengerStats.ResponseTimeouts))
	}
	if report.Results.BaselineStats != nil && report.Mode != string(ModeSelfPlay) && report.Results.BaselineStats.ResponsesTracked > 0 {
		sb.WriteString(fmt.Sprintf("Baseline latency p95: %.1f ms (avg %.1f, max %.1f, std %.1f, samples %.0f, timeouts %.0f)\n",
			report.Results.BaselineStats.P95ResponseMs,
			report.Results.BaselineStats.AvgResponseMs,
			report.Results.BaselineStats.MaxResponseMs,
			report.Results.BaselineStats.ResponseStdMs,
			report.Results.BaselineStats.ResponsesTracked,
			report.Results.BaselineStats.ResponseTimeouts))
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

	// Latency warning if present
	if report.Results.LatencyWarning != "" {
		sb.WriteString(fmt.Sprintf("\n%s\n", report.Results.LatencyWarning))
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
