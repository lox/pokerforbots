package statistics

import (
	"math"
	"testing"
)

func TestStatistics_Empty(t *testing.T) {
	stats := &Statistics{}

	if stats.Mean() != 0 {
		t.Errorf("Expected mean of 0 for empty stats, got %f", stats.Mean())
	}
	if stats.Variance() != 0 {
		t.Errorf("Expected variance of 0 for empty stats, got %f", stats.Variance())
	}
	if stats.StdDev() != 0 {
		t.Errorf("Expected stddev of 0 for empty stats, got %f", stats.StdDev())
	}
	if stats.StdError() != 0 {
		t.Errorf("Expected stderr of 0 for empty stats, got %f", stats.StdError())
	}
	if stats.Median() != 0 {
		t.Errorf("Expected median of 0 for empty stats, got %f", stats.Median())
	}
	if stats.Percentile(0.5) != 0 {
		t.Errorf("Expected percentile of 0 for empty stats, got %f", stats.Percentile(0.5))
	}
}

func TestStatistics_SingleValue(t *testing.T) {
	stats := &Statistics{}
	result := HandResult{
		NetBB:          2.5,
		Seed:           12345,
		Position:       3,
		WentToShowdown: true,
		FinalPotSize:   20,
		StreetReached:  "River",
	}

	stats.Add(result)

	if stats.Hands != 1 {
		t.Errorf("Expected 1 hand, got %d", stats.Hands)
	}
	if stats.Mean() != 2.5 {
		t.Errorf("Expected mean of 2.5, got %f", stats.Mean())
	}
	if stats.Variance() != 0 {
		t.Errorf("Expected variance of 0 for single value, got %f", stats.Variance())
	}
	if stats.StdDev() != 0 {
		t.Errorf("Expected stddev of 0 for single value, got %f", stats.StdDev())
	}
	if stats.Median() != 2.5 {
		t.Errorf("Expected median of 2.5, got %f", stats.Median())
	}
	if stats.ShowdownWins != 1 {
		t.Errorf("Expected 1 showdown win, got %d", stats.ShowdownWins)
	}
	if stats.NonShowdownWins != 0 {
		t.Errorf("Expected 0 non-showdown wins, got %d", stats.NonShowdownWins)
	}
	if !stats.IsLedgerBalanced() {
		t.Error("Expected ledger to be balanced")
	}
}

func TestStatistics_MultipleValues(t *testing.T) {
	stats := &Statistics{}

	// Add several hand results with known values
	results := []HandResult{
		{NetBB: 1.0, Position: 1, WentToShowdown: false, FinalPotSize: 4},
		{NetBB: -2.0, Position: 2, WentToShowdown: true, FinalPotSize: 8},
		{NetBB: 3.0, Position: 3, WentToShowdown: true, FinalPotSize: 12},
		{NetBB: 0.0, Position: 1, WentToShowdown: false, FinalPotSize: 2},
		{NetBB: -1.0, Position: 2, WentToShowdown: false, FinalPotSize: 6},
	}

	for _, result := range results {
		stats.Add(result)
	}

	// Test basic statistics
	expectedMean := (1.0 - 2.0 + 3.0 + 0.0 - 1.0) / 5.0
	if math.Abs(stats.Mean()-expectedMean) > 1e-9 {
		t.Errorf("Expected mean of %f, got %f", expectedMean, stats.Mean())
	}

	if stats.Hands != 5 {
		t.Errorf("Expected 5 hands, got %d", stats.Hands)
	}

	// Test median (sorted values: -2, -1, 0, 1, 3)
	if stats.Median() != 0.0 {
		t.Errorf("Expected median of 0.0, got %f", stats.Median())
	}

	// Test showdown tracking
	if stats.ShowdownWins != 1 { // Only the +3.0 hand was showdown win
		t.Errorf("Expected 1 showdown win, got %d", stats.ShowdownWins)
	}
	if stats.NonShowdownWins != 1 { // Only the +1.0 hand was non-showdown win
		t.Errorf("Expected 1 non-showdown win, got %d", stats.NonShowdownWins)
	}

	// Test position tracking
	if stats.PositionResults[1].Hands != 2 {
		t.Errorf("Expected 2 hands in position 1, got %d", stats.PositionResults[1].Hands)
	}
	if stats.PositionResults[2].Hands != 2 {
		t.Errorf("Expected 2 hands in position 2, got %d", stats.PositionResults[2].Hands)
	}
	if stats.PositionResults[3].Hands != 1 {
		t.Errorf("Expected 1 hand in position 3, got %d", stats.PositionResults[3].Hands)
	}

	if !stats.IsLedgerBalanced() {
		t.Error("Expected ledger to be balanced")
	}
}

