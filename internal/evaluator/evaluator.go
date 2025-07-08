package evaluator

// Simple 7-card hand evaluator - ported from Zig implementation
// Lower rank values are better hands (0 = Royal Flush)

import (
	"math/bits"

	"github.com/lox/pokerforbots/sdk/deck"
)

// HandRank represents the strength of a poker hand (lower is better)
type HandRank uint16

// String returns a description of the hand rank
func (h HandRank) String() string {
	e := NewEvaluator()
	return e.GetHandClass(int(h))
}

// Compare compares two HandRank values (returns 1 if h is better, -1 if other is better, 0 if equal)
func (h HandRank) Compare(other HandRank) int {
	if h < other {
		return 1 // h is better (lower rank wins)
	} else if h > other {
		return -1 // other is better
	}
	return 0 // equal
}

// Hand ranking constants
const (
	HandCategoryHighCard = iota
	HandCategoryPair
	HandCategoryTwoPair
	HandCategoryThreeOfAKind
	HandCategoryStraight
	HandCategoryFlush
	HandCategoryFullHouse
	HandCategoryFourOfAKind
	HandCategoryStraightFlush
)

// Card suits
const (
	Clubs = iota
	Diamonds
	Hearts
	Spades
)

// Card represents a playing card
// Encoded as uint64 with bit position = suit * 13 + rank
// Rank: 0=2, 1=3, ..., 11=K, 12=A
type Card uint64

// Hand is a bitfield representing up to 7 cards
type Hand uint64

// MakeCard creates a card from suit (0-3) and rank (0-12)
func MakeCard(suit, rank int) Card {
	return Card(1) << (suit*13 + rank)
}

// Evaluator provides poker hand evaluation
type Evaluator struct{}

// NewEvaluator creates a new evaluator
func NewEvaluator() *Evaluator {
	return &Evaluator{}
}

// getRankMask returns a bitmask of all ranks present in the hand
func getRankMask(hand Hand) uint16 {
	clubs := getSuitMask(hand, Clubs)
	diamonds := getSuitMask(hand, Diamonds)
	hearts := getSuitMask(hand, Hearts)
	spades := getSuitMask(hand, Spades)
	return clubs | diamonds | hearts | spades
}

// getSuitMask returns a bitmask of ranks for a specific suit
func getSuitMask(hand Hand, suit int) uint16 {
	suitBits := uint64(hand) >> (suit * 13)
	return uint16(suitBits & 0x1FFF) // 13 bits for ranks
}

// getSuitMasks returns rank masks for all 4 suits
func getSuitMasks(hand Hand) [4]uint16 {
	return [4]uint16{
		getSuitMask(hand, Clubs),
		getSuitMask(hand, Diamonds),
		getSuitMask(hand, Hearts),
		getSuitMask(hand, Spades),
	}
}

// hasFlush checks if the hand contains a flush
func hasFlush(hand Hand) bool {
	suits := getSuitMasks(hand)
	for _, suit := range suits {
		if bits.OnesCount16(suit) >= 5 {
			return true
		}
	}
	return false
}

// getHighestRanks returns the highest N ranks from a rank mask
func getHighestRanks(ranks uint16, count int) uint16 {
	result := uint16(0)
	remaining := count

	// Start from Ace (bit 12) and work down
	for bit := 12; bit >= 0 && remaining > 0; bit-- {
		if (ranks & (1 << bit)) != 0 {
			result |= 1 << bit
			remaining--
		}
	}

	return result
}

// getStraightMask checks for a straight in the rank mask
func getStraightMask(ranks uint16) uint16 {
	// Check for regular straights from highest to lowest
	// A-K-Q-J-T = 0x1F00, K-Q-J-T-9 = 0x0F80, etc.
	straightMask := uint16(0x1F00) // Start with A-K-Q-J-T

	for i := 0; i <= 8; i++ {
		if (ranks & straightMask) == straightMask {
			return straightMask
		}
		straightMask >>= 1
	}

	// Check for wheel (A-2-3-4-5)
	if (ranks & 0x100F) == 0x100F { // A,2,3,4,5
		return 0x100F
	}

	return 0
}

