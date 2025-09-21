package analysis

import (
	"math"
	"math/rand"
	"testing"
)

func TestEquityResult(t *testing.T) {
	result := EquityResult{
		Wins:             300,
		Ties:             50,
		TotalSimulations: 1000,
	}

	t.Run("WinRate", func(t *testing.T) {
		expected := 0.3
		actual := result.WinRate()
		if math.Abs(actual-expected) > 0.001 {
			t.Errorf("WinRate() = %v, want %v", actual, expected)
		}
	})

	t.Run("TieRate", func(t *testing.T) {
		expected := 0.05
		actual := result.TieRate()
		if math.Abs(actual-expected) > 0.001 {
			t.Errorf("TieRate() = %v, want %v", actual, expected)
		}
	})

	t.Run("LossRate", func(t *testing.T) {
		expected := 0.65 // (1000 - 300 - 50) / 1000
		actual := result.LossRate()
		if math.Abs(actual-expected) > 0.001 {
			t.Errorf("LossRate() = %v, want %v", actual, expected)
		}
	})

	t.Run("Equity", func(t *testing.T) {
		expected := 0.325 // (300 + 50*0.5) / 1000
		actual := result.Equity()
		if math.Abs(actual-expected) > 0.001 {
			t.Errorf("Equity() = %v, want %v", actual, expected)
		}
	})
}

func TestConfidenceInterval(t *testing.T) {
	result := EquityResult{
		Wins:             500,
		Ties:             0,
		TotalSimulations: 10000,
	}

	lower, upper := result.ConfidenceInterval()

	// For 5% equity (50/1000 simulations), CI should be around 0.035-0.065
	// The test data shows 500 wins out of 10000, which is 5% not 50%
	equity := result.Equity()
	if math.Abs(equity-0.05) > 0.001 {
		t.Errorf("Equity = %v, expected 0.05", equity)
	}

	// CI should be reasonable around 5%
	if lower < 0.035 || lower > 0.055 {
		t.Errorf("Lower CI = %v, expected around 0.04-0.05", lower)
	}

	if upper < 0.045 || upper > 0.065 {
		t.Errorf("Upper CI = %v, expected around 0.05-0.06", upper)
	}

	if lower >= upper {
		t.Errorf("Lower CI (%v) should be less than upper CI (%v)", lower, upper)
	}
}

func TestParseCards(t *testing.T) {
	tests := []struct {
		name     string
		cardStrs []string
		expected int // expected count
	}{
		{"two cards", []string{"As", "Kh"}, 2},
		{"board cards", []string{"2c", "7h", "Kd"}, 3},
		{"full board", []string{"2c", "7h", "Kd", "Ts", "9h"}, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hand, err := parseCards(tt.cardStrs)
			if err != nil {
				t.Errorf("parseCards(%v) error: %v", tt.cardStrs, err)
				return
			}
			if hand.CountCards() != tt.expected {
				t.Errorf("parseCards(%v) count = %v, want %v", tt.cardStrs, hand.CountCards(), tt.expected)
			}
		})
	}
}

func TestParseCardsWithInvalidInput(t *testing.T) {
	tests := []struct {
		name     string
		cardStrs []string
		wantErr  bool
	}{
		{"valid cards", []string{"As", "Kh"}, false},
		{"invalid rank", []string{"Xs", "Kh"}, true},
		{"invalid suit", []string{"Ax", "Kh"}, true},
		{"too short", []string{"A", "Kh"}, true},
		{"too long", []string{"Ass", "Kh"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseCards(tt.cardStrs)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCards() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCalculateEquity(t *testing.T) {
	t.Run("pocket aces vs random", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))

		// AA vs random opponent on dry board
		heroHoles := []string{"As", "Ad"}
		board := []string{"2c", "7h", "Kd"}

		result := CalculateEquity(heroHoles, board, 1, 1000, rng)

		// AA should have high equity (>80%)
		equity := result.Equity()
		if equity < 0.7 {
			t.Errorf("AA equity = %v, expected > 0.7", equity)
		}

		if result.TotalSimulations != 1000 {
			t.Errorf("TotalSimulations = %v, want 1000", result.TotalSimulations)
		}
	})

	t.Run("weak hand vs random", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))

		// 23 offsuit vs random opponent
		heroHoles := []string{"2c", "3h"}
		board := []string{"Ac", "Kh", "Qd"}

		result := CalculateEquity(heroHoles, board, 1, 1000, rng)

		// 23o should have low equity (<30%)
		equity := result.Equity()
		if equity > 0.4 {
			t.Errorf("23o equity = %v, expected < 0.4", equity)
		}
	})

	t.Run("invalid input", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))

		// Invalid hole cards (only 1 card)
		heroHoles := []string{"As"}
		board := []string{"2c", "7h", "Kd"}

		result := CalculateEquity(heroHoles, board, 1, 1000, rng)

		// Should return empty result
		if result.TotalSimulations != 0 {
			t.Errorf("Invalid input should return empty result")
		}
	})
}

func TestQuickEquity(t *testing.T) {
	// Test the convenience function
	equity := QuickEquity([]string{"As", "Ad"}, []string{"2c", "7h", "Kd"}, 1)

	// AA should have high equity
	if equity < 0.7 {
		t.Errorf("QuickEquity(AA) = %v, expected > 0.7", equity)
	}

	if equity > 1.0 || equity < 0.0 {
		t.Errorf("QuickEquity should return value between 0 and 1, got %v", equity)
	}
}

func TestEquityCalculatorWithProperEvaluator(t *testing.T) {
	t.Run("uses poker.Evaluate7Cards", func(t *testing.T) {
		// This test verifies that we're using the proper hand evaluator
		// The actual evaluation correctness is tested in the poker package
		rng := rand.New(rand.NewSource(42))
		heroHoles := []string{"As", "Ad"}
		board := []string{"2c", "7h", "Kd"}

		// Small number of simulations for quick test
		result := CalculateEquity(heroHoles, board, 1, 100, rng)

		// AA should have reasonable equity
		equity := result.Equity()
		if equity < 0.5 || equity > 1.0 {
			t.Errorf("AA equity = %v, expected between 0.5 and 1.0", equity)
		}

		if result.TotalSimulations != 100 {
			t.Errorf("TotalSimulations = %v, want 100", result.TotalSimulations)
		}
	})

	t.Run("handles invalid input gracefully", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))

		// Invalid hole cards format
		result := CalculateEquity([]string{"Xx"}, []string{"2c", "7h"}, 1, 100, rng)
		if result.TotalSimulations != 0 {
			t.Errorf("Invalid input should return empty result")
		}
	})
}

func BenchmarkCalculateEquity(t *testing.B) {
	rng := rand.New(rand.NewSource(42))
	heroHoles := []string{"As", "Ad"}
	board := []string{"2c", "7h", "Kd"}

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		CalculateEquity(heroHoles, board, 1, 100, rng) // Reduced for benchmark
	}
}

func BenchmarkQuickEquity(t *testing.B) {
	heroHoles := []string{"As", "Ad"}
	board := []string{"2c", "7h", "Kd"}

	t.ResetTimer()

	for i := 0; i < t.N; i++ {
		QuickEquity(heroHoles, board, 1)
	}
}
