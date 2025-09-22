package statistics

import (
	"fmt"
	"maps"
	"math"
	"sort"
	"sync"
)

// CategorizeHoleCards provides a simple preflop hand categorization used by development stats.
// Categories: Premium, Strong, Medium, Weak, Trash. Input cards like ["As","Kd"].
func CategorizeHoleCards(cards []string) string {
	if len(cards) != 2 {
		return "unknown"
	}
	r1, r2 := cardRank(cards[0]), cardRank(cards[1])
	suited := len(cards[0]) >= 2 && len(cards[1]) >= 2 && cards[0][1] == cards[1][1]
	if r1 > r2 {
		r1, r2 = r2, r1
	}
	if (r1 >= 11 && r2 >= 11) || (r1 == 12 && r2 == 13) { // JJ+, AK
		return "Premium"
	}
	if (r1 >= 10 && r2 >= 10) || (r1 >= 12 && r2 >= 11) { // TT+, AQ/AJ
		return "Strong"
	}
	if (r1 >= 7 && r2 >= 7) || (suited && r1 >= 10 && r2 >= 10) { // 77+, suited broadway
		return "Medium"
	}
	if r1 >= 2 || (suited && absDiff(r1, r2) <= 2) { // small pairs / suited connectors
		return "Weak"
	}
	return "Trash"
}

func cardRank(card string) int {
	if len(card) < 1 {
		return 0
	}
	switch card[0] {
	case 'A':
		return 14
	case 'K':
		return 13
	case 'Q':
		return 12
	case 'J':
		return 11
	case 'T':
		return 10
	default:
		if card[0] >= '2' && card[0] <= '9' {
			return int(card[0] - '0')
		}
		return 0
	}
}

