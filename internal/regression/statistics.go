package regression

import (
	"math"
	"strings"
	"sync"

	"github.com/rs/zerolog"
	"gonum.org/v1/gonum/stat"
	"gonum.org/v1/gonum/stat/distuv"
)

const (
	defaultMinStdDevBB100      = 5.0
	defaultFallbackStdDevBB100 = 50.0
)

type statisticsClampConfig struct {
	minStdDevBB100      float64
	fallbackStdDevBB100 float64
	warnOnClamp         bool
	logger              zerolog.Logger
}

var (
	clampMu            sync.RWMutex
	currentClampConfig = statisticsClampConfig{
		minStdDevBB100:      defaultMinStdDevBB100,
		fallbackStdDevBB100: defaultFallbackStdDevBB100,
		warnOnClamp:         false,
		logger:              zerolog.Nop(),
	}
)

type clampReason string

const (
	clampReasonBelowMin  clampReason = "below_min_threshold"
	clampReasonMissing   clampReason = "missing_std_dev"
	clampReasonAggregate clampReason = "aggregate_clamp"
)

type clampNotice struct {
	metricKey string
	observed  float64
	hands     int
	reason    clampReason
}

// StatisticsClampConfig configures how standard deviations are clamped during analysis.
type StatisticsClampConfig struct {
	MinStdDevBB100      float64
	FallbackStdDevBB100 float64
	WarnOnClamp         bool
	Logger              *zerolog.Logger
}

// ConfigureStatisticsClamp overrides the default clamp behaviour for statistical calculations.
func ConfigureStatisticsClamp(cfg StatisticsClampConfig) {
	clampMu.Lock()
	defer clampMu.Unlock()

	min := cfg.MinStdDevBB100
	if min <= 0 {
		min = defaultMinStdDevBB100
	}
	fallback := cfg.FallbackStdDevBB100
	if fallback <= 0 {
		fallback = defaultFallbackStdDevBB100
	}
	if fallback < min {
		fallback = min
	}

	logger := zerolog.Nop()
	if cfg.Logger != nil {
		logger = cfg.Logger.With().Logger()
	}

	currentClampConfig = statisticsClampConfig{
		minStdDevBB100:      min,
		fallbackStdDevBB100: fallback,
		warnOnClamp:         cfg.WarnOnClamp,
		logger:              logger,
	}
}

func getClampConfig() statisticsClampConfig {
	clampMu.RLock()
	defer clampMu.RUnlock()
	return currentClampConfig
}

func logClampNotices(cfg statisticsClampConfig, notices []clampNotice) {
	if !cfg.warnOnClamp || len(notices) == 0 {
		return
	}

	logger := cfg.logger
	for _, notice := range notices {
		event := logger.Warn().
			Str("metric", notice.metricKey).
			Float64("min_std_dev_bb100", cfg.minStdDevBB100).
			Float64("fallback_std_dev_bb100", cfg.fallbackStdDevBB100).
			Str("reason", string(notice.reason))

		if notice.hands > 0 {
			event = event.Int("hands", notice.hands)
		}
		if notice.observed > 0 {
			event = event.Float64("observed_std_dev_bb100", notice.observed)
		}

		event.Msg("standard deviation clamped")
	}
}

// StatisticalResult contains the results of statistical analysis
type StatisticalResult struct {
	Mean       float64
	StdDev     float64
	CI95Low    float64
	CI95High   float64
	SampleSize int
}

// StatisticalComparison contains the results of comparing two samples
type StatisticalComparison struct {
	Difference float64 // Mean difference
	StdError   float64 // Standard error of difference
	TStatistic float64 // T-statistic
	PValue     float64 // P-value from t-test
	EffectSize float64 // Cohen's d
	CI95Low    float64 // 95% CI for difference
	CI95High   float64 // 95% CI for difference
}

