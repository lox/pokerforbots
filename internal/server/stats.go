package server

import (
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/lox/pokerforbots/protocol"
)

// BotStatistics tracks statistics for a single bot
type BotStatistics struct {
	mu              sync.RWMutex
	bigBlind        int
	hands           int
	sumBB           float64
	sumBB2          float64   // Sum of squares for variance
	values          []float64 // All BB results for median/percentile
	winningHands    int
	showdownWins    int
	nonShowdownWins int
	showdownLosses  int
	showdownBB      float64
	nonShowdownBB   float64
	vpipHands       int
	pfrHands        int
	timeoutCount    int
	bustCount       int
}

// NewBotStatistics creates a new BotStatistics instance
func NewBotStatistics(bigBlind int) *BotStatistics {
	return &BotStatistics{
		bigBlind: bigBlind,
		values:   make([]float64, 0),
	}
}

// AddResult incorporates a new hand result
func (b *BotStatistics) AddResult(netBB float64, wentToShowdown, wonAtShowdown bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.hands++
	b.sumBB += netBB
	b.sumBB2 += netBB * netBB
	b.values = append(b.values, netBB)

	// Track wins/losses
	if netBB > 0 {
		b.winningHands++
		if wentToShowdown {
			b.showdownWins++
		} else {
			b.nonShowdownWins++
		}
	} else if netBB < 0 && wentToShowdown {
		b.showdownLosses++
	}

	// Track showdown vs non-showdown BB
	if wentToShowdown {
		b.showdownBB += netBB
	} else {
		b.nonShowdownBB += netBB
	}
}

// RecordPreflopAction updates VPIP/PFR counters based on the action taken.
func (b *BotStatistics) RecordPreflopAction(action string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch action {
	case "call", "raise", "allin", "bet":
		b.vpipHands++
		if action == "raise" || action == "allin" || action == "bet" {
			b.pfrHands++
		}
	}
}

// RecordTimeout increments the timeout counter for the bot.
func (b *BotStatistics) RecordTimeout() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.timeoutCount++
}

// RecordBust increments the bust counter for the bot.
func (b *BotStatistics) RecordBust() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bustCount++
}

// ToProtocolStats converts to protocol.PlayerDetailedStats
func (b *BotStatistics) ToProtocolStats() *protocol.PlayerDetailedStats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.hands == 0 {
		return nil
	}

	mean := b.sumBB / float64(b.hands)
	bb100 := mean * 100

	result := &protocol.PlayerDetailedStats{
		Hands:           b.hands,
		NetBB:           b.sumBB,
		BB100:           bb100,
		Mean:            mean,
		Median:          b.median(),
		StdDev:          b.stdDev(),
		WinningHands:    b.winningHands,
		WinRate:         float64(b.winningHands) / float64(b.hands) * 100,
		ShowdownWins:    b.showdownWins,
		NonShowdownWins: b.nonShowdownWins,
		ShowdownBB:      b.showdownBB,
		NonShowdownBB:   b.nonShowdownBB,
	}

	if b.hands > 0 {
		result.VPIP = float64(b.vpipHands) / float64(b.hands)
		result.PFR = float64(b.pfrHands) / float64(b.hands)
	}

	result.Timeouts = b.timeoutCount
	result.Busts = b.bustCount

	// Calculate CI if we have enough hands
	if b.hands >= 30 {
		se := b.stdError()
		margin := 1.96 * se
		result.CI95Low = (mean - margin) * 100
		result.CI95High = (mean + margin) * 100
	}

	// Calculate showdown win rate
	showdownsTotal := b.showdownWins + b.showdownLosses
	if showdownsTotal > 0 {
		result.ShowdownWinRate = float64(b.showdownWins) / float64(showdownsTotal) * 100
	}

	return result
}

// Hands returns the total number of hands
func (b *BotStatistics) Hands() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.hands
}

// Mean returns the arithmetic mean in BB/hand
func (b *BotStatistics) Mean() float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.hands == 0 {
		return 0
	}
	return b.sumBB / float64(b.hands)
}

// BB100 returns big blinds per 100 hands
func (b *BotStatistics) BB100() float64 {
	return b.Mean() * 100
}

