package regression

import (
	"fmt"
	"math"
	"strings"

	"github.com/lox/pokerforbots/internal/server"
)

// AggregationResult holds both results and standard deviations from aggregation
type AggregationResult struct {
	Results map[string]float64
	StdDevs map[string]float64
}

// AggregateHeadsUpStats aggregates stats for heads-up mode
func AggregateHeadsUpStats(stats *server.GameStats) (map[string]float64, error) {
	if len(stats.Players) != 2 {
		return nil, fmt.Errorf("expected 2 players in heads-up stats, got %d", len(stats.Players))
	}

	results := make(map[string]float64)

	// Challenger is the first player (first --bot-cmd by ConnectOrder)
	challenger := stats.Players[0]
	results["challenger_hands"] = float64(challenger.Hands)
	if challenger.DetailedStats != nil {
		results["challenger_bb_per_100"] = challenger.DetailedStats.BB100
		results["challenger_vpip"] = challenger.DetailedStats.VPIP
		results["challenger_pfr"] = challenger.DetailedStats.PFR
		if challenger.Hands > 0 {
			results["challenger_timeouts"] = float64(challenger.DetailedStats.Timeouts) / float64(challenger.Hands)
			results["challenger_busts"] = float64(challenger.DetailedStats.Busts) / float64(challenger.Hands)
		}
		results["challenger_std_dev"] = challenger.DetailedStats.StdDev
	} else if challenger.Hands > 0 && stats.BigBlind > 0 {
		// Calculate BB/100 from basic stats if detailed stats not available
		results["challenger_bb_per_100"] = (float64(challenger.NetChips) / float64(stats.BigBlind)) / float64(challenger.Hands) * 100
	}

	// Baseline is the second player (second --bot-cmd)
	baseline := stats.Players[1]
	results["baseline_hands"] = float64(baseline.Hands)
	if baseline.DetailedStats != nil {
		results["baseline_bb_per_100"] = baseline.DetailedStats.BB100
		results["baseline_vpip"] = baseline.DetailedStats.VPIP
		results["baseline_pfr"] = baseline.DetailedStats.PFR
		if baseline.Hands > 0 {
			results["baseline_timeouts"] = float64(baseline.DetailedStats.Timeouts) / float64(baseline.Hands)
			results["baseline_busts"] = float64(baseline.DetailedStats.Busts) / float64(baseline.Hands)
		}
		results["baseline_std_dev"] = baseline.DetailedStats.StdDev
	} else if baseline.Hands > 0 && stats.BigBlind > 0 {
		// Calculate BB/100 from basic stats if detailed stats not available
		results["baseline_bb_per_100"] = (float64(baseline.NetChips) / float64(stats.BigBlind)) / float64(baseline.Hands) * 100
	}

	return results, nil
}

