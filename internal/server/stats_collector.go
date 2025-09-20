package server

import (
	"sync"

	"github.com/lox/pokerforbots/internal/protocol"
	"github.com/lox/pokerforbots/internal/server/statistics"
)

// Detailed stats are collected when a DetailedStatsCollector is used.
// Basic stats are always collected in BotPool regardless.

// HandOutcomeDetail contains comprehensive information about a completed hand
type HandOutcomeDetail struct {
	HandID         string
	ButtonPosition int
	StreetReached  string
	Board          []string
	BotOutcomes    []BotHandOutcome
}

// BotHandOutcome contains per-bot information from a completed hand
type BotHandOutcome struct {
	Bot            *Bot
	Position       int      // Seat position (0-5)
	ButtonDistance int      // Distance from button (0=button, 1=CO, etc)
	HoleCards      []string // Bot's hole cards
	NetChips       int      // Chips won/lost
	WentToShowdown bool
	WonAtShowdown  bool
	Actions        map[string]string // Street -> action taken
}

// StatsCollector defines the interface for collecting game statistics
type StatsCollector interface {
	// RecordHandOutcome records the outcome of a completed hand
	RecordHandOutcome(detail HandOutcomeDetail) error

	// GetPlayerStats returns basic statistics for all players (collectors may return nil)
	GetPlayerStats() []PlayerStats

	// GetDetailedStats returns comprehensive statistics for a specific bot
	GetDetailedStats(botID string) *protocol.PlayerDetailedStats

	// Reset clears all statistics
	Reset()

	// IsEnabled returns whether statistics collection is active
	IsEnabled() bool
}

// NullStatsCollector provides a zero-overhead implementation for production use
type NullStatsCollector struct{}

// RecordHandOutcome does nothing (zero overhead)
func (n *NullStatsCollector) RecordHandOutcome(HandOutcomeDetail) error {
	return nil
}

// GetPlayerStats returns nil
func (n *NullStatsCollector) GetPlayerStats() []PlayerStats { return nil }

// GetDetailedStats returns nil
func (n *NullStatsCollector) GetDetailedStats(botID string) *protocol.PlayerDetailedStats { return nil }

// Reset does nothing
func (n *NullStatsCollector) Reset() {}

// IsEnabled always returns false
func (n *NullStatsCollector) IsEnabled() bool {
	return false
}

// DetailedStatsCollector provides comprehensive statistics collection for development
// It tracks advanced analytics beyond the basic stats maintained by BotPool
type DetailedStatsCollector struct {
	mu           sync.RWMutex
	stats        map[string]*statistics.Statistics // Per-bot advanced statistics
	maxHands     int                               // Maximum hands to track (memory limit)
	currentHands int
	bigBlind     int
}

// NewDetailedStatsCollector creates a new detailed statistics collector
func NewDetailedStatsCollector(maxHands int, bigBlind int) *DetailedStatsCollector {
	if maxHands <= 0 {
		maxHands = 10000 // Default limit
	}
	return &DetailedStatsCollector{
		stats:    make(map[string]*statistics.Statistics),
		maxHands: maxHands,
		bigBlind: bigBlind,
	}
}

// RecordHandOutcome records detailed statistics for a completed hand
func (d *DetailedStatsCollector) RecordHandOutcome(detail HandOutcomeDetail) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Implement circular buffer behavior - when we hit max, reset and start over
	if d.currentHands >= d.maxHands {
		// Reset all statistics to implement circular buffer
		d.stats = make(map[string]*statistics.Statistics)
		d.currentHands = 0
	}

	d.currentHands++

	for _, outcome := range detail.BotOutcomes {
		if outcome.Bot == nil {
			continue
		}

		botID := outcome.Bot.ID

		// Only track detailed stats - basic stats are handled by BotPool
		{
			if _, exists := d.stats[botID]; !exists {
				d.stats[botID] = statistics.NewStatistics(d.bigBlind)
			}

			// Convert to BB
			netBB := float64(outcome.NetChips) / float64(d.bigBlind)

			// Determine hand category
			handCategory := ""
			if len(outcome.HoleCards) == 2 {
				handCategory = categorizeHoleCards(outcome.HoleCards)
			}

			// Normalize street: treat showdown as river for street aggregation
			nStreet := detail.StreetReached
			if nStreet == "showdown" {
				nStreet = "river"
			}
			// Build HandResult
			handResult := statistics.HandResult{
				HandNum:        d.currentHands,
				NetBB:          netBB,
				Position:       outcome.Position,
				ButtonDistance: outcome.ButtonDistance,
				WentToShowdown: outcome.WentToShowdown,
				WonAtShowdown:  outcome.WonAtShowdown,
				FinalPotBB:     0, // Would need to calculate from detail
				StreetReached:  nStreet,
				HoleCards:      joinCards(outcome.HoleCards),
				HandCategory:   handCategory,
			}

			// Add actions if present
			if outcome.Actions != nil {
				handResult.PreflopAction = outcome.Actions["preflop"]
				handResult.FlopAction = outcome.Actions["flop"]
				handResult.TurnAction = outcome.Actions["turn"]
				handResult.RiverAction = outcome.Actions["river"]
			}

			// Add to statistics
			if err := d.stats[botID].Add(handResult); err != nil {
				// Log but don't fail - statistics should never break the game
				return nil
			}
		}
	}

	return nil
}

// GetPlayerStats returns nil - basic stats are maintained by BotPool
func (d *DetailedStatsCollector) GetPlayerStats() []PlayerStats { return nil }

