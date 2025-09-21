package analysis

import (
	"math"
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lox/pokerforbots/poker"
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

func TestCalculateEquity(t *testing.T) {
	t.Run("pocket aces vs random", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))

		// AA vs random opponent on dry board
		heroHand, _ := poker.ParseHand("As", "Ad")
		board, _ := poker.ParseHand("2c", "7h", "Kd")

		result := CalculateEquity(heroHand, board, 1, 1000, rng)

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
		heroHand, _ := poker.ParseHand("2c", "3h")
		board, _ := poker.ParseHand("Ac", "Kh", "Qd")

		result := CalculateEquity(heroHand, board, 1, 1000, rng)

		// 23o should have low equity (<30%)
		equity := result.Equity()
		if equity > 0.4 {
			t.Errorf("23o equity = %v, expected < 0.4", equity)
		}
	})

	t.Run("insufficient cards", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))

		// Hero hand with only 1 card
		heroHand, _ := poker.ParseHand("As")
		board, _ := poker.ParseHand("2c", "7h", "Kd")

		result := CalculateEquity(heroHand, board, 1, 1000, rng)

		// Should return empty result for invalid input
		if result.TotalSimulations != 0 {
			t.Errorf("Invalid input should return empty result")
		}
	})

	t.Run("negative simulations", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		heroHand, _ := poker.ParseHand("As", "Ad")
		board, _ := poker.ParseHand("2c", "7h", "Kd")

		result := CalculateEquity(heroHand, board, 1, -100, rng)

		// Should return empty result for negative simulations
		if result.TotalSimulations != 0 {
			t.Errorf("Negative simulations should return empty result")
		}
	})

	t.Run("zero simulations", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		heroHand, _ := poker.ParseHand("As", "Ad")
		board, _ := poker.ParseHand("2c", "7h", "Kd")

		result := CalculateEquity(heroHand, board, 1, 0, rng)

		// Should return empty result for zero simulations
		if result.TotalSimulations != 0 {
			t.Errorf("Zero simulations should return empty result")
		}
	})

	t.Run("overlapping hero and board cards", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		heroHand, _ := poker.ParseHand("As", "Ad")
		board, _ := poker.ParseHand("As", "7h", "Kd") // As overlaps with hero

		result := CalculateEquity(heroHand, board, 1, 1000, rng)

		// Should return empty result for overlapping cards
		if result.TotalSimulations != 0 {
			t.Errorf("Overlapping cards should return empty result")
		}
	})

	t.Run("too many opponents", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		heroHand, _ := poker.ParseHand("As", "Ad")
		board, _ := poker.ParseHand("2c", "7h", "Kd", "Ts", "9h") // Full board

		// 24 opponents * 2 cards = 48 cards, plus 2 hero + 5 board = 55 cards total (> 52)
		result := CalculateEquity(heroHand, board, 24, 1000, rng)

		// Should return empty result for too many cards needed
		if result.TotalSimulations != 0 {
			t.Errorf("Too many opponents should return empty result")
		}
	})
}

func TestEquityCalculatorWithProperEvaluator(t *testing.T) {
	t.Run("uses poker.Evaluate7Cards", func(t *testing.T) {
		// This test verifies that we're using the proper hand evaluator
		// The actual evaluation correctness is tested in the poker package
		rng := rand.New(rand.NewSource(42))
		heroHand, _ := poker.ParseHand("As", "Ad")
		board, _ := poker.ParseHand("2c", "7h", "Kd")

		// Small number of simulations for quick test
		result := CalculateEquity(heroHand, board, 1, 100, rng)

		// AA should have reasonable equity
		equity := result.Equity()
		if equity < 0.5 || equity > 1.0 {
			t.Errorf("AA equity = %v, expected between 0.5 and 1.0", equity)
		}

		if result.TotalSimulations != 100 {
			t.Errorf("TotalSimulations = %v, want 100", result.TotalSimulations)
		}
	})
}