func absDiff(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

// HandResult represents the outcome of a single poker hand
type HandResult struct {
	HandNum        int     // Hand number in sequence
	NetBB          float64 // Net big blinds won/lost
	Position       int     // Player's seat position (1-6)
	ButtonDistance int     // Distance from button (0=button, 1=CO, etc)
	WentToShowdown bool    // Did hand go to showdown?
	WonAtShowdown  bool    // If showdown, did we win?
	FinalPotBB     float64 // Final pot size in BB
	StreetReached  string  // Furthest street reached (Preflop, Flop, Turn, River)
	HoleCards      string  // Our hole cards (e.g. "AhKs")
	HandCategory   string  // Preflop hand category (Premium, Strong, Medium, Weak, Trash)

	// Action tracking
	PreflopAction string // Last preflop action (fold, call, raise)
	FlopAction    string // Last flop action if reached
	TurnAction    string // Last turn action if reached
	RiverAction   string // Last river action if reached

	// Opponent info
	NumOpponents  int    // Number of opponents in hand
	OpponentTypes string // Mix of opponent types faced
}

// Constants for the statistics package
const (
	// BigPotThreshold is the minimum BB for a pot to be considered "big"
	BigPotThreshold = 50.0
	// SmallPotThreshold is the maximum BB for a pot to be considered "small"
	SmallPotThreshold = 10.0
)

// Statistics tracks comprehensive poker simulation statistics
type Statistics struct {
	mu       sync.RWMutex
	bigBlind int // The big blind amount in chips
	hands    int
	sumBB    float64
	sumBB2   float64   // Sum of squares for variance
	values   []float64 // All BB results for median/percentile

	// Core win/loss tracking
	winningHands    int     // Total winning hands
	losingHands     int     // Total losing hands
	showdownWins    int     // Hands won at showdown
	nonShowdownWins int     // Hands won without showdown
	showdownLosses  int     // Hands lost at showdown
	showdownBB      float64 // Total BB from showdown
	nonShowdownBB   float64 // Total BB from non-showdown

	// Position statistics (1-6 for seat, 0-5 for button distance)
	positionResults       [7]PositionStats // By seat position
	buttonDistanceResults [6]PositionStats // By button distance

	// Street statistics
	streetStats map[string]*StreetStat

	// Hand category performance
	categoryStats map[string]*CategoryStat

	// Action frequency tracking
	preflopActions map[string]int
	totalActions   map[string]int // All streets combined

	// Pot size analytics
	maxPotBB    float64 // Largest pot observed
	bigPots     int     // Pots >= 50bb
	bigPotsBB   float64 // BB from big pots
	smallPots   int     // Pots < 10bb
	smallPotsBB float64 // BB from small pots

	// Detailed hand results for analysis
	handResults []HandResult

	// Additional tracking for new stats
	vpipCount int // Hands where player voluntarily put money in pot
	pfrCount  int // Hands where player raised preflop
	timeouts  int // Number of timeout actions
	busts     int // Number of times player went broke
}

// PositionStats tracks statistics for a specific position
type PositionStats struct {
	Hands  int
	SumBB  float64
	SumBB2 float64
	Wins   int
	Losses int
}

// StreetStat tracks statistics per street
type StreetStat struct {
	HandsReached int     // Hands that reached this street
	NetBB        float64 // Total BB won/lost on this street
	Wins         int     // Hands won on this street
	Losses       int     // Hands lost on this street
}

// CategoryStat tracks statistics per hand category
type CategoryStat struct {
	Hands          int
	NetBB          float64
	Wins           int
	ShowdownWins   int
	WentToShowdown int
}

// NewStatistics creates a new Statistics instance
func NewStatistics(bigBlind int) *Statistics {
	return &Statistics{
		bigBlind:       bigBlind,
		values:         make([]float64, 0),
		streetStats:    make(map[string]*StreetStat),
		categoryStats:  make(map[string]*CategoryStat),
		preflopActions: make(map[string]int),
		totalActions:   make(map[string]int),
		handResults:    make([]HandResult, 0),
	}
}

// Add incorporates a new hand result into the statistics
func (s *Statistics) Add(result HandResult) error {
	// Validate input
	if result.Position < 0 || result.Position > 6 {
		return fmt.Errorf("invalid position: %d", result.Position)
	}
	if result.ButtonDistance < 0 || result.ButtonDistance > 5 {
		return fmt.Errorf("invalid button distance: %d", result.ButtonDistance)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	netBB := result.NetBB
	s.hands++
	s.sumBB += netBB
	s.sumBB2 += netBB * netBB
	s.values = append(s.values, netBB)
	s.handResults = append(s.handResults, result)

	// Track wins/losses
	if netBB > 0 {
		s.winningHands++
		if result.WentToShowdown {
			s.showdownWins++
		} else {
			s.nonShowdownWins++
		}
	} else if netBB < 0 {
		s.losingHands++
		if result.WentToShowdown {
			s.showdownLosses++
		}
	}

	// Track showdown vs non-showdown BB
	if result.WentToShowdown {
		s.showdownBB += netBB
	} else {
		s.nonShowdownBB += netBB
	}

	// Track by seat position
	if result.Position >= 1 && result.Position <= 6 {
		ps := &s.positionResults[result.Position]
		ps.Hands++
		ps.SumBB += netBB
		ps.SumBB2 += netBB * netBB
		if netBB > 0 {
			ps.Wins++
		} else if netBB < 0 {
			ps.Losses++
		}
	}

	// Track by button distance
	if result.ButtonDistance >= 0 && result.ButtonDistance < 6 {
		bd := &s.buttonDistanceResults[result.ButtonDistance]
		bd.Hands++
		bd.SumBB += netBB
		bd.SumBB2 += netBB * netBB
		if netBB > 0 {
			bd.Wins++
		} else if netBB < 0 {
			bd.Losses++
		}
	}

	// Track street statistics
	if s.streetStats[result.StreetReached] == nil {
		s.streetStats[result.StreetReached] = &StreetStat{}
	}
	street := s.streetStats[result.StreetReached]
	street.HandsReached++
	street.NetBB += netBB
	if netBB > 0 {
		street.Wins++
	} else if netBB < 0 {
		street.Losses++
	}

	// Track hand category performance
	if result.HandCategory != "" {
		if s.categoryStats[result.HandCategory] == nil {
			s.categoryStats[result.HandCategory] = &CategoryStat{}
		}
		cat := s.categoryStats[result.HandCategory]
		cat.Hands++
		cat.NetBB += netBB
		if netBB > 0 {
			cat.Wins++
			if result.WentToShowdown && result.WonAtShowdown {
				cat.ShowdownWins++
			}
		}
		if result.WentToShowdown {
			cat.WentToShowdown++
		}
	}

	// Track actions
	if result.PreflopAction != "" {
		s.preflopActions[result.PreflopAction]++
		s.totalActions[result.PreflopAction]++

		// Track VPIP (call or raise preflop)
		if result.PreflopAction == "call" || result.PreflopAction == "raise" {
			s.vpipCount++
		}

		// Track PFR (raise preflop)
		if result.PreflopAction == "raise" {
			s.pfrCount++
		}
	}
	for _, action := range []string{result.FlopAction, result.TurnAction, result.RiverAction} {
		if action != "" {
			s.totalActions[action]++
		}
	}

	// Track pot sizes
	if result.FinalPotBB > s.maxPotBB {
		s.maxPotBB = result.FinalPotBB
	}
	if result.FinalPotBB >= BigPotThreshold {
		s.bigPots++
		s.bigPotsBB += netBB
	} else if result.FinalPotBB < SmallPotThreshold {
		s.smallPots++
		s.smallPotsBB += netBB
	}

	return nil
}

// Hands returns the total number of hands tracked
func (s *Statistics) Hands() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hands
}

// Mean returns the arithmetic mean in BB/hand
func (s *Statistics) Mean() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.hands == 0 {
		return 0
	}
	return s.sumBB / float64(s.hands)
}

// BB100 returns big blinds per 100 hands
func (s *Statistics) BB100() float64 {
	return s.Mean() * 100
}