func TestStatistics_Percentiles(t *testing.T) {
	stats := &Statistics{}

	// Add values: 1, 2, 3, 4, 5
	for i := 1; i <= 5; i++ {
		stats.Add(HandResult{NetBB: float64(i), Position: 1})
	}

	tests := []struct {
		percentile float64
		expected   float64
	}{
		{0.0, 1.0},
		{0.25, 2.0},
		{0.5, 3.0},
		{0.75, 4.0},
		{1.0, 5.0},
	}

	for _, test := range tests {
		result := stats.Percentile(test.percentile)
		if math.Abs(result-test.expected) > 1e-9 {
			t.Errorf("Percentile %.2f: expected %f, got %f", test.percentile, test.expected, result)
		}
	}
}

func TestStatistics_ConfidenceInterval(t *testing.T) {
	stats := &Statistics{}

	// Add some values with known statistical properties
	values := []float64{1, 2, 3, 4, 5}
	for _, v := range values {
		stats.Add(HandResult{NetBB: v, Position: 1})
	}

	low, high := stats.ConfidenceInterval95()
	mean := stats.Mean()

	// CI should be symmetric around the mean
	if math.Abs((low+high)/2-mean) > 1e-9 {
		t.Errorf("Confidence interval not symmetric around mean. Low: %f, High: %f, Mean: %f", low, high, mean)
	}

	// CI should be wider than zero for multiple values
	if high-low <= 0 {
		t.Errorf("Confidence interval should be positive width, got %f", high-low)
	}
}

func TestStatistics_PositionAnalysis(t *testing.T) {
	stats := &Statistics{}

	// Add different results for different positions
	stats.Add(HandResult{NetBB: 2.0, Position: 1})
	stats.Add(HandResult{NetBB: 3.0, Position: 1})
	stats.Add(HandResult{NetBB: -1.0, Position: 2})
	stats.Add(HandResult{NetBB: 1.0, Position: 2})

	// Test position means
	pos1Mean := stats.PositionMean(1)
	expectedPos1Mean := (2.0 + 3.0) / 2.0
	if math.Abs(pos1Mean-expectedPos1Mean) > 1e-9 {
		t.Errorf("Position 1 mean: expected %f, got %f", expectedPos1Mean, pos1Mean)
	}

	pos2Mean := stats.PositionMean(2)
	expectedPos2Mean := (-1.0 + 1.0) / 2.0
	if math.Abs(pos2Mean-expectedPos2Mean) > 1e-9 {
		t.Errorf("Position 2 mean: expected %f, got %f", expectedPos2Mean, pos2Mean)
	}

	// Test invalid positions
	if stats.PositionMean(0) != 0 {
		t.Errorf("Expected 0 for invalid position 0, got %f", stats.PositionMean(0))
	}
	if stats.PositionMean(7) != 0 {
		t.Errorf("Expected 0 for invalid position 7, got %f", stats.PositionMean(7))
	}
}

func TestStatistics_PotSizeTracking(t *testing.T) {
	stats := &Statistics{}

	// Add hands with different pot sizes
	stats.Add(HandResult{NetBB: 1.0, FinalPotSize: 20})  // 10bb pot
	stats.Add(HandResult{NetBB: 5.0, FinalPotSize: 200}) // 100bb pot (big pot)
	stats.Add(HandResult{NetBB: -1.0, FinalPotSize: 4})  // 2bb pot

	if stats.MaxPotChips != 200 {
		t.Errorf("Expected max pot of 200 chips, got %d", stats.MaxPotChips)
	}
	if math.Abs(stats.MaxPotBB-100.0) > 1e-9 {
		t.Errorf("Expected max pot of 100bb, got %f", stats.MaxPotBB)
	}
	if stats.BigPots != 1 {
		t.Errorf("Expected 1 big pot (>=50bb), got %d", stats.BigPots)
	}
	if math.Abs(stats.BigPotsBB-5.0) > 1e-9 {
		t.Errorf("Expected big pot BB of 5.0, got %f", stats.BigPotsBB)
	}
}

func TestStatistics_Variance(t *testing.T) {
	stats := &Statistics{}

	// Add values with known variance: [1, 3, 5] -> variance = 4.0
	values := []float64{1, 3, 5}
	for _, v := range values {
		stats.Add(HandResult{NetBB: v, Position: 1})
	}

	expectedVariance := 4.0 // Sample variance of [1,3,5]
	if math.Abs(stats.Variance()-expectedVariance) > 1e-9 {
		t.Errorf("Expected variance of %f, got %f", expectedVariance, stats.Variance())
	}

	expectedStdDev := 2.0 // sqrt(4)
	if math.Abs(stats.StdDev()-expectedStdDev) > 1e-9 {
		t.Errorf("Expected stddev of %f, got %f", expectedStdDev, stats.StdDev())
	}
}