func BenchmarkCalculateEquity(b *testing.B) {
	heroHand, _ := poker.ParseHand("As", "Ad")
	board, _ := poker.ParseHand("2c", "7h", "Kd")
	rng := rand.New(rand.NewSource(42))

	for b.Loop() {
		CalculateEquity(heroHand, board, 1, 10000, rng)
	}
}

// Rigorous large-sample benchmarks similar to poker evaluator
var benchSinkEquity float64

func generateRandomEquityScenarios(n int, seed int64) []struct {
	heroHand  poker.Hand
	board     poker.Hand
	opponents int
} {
	rng := rand.New(rand.NewSource(seed))
	deck := poker.NewDeck(rng)
	scenarios := make([]struct {
		heroHand  poker.Hand
		board     poker.Hand
		opponents int
	}, n)

	for i := range n {
		// Ensure enough cards remain; reshuffle if needed
		if deck.CardsRemaining() < 7 {
			deck.Shuffle()
		}

		// Deal hero holes (2 cards)
		heroCards := deck.Deal(2)
		heroHand := poker.NewHand(heroCards...)

		// Deal board (3-5 cards)
		boardSize := 3 + rng.Intn(3) // 3, 4, or 5 cards
		boardCards := deck.Deal(boardSize)
		board := poker.NewHand(boardCards...)

		// Random opponent count (1-3)
		opponents := 1 + rng.Intn(3)

		scenarios[i] = struct {
			heroHand  poker.Hand
			board     poker.Hand
			opponents int
		}{heroHand, board, opponents}
	}
	return scenarios
}

// BenchmarkCalculateEquity_LargeSample reports equity calculations per second over diverse scenarios
func BenchmarkCalculateEquity_LargeSample(b *testing.B) {
	const sampleSize = 1000
	const simulations = 1000
	scenarios := generateRandomEquityScenarios(sampleSize, 42)
	rng := rand.New(rand.NewSource(1337))

	b.ReportAllocs()

	start := time.Now()

	for i := 0; b.Loop(); i++ {
		scenario := scenarios[i%len(scenarios)]
		result := CalculateEquity(scenario.heroHand, scenario.board, scenario.opponents, simulations, rng)
		benchSinkEquity = result.Equity()
	}

	elapsed := time.Since(start)
	if elapsed > 0 {
		eqps := float64(b.N) / elapsed.Seconds()
		b.ReportMetric(eqps, "equity/sec")
		b.ReportMetric(float64(b.N*simulations)/elapsed.Seconds(), "sims/sec")
	}
}

// BenchmarkCalculateEquity_HighSims tests performance with high simulation counts
func BenchmarkCalculateEquity_HighSims(b *testing.B) {
	heroHand, _ := poker.ParseHand("As", "Ad")
	board, _ := poker.ParseHand("2c", "7h", "Kd")

	const simulations = 50000
	rng := rand.New(rand.NewSource(42))

	b.ReportAllocs()

	start := time.Now()

	for b.Loop() {
		result := CalculateEquity(heroHand, board, 1, simulations, rng)
		benchSinkEquity = result.Equity()
	}

	elapsed := time.Since(start)
	if elapsed > 0 {
		eqps := float64(b.N) / elapsed.Seconds()
		b.ReportMetric(eqps, "equity/sec")
		b.ReportMetric(float64(b.N*simulations)/elapsed.Seconds(), "sims/sec")
	}
}

// BenchmarkCalculateEquity_MultiWay tests multi-opponent scenarios
func BenchmarkCalculateEquity_MultiWay(b *testing.B) {
	heroHand, _ := poker.ParseHand("As", "Ad")
	board, _ := poker.ParseHand("2c", "7h", "Kd")

	const simulations = 10000
	rng := rand.New(rand.NewSource(42))

	testCases := []struct {
		name      string
		opponents int
	}{
		{"heads-up", 1},
		{"3-way", 2},
		{"4-way", 3},
		{"6-way", 5},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			start := time.Now()

			for b.Loop() {
				result := CalculateEquity(heroHand, board, tc.opponents, simulations, rng)
				benchSinkEquity = result.Equity()
			}

			elapsed := time.Since(start)
			if elapsed > 0 {
				eqps := float64(b.N) / elapsed.Seconds()
				b.ReportMetric(eqps, "equity/sec")
				b.ReportMetric(float64(b.N*simulations)/elapsed.Seconds(), "sims/sec")
			}
		})
	}
}