// median returns the median BB/hand (internal, called with lock held)
func (b *BotStatistics) median() float64 {
	if len(b.values) == 0 {
		return 0
	}
	sorted := make([]float64, len(b.values))
	copy(sorted, b.values)
	sort.Float64s(sorted)

	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

// variance returns the sample variance (internal, called with lock held)
func (b *BotStatistics) variance() float64 {
	if b.hands < 2 {
		return 0
	}
	mean := b.sumBB / float64(b.hands)
	return (b.sumBB2 - float64(b.hands)*mean*mean) / float64(b.hands-1)
}

// stdDev returns the sample standard deviation (internal, called with lock held)
func (b *BotStatistics) stdDev() float64 {
	return math.Sqrt(b.variance())
}

// stdError returns the standard error of the mean (internal, called with lock held)
func (b *BotStatistics) stdError() float64 {
	if b.hands == 0 {
		return 0
	}
	return b.stdDev() / math.Sqrt(float64(b.hands))
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

// StatsCollector defines the interface for collecting game statistics
type StatsCollector interface {
	IsEnabled() bool
	RecordHandOutcome(detail HandOutcomeDetail) error
	GetPlayerStats() map[string]PlayerStats
	GetDetailedStats(botID string) *protocol.PlayerDetailedStats
	Reset()
}

// HandOutcomeDetail contains the complete outcome of a hand for statistics tracking
type HandOutcomeDetail struct {
	HandID         string
	ButtonPosition int
	StreetReached  string
	Board          []string
	BotOutcomes    []BotHandOutcome
}

// BotHandOutcome contains per-bot outcome details
type BotHandOutcome struct {
	Bot            *Bot
	Position       int
	ButtonDistance int
	HoleCards      []string
	NetChips       int
	WentToShowdown bool
	WonAtShowdown  bool
	Actions        map[string]string // Street -> last action taken
	TimedOut       bool              // Whether the bot timed out during the hand
	WentBroke      bool              // Whether the bot went broke in this hand
}

// NullStatsCollector implements StatsCollector with no-ops for when stats are disabled
type NullStatsCollector struct{}

func (n *NullStatsCollector) IsEnabled() bool                                         { return false }
func (n *NullStatsCollector) RecordHandOutcome(_ HandOutcomeDetail) error             { return nil }
func (n *NullStatsCollector) GetPlayerStats() map[string]PlayerStats                  { return nil }
func (n *NullStatsCollector) GetDetailedStats(_ string) *protocol.PlayerDetailedStats { return nil }
func (n *NullStatsCollector) Reset()                                                  {}

// DetailedStatsCollector implements StatsCollector with comprehensive statistics tracking
type DetailedStatsCollector struct {
	mu           sync.RWMutex
	bigBlind     int
	stats        map[string]*BotStatistics // Per-bot statistics
	maxHands     int                       // Maximum hands to store before reset
	currentHands int                       // Current number of hands stored
}

// NewDetailedStatsCollector creates a new DetailedStatsCollector with memory limits
func NewDetailedStatsCollector(maxHands, bigBlind int) *DetailedStatsCollector {
	return &DetailedStatsCollector{
		bigBlind: bigBlind,
		stats:    make(map[string]*BotStatistics),
		maxHands: maxHands,
	}
}

func (d *DetailedStatsCollector) IsEnabled() bool { return true }

// RecordHandOutcome records the outcome of a hand for all participating bots
func (d *DetailedStatsCollector) RecordHandOutcome(detail HandOutcomeDetail) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check memory limit and reset if necessary
	d.currentHands++
	if d.maxHands > 0 && d.currentHands > d.maxHands {
		// Reset to avoid unbounded memory growth
		d.stats = make(map[string]*BotStatistics)
		d.currentHands = 1
	}

	// Process each bot's outcome
	for _, outcome := range detail.BotOutcomes {
		if outcome.Bot == nil {
			continue
		}

		botID := outcome.Bot.ID
		if d.stats[botID] == nil {
			d.stats[botID] = NewBotStatistics(d.bigBlind)
		}

		// Convert chips to big blinds
		netBB := float64(outcome.NetChips) / float64(d.bigBlind)

		// Add the result
		stat := d.stats[botID]
		stat.AddResult(netBB, outcome.WentToShowdown, outcome.WonAtShowdown)

		if outcome.Actions != nil {
			stat.RecordPreflopAction(outcome.Actions["preflop"])
		}

		if outcome.TimedOut {
			stat.RecordTimeout()
		}

		if outcome.WentBroke {
			stat.RecordBust()
		}
	}

	return nil
}

// GetPlayerStats returns nil as basic stats are maintained by BotPool
func (d *DetailedStatsCollector) GetPlayerStats() map[string]PlayerStats {
	return nil
}

// GetDetailedStats returns comprehensive statistics for a specific bot
func (d *DetailedStatsCollector) GetDetailedStats(botID string) *protocol.PlayerDetailedStats {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stats, exists := d.stats[botID]
	if !exists || stats == nil || stats.Hands() == 0 {
		return nil
	}

	return stats.ToProtocolStats()
}

// Reset clears all collected statistics
func (d *DetailedStatsCollector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stats = make(map[string]*BotStatistics)
	d.currentHands = 0
}

// GetMemoryUsage returns current and maximum hands stored
func (d *DetailedStatsCollector) GetMemoryUsage() (current, max int) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.currentHands, d.maxHands
}