// CalculateStatistics computes statistics for a set of batch results
func CalculateStatistics(batches []BatchResult, metricKey string, handsKey string) StatisticalResult {
	cfg := getClampConfig()
	var clampNotices []clampNotice

	var values []float64
	var weights []float64
	var stdDevs []float64

	// Determine the standard deviation key based on the metric key
	stdDevKey := ""
	switch {
	case strings.Contains(metricKey, "challenger"):
		stdDevKey = "challenger_std_dev"
	case strings.Contains(metricKey, "baseline"):
		stdDevKey = "baseline_std_dev"
	case strings.Contains(metricKey, "bot"):
		stdDevKey = "bot_std_dev"
	}

	// Collect all values, weights, and standard deviations
	for _, batch := range batches {
		if value, exists := batch.Results[metricKey]; exists {
			hands := ExtractActualHands(batch, handsKey)
			if hands > 0 {
				values = append(values, value)
				weights = append(weights, float64(hands))

				// Try to get the per-hand standard deviation
				if stdDevKey != "" {
					if sd, exists := batch.Results[stdDevKey]; exists && sd > 0 {
						// The std dev from stats is in BB/hand, need to convert to BB/100
						// Standard deviation scales with sqrt(n) for different sample sizes
						// To convert from per-hand to per-100-hands: multiply by sqrt(100) = 10
						sdBB100 := sd * 10

						if sdBB100 < cfg.minStdDevBB100 {
							stdDevs = append(stdDevs, cfg.fallbackStdDevBB100)
							clampNotices = append(clampNotices, clampNotice{
								metricKey: metricKey,
								observed:  sdBB100,
								hands:     hands,
								reason:    clampReasonBelowMin,
							})
						} else {
							stdDevs = append(stdDevs, sdBB100)
						}
					} else {
						stdDevs = append(stdDevs, cfg.fallbackStdDevBB100)
						clampNotices = append(clampNotices, clampNotice{
							metricKey: metricKey,
							observed:  0,
							hands:     hands,
							reason:    clampReasonMissing,
						})
					}
				}
			}
		}
	}

	if len(values) == 0 {
		return StatisticalResult{}
	}

	// Calculate weighted mean using gonum
	mean := stat.Mean(values, weights)

	// Calculate standard deviation
	var stdDev float64
	if len(stdDevs) > 0 {
		// Use pooled standard deviation from per-hand std devs
		stdDev = calculatePooledStdDevWeighted(stdDevs, weights)
	} else {
		// Fall back to calculating from batch means using gonum
		// This gives us the standard deviation of the batch means
		variance := stat.Variance(values, weights)
		stdDev = math.Sqrt(variance)

		// For poker, if the batch-level std dev is very small, use fallback
	}

	if stdDev < cfg.minStdDevBB100 {
		clampNotices = append(clampNotices, clampNotice{
			metricKey: metricKey,
			observed:  stdDev,
			reason:    clampReasonAggregate,
		})
		stdDev = cfg.fallbackStdDevBB100
	}

	logClampNotices(cfg, clampNotices)

	// Calculate total sample size
	totalHands := 0
	for _, w := range weights {
		totalHands += int(w)
	}

	// Calculate 95% confidence interval using t-distribution
	ci95Low, ci95High := calculateCI95(mean, stdDev, totalHands)

	return StatisticalResult{
		Mean:       mean,
		StdDev:     stdDev,
		CI95Low:    ci95Low,
		CI95High:   ci95High,
		SampleSize: totalHands,
	}
}

// CompareStatistics performs statistical comparison between two groups
func CompareStatistics(group1Stats, group2Stats StatisticalResult) StatisticalComparison {
	// Calculate difference
	difference := group1Stats.Mean - group2Stats.Mean

	// Calculate pooled standard deviation (for effect size)
	pooledStdDev := calculatePooledStdDev(
		group1Stats.StdDev, group1Stats.SampleSize,
		group2Stats.StdDev, group2Stats.SampleSize,
	)

	// Calculate Cohen's d effect size
	effectSize := 0.0
	if pooledStdDev > 0 {
		effectSize = difference / pooledStdDev
	}

	// Calculate standard error of difference
	se1 := group1Stats.StdDev / math.Sqrt(float64(group1Stats.SampleSize))
	se2 := group2Stats.StdDev / math.Sqrt(float64(group2Stats.SampleSize))
	sePooled := math.Sqrt(se1*se1 + se2*se2)

	// Calculate t-statistic
	tStat := 0.0
	if sePooled > 0 {
		tStat = difference / sePooled
	}

	// Calculate degrees of freedom using Welch's approximation
	df := calculateWelchDF(
		group1Stats.StdDev, group1Stats.SampleSize,
		group2Stats.StdDev, group2Stats.SampleSize,
	)

	// Calculate p-value from t-distribution using gonum
	pValue := calculatePValue(tStat, df)

	// Calculate 95% CI for difference using t-distribution
	tDist := distuv.StudentsT{
		Nu:    float64(df),
		Mu:    0,
		Sigma: 1,
	}
	// Two-tailed 95% CI uses 97.5th percentile
	tCritical := tDist.Quantile(0.975)
	ciMargin := tCritical * sePooled

	return StatisticalComparison{
		Difference: difference,
		StdError:   sePooled,
		TStatistic: tStat,
		PValue:     pValue,
		EffectSize: effectSize,
		CI95Low:    difference - ciMargin,
		CI95High:   difference + ciMargin,
	}
}