// AggregatePopulationStats aggregates stats for population mode
func AggregatePopulationStats(stats *server.GameStats, challengerSeats, baselineSeats int) map[string]float64 {
	results := make(map[string]float64)

	// Aggregate stats for challenger bots (first N seats)
	var challengerNetChips int64
	var challengerHands int
	var challengerVPIPWeighted, challengerPFRWeighted float64
	var challengerTimeouts, challengerBusts int
	var challengerStdDevs []float64
	var challengerStdWeights []float64

	// Aggregate stats for baseline bots (next M seats)
	var baselineNetChips int64
	var baselineHands int
	var baselineVPIPWeighted, baselinePFRWeighted float64
	var baselineTimeouts, baselineBusts int
	var baselineStdDevs []float64
	var baselineStdWeights []float64

	for i, player := range stats.Players {
		if i < challengerSeats {
			// Challenger bot
			challengerNetChips += player.NetChips
			challengerHands += player.Hands
			if player.DetailedStats != nil {
				hands := float64(player.Hands)
				challengerVPIPWeighted += player.DetailedStats.VPIP * hands
				challengerPFRWeighted += player.DetailedStats.PFR * hands
				challengerTimeouts += player.DetailedStats.Timeouts
				challengerBusts += player.DetailedStats.Busts
				if player.Hands > 1 && player.DetailedStats.StdDev > 0 {
					challengerStdDevs = append(challengerStdDevs, player.DetailedStats.StdDev)
					challengerStdWeights = append(challengerStdWeights, hands)
				}
			}
		} else if i < challengerSeats+baselineSeats {
			// Baseline bot
			baselineNetChips += player.NetChips
			baselineHands += player.Hands
			if player.DetailedStats != nil {
				hands := float64(player.Hands)
				baselineVPIPWeighted += player.DetailedStats.VPIP * hands
				baselinePFRWeighted += player.DetailedStats.PFR * hands
				baselineTimeouts += player.DetailedStats.Timeouts
				baselineBusts += player.DetailedStats.Busts
				if player.Hands > 1 && player.DetailedStats.StdDev > 0 {
					baselineStdDevs = append(baselineStdDevs, player.DetailedStats.StdDev)
					baselineStdWeights = append(baselineStdWeights, hands)
				}
			}
		}
	}

	// Calculate aggregate BB/100
	bigBlind := float64(stats.BigBlind)
	if challengerHands > 0 && bigBlind > 0 {
		results["challenger_bb_per_100"] = (float64(challengerNetChips) / bigBlind) / float64(challengerHands) * 100
	}
	if baselineHands > 0 && bigBlind > 0 {
		results["baseline_bb_per_100"] = (float64(baselineNetChips) / bigBlind) / float64(baselineHands) * 100
	}

	// Average the strategy metrics
	if challengerHands > 0 {
		results["challenger_vpip"] = challengerVPIPWeighted / float64(challengerHands)
		results["challenger_pfr"] = challengerPFRWeighted / float64(challengerHands)
		results["challenger_timeouts"] = float64(challengerTimeouts) / float64(challengerHands)
		results["challenger_busts"] = float64(challengerBusts) / float64(challengerHands)
	}
	if baselineHands > 0 {
		results["baseline_vpip"] = baselineVPIPWeighted / float64(baselineHands)
		results["baseline_pfr"] = baselinePFRWeighted / float64(baselineHands)
		results["baseline_timeouts"] = float64(baselineTimeouts) / float64(baselineHands)
		results["baseline_busts"] = float64(baselineBusts) / float64(baselineHands)
	}

	if len(challengerStdDevs) > 0 {
		results["challenger_std_dev"] = calculatePooledStdDevWeighted(challengerStdDevs, challengerStdWeights)
	}
	if len(baselineStdDevs) > 0 {
		results["baseline_std_dev"] = calculatePooledStdDevWeighted(baselineStdDevs, baselineStdWeights)
	}

	// Store hands for weighting in batch aggregation
	results["challenger_hands"] = float64(challengerHands)
	results["baseline_hands"] = float64(baselineHands)

	return results
}

// AggregateNPCStats aggregates stats for NPC benchmark mode
func AggregateNPCStats(stats *server.GameStats, isChallenger bool) map[string]float64 {
	results := make(map[string]float64)

	// Determine the prefix to use based on whether this is a challenger or baseline run
	prefix := "challenger"
	if !isChallenger {
		prefix = "baseline"
	}

	// Aggregate all non-NPC bot stats
	var totalNetChips int64
	var totalHands int
	var totalVPIPWeighted, totalPFRWeighted float64
	var totalTimeouts, totalBusts int
	var stdDevs []float64
	var stdWeights []float64

	// Helper function to check if a bot is an NPC based on its display name
	isNPCBot := func(name string) bool {
		// NPCs use specific prefixes - calling, aggressive, random
		// Check for exact NPC bot patterns (not just any bot with those words)
		return (strings.HasPrefix(name, "calling-bot-") ||
			strings.HasPrefix(name, "aggressive-bot-") ||
			strings.HasPrefix(name, "random-bot-") ||
			strings.HasPrefix(name, "npc-"))
	}

	for _, player := range stats.Players {
		// Skip NPC bots
		if isNPCBot(player.DisplayName) {
			continue
		}
		// This is one of our test bot instances (complex-* or whatever the test bot uses)
		totalNetChips += player.NetChips
		totalHands += player.Hands

		// Aggregate detailed stats if available
		if player.DetailedStats != nil {
			hands := float64(player.Hands)
			totalVPIPWeighted += player.DetailedStats.VPIP * hands
			totalPFRWeighted += player.DetailedStats.PFR * hands
			totalTimeouts += player.DetailedStats.Timeouts
			totalBusts += player.DetailedStats.Busts
			if player.Hands > 1 && player.DetailedStats.StdDev > 0 {
				stdDevs = append(stdDevs, player.DetailedStats.StdDev)
				stdWeights = append(stdWeights, hands)
			}
		}
	}

	// Calculate aggregate BB/100
	bigBlind := float64(stats.BigBlind)
	if totalHands > 0 && bigBlind > 0 {
		results[prefix+"_bb_per_100"] = (float64(totalNetChips) / bigBlind) / float64(totalHands) * 100
	}

	// Average the strategy metrics
	if totalHands > 0 {
		results[prefix+"_vpip"] = totalVPIPWeighted / float64(totalHands)
		results[prefix+"_pfr"] = totalPFRWeighted / float64(totalHands)
		results[prefix+"_timeouts"] = float64(totalTimeouts) / float64(totalHands)
		results[prefix+"_busts"] = float64(totalBusts) / float64(totalHands)
	}

	// Store hands for weighting
	results[prefix+"_hands"] = float64(totalHands)

	if len(stdDevs) > 0 {
		results[prefix+"_std_dev"] = calculatePooledStdDevWeighted(stdDevs, stdWeights)
	}

	return results
}