// GetDetailedStats returns comprehensive statistics for a specific bot
func (d *DetailedStatsCollector) GetDetailedStats(botID string) *protocol.PlayerDetailedStats {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stats, exists := d.stats[botID]
	if !exists {
		return nil
	}

	// Build detailed statistics summary
	hands, sumBB, winningHands, _, showdownWins, nonShowdownWins, showdownLosses, showdownBB, nonShowdownBB, maxPotBB, bigPots, _, _, _ := stats.GetStats()
	mean := stats.Mean()
	bb100 := stats.BB100()
	median := stats.Median()
	stdDev := stats.StdDev()
	low, high := stats.ConfidenceInterval95()

	detailed := &protocol.PlayerDetailedStats{
		Hands:           hands,
		NetBB:           sumBB,
		BB100:           bb100,
		Mean:            mean,
		Median:          median,
		StdDev:          stdDev,
		CI95Low:         low,
		CI95High:        high,
		WinningHands:    winningHands,
		ShowdownWins:    showdownWins,
		NonShowdownWins: nonShowdownWins,
		ShowdownBB:      showdownBB,
		NonShowdownBB:   nonShowdownBB,
		MaxPotBB:        maxPotBB,
		BigPots:         bigPots,
	}

	if hands > 0 {
		detailed.WinRate = float64(winningHands) / float64(hands) * 100
	}

	totalShowdowns := showdownWins + showdownLosses
	if totalShowdowns > 0 {
		detailed.ShowdownWinRate = float64(showdownWins) / float64(totalShowdowns) * 100
	}

	// Add position stats
	detailed.PositionStats = make(map[string]protocol.PositionStatSummary)
	buttonDist := stats.ButtonDistanceResults()
	for dist := 0; dist < 6; dist++ {
		bd := buttonDist[dist]
		if bd.Hands > 0 {
			posName := statistics.GetPositionName(dist)
			detailed.PositionStats[posName] = protocol.PositionStatSummary{
				Hands:     bd.Hands,
				NetBB:     bd.SumBB,
				BBPerHand: bd.SumBB / float64(bd.Hands),
			}
		}
	}

	// Add street stats
	detailed.StreetStats = make(map[string]protocol.StreetStatSummary)
	for street, stat := range stats.StreetStats() {
		if stat.HandsReached > 0 {
			detailed.StreetStats[street] = protocol.StreetStatSummary{
				HandsEnded: stat.HandsReached,
				NetBB:      stat.NetBB,
				BBPerHand:  stat.NetBB / float64(stat.HandsReached),
			}
		}
	}

	// Add hand category stats
	detailed.HandCategoryStats = make(map[string]protocol.CategoryStatSummary)
	for cat, stat := range stats.CategoryStats() {
		if stat.Hands > 0 {
			detailed.HandCategoryStats[cat] = protocol.CategoryStatSummary{
				Hands:     stat.Hands,
				NetBB:     stat.NetBB,
				BBPerHand: stat.NetBB / float64(stat.Hands),
			}
		}
	}

	return detailed
}

// Reset clears all statistics
func (d *DetailedStatsCollector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stats = make(map[string]*statistics.Statistics)
	d.currentHands = 0
}

// IsEnabled always returns true for DetailedStatsCollector
func (d *DetailedStatsCollector) IsEnabled() bool {
	return true
}

// GetMemoryUsage returns current memory usage statistics
func (d *DetailedStatsCollector) GetMemoryUsage() (currentHands int, maxHands int) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.currentHands, d.maxHands
}

// Helper function to categorize hole cards (delegates to statistics)
func categorizeHoleCards(cards []string) string {
	return statistics.CategorizeHoleCards(cards)
}

func joinCards(cards []string) string {
	if len(cards) == 0 {
		return ""
	}
	result := cards[0]
	for i := 1; i < len(cards); i++ {
		result += cards[i]
	}
	return result
}

// DetailedStats contains comprehensive statistics for protocol messages
type DetailedStats struct {
	// Summary
	Hands    int     `json:"hands"`
	NetBB    float64 `json:"net_bb"`
	BB100    float64 `json:"bb_per_100"`
	Mean     float64 `json:"mean"`
	Median   float64 `json:"median"`
	StdDev   float64 `json:"std_dev"`
	CI95Low  float64 `json:"ci_95_low"`
	CI95High float64 `json:"ci_95_high"`

	// Win/loss
	WinningHands    int     `json:"winning_hands"`
	WinRate         float64 `json:"win_rate"`
	ShowdownWins    int     `json:"showdown_wins"`
	NonShowdownWins int     `json:"non_showdown_wins"`
	ShowdownWinRate float64 `json:"showdown_win_rate"`
	ShowdownBB      float64 `json:"showdown_bb"`
	NonShowdownBB   float64 `json:"non_showdown_bb"`

	// Pots
	MaxPotBB float64 `json:"max_pot_bb"`
	BigPots  int     `json:"big_pots"`

	// Breakdown (optional by depth)
	PositionStats     map[string]PositionSummary `json:"position_stats,omitempty"`
	StreetStats       map[string]StreetSummary   `json:"street_stats,omitempty"`
	HandCategoryStats map[string]CategorySummary `json:"hand_category_stats,omitempty"`
}

// PositionSummary contains position-specific statistics
type PositionSummary struct {
	Hands     int     `json:"hands"`
	NetBB     float64 `json:"net_bb"`
	BBPerHand float64 `json:"bb_per_hand"`
}

// StreetSummary contains street-specific statistics
type StreetSummary struct {
	HandsEnded int     `json:"hands_ended"`
	NetBB      float64 `json:"net_bb"`
	BBPerHand  float64 `json:"bb_per_hand"`
}

// CategorySummary contains hand category statistics
type CategorySummary struct {
	Hands     int     `json:"hands"`
	NetBB     float64 `json:"net_bb"`
	BBPerHand float64 `json:"bb_per_hand"`
}
