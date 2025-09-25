package regression

import (
	"math"
	"testing"
)

// TestCalculateStatistics tests the statistical calculations
func TestCalculateStatistics(t *testing.T) {
	tests := []struct {
		name     string
		batches  []BatchResult
		metric   string
		hands    string
		wantMean float64
		wantSize int
	}{
		{
			name: "single batch",
			batches: []BatchResult{
				{
					Seed:  42,
					Hands: 1000,
					Results: map[string]float64{
						"challenger_bb_per_100": 5.0,
						"challenger_hands":      1000,
						"challenger_std_dev":    10.0, // This is in BB/hand, will be scaled to 100
					},
				},
			},
			metric:   "challenger_bb_per_100",
			hands:    "challenger_hands",
			wantMean: 5.0,
			wantSize: 1000,
		},
		{
			name: "weighted average of multiple batches",
			batches: []BatchResult{
				{
					Seed:  42,
					Hands: 1000,
					Results: map[string]float64{
						"challenger_bb_per_100": 10.0,
						"challenger_hands":      1000,
					},
				},
				{
					Seed:  43,
					Hands: 500,
					Results: map[string]float64{
						"challenger_bb_per_100": -5.0,
						"challenger_hands":      500,
					},
				},
			},
			metric:   "challenger_bb_per_100",
			hands:    "challenger_hands",
			wantMean: 5.0, // (10*1000 + -5*500) / 1500 = 5
			wantSize: 1500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateStatistics(tt.batches, tt.metric, tt.hands)

			if math.Abs(result.Mean-tt.wantMean) > 0.001 {
				t.Errorf("Mean = %v, want %v", result.Mean, tt.wantMean)
			}

			if result.SampleSize != tt.wantSize {
				t.Errorf("SampleSize = %v, want %v", result.SampleSize, tt.wantSize)
			}

			// Check that CI contains the mean
			if result.CI95Low > result.Mean || result.CI95High < result.Mean {
				t.Errorf("CI [%v, %v] doesn't contain mean %v",
					result.CI95Low, result.CI95High, result.Mean)
			}

			// Check that std dev is reasonable for poker (between 50 and 200 BB/100)
			if result.StdDev < 50 || result.StdDev > 200 {
				t.Logf("Warning: StdDev %v is outside typical poker range [50, 200]", result.StdDev)
			}
		})
	}
}

// TestCompareStatistics tests the statistical comparison
func TestCompareStatistics(t *testing.T) {
	tests := []struct {
		name            string
		group1          StatisticalResult
		group2          StatisticalResult
		wantDifference  float64
		wantSignificant bool // At 0.05 level
	}{
		{
			name: "identical groups",
			group1: StatisticalResult{
				Mean:       10.0,
				StdDev:     100.0,
				SampleSize: 1000,
			},
			group2: StatisticalResult{
				Mean:       10.0,
				StdDev:     100.0,
				SampleSize: 1000,
			},
			wantDifference:  0.0,
			wantSignificant: false,
		},
		{
			name: "small difference with large samples",
			group1: StatisticalResult{
				Mean:       15.0,
				StdDev:     100.0,
				SampleSize: 10000,
			},
			group2: StatisticalResult{
				Mean:       10.0,
				StdDev:     100.0,
				SampleSize: 10000,
			},
			wantDifference:  5.0,
			wantSignificant: true, // With 10k hands each, 5 BB/100 should be significant
		},
		{
			name: "large difference with small samples",
			group1: StatisticalResult{
				Mean:       50.0,
				StdDev:     100.0,
				SampleSize: 100,
			},
			group2: StatisticalResult{
				Mean:       -50.0,
				StdDev:     100.0,
				SampleSize: 100,
			},
			wantDifference:  100.0,
			wantSignificant: true, // 100 BB/100 difference should be significant even with small samples
		},
		{
			name: "small difference with small samples",
			group1: StatisticalResult{
				Mean:       12.0,
				StdDev:     100.0,
				SampleSize: 100,
			},
			group2: StatisticalResult{
				Mean:       10.0,
				StdDev:     100.0,
				SampleSize: 100,
			},
			wantDifference:  2.0,
			wantSignificant: false, // 2 BB/100 with only 100 hands shouldn't be significant
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CompareStatistics(tt.group1, tt.group2)

			if math.Abs(result.Difference-tt.wantDifference) > 0.001 {
				t.Errorf("Difference = %v, want %v", result.Difference, tt.wantDifference)
			}

			isSignificant := result.PValue < 0.05
			if isSignificant != tt.wantSignificant {
				t.Errorf("PValue = %v (significant=%v), want significant=%v",
					result.PValue, isSignificant, tt.wantSignificant)
			}

			// Check that CI contains the difference
			if result.CI95Low > result.Difference || result.CI95High < result.Difference {
				t.Errorf("CI [%v, %v] doesn't contain difference %v",
					result.CI95Low, result.CI95High, result.Difference)
			}

			// Validate p-value is in [0, 1]
			if result.PValue < 0 || result.PValue > 1 {
				t.Errorf("PValue %v is outside [0, 1]", result.PValue)
			}

			// Check effect size calculation
			if tt.wantDifference == 0 {
				if result.EffectSize != 0 {
					t.Errorf("EffectSize = %v, want 0 for no difference", result.EffectSize)
				}
			} else {
				// Effect size should have same sign as difference
				if (result.EffectSize > 0) != (result.Difference > 0) && result.EffectSize != 0 {
					t.Errorf("EffectSize %v has wrong sign for difference %v",
						result.EffectSize, result.Difference)
				}
			}
		})
	}
}