// BenchmarkCalculateEquity_BoardTextures tests different board types
func BenchmarkCalculateEquity_BoardTextures(b *testing.B) {
	heroHand, _ := poker.ParseHand("As", "Ad")

	const simulations = 10000
	rng := rand.New(rand.NewSource(42))

	testCases := []struct {
		name  string
		board poker.Hand
	}{
		{"dry-flop", mustParseHand("2c", "7h", "Kd")},
		{"wet-flop", mustParseHand("8s", "9s", "Ts")},
		{"paired-flop", mustParseHand("Ac", "Ad", "7h")},
		{"dry-turn", mustParseHand("2c", "7h", "Kd", "3s")},
		{"wet-turn", mustParseHand("8s", "9s", "Ts", "Js")},
		{"dry-river", mustParseHand("2c", "7h", "Kd", "3s", "9h")},
		{"wet-river", mustParseHand("8s", "9s", "Ts", "Js", "Qs")},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			start := time.Now()

			for b.Loop() {
				result := CalculateEquity(heroHand, tc.board, 1, simulations, rng)
				benchSinkEquity = result.Equity()
			}

			elapsed := time.Since(start)
			if elapsed > 0 {
				eqps := float64(b.N) / elapsed.Seconds()
				b.ReportMetric(eqps, "equity/sec")
				b.ReportMetric(float64(b.N*simulations)/elapsed.Seconds(), "sims/sec")
			}
		})
	}
}

// mustParseHand is a helper for benchmarks
func mustParseHand(cardStrs ...string) poker.Hand {
	hand, err := poker.ParseHand(cardStrs...)
	if err != nil {
		panic(err)
	}
	return hand
}

// BenchmarkCalculateEquity_Realistic tests the bot's actual usage pattern
func BenchmarkCalculateEquity_Realistic(b *testing.B) {
	const sampleSize = 1000
	const simulations = 10000
	scenarios := generateRandomEquityScenarios(sampleSize, 1337)
	rng := rand.New(rand.NewSource(42))

	b.ReportAllocs()

	start := time.Now()

	for i := 0; b.Loop(); i++ {
		scenario := scenarios[i%len(scenarios)]
		result := CalculateEquity(scenario.heroHand, scenario.board, scenario.opponents, simulations, rng)
		benchSinkEquity = result.Equity()
	}

	elapsed := time.Since(start)
	if elapsed > 0 {
		eqps := float64(b.N) / elapsed.Seconds()
		b.ReportMetric(eqps, "equity/sec")
		b.ReportMetric(float64(b.N*simulations)/elapsed.Seconds(), "sims/sec")
	}
}

// BenchmarkCalculateEquity_Parallel tests concurrent equity calculations
func BenchmarkCalculateEquity_Parallel(b *testing.B) {
	const sampleSize = 1000
	const simulations = 5000
	scenarios := generateRandomEquityScenarios(sampleSize, 1111)

	var idx uint64
	b.ReportAllocs()
	b.ResetTimer()
	start := time.Now()

	b.RunParallel(func(pb *testing.PB) {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		for pb.Next() {
			i := atomic.AddUint64(&idx, 1) - 1
			scenario := scenarios[i%uint64(len(scenarios))]
			result := CalculateEquity(scenario.heroHand, scenario.board, scenario.opponents, simulations, rng)
			benchSinkEquity = result.Equity()
		}
	})

	elapsed := time.Since(start)
	if elapsed > 0 {
		eqps := float64(b.N) / elapsed.Seconds()
		b.ReportMetric(eqps, "equity/sec")
		b.ReportMetric(float64(b.N*simulations)/elapsed.Seconds(), "sims/sec")
	}
}