// AggregateSelfPlayStats aggregates stats for self-play mode
func AggregateSelfPlayStats(stats *server.GameStats) map[string]float64 {
	results := make(map[string]float64)

	// Calculate stats across all bot instances
	var totalChips int64
	var totalHands int
	var sumVPIP, sumPFR float64
	var maxBB100, minBB100 float64
	first := true

	bigBlind := float64(stats.BigBlind)

	for _, player := range stats.Players {
		if player.Hands > 0 {
			// Calculate BB/100 for this player
			var bb100 float64
			if player.DetailedStats != nil {
				bb100 = player.DetailedStats.BB100
			} else if bigBlind > 0 {
				bb100 = (float64(player.NetChips) / bigBlind) / float64(player.Hands) * 100
			}

			totalChips += player.NetChips
			totalHands += player.Hands

			// Extract VPIP/PFR from detailed stats if available
			if player.DetailedStats != nil {
				sumVPIP += player.DetailedStats.VPIP * float64(player.Hands)
				sumPFR += player.DetailedStats.PFR * float64(player.Hands)
			}

			if first {
				maxBB100 = bb100
				minBB100 = bb100
				first = false
			} else {
				if bb100 > maxBB100 {
					maxBB100 = bb100
				}
				if bb100 < minBB100 {
					minBB100 = bb100
				}
			}
		}
	}

	// Calculate averages
	if totalHands > 0 && bigBlind > 0 {
		results["avg_bb_per_100"] = (float64(totalChips) / bigBlind) / float64(totalHands) * 100
		results["avg_vpip"] = sumVPIP / float64(totalHands)
		results["avg_pfr"] = sumPFR / float64(totalHands)
	}

	results["max_bb_per_100"] = maxBB100
	results["min_bb_per_100"] = minBB100
	results["actual_hands"] = float64(stats.HandsCompleted)
	results["total_hands"] = float64(totalHands)

	return results
}

// WeightedBB100 calculates weighted average BB/100 from batch results
func WeightedBB100(batches []BatchResult, metricKey string, handsKey string) (bb100 float64, totalHands int) {
	var totalChips float64

	for _, batch := range batches {
		// Get BB/100 for this batch and convert to total chips
		if bb100Val, exists := batch.Results[metricKey]; exists {
			actualHands := ExtractActualHands(batch, handsKey)
			totalChips += (bb100Val / 100.0) * float64(actualHands)
			totalHands += actualHands
		}
	}

	// Calculate weighted average BB/100
	if totalHands > 0 {
		bb100 = (totalChips / float64(totalHands)) * 100.0
	}

	return bb100, totalHands
}

// WeightedAverage calculates weighted average for any metric across batches
func WeightedAverage(batches []BatchResult, metricKey string, handsKey string) float64 {
	var totalValue float64
	var totalHands int

	for _, batch := range batches {
		// Get actual hands for this batch
		actualHands := batch.Hands
		if handsFromStats, exists := batch.Results[handsKey]; exists {
			actualHands = int(handsFromStats)
		}

		// Get metric value and weight by hands
		if value, exists := batch.Results[metricKey]; exists {
			totalValue += value * float64(actualHands)
			totalHands += actualHands
		}
	}

	if totalHands > 0 {
		return totalValue / float64(totalHands)
	}

	return 0.0
}

