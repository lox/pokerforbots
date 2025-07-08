package sdk

import "github.com/lox/pokerforbots/sdk/deck"

// HandStrength represents the strength of a poker hand
type HandStrength struct {
	Rank        int     `json:"rank"`        // Numeric rank (higher is better)
	Class       string  `json:"class"`       // Hand class (High Card, Pair, etc.)
	Description string  `json:"description"` // Human-readable description
	Percentile  float64 `json:"percentile"`  // Percentile rank (0-100)
}

// Evaluator provides poker hand evaluation utilities for bots
// This is a placeholder implementation - for full functionality,
// bots should use the server's evaluation API or implement their own
type Evaluator struct{}

// NewEvaluator creates a new hand evaluator
func NewEvaluator() *Evaluator {
	return &Evaluator{}
}

// EvaluateHand evaluates the strength of a poker hand
// This is a simplified implementation for demonstration
func (e *Evaluator) EvaluateHand(holeCards []deck.Card, communityCards []deck.Card) HandStrength {
	// Simplified hand evaluation - in a real implementation,
	// this would use sophisticated poker hand evaluation algorithms

	// For now, return a basic assessment
	rank := e.calculateBasicRank(holeCards, communityCards)

	return HandStrength{
		Rank:        rank,
		Class:       e.getHandClass(rank),
		Description: e.describeHand(holeCards, communityCards),
		Percentile:  float64(rank) / 10000.0 * 100,
	}
}

// CalculateEquity calculates win probability against a number of random opponents
// This is a placeholder - real implementation would use Monte Carlo simulation
func (e *Evaluator) CalculateEquity(holeCards []deck.Card, communityCards []deck.Card, numOpponents int, iterations int) float64 {
	// Simplified equity calculation
	// Real implementation would simulate random hands
	strength := e.EvaluateHand(holeCards, communityCards)
	baseEquity := strength.Percentile / 100.0

	// Adjust for number of opponents (more opponents = lower equity)
	adjustment := 1.0 / float64(numOpponents+1)
	return baseEquity * adjustment
}

// GetHandClass returns the class of a hand given its rank
func (e *Evaluator) GetHandClass(rank int) string {
	return e.getHandClass(rank)
}

// IsStrongHand returns true if the hand is considered strong (top 20%)
func (e *Evaluator) IsStrongHand(holeCards []deck.Card, communityCards []deck.Card) bool {
	strength := e.EvaluateHand(holeCards, communityCards)
	return strength.Percentile >= 80.0
}

// IsWeakHand returns true if the hand is considered weak (bottom 40%)
func (e *Evaluator) IsWeakHand(holeCards []deck.Card, communityCards []deck.Card) bool {
	strength := e.EvaluateHand(holeCards, communityCards)
	return strength.Percentile <= 40.0
}

// IsPremiumPreflop returns true if the hole cards are premium preflop hands
func (e *Evaluator) IsPremiumPreflop(holeCards []deck.Card) bool {
	if len(holeCards) != 2 {
		return false
	}

	card1, card2 := holeCards[0], holeCards[1]

	// Premium pairs: AA, KK, QQ, JJ
	if card1.Rank == card2.Rank {
		return card1.Rank == deck.Ace || card1.Rank == deck.King ||
			card1.Rank == deck.Queen || card1.Rank == deck.Jack
	}

	// Premium suited: AKs, AQs, AJs
	if card1.Suit == card2.Suit {
		if (card1.Rank == deck.Ace && (card2.Rank == deck.King || card2.Rank == deck.Queen || card2.Rank == deck.Jack)) ||
			(card2.Rank == deck.Ace && (card1.Rank == deck.King || card1.Rank == deck.Queen || card1.Rank == deck.Jack)) {
			return true
		}
	}

	// Premium offsuit: AK
	if (card1.Rank == deck.Ace && card2.Rank == deck.King) ||
		(card2.Rank == deck.Ace && card1.Rank == deck.King) {
		return true
	}

	return false
}

// IsPlayablePreflop returns true if the hole cards are worth playing preflop
func (e *Evaluator) IsPlayablePreflop(holeCards []deck.Card) bool {
	if len(holeCards) != 2 {
		return false
	}

	// Premium hands are always playable
	if e.IsPremiumPreflop(holeCards) {
		return true
	}

	card1, card2 := holeCards[0], holeCards[1]

	// Any pocket pair is playable
	if card1.Rank == card2.Rank {
		return true
	}

	// Suited connectors and one-gappers
	if card1.Suit == card2.Suit {
		rank1 := card1.Rank
		rank2 := card2.Rank
		gap := abs(rank1 - rank2)

		// Suited connectors or one-gap suited
		if gap <= 2 && (rank1 >= 7 || rank2 >= 7) { // 7+ high
			return true
		}
	}

	// Broadway cards (T, J, Q, K, A)
	if isBroadway(card1.Rank) && isBroadway(card2.Rank) {
		return true
	}

	return false
}

// Internal helper methods

func (e *Evaluator) calculateBasicRank(holeCards []deck.Card, communityCards []deck.Card) int {
	// Very simplified ranking system
	// Real implementation would use proper poker hand evaluation

	allCards := append(holeCards, communityCards...)

	// Count pairs, check for flush, etc.
	rankCounts := make(map[int]int)
	suitCounts := make(map[int]int)

	for _, card := range allCards {
		rankCounts[card.Rank]++
		suitCounts[card.Suit]++
	}

	// Check for pairs, trips, quads
	maxRankCount := 0
	for _, count := range rankCounts {
		if count > maxRankCount {
			maxRankCount = count
		}
	}

	// Check for flush
	hasFlush := false
	for _, count := range suitCounts {
		if count >= 5 {
			hasFlush = true
			break
		}
	}

	// Simple ranking (lower is better)
	switch {
	case maxRankCount == 4:
		return 1000 // Four of a kind
	case hasFlush:
		return 2000 // Flush
	case maxRankCount == 3:
		return 3000 // Three of a kind
	case maxRankCount == 2:
		return 5000 // Pair
	default:
		return 8000 // High card
	}
}

func (e *Evaluator) getHandClass(rank int) string {
	switch {
	case rank < 2000:
		return "Four of a Kind"
	case rank < 3000:
		return "Flush"
	case rank < 4000:
		return "Three of a Kind"
	case rank < 6000:
		return "Pair"
	default:
		return "High Card"
	}
}

func (e *Evaluator) describeHand(holeCards []deck.Card, communityCards []deck.Card) string {
	strength := e.calculateBasicRank(holeCards, communityCards)
	return e.getHandClass(strength)
}

// Helper functions

func isBroadway(rank int) bool {
	return rank == deck.Ten || rank == deck.Jack || rank == deck.Queen ||
		rank == deck.King || rank == deck.Ace
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
