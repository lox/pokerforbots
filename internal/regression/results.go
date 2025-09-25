package regression

import "time"

// TestResult holds results from a test run
type TestResult struct {
	TestID      string             `json:"test_id"`
	Mode        TestMode           `json:"mode"`
	Metadata    TestMetadata       `json:"metadata"`
	Config      TestConfigSummary  `json:"configuration"`
	Batches     []BatchResult      `json:"batches"`
	Aggregate   AggregateResults   `json:"aggregate"`
	Performance PerformanceMetrics `json:"performance_metrics"`
	Errors      ErrorSummary       `json:"error_summary"`
	Verdict     TestVerdict        `json:"verdict"`
}

// TestMetadata contains test execution metadata
type TestMetadata struct {
	StartTime       time.Time `json:"start_time"`
	DurationSeconds float64   `json:"duration_seconds"`
	ServerVersion   string    `json:"server_version"`
	TestEnvironment string    `json:"test_environment"`
}

// TestConfigSummary summarizes test configuration
type TestConfigSummary struct {
	Challenger             string  `json:"challenger"`
	Baseline               string  `json:"baseline"`
	HandsTotal             int     `json:"hands_total"`
	Batches                int     `json:"batches"`
	BatchSize              int     `json:"batch_size"`
	SignificanceLevel      float64 `json:"significance_level"`
	MultipleTestCorrection bool    `json:"multiple_test_correction"`
}

// BatchResult contains results from a single batch
type BatchResult struct {
	Seed    int64              `json:"seed"`
	Hands   int                `json:"hands"`
	Results map[string]float64 `json:"results"`
	StdDevs map[string]float64 `json:"std_devs,omitempty"` // Standard deviations for each metric
}

// AggregateResults contains aggregated bot results
type AggregateResults struct {
	Challenger *BotResults `json:"challenger,omitempty"`
	Baseline   *BotResults `json:"baseline,omitempty"`
}

// BotResults contains statistical results for a bot
type BotResults struct {
	BBPer100            float64 `json:"bb_per_100"`
	CI95Low             float64 `json:"ci_95_low"`
	CI95High            float64 `json:"ci_95_high"`
	VPIP                float64 `json:"vpip"`
	PFR                 float64 `json:"pfr"`
	AggressionFactor    float64 `json:"aggression_factor"`
	BustRate            float64 `json:"bust_rate"`
	EffectSize          float64 `json:"effect_size,omitempty"`
	AvgResponseMs       float64 `json:"avg_response_ms,omitempty"`
	P95ResponseMs       float64 `json:"p95_response_ms,omitempty"`
	MaxResponseMs       float64 `json:"max_response_ms,omitempty"`
	MinResponseMs       float64 `json:"min_response_ms,omitempty"`
	ResponseStdMs       float64 `json:"response_std_ms,omitempty"`
	ResponsesTracked    float64 `json:"responses_tracked,omitempty"`
	ResponseTimeouts    float64 `json:"response_timeouts,omitempty"`
	ResponseDisconnects float64 `json:"response_disconnects,omitempty"`
}

// PerformanceMetrics contains performance data
type PerformanceMetrics struct {
	HandsPerSecond   float64 `json:"hands_per_second"`
	SampleAssessment string  `json:"sample_assessment,omitempty"`
}

// ErrorSummary contains error statistics
type ErrorSummary struct {
	BotCrashes       int `json:"bot_crashes"`
	Timeouts         int `json:"timeouts"`
	ConnectionErrors int `json:"connection_errors"`
	RecoveredCrashes int `json:"recovered_crashes"`
}

// TestVerdict contains the final test verdict
type TestVerdict struct {
	SignificantDifference bool    `json:"significant_difference"`
	PValue                float64 `json:"p_value"`
	AdjustedPValue        float64 `json:"adjusted_p_value,omitempty"`
	EffectSize            float64 `json:"effect_size"`
	Direction             string  `json:"direction"`
	Confidence            float64 `json:"confidence"`
	Recommendation        string  `json:"recommendation"`
}
