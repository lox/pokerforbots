package statistics

import (
	"math"
	"testing"
)

func TestNewStatistics(t *testing.T) {
	stats := NewStatistics(10)
	if stats == nil {
		t.Fatal("Expected non-nil statistics")
	}

	// Check initial values
	if stats.BB100() != 0 {
		t.Errorf("Initial BB100 should be 0, got %f", stats.BB100())
	}
	if stats.Mean() != 0 {
		t.Errorf("Initial mean should be 0, got %f", stats.Mean())
	}
	if stats.StdDev() != 0 {
		t.Errorf("Initial StdDev should be 0, got %f", stats.StdDev())
	}
}

func TestStatisticsAdd(t *testing.T) {
	stats := NewStatistics(10)

	// Add a hand result
	result := HandResult{
		HandNum:        1,
		NetBB:          5.0,
		Position:       0,
		ButtonDistance: 0,
		WentToShowdown: true,
		WonAtShowdown:  true,
		FinalPotBB:     10.0,
		StreetReached:  "river",
		HoleCards:      "AsKd",
		HandCategory:   "Premium",
		PreflopAction:  "raise",
		FlopAction:     "bet",
		TurnAction:     "check",
		RiverAction:    "call",
	}

	err := stats.Add(result)
	if err != nil {
		t.Errorf("Add failed: %v", err)
	}

	// Check updated values
	hands, sumBB, _, _, _, _, _, _, _, _, _, _, _, _ := stats.GetStats()
	if hands != 1 {
		t.Errorf("Expected 1 hand, got %d", hands)
	}
	if sumBB != 5.0 {
		t.Errorf("Expected sum BB 5.0, got %f", sumBB)
	}

	// Check BB/100
	expectedBB100 := 500.0 // 5 BB per hand * 100
	if stats.BB100() != expectedBB100 {
		t.Errorf("Expected BB/100 %f, got %f", expectedBB100, stats.BB100())
	}
}

func TestStatisticsMultipleHands(t *testing.T) {
	stats := NewStatistics(10)

	// Add multiple hands with different outcomes
	hands := []HandResult{
		{HandNum: 1, NetBB: 5.0, WentToShowdown: true, WonAtShowdown: true},
		{HandNum: 2, NetBB: -3.0, WentToShowdown: true, WonAtShowdown: false},
		{HandNum: 3, NetBB: 8.0, WentToShowdown: false, WonAtShowdown: false},
		{HandNum: 4, NetBB: -2.0, WentToShowdown: false, WonAtShowdown: false},
		{HandNum: 5, NetBB: 2.0, WentToShowdown: true, WonAtShowdown: true},
	}

	for _, hand := range hands {
		if err := stats.Add(hand); err != nil {
			t.Errorf("Add failed: %v", err)
		}
	}

	// Check aggregate stats
	handCount, sumBB, winningHands, _, showdownWins, _, showdownLosses, _, _, _, _, _, _, _ := stats.GetStats()

	if handCount != 5 {
		t.Errorf("Expected 5 hands, got %d", handCount)
	}

	expectedSum := 10.0 // 5 - 3 + 8 - 2 + 2
	if math.Abs(sumBB-expectedSum) > 0.001 {
		t.Errorf("Expected sum %f, got %f", expectedSum, sumBB)
	}

	expectedWinning := 3 // hands 1, 3, 5 are profitable
	if winningHands != expectedWinning {
		t.Errorf("Expected %d winning hands, got %d", expectedWinning, winningHands)
	}

	if showdownWins != 2 {
		t.Errorf("Expected 2 showdown wins, got %d", showdownWins)
	}

	if showdownLosses != 1 {
		t.Errorf("Expected 1 showdown loss, got %d", showdownLosses)
	}

	// Check BB/100
	expectedBB100 := (expectedSum / 5.0) * 100
	if math.Abs(stats.BB100()-expectedBB100) > 0.001 {
		t.Errorf("Expected BB/100 %f, got %f", expectedBB100, stats.BB100())
	}

	// Check mean
	expectedMean := expectedSum / 5.0
	if math.Abs(stats.Mean()-expectedMean) > 0.001 {
		t.Errorf("Expected mean %f, got %f", expectedMean, stats.Mean())
	}
}