// ExtractActualHands gets the actual hands played from batch results
func ExtractActualHands(batch BatchResult, handsKey string) int {
	if handsFromStats, exists := batch.Results[handsKey]; exists {
		return int(handsFromStats)
	}
	return batch.Hands
}

// CombinedStats holds aggregated statistics from multiple batches
type CombinedStats struct {
	BB100      float64
	VPIP       float64
	PFR        float64
	TotalHands int
	Timeouts   float64
	Busts      float64
}

// CombineBatches aggregates batch results for a specific bot type
func CombineBatches(batches []BatchResult, prefix string) CombinedStats {
	var result CombinedStats

	// Keys for this bot type
	bb100Key := prefix + "_bb_per_100"
	vpipKey := prefix + "_vpip"
	pfrKey := prefix + "_pfr"
	handsKey := prefix + "_hands"
	timeoutsKey := prefix + "_timeouts"
	bustsKey := prefix + "_busts"

	// Calculate weighted BB/100
	result.BB100, result.TotalHands = WeightedBB100(batches, bb100Key, handsKey)

	// Calculate weighted averages for other metrics
	result.VPIP = WeightedAverage(batches, vpipKey, handsKey)
	result.PFR = WeightedAverage(batches, pfrKey, handsKey)
	result.Timeouts = WeightedAverage(batches, timeoutsKey, handsKey)
	result.Busts = WeightedAverage(batches, bustsKey, handsKey)

	return result
}

// CalculateTotalHands sums actual hands played across all batches
func CalculateTotalHands(batches []BatchResult, handsKey string) int {
	totalHands := 0
	for _, batch := range batches {
		totalHands += ExtractActualHands(batch, handsKey)
	}
	return totalHands
}

// CalculateConfidenceInterval computes 95% confidence interval from standard error
func CalculateConfidenceInterval(mean, stdDev float64, n int) (float64, float64) {
	if n <= 1 {
		// Return wide interval for small samples
		return mean - 50, mean + 50
	}

	se := stdDev / math.Sqrt(float64(n))
	margin := 1.96 * se // 95% CI
	return mean - margin, mean + margin
}

// CalculatePooledStdDev calculates pooled standard deviation from batch results
func CalculatePooledStdDev(batches []BatchResult, metricKey, stdDevKey, handsKey string) float64 {
	var sumSquaredDeviations float64
	var totalHands int
	var overallMean float64

	// First pass: calculate overall mean
	var totalWeightedValue float64
	for _, batch := range batches {
		actualHands := ExtractActualHands(batch, handsKey)
		if value, exists := batch.Results[metricKey]; exists {
			totalWeightedValue += value * float64(actualHands)
			totalHands += actualHands
		}
	}

	if totalHands > 0 {
		overallMean = totalWeightedValue / float64(totalHands)
	}

	// Second pass: calculate pooled variance
	var totalDf int // degrees of freedom
	for _, batch := range batches {
		actualHands := ExtractActualHands(batch, handsKey)
		if actualHands <= 1 {
			continue // Skip batches with insufficient data
		}

		// Use batch standard deviation if available
		if batchStdDev, exists := batch.StdDevs[stdDevKey]; exists && batchStdDev > 0 {
			// Add this batch's contribution to pooled variance
			batchVariance := batchStdDev * batchStdDev
			sumSquaredDeviations += batchVariance * float64(actualHands-1)
			totalDf += actualHands - 1
		} else if batchMean, exists := batch.Results[metricKey]; exists {
			// Fallback: estimate variance from deviation from overall mean
			deviation := batchMean - overallMean
			sumSquaredDeviations += deviation * deviation * float64(actualHands)
			totalDf += actualHands - 1
		}
	}

	if totalDf > 0 {
		pooledVariance := sumSquaredDeviations / float64(totalDf)
		return math.Sqrt(pooledVariance)
	}

	// Fallback: use typical poker variance (approximately 100 BB/100)
	return 100.0
}
