package statistics

import (
	"fmt"
	"math"
	"sort"
)

// HandResult represents the outcome of a single poker hand
type HandResult struct {
	NetBB          float64 // Net big blinds won/lost for our bot
	Seed           int64   // RNG seed for this hand (for replay)
	Position       int     // OurBot's position (1-6)
	WentToShowdown bool    // Did hand go to showdown?
	FinalPotSize   int     // Final pot size in chips
	StreetReached  string  // Furthest street reached (Pre-flop, Flop, Turn, River)
}

// PositionStats tracks statistics for a specific table position
type PositionStats struct {
	Hands  int
	SumBB  float64
	SumBB2 float64
}

// Statistics tracks comprehensive poker simulation statistics
type Statistics struct {
	Hands  int
	SumBB  float64
	SumBB2 float64   // Sum of squares for variance calculation
	Values []float64 // Store all values for median/percentile calculation

	// Detailed analytics - track ALL results, not just wins
	ShowdownWins    int     // Hands won at showdown
	NonShowdownWins int     // Hands won without showdown (fold equity)
	ShowdownBB      float64 // BB from showdown (wins AND losses)
	NonShowdownBB   float64 // BB from fold equity (wins AND losses)
	AllBB           float64 // Total BB for sanity check

	// Position analytics
	PositionResults [7]PositionStats // Index 0 unused, 1-6 for positions

	// Pot size analytics
	MaxPotChips int     // Largest pot observed (in chips)
	MaxPotBB    float64 // Largest pot observed (in bb)
	BigPots     int     // Pots >= 50bb (high action hands)
	BigPotsBB   float64 // BB from big pots
}

// Mean returns the arithmetic mean of all results in big blinds per hand
func (s *Statistics) Mean() float64 {
	if s.Hands == 0 {
		return 0
	}
	return s.SumBB / float64(s.Hands)
}

// Variance returns the sample variance of all results
func (s *Statistics) Variance() float64 {
	if s.Hands < 2 {
		return 0
	}
	mean := s.Mean()
	return (s.SumBB2 - float64(s.Hands)*mean*mean) / float64(s.Hands-1)
}

// StdDev returns the sample standard deviation of all results
func (s *Statistics) StdDev() float64 {
	return math.Sqrt(s.Variance())
}

// StdError returns the standard error of the mean
func (s *Statistics) StdError() float64 {
	if s.Hands == 0 {
		return 0
	}
	return s.StdDev() / math.Sqrt(float64(s.Hands))
}

// ConfidenceInterval95 returns the 95% confidence interval for the mean
func (s *Statistics) ConfidenceInterval95() (float64, float64) {
	mean := s.Mean()
	se := s.StdError()
	margin := 1.96 * se // 95% confidence
	return mean - margin, mean + margin
}

// Add incorporates a new hand result into the statistics
func (s *Statistics) Add(result HandResult) {
	netBB := result.NetBB
	s.Hands++
	s.SumBB += netBB
	s.SumBB2 += netBB * netBB
	s.Values = append(s.Values, netBB)

	// Track showdown vs non-showdown wins
	if netBB > 0 {
		if result.WentToShowdown {
			s.ShowdownWins++
		} else {
			s.NonShowdownWins++
		}
	}

	// Track ALL results (wins and losses) in appropriate buckets
	if result.WentToShowdown {
		s.ShowdownBB += netBB
	} else {
		s.NonShowdownBB += netBB
	}
	s.AllBB += netBB // Total for sanity check

	// Track by position
	pos := result.Position
	if pos >= 1 && pos <= 6 {
		s.PositionResults[pos].Hands++
		s.PositionResults[pos].SumBB += netBB
		s.PositionResults[pos].SumBB2 += netBB * netBB
	}

	// Track pot sizes and limits
	potChips := result.FinalPotSize
	potBB := float64(potChips) / 2.0 // 2 chips = 1 big blind

	// Update max pot if this is largest seen
	if potChips > s.MaxPotChips {
		s.MaxPotChips = potChips
		s.MaxPotBB = potBB
	}

	// Track big pots (>= 50bb)
	if potBB >= 50 {
		s.BigPots++
		s.BigPotsBB += netBB
	}
}

// Median returns the median value of all results
func (s *Statistics) Median() float64 {
	if len(s.Values) == 0 {
		return 0
	}
	sorted := make([]float64, len(s.Values))
	copy(sorted, s.Values)
	sort.Float64s(sorted)

	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

// Percentile returns the value at the given percentile (0.0 to 1.0)
func (s *Statistics) Percentile(p float64) float64 {
	if len(s.Values) == 0 {
		return 0
	}
	sorted := make([]float64, len(s.Values))
	copy(sorted, s.Values)
	sort.Float64s(sorted)

	index := p * float64(len(sorted)-1)
	lower := int(index)
	upper := lower + 1

	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}

	weight := index - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

// PositionMean returns the mean result for a specific position (1-6)
func (s *Statistics) PositionMean(position int) float64 {
	if position < 1 || position > 6 {
		return 0
	}
	ps := s.PositionResults[position]
	if ps.Hands == 0 {
		return 0
	}
	return ps.SumBB / float64(ps.Hands)
}

// IsLedgerBalanced checks if the accounting is consistent
func (s *Statistics) IsLedgerBalanced() bool {
	return math.Abs(s.AllBB-s.ShowdownBB-s.NonShowdownBB) <= 1e-6
}

// Validate performs comprehensive validation of statistics data
func (s *Statistics) Validate() error {
	// Check ledger balance
	if !s.IsLedgerBalanced() {
		return fmt.Errorf("ledger mismatch: AllBB=%.6f, ShowdownBB=%.6f, NonShowdownBB=%.6f", 
			s.AllBB, s.ShowdownBB, s.NonShowdownBB)
	}
	
	// Check that hands count is positive
	if s.Hands <= 0 {
		return fmt.Errorf("invalid hands count: %d", s.Hands)
	}
	
	// Check that values array matches hands count
	if len(s.Values) != s.Hands {
		return fmt.Errorf("values array length (%d) does not match hands count (%d)", 
			len(s.Values), s.Hands)
	}
	
	// Check that wins don't exceed total hands
	totalWins := s.ShowdownWins + s.NonShowdownWins
	if totalWins > s.Hands {
		return fmt.Errorf("total wins (%d) exceeds total hands (%d)", totalWins, s.Hands)
	}
	
	// Check position data consistency
	totalPositionHands := 0
	for pos := 1; pos <= 6; pos++ {
		totalPositionHands += s.PositionResults[pos].Hands
	}
	if totalPositionHands != s.Hands {
		return fmt.Errorf("position hands total (%d) does not match total hands (%d)", 
			totalPositionHands, s.Hands)
	}
	
	return nil
}