// EvaluateHand evaluates a 7-card poker hand
func (e *Evaluator) EvaluateHand(hand Hand) HandRank {
	ranks := getRankMask(hand)
	suits := getSuitMasks(hand)

	// Check for flush
	var flushSuit = -1
	var flushRanks uint16 = 0
	for i, suit := range suits {
		if bits.OnesCount16(suit) >= 5 {
			flushSuit = i
			flushRanks = getHighestRanks(suit, 5)
			break
		}
	}

	// Check for straight
	straightMask := getStraightMask(ranks)
	hasStraight := straightMask != 0

	// Straight flush
	if flushSuit != -1 && hasStraight {
		straightInFlush := getStraightMask(suits[flushSuit])
		if straightInFlush != 0 {
			// Royal flush (AKQJT of same suit)
			if straightInFlush == 0x1F00 { // 10,J,Q,K,A
				return 0 // Royal flush (rank 0 = best possible hand)
			}
			// Wheel straight flush (A-2-3-4-5) - 5-high
			if straightInFlush == 0x100F { // A,2,3,4,5 wheel
				return 9 // Worst straight flush
			}
			// Other straight flushes: K-high=1, Q-high=2, ..., 6-high=8
			highCardBit := bits.LeadingZeros16(straightInFlush)
			highCardRank := 15 - highCardBit
			return HandRank(12 - highCardRank)
		}
	}

	// Count rank frequencies
	rankCounts := [13]int{}
	for i := 0; i < 13; i++ {
		if (ranks & (1 << i)) != 0 {
			// Count how many cards of this rank
			count := 0
			for _, suit := range suits {
				if (suit & (1 << i)) != 0 {
					count++
				}
			}
			rankCounts[i] = count
		}
	}

	// Find pairs, trips, quads
	quads := 0
	trips := 0
	pairs := 0

	for _, count := range rankCounts {
		switch count {
		case 4:
			quads++
		case 3:
			trips++
		case 2:
			pairs++
		}
	}

	// Four of a kind (ranks 10-165)
	if quads > 0 {
		quadRank := 0
		kickerRank := 0

		for rank, count := range rankCounts {
			if count == 4 {
				quadRank = rank
			} else if count >= 1 && rank != quadRank && rank > kickerRank {
				kickerRank = rank
			}
		}

		return HandRank(10 + (12-quadRank)*12 + (12 - kickerRank))
	}

	// Full house (ranks 166-321)
	if trips > 0 && (pairs > 0 || trips > 1) {
		tripRank := 0
		pairRank := 0
		secondTripRank := 0

		// Find the highest trip
		for rank, count := range rankCounts {
			if count == 3 && rank > tripRank {
				secondTripRank = tripRank
				tripRank = rank
			} else if count == 3 && rank > secondTripRank {
				secondTripRank = rank
			} else if count == 2 && rank > pairRank {
				pairRank = rank
			}
		}

		// If we have two trips, use the lower trip as the pair
		if trips > 1 {
			pairRank = secondTripRank
		}

		return HandRank(166 + (12-tripRank)*12 + (12 - pairRank))
	}

	// Flush (not straight) (ranks 322-1598)
	if flushSuit != -1 {
		highCardBit := bits.LeadingZeros16(flushRanks)
		highCardRank := 15 - highCardBit
		return HandRank(322 + (12-highCardRank)*100)
	}

	// Straight (not flush) (ranks 1599-1608)
	if hasStraight {
		if straightMask == 0x100F { // A-2-3-4-5 wheel (5-high)
			return 1608 // Worst straight
		}
		highCardBit := bits.LeadingZeros16(straightMask)
		highCardRank := 15 - highCardBit
		return HandRank(1599 + (12 - highCardRank))
	}

	// Three of a kind (ranks 1609-2466)
	if trips > 0 {
		tripRank := 0
		for rank, count := range rankCounts {
			if count == 3 {
				tripRank = rank
				break
			}
		}
		return HandRank(1609 + (12-tripRank)*65)
	}

	// Two pair (ranks 2467-3324)
	if pairs >= 2 {
		highPair := 0
		lowPair := 0

		for rank, count := range rankCounts {
			if count == 2 {
				if rank > highPair {
					lowPair = highPair
					highPair = rank
				} else if rank > lowPair {
					lowPair = rank
				}
			}
		}

		return HandRank(2467 + (12-highPair)*65 + (12 - lowPair))
	}

	// One pair (ranks 3325-6184)
	if pairs == 1 {
		pairRank := 0
		for rank, count := range rankCounts {
			if count == 2 {
				pairRank = rank
				break
			}
		}
		return HandRank(3325 + (12-pairRank)*220)
	}

	// High card (ranks 6185-7461)
	highCardBit := bits.LeadingZeros16(ranks)
	highCardRank := 15 - highCardBit
	return HandRank(6185 + (12-highCardRank)*100)
}