// Variance returns the sample variance
func (s *Statistics) Variance() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.hands < 2 {
		return 0
	}
	mean := s.sumBB / float64(s.hands)
	return (s.sumBB2 - float64(s.hands)*mean*mean) / float64(s.hands-1)
}

// StdDev returns the sample standard deviation
func (s *Statistics) StdDev() float64 {
	return math.Sqrt(s.Variance())
}

// StdError returns the standard error of the mean
func (s *Statistics) StdError() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.hands == 0 {
		return 0
	}
	// Calculate StdDev inline to avoid deadlock
	if s.hands < 2 {
		return 0
	}
	mean := s.sumBB / float64(s.hands)
	variance := (s.sumBB2 - float64(s.hands)*mean*mean) / float64(s.hands-1)
	return math.Sqrt(variance) / math.Sqrt(float64(s.hands))
}

// ConfidenceInterval95 returns the 95% confidence interval
func (s *Statistics) ConfidenceInterval95() (float64, float64) {
	mean := s.Mean()
	se := s.StdError()
	margin := 1.96 * se
	return mean - margin, mean + margin
}

// Median returns the median BB/hand
func (s *Statistics) Median() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.values) == 0 {
		return 0
	}
	sorted := make([]float64, len(s.values))
	copy(sorted, s.values)
	sort.Float64s(sorted)

	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

// Percentile returns the value at the given percentile (0.0 to 1.0)
func (s *Statistics) Percentile(p float64) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.values) == 0 {
		return 0
	}
	sorted := make([]float64, len(s.values))
	copy(sorted, s.values)
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

// PositionMean returns mean BB/hand for a seat position
func (s *Statistics) PositionMean(position int) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if position < 1 || position > 6 {
		return 0
	}
	ps := s.positionResults[position]
	if ps.Hands == 0 {
		return 0
	}
	return ps.SumBB / float64(ps.Hands)
}

// ButtonDistanceMean returns mean BB/hand for button distance
func (s *Statistics) ButtonDistanceMean(distance int) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if distance < 0 || distance >= 6 {
		return 0
	}
	bd := s.buttonDistanceResults[distance]
	if bd.Hands == 0 {
		return 0
	}
	return bd.SumBB / float64(bd.Hands)
}

// GetPositionName returns friendly name for button distance
func GetPositionName(distance int) string {
	switch distance {
	case 0:
		return "Button"
	case 1:
		return "Cutoff"
	case 2:
		return "Hijack"
	case 3:
		return "MP"
	case 4:
		return "EP2"
	case 5:
		return "EP1"
	default:
		return fmt.Sprintf("Pos%d", distance)
	}
}

// GetStats returns all statistics in a thread-safe manner
func (s *Statistics) GetStats() (hands int, sumBB float64, winningHands int, losingHands int, showdownWins int, nonShowdownWins int, showdownLosses int, showdownBB float64, nonShowdownBB float64, maxPotBB float64, bigPots int, smallPots int, bigPotsBB float64, smallPotsBB float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hands, s.sumBB, s.winningHands, s.losingHands, s.showdownWins, s.nonShowdownWins, s.showdownLosses, s.showdownBB, s.nonShowdownBB, s.maxPotBB, s.bigPots, s.smallPots, s.bigPotsBB, s.smallPotsBB
}

// ButtonDistanceResults returns button distance statistics
func (s *Statistics) ButtonDistanceResults() [6]PositionStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.buttonDistanceResults
}

// StreetStats returns a copy of street statistics
func (s *Statistics) StreetStats() map[string]*StreetStat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	copy := make(map[string]*StreetStat)
	maps.Copy(copy, s.streetStats)
	return copy
}

// CategoryStats returns a copy of category statistics
func (s *Statistics) CategoryStats() map[string]*CategoryStat {
	s.mu.RLock()
	defer s.mu.RUnlock()
	copy := make(map[string]*CategoryStat)
	maps.Copy(copy, s.categoryStats)
	return copy
}

// GetVPIP returns the VPIP percentage
func (s *Statistics) GetVPIP() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.hands == 0 {
		return 0
	}
	return float64(s.vpipCount) / float64(s.hands)
}

// GetPFR returns the PFR percentage
func (s *Statistics) GetPFR() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.hands == 0 {
		return 0
	}
	return float64(s.pfrCount) / float64(s.hands)
}

// GetTimeouts returns the number of timeouts
func (s *Statistics) GetTimeouts() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.timeouts
}

// GetBusts returns the number of busts
func (s *Statistics) GetBusts() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.busts
}

// IncrementTimeouts increments the timeout counter
func (s *Statistics) IncrementTimeouts() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timeouts++
}

// IncrementBusts increments the bust counter
func (s *Statistics) IncrementBusts() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.busts++
}

// Summary method removed - formatting now handled by external tools (e.g., cmd/spawner)
// The statistics package focuses on data collection, not presentation.
// All statistics data is available via the DetailedStats struct for JSON serialization.