func TestStatistics_Validate_Valid(t *testing.T) {
	stats := &Statistics{}

	// Add some valid data
	stats.Add(HandResult{NetBB: 1.0, Position: 1})
	stats.Add(HandResult{NetBB: -1.0, Position: 2})
	stats.Add(HandResult{NetBB: 0.5, Position: 1})

	err := stats.Validate()
	if err != nil {
		t.Errorf("Expected valid stats to pass validation, got error: %v", err)
	}
}

func TestStatistics_Validate_LedgerMismatch(t *testing.T) {
	stats := &Statistics{}
	stats.Hands = 1
	stats.SumBB = 1.0
	stats.Values = []float64{1.0}

	// Intentionally create ledger mismatch
	stats.AllBB = 1.0
	stats.ShowdownBB = 0.5
	stats.NonShowdownBB = 0.6 // Should be 0.5 to balance

	// Add position data
	stats.PositionResults[1].Hands = 1

	err := stats.Validate()
	if err == nil {
		t.Error("Expected validation to fail with ledger mismatch")
	}
	if !containsString(err.Error(), "ledger mismatch") {
		t.Errorf("Expected ledger mismatch error, got: %v", err)
	}
}

func TestStatistics_Validate_InvalidHandsCount(t *testing.T) {
	stats := &Statistics{}
	stats.Hands = 0 // Invalid

	err := stats.Validate()
	if err == nil {
		t.Error("Expected validation to fail with invalid hands count")
	}
	if !containsString(err.Error(), "invalid hands count") {
		t.Errorf("Expected invalid hands count error, got: %v", err)
	}
}

func TestStatistics_Validate_ValuesMismatch(t *testing.T) {
	stats := &Statistics{}
	stats.Hands = 2
	stats.Values = []float64{1.0} // Should have 2 values
	stats.AllBB = 1.0
	stats.ShowdownBB = 0.0
	stats.NonShowdownBB = 1.0

	err := stats.Validate()
	if err == nil {
		t.Error("Expected validation to fail with values array mismatch")
	}
	if !containsString(err.Error(), "values array length") {
		t.Errorf("Expected values array length error, got: %v", err)
	}
}

func TestStatistics_Validate_TooManyWins(t *testing.T) {
	stats := &Statistics{}
	stats.Hands = 2
	stats.Values = []float64{1.0, 1.0}
	stats.AllBB = 2.0
	stats.ShowdownBB = 1.0
	stats.NonShowdownBB = 1.0
	stats.ShowdownWins = 2
	stats.NonShowdownWins = 2 // Total wins = 4, but only 2 hands

	// Add position data
	stats.PositionResults[1].Hands = 2

	err := stats.Validate()
	if err == nil {
		t.Error("Expected validation to fail with too many wins")
	}
	if !containsString(err.Error(), "total wins") && !containsString(err.Error(), "exceeds total hands") {
		t.Errorf("Expected too many wins error, got: %v", err)
	}
}

func TestStatistics_Validate_PositionMismatch(t *testing.T) {
	stats := &Statistics{}
	stats.Hands = 2
	stats.Values = []float64{1.0, 1.0}
	stats.AllBB = 2.0
	stats.ShowdownBB = 1.0
	stats.NonShowdownBB = 1.0

	// Add wrong position data - should total to 2 but we'll make it 1
	stats.PositionResults[1].Hands = 1
	// Missing one hand in position data

	err := stats.Validate()
	if err == nil {
		t.Error("Expected validation to fail with position hands mismatch")
	}
	if !containsString(err.Error(), "position hands total") {
		t.Errorf("Expected position hands total error, got: %v", err)
	}
}

func TestHandResult_Fields(t *testing.T) {
	result := HandResult{
		NetBB:          1.5,
		Seed:           12345,
		Position:       4,
		WentToShowdown: true,
		FinalPotSize:   30,
		StreetReached:  "Turn",
	}

	if result.NetBB != 1.5 {
		t.Errorf("Expected NetBB of 1.5, got %f", result.NetBB)
	}
	if result.Seed != 12345 {
		t.Errorf("Expected Seed of 12345, got %d", result.Seed)
	}
	if result.Position != 4 {
		t.Errorf("Expected Position of 4, got %d", result.Position)
	}
	if !result.WentToShowdown {
		t.Error("Expected WentToShowdown to be true")
	}
	if result.FinalPotSize != 30 {
		t.Errorf("Expected FinalPotSize of 30, got %d", result.FinalPotSize)
	}
	if result.StreetReached != "Turn" {
		t.Errorf("Expected StreetReached of 'Turn', got '%s'", result.StreetReached)
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
