package evaluator

import "github.com/lox/holdem-cli/internal/deck"

// ScoreToHandName converts an integer score to a readable hand name
func ScoreToHandName(score int) string {
	handType := score >> 20
	switch handType {
	case RoyalFlushType:
		return "Royal Flush"
	case StraightFlushType:
		return "Straight Flush"
	case FourOfAKindType:
		return "Four of a Kind"
	case FullHouseType:
		return "Full House"
	case FlushType:
		return "Flush"
	case StraightType:
		return "Straight"
	case ThreeOfAKindType:
		return "Three of a Kind"
	case TwoPairType:
		return "Two Pair"
	case OnePairType:
		return "One Pair"
	case HighCardType:
		return "High Card"
	default:
		return "Unknown"
	}
}

// GetHandType extracts the hand type from a score
func GetHandType(score int) int {
	return score >> 20
}

// GetPairRank extracts the pair rank from a one pair hand score
func GetPairRank(score int) int {
	if GetHandType(score) != OnePairType {
		return 0
	}
	tiebreakData := score & 0xFFFFF
	encodedPairRank := (tiebreakData >> 12) & 0xF
	return 15 - encodedPairRank
}

// GetHighCardRank extracts the highest card rank from a high card hand score
func GetHighCardRank(score int) int {
	if GetHandType(score) != HighCardType {
		return 0
	}
	tiebreakData := score & 0xFFFFF
	encodedHighRank := tiebreakData & 0xF
	return 15 - encodedHighRank
}

// Hand type constants (lower number = stronger hand)
const (
	RoyalFlushType    = 1
	StraightFlushType = 2
	FourOfAKindType   = 3
	FullHouseType     = 4
	FlushType         = 5
	StraightType      = 6
	ThreeOfAKindType  = 7
	TwoPairType       = 8
	OnePairType       = 9
	HighCardType      = 10
)

// Evaluate7 evaluates 7 cards and returns an integer score where lower = stronger
// Encoding: (handType << 20) | tiebreaker_info
func Evaluate7(cards []deck.Card) int {
	if len(cards) != 7 {
		panic("Evaluate7 requires exactly 7 cards")
	}

	// Preprocessing: count ranks, suits, and build rank bitmap
	var rankCounts [15]int // index 0 unused, 2-14 for card ranks
	var suitCounts [4]int
	var rankBits uint32

	for _, card := range cards {
		rankCounts[card.Rank]++
		suitCounts[card.Suit]++
		rankBits |= 1 << uint(card.Rank)
	}

	// Check for flush
	flushSuit := -1
	for suit := 0; suit < 4; suit++ {
		if suitCounts[suit] >= 5 {
			flushSuit = suit
			break
		}
	}

	// If flush exists, check for straight flush
	if flushSuit != -1 {
		var flushRankBits uint32
		var flushCards []int

		// Collect all cards of flush suit and build flush rank bitmap
		for _, card := range cards {
			if card.Suit == flushSuit {
				flushRankBits |= 1 << uint(card.Rank)
				flushCards = append(flushCards, card.Rank)
			}
		}

		// Check for straight flush
		straightHigh := findStraightInBitmap(flushRankBits)
		if straightHigh > 0 {
			// Royal flush: A-K-Q-J-10
			if straightHigh == 14 && (flushRankBits&(1<<13)) != 0 {
				return (RoyalFlushType << 20) | 14
			}
			// Straight flush
			return (StraightFlushType << 20) | straightHigh
		}

		// Regular flush - use 5 highest cards
		flushRanks := getHighestRanks(flushCards, 5)
		return (FlushType << 20) | encodeMultipleRanks(flushRanks)
	}

	// Find groups (4-of-a-kind, 3-of-a-kind, pairs)
	var fours, threes, pairs []int

	for rank := 14; rank >= 2; rank-- {
		count := rankCounts[rank]
		if count == 4 {
			fours = append(fours, rank)
		} else if count == 3 {
			threes = append(threes, rank)
		} else if count == 2 {
			pairs = append(pairs, rank)
		}
	}

	// Four of a kind
	if len(fours) > 0 {
		kicker := findHighestKicker(rankCounts, fours[0])
		return (FourOfAKindType << 20) | (fours[0] << 4) | kicker
	}

	// Full house
	if len(threes) > 0 && (len(pairs) > 0 || len(threes) > 1) {
		threeRank := threes[0]
		var pairRank int
		if len(threes) > 1 {
			pairRank = threes[1] // Two three-of-a-kinds, use lower as pair
		} else {
			pairRank = pairs[0]
		}
		return (FullHouseType << 20) | (threeRank << 4) | pairRank
	}

	// Check for straight
	straightHigh := findStraightInBitmap(rankBits)
	if straightHigh > 0 {
		return (StraightType << 20) | straightHigh
	}

	// Three of a kind
	if len(threes) > 0 {
		kickers := findHighestKickers(rankCounts, threes[0], 2)
		return (ThreeOfAKindType << 20) | (threes[0] << 8) | (kickers[0] << 4) | kickers[1]
	}

	// Two pair
	if len(pairs) >= 2 {
		kicker := findHighestKicker(rankCounts, pairs[0], pairs[1])
		// Use reverse encoding so higher pairs have lower scores
		return (TwoPairType << 20) | ((15-pairs[0]) << 8) | ((15-pairs[1]) << 4) | (15-kicker)
	}

	// One pair
	if len(pairs) == 1 {
		kickers := findHighestKickers(rankCounts, pairs[0], 3)
		// Use reverse encoding for pair rank so higher pairs have lower scores
		return (OnePairType << 20) | ((15-pairs[0]) << 12) | ((15-kickers[0]) << 8) | ((15-kickers[1]) << 4) | (15-kickers[2])
	}

	// High card
	highCards := findHighestKickers(rankCounts, 0, 5)
	return (HighCardType << 20) | encodeMultipleRanksReverse(highCards)
}