// calculatePooledStdDev calculates pooled standard deviation for two groups
func calculatePooledStdDev(sd1 float64, n1 int, sd2 float64, n2 int) float64 {
	if n1+n2 <= 2 {
		return 0
	}

	// Pooled variance formula
	var1 := sd1 * sd1
	var2 := sd2 * sd2

	pooledVar := ((float64(n1-1) * var1) + (float64(n2-1) * var2)) / float64(n1+n2-2)

	return math.Sqrt(pooledVar)
}

// calculatePooledStdDevWeighted calculates pooled standard deviation from weighted samples
func calculatePooledStdDevWeighted(stdDevs []float64, weights []float64) float64 {
	if len(stdDevs) == 0 || len(weights) == 0 {
		return 0
	}

	// Calculate weighted pooled variance
	var sumWeightedVar float64
	var sumWeights float64

	for i := range stdDevs {
		if i < len(weights) {
			variance := stdDevs[i] * stdDevs[i]
			sumWeightedVar += weights[i] * variance
			sumWeights += weights[i]
		}
	}

	if sumWeights <= 0 {
		return 0
	}

	pooledVar := sumWeightedVar / sumWeights
	return math.Sqrt(pooledVar)
}

// calculateCI95 calculates 95% confidence interval using t-distribution
func calculateCI95(mean, stdDev float64, n int) (float64, float64) {
	if n <= 1 {
		// Return wide interval for tiny samples
		return mean - 100, mean + 100
	}

	// Standard error
	se := stdDev / math.Sqrt(float64(n))

	// Use t-distribution from gonum
	df := float64(n - 1)
	tDist := distuv.StudentsT{
		Nu:    df,
		Mu:    0,
		Sigma: 1,
	}

	// Two-tailed 95% CI uses 97.5th percentile
	tCritical := tDist.Quantile(0.975)

	// Margin of error
	margin := tCritical * se

	return mean - margin, mean + margin
}

// calculateWelchDF calculates degrees of freedom using Welch's approximation
func calculateWelchDF(sd1 float64, n1 int, sd2 float64, n2 int) int {
	if n1 <= 1 || n2 <= 1 {
		// Avoid division by zero
		return 2
	}

	v1 := sd1 * sd1 / float64(n1)
	v2 := sd2 * sd2 / float64(n2)

	numerator := (v1 + v2) * (v1 + v2)
	denominator := (v1*v1)/float64(n1-1) + (v2*v2)/float64(n2-1)

	if denominator == 0 {
		return n1 + n2 - 2 // Fall back to pooled df
	}

	df := numerator / denominator
	return int(math.Floor(df))
}

// calculatePValue calculates p-value from t-statistic and df using proper t-distribution
func calculatePValue(tStat float64, df int) float64 {
	if df <= 0 {
		return 1.0
	}

	// Create t-distribution with given degrees of freedom
	tDist := distuv.StudentsT{
		Nu:    float64(df),
		Mu:    0,
		Sigma: 1,
	}

	// For two-tailed test, calculate P(|T| > |t|)
	// This is 2 * P(T > |t|) due to symmetry
	absTStat := math.Abs(tStat)

	// CDF gives P(T <= t), so P(T > t) = 1 - CDF(t)
	pOneTail := 1 - tDist.CDF(absTStat)

	// Two-tailed p-value
	pValue := 2 * pOneTail

	// Clamp to [0, 1] to handle numerical issues
	if pValue > 1 {
		pValue = 1
	} else if pValue < 0 {
		pValue = 0
	}

	return pValue
}

// InterpretEffectSize returns a human-readable interpretation of Cohen's d
func InterpretEffectSize(d float64) string {
	absd := math.Abs(d)
	switch {
	case absd < 0.2:
		return "negligible"
	case absd < 0.5:
		return "small"
	case absd < 0.8:
		return "medium"
	default:
		return "large"
	}
}

// InterpretPValue returns a human-readable interpretation of p-value
func InterpretPValue(p float64, alpha float64) string {
	switch {
	case p < 0.001:
		return "highly significant"
	case p < 0.01:
		return "very significant"
	case p < alpha:
		return "significant"
	case p < 0.10:
		return "marginally significant"
	default:
		return "not significant"
	}
}