func TestStatisticsButtonDistance(t *testing.T) {
	stats := NewStatistics(10)

	// Add hands at different positions
	positions := []int{0, 1, 2, 3, 4, 5} // BTN, CO, MP, UTG+2, UTG+1, UTG
	for i, pos := range positions {
		result := HandResult{
			HandNum:        i + 1,
			NetBB:          float64(pos + 1), // Different profit for each position
			ButtonDistance: pos,
		}
		if err := stats.Add(result); err != nil {
			t.Errorf("Add failed: %v", err)
		}
	}

	// Check button distance results
	bdResults := stats.ButtonDistanceResults()
	for pos := range 6 {
		bd := bdResults[pos]
		if bd.Hands != 1 {
			t.Errorf("Position %d: expected 1 hand, got %d", pos, bd.Hands)
		}
		expectedBB := float64(pos + 1)
		if math.Abs(bd.SumBB-expectedBB) > 0.001 {
			t.Errorf("Position %d: expected sum BB %f, got %f", pos, expectedBB, bd.SumBB)
		}
	}
}

func TestStatisticsStreetStats(t *testing.T) {
	stats := NewStatistics(10)

	// Add hands ending on different streets
	streets := []string{"preflop", "flop", "turn", "river"}
	for i, street := range streets {
		result := HandResult{
			HandNum:       i + 1,
			NetBB:         float64(i + 1),
			StreetReached: street,
		}
		if err := stats.Add(result); err != nil {
			t.Errorf("Add failed: %v", err)
		}
	}

	// Check street stats
	streetStats := stats.StreetStats()
	for i, street := range streets {
		stat, exists := streetStats[street]
		if !exists {
			t.Errorf("Missing stats for street %s", street)
			continue
		}
		if stat.HandsReached != 1 {
			t.Errorf("Street %s: expected 1 hand, got %d", street, stat.HandsReached)
		}
		expectedBB := float64(i + 1)
		if math.Abs(stat.NetBB-expectedBB) > 0.001 {
			t.Errorf("Street %s: expected net BB %f, got %f", street, expectedBB, stat.NetBB)
		}
	}
}

func TestStatisticsCategoryStats(t *testing.T) {
	stats := NewStatistics(10)

	// Add hands with different categories
	categories := []string{"Premium", "Strong", "Medium", "Weak", "Trash"}
	for i, cat := range categories {
		result := HandResult{
			HandNum:      i + 1,
			NetBB:        float64(5 - i), // Premium hands win more
			HandCategory: cat,
		}
		if err := stats.Add(result); err != nil {
			t.Errorf("Add failed: %v", err)
		}
	}

	// Check category stats
	catStats := stats.CategoryStats()
	for i, cat := range categories {
		stat, exists := catStats[cat]
		if !exists {
			t.Errorf("Missing stats for category %s", cat)
			continue
		}
		if stat.Hands != 1 {
			t.Errorf("Category %s: expected 1 hand, got %d", cat, stat.Hands)
		}
		expectedBB := float64(5 - i)
		if math.Abs(stat.NetBB-expectedBB) > 0.001 {
			t.Errorf("Category %s: expected net BB %f, got %f", cat, expectedBB, stat.NetBB)
		}
	}
}