func TestCombineBatchesLatency(t *testing.T) {
	batches := []BatchResult{
		{
			Seed:  1,
			Hands: 1000,
			Results: map[string]float64{
				"challenger_bb_per_100":           5,
				"challenger_hands":                1000,
				"challenger_avg_response_ms":      80,
				"challenger_response_std_ms":      10,
				"challenger_max_response_ms":      120,
				"challenger_min_response_ms":      60,
				"challenger_p95_response_ms":      110,
				"challenger_responses_tracked":    10,
				"challenger_response_timeouts":    1,
				"challenger_response_disconnects": 0,
			},
		},
		{
			Seed:  2,
			Hands: 1000,
			Results: map[string]float64{
				"challenger_bb_per_100":           -2,
				"challenger_hands":                1000,
				"challenger_avg_response_ms":      100,
				"challenger_response_std_ms":      20,
				"challenger_max_response_ms":      150,
				"challenger_min_response_ms":      70,
				"challenger_p95_response_ms":      140,
				"challenger_responses_tracked":    5,
				"challenger_response_timeouts":    2,
				"challenger_response_disconnects": 1,
			},
		},
	}

	combined := CombineBatches(batches, "challenger")

	if combined.ResponsesTracked != 15 {
		t.Fatalf("expected 15 responses, got %.0f", combined.ResponsesTracked)
	}
	if math.Abs(combined.AvgResponseMs-86.6667) > 0.1 {
		t.Errorf("expected avg response ~86.67 ms, got %.2f", combined.AvgResponseMs)
	}
	if math.Abs(combined.ResponseStdMs-17.0) > 0.5 {
		t.Errorf("expected stddev ~17.0 ms, got %.2f", combined.ResponseStdMs)
	}
	if combined.MaxResponseMs != 150 {
		t.Errorf("expected max response 150 ms, got %.2f", combined.MaxResponseMs)
	}
	if combined.MinResponseMs != 60 {
		t.Errorf("expected min response 60 ms, got %.2f", combined.MinResponseMs)
	}
	if combined.P95ResponseMs != 140 {
		t.Errorf("expected p95 140 ms, got %.2f", combined.P95ResponseMs)
	}
	if combined.ResponseTimeouts != 3 {
		t.Errorf("expected 3 response timeouts, got %.0f", combined.ResponseTimeouts)
	}
	if combined.ResponseDisconnects != 1 {
		t.Errorf("expected 1 response disconnect, got %.0f", combined.ResponseDisconnects)
	}
}