// GetHandCategory returns the hand category (0-8) for a given hand
func (e *Evaluator) GetHandCategory(hand Hand) int {
	rank := e.EvaluateHand(hand)

	switch {
	case rank == 0:
		return HandCategoryStraightFlush // Royal flush
	case rank <= 9:
		return HandCategoryStraightFlush
	case rank <= 165:
		return HandCategoryFourOfAKind
	case rank <= 321:
		return HandCategoryFullHouse
	case rank <= 1598:
		return HandCategoryFlush
	case rank <= 1608:
		return HandCategoryStraight
	case rank <= 2466:
		return HandCategoryThreeOfAKind
	case rank <= 3324:
		return HandCategoryTwoPair
	case rank <= 6184:
		return HandCategoryPair
	default:
		return HandCategoryHighCard
	}
}

// GetHandClass returns a string description of the hand class
func (e *Evaluator) GetHandClass(rank int) string {
	switch {
	case rank == 0:
		return "Royal Flush"
	case rank <= 9:
		return "Straight Flush"
	case rank <= 165:
		return "Four of a Kind"
	case rank <= 321:
		return "Full House"
	case rank <= 1598:
		return "Flush"
	case rank <= 1608:
		return "Straight"
	case rank <= 2466:
		return "Three of a Kind"
	case rank <= 3324:
		return "Two Pair"
	case rank <= 6184:
		return "One Pair"
	default:
		return "High Card"
	}
}

// EvaluateCards is a convenience method that takes individual cards
func (e *Evaluator) EvaluateCards(cards []Card) int {
	hand := Hand(0)
	for _, card := range cards {
		hand |= Hand(card)
	}
	return int(e.EvaluateHand(hand))
}

// DescribeHand returns a description of the hand
func (e *Evaluator) DescribeHand(cards []Card) string {
	rank := e.EvaluateCards(cards)
	return e.GetHandClass(rank)
}

// GetPercentile returns the percentile rank (0-100) where 100 is best
func (e *Evaluator) GetPercentile(rank int) float64 {
	// Total possible ranks: 0-7461
	// Convert so that lower rank = higher percentile
	return 100.0 * (1.0 - float64(rank)/7461.0)
}

// CalculateEquity is a placeholder for SDK compatibility
func (e *Evaluator) CalculateEquity(hole []Card, board []Card, numOpponents int, iterations int) float64 {
	// Simplified equity calculation based on hand strength
	allCards := append(hole, board...)
	rank := e.EvaluateCards(allCards)
	percentile := e.GetPercentile(rank)

	// Adjust for number of opponents
	baseEquity := percentile / 100.0
	adjustment := 1.0 / float64(numOpponents+1)
	return baseEquity * adjustment
}

// Evaluate7 evaluates a 7-card hand for compatibility with existing code
func Evaluate7(cards interface{}) HandRank {
	e := NewEvaluator()

	// Convert deck.Card slice to evaluator cards
	switch v := cards.(type) {
	case []deck.Card:
		if len(v) != 7 {
			return HandRank(7462) // Return worst possible hand if not 7 cards
		}

		// Convert deck.Card to evaluator Card format
		evalCards := make([]Card, 7)
		for i, deckCard := range v {
			// deck.Card format: Rank is 2-14 (2=Two, 14=Ace), Suit is 0-3 (S,H,D,C)
			// evaluator Card: bit position = suit*13 + rank (where rank is 0-12)
			// Convert rank from 2-14 to 0-12
			rank := deckCard.Rank - 2
			evalCards[i] = Card(1 << (deckCard.Suit*13 + rank))
		}

		// Create hand by ORing all cards
		hand := Hand(0)
		for _, card := range evalCards {
			hand |= Hand(card)
		}

		return e.EvaluateHand(hand)
	default:
		return HandRank(7462) // Return worst possible hand for unknown type
	}
}