func TestStatisticsThreadSafety(t *testing.T) {
	stats := NewStatistics(10)
	done := make(chan bool)

	// Concurrent writes
	go func() {
		for i := range 100 {
			result := HandResult{
				HandNum: i + 1,
				NetBB:   1.0,
			}
			_ = stats.Add(result)
		}
		done <- true
	}()

	// Concurrent reads
	go func() {
		for range 100 {
			_ = stats.BB100()
			_ = stats.Mean()
			_ = stats.StdDev()
			_ = stats.ButtonDistanceResults()
			_ = stats.StreetStats()
			_ = stats.CategoryStats()
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// Verify data integrity
	hands, _, _, _, _, _, _, _, _, _, _, _, _, _ := stats.GetStats()
	if hands != 100 {
		t.Errorf("Expected 100 hands after concurrent writes, got %d", hands)
	}
}

func TestStatisticsValidation(t *testing.T) {
	stats := NewStatistics(10)

	// Test invalid button distance
	result := HandResult{
		HandNum:        1,
		NetBB:          1.0,
		ButtonDistance: -1, // Invalid
	}
	err := stats.Add(result)
	if err == nil {
		t.Error("Expected error for invalid button distance")
	}

	// Test button distance too large
	result.ButtonDistance = 10 // Too large
	err = stats.Add(result)
	if err == nil {
		t.Error("Expected error for button distance too large")
	}

	// Test invalid position
	result.ButtonDistance = 0
	result.Position = -1 // Invalid
	err = stats.Add(result)
	if err == nil {
		t.Error("Expected error for invalid position")
	}
}

func TestStatisticsStdDev(t *testing.T) {
	stats := NewStatistics(10)

	// Add hands with known variance
	values := []float64{1.0, 2.0, 3.0, 4.0, 5.0}
	for i, val := range values {
		result := HandResult{
			HandNum: i + 1,
			NetBB:   val,
		}
		if err := stats.Add(result); err != nil {
			t.Errorf("Add failed: %v", err)
		}
	}

	// Mean should be 3.0
	expectedMean := 3.0
	if math.Abs(stats.Mean()-expectedMean) > 0.001 {
		t.Errorf("Expected mean %f, got %f", expectedMean, stats.Mean())
	}

	// Standard deviation calculation
	// The implementation uses sample standard deviation (n-1 divisor)
	// Variance = ((1-3)² + (2-3)² + (3-3)² + (4-3)² + (5-3)²) / 4
	//          = (4 + 1 + 0 + 1 + 4) / 4 = 2.5
	// StdDev = sqrt(2.5) ≈ 1.581
	expectedStdDev := math.Sqrt(2.5)
	if math.Abs(stats.StdDev()-expectedStdDev) > 0.01 {
		t.Errorf("Expected StdDev %f, got %f", expectedStdDev, stats.StdDev())
	}
}

func TestGetPositionName(t *testing.T) {
	tests := []struct {
		distance int
		expected string
	}{
		{0, "Button"},
		{1, "Cutoff"},
		{2, "Hijack"},
		{3, "MP"},
		{4, "EP2"},
		{5, "EP1"},
		{6, "Pos6"},
		{-1, "Pos-1"},
	}

	for _, tt := range tests {
		result := GetPositionName(tt.distance)
		if result != tt.expected {
			t.Errorf("GetPositionName(%d) = %s, want %s", tt.distance, result, tt.expected)
		}
	}
}

func BenchmarkStatisticsAdd(b *testing.B) {
	stats := NewStatistics(10)
	result := HandResult{
		HandNum:        1,
		NetBB:          5.0,
		ButtonDistance: 0,
		WentToShowdown: true,
		WonAtShowdown:  true,
		StreetReached:  "river",
		HandCategory:   "Premium",
	}

	for i := 0; b.Loop(); i++ {
		result.HandNum = i + 1
		_ = stats.Add(result)
	}
}

func BenchmarkStatisticsBB100(b *testing.B) {
	stats := NewStatistics(10)
	// Add some data
	for i := range 100 {
		_ = stats.Add(HandResult{HandNum: i + 1, NetBB: 1.0})
	}

	for b.Loop() {
		_ = stats.BB100()
	}
}

func BenchmarkStatisticsGetStats(b *testing.B) {
	stats := NewStatistics(10)
	// Add some data
	for i := range 100 {
		_ = stats.Add(HandResult{HandNum: i + 1, NetBB: 1.0})
	}

	for b.Loop() {
		stats.GetStats()
	}
}