// TestPValueAccuracy tests p-value calculation against known values
func TestPValueAccuracy(t *testing.T) {
	tests := []struct {
		name      string
		tStat     float64
		df        int
		wantP     float64
		tolerance float64
	}{
		// Values verified against R's pt() function
		{
			name:      "t=0 should give p=1",
			tStat:     0,
			df:        10,
			wantP:     1.0,
			tolerance: 0.001,
		},
		{
			name:      "t=2.228 with df=10 should give p≈0.05",
			tStat:     2.228,
			df:        10,
			wantP:     0.05,
			tolerance: 0.01,
		},
		{
			name:      "t=1.96 with large df should approach p≈0.05",
			tStat:     1.96,
			df:        1000,
			wantP:     0.05,
			tolerance: 0.01,
		},
		{
			name:      "t=3.0 with df=20 should give p≈0.007",
			tStat:     3.0,
			df:        20,
			wantP:     0.007,
			tolerance: 0.002,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := calculatePValue(tt.tStat, tt.df)

			if math.Abs(p-tt.wantP) > tt.tolerance {
				t.Errorf("calculatePValue(%v, %v) = %v, want %v ± %v",
					tt.tStat, tt.df, p, tt.wantP, tt.tolerance)
			}
		})
	}
}

// TestWelchDF tests Welch's degrees of freedom calculation
func TestWelchDF(t *testing.T) {
	tests := []struct {
		name  string
		sd1   float64
		n1    int
		sd2   float64
		n2    int
		minDF int
		maxDF int
	}{
		{
			name:  "equal variances and sizes",
			sd1:   100,
			n1:    100,
			sd2:   100,
			n2:    100,
			minDF: 190, // Should be close to n1+n2-2 = 198
			maxDF: 198,
		},
		{
			name:  "very different variances",
			sd1:   100,
			n1:    100,
			sd2:   10,
			n2:    100,
			minDF: 50, // Welch adjustment should reduce df
			maxDF: 150,
		},
		{
			name:  "small samples",
			sd1:   100,
			n1:    10,
			sd2:   100,
			n2:    10,
			minDF: 10,
			maxDF: 18,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			df := calculateWelchDF(tt.sd1, tt.n1, tt.sd2, tt.n2)

			if df < tt.minDF || df > tt.maxDF {
				t.Errorf("calculateWelchDF(%v,%v,%v,%v) = %v, want between %v and %v",
					tt.sd1, tt.n1, tt.sd2, tt.n2, df, tt.minDF, tt.maxDF)
			}
		})
	}
}

// TestEffectSizeInterpretation tests Cohen's d interpretation
func TestEffectSizeInterpretation(t *testing.T) {
	tests := []struct {
		d    float64
		want string
	}{
		{0.0, "negligible"},
		{0.1, "negligible"},
		{0.2, "small"},
		{0.4, "small"},
		{0.5, "medium"},
		{0.7, "medium"},
		{0.8, "large"},
		{1.5, "large"},
		{-0.3, "small"}, // Negative values
		{-0.9, "large"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := InterpretEffectSize(tt.d)
			if got != tt.want {
				t.Errorf("InterpretEffectSize(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

// TestPooledStdDev tests pooled standard deviation calculation
func TestPooledStdDev(t *testing.T) {
	tests := []struct {
		name       string
		stdDevs    []float64
		weights    []float64
		wantStdDev float64
		tolerance  float64
	}{
		{
			name:       "equal weights same variance",
			stdDevs:    []float64{100, 100, 100},
			weights:    []float64{1000, 1000, 1000},
			wantStdDev: 100,
			tolerance:  0.001,
		},
		{
			name:       "different weights same variance",
			stdDevs:    []float64{100, 100},
			weights:    []float64{1000, 100},
			wantStdDev: 100,
			tolerance:  0.001,
		},
		{
			name:       "weighted average of variances",
			stdDevs:    []float64{100, 200},
			weights:    []float64{1000, 1000},
			wantStdDev: math.Sqrt((100*100 + 200*200) / 2), // ~158.11
			tolerance:  1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculatePooledStdDevWeighted(tt.stdDevs, tt.weights)

			if math.Abs(result-tt.wantStdDev) > tt.tolerance {
				t.Errorf("calculatePooledStdDevWeighted() = %v, want %v ± %v",
					result, tt.wantStdDev, tt.tolerance)
			}
		})
	}
}