// findStraightInBitmap checks for 5 consecutive bits in rank bitmap
// Returns the high card of the straight, or 0 if no straight
func findStraightInBitmap(rankBits uint32) int {
	// Check for wheel straight (A-2-3-4-5)
	wheel := uint32(1<<14 | 1<<5 | 1<<4 | 1<<3 | 1<<2)
	if (rankBits & wheel) == wheel {
		return 5 // In wheel, 5 is high card
	}

	// Check for other straights (need 5 consecutive bits)
	for high := 14; high >= 6; high-- {
		mask := uint32(0x1F) << uint(high-4) // 5 consecutive bits
		if (rankBits & mask) == mask {
			return high
		}
	}

	return 0
}

// findHighestKicker finds the highest single card excluding given ranks
func findHighestKicker(rankCounts [15]int, excludeRanks ...int) int {
	excluded := make(map[int]bool)
	for _, rank := range excludeRanks {
		excluded[rank] = true
	}

	for rank := 14; rank >= 2; rank-- {
		if rankCounts[rank] == 1 && !excluded[rank] {
			return rank
		}
	}
	return 0
}

// findHighestKickers finds n highest single cards excluding given rank
func findHighestKickers(rankCounts [15]int, excludeRank int, n int) []int {
	var kickers []int
	for rank := 14; rank >= 2 && len(kickers) < n; rank-- {
		if rankCounts[rank] == 1 && rank != excludeRank {
			kickers = append(kickers, rank)
		}
	}
	return kickers
}

// getHighestRanks returns n highest ranks from a slice
func getHighestRanks(ranks []int, n int) []int {
	// Sort in descending order
	sorted := make([]int, len(ranks))
	copy(sorted, ranks)

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] > sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	if len(sorted) > n {
		sorted = sorted[:n]
	}
	return sorted
}

// encodeMultipleRanks encodes up to 5 ranks into a single integer
func encodeMultipleRanks(ranks []int) int {
	result := 0
	for i, rank := range ranks {
		if i >= 5 {
			break
		}
		result |= rank << uint(4*i)
	}
	return result
}

// encodeMultipleRanksReverse encodes ranks where higher ranks give lower scores
func encodeMultipleRanksReverse(ranks []int) int {
	result := 0
	for i, rank := range ranks {
		if i >= 5 {
			break
		}
		// Subtract from 15 to make higher ranks give lower values
		result |= (15 - rank) << uint(4*i)
	}
	return result
}
