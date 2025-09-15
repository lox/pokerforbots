package game

import (
	"math/bits"
)

// HandRank represents the strength of a poker hand
type HandRank uint32

// The high 4 bits are the hand type, the remaining bits are for tie-breaking
const (
	HighCard HandRank = iota << 28
	Pair
	TwoPair
	ThreeOfAKind
	Straight
	Flush
	FullHouse
	FourOfAKind
	StraightFlush
)

// HandType returns the type of hand (pair, flush, etc.)
func (hr HandRank) Type() HandRank {
	return hr & 0xF0000000
}

// String returns a human-readable hand description
func (hr HandRank) String() string {
	switch hr.Type() {
	case HighCard:
		return "High Card"
	case Pair:
		return "Pair"
	case TwoPair:
		return "Two Pair"
	case ThreeOfAKind:
		return "Three of a Kind"
	case Straight:
		return "Straight"
	case Flush:
		return "Flush"
	case FullHouse:
		return "Full House"
	case FourOfAKind:
		return "Four of a Kind"
	case StraightFlush:
		return "Straight Flush"
	default:
		return "Unknown"
	}
}

// Evaluate7Cards evaluates the best 5-card hand from 7 cards
func Evaluate7Cards(hand Hand) HandRank {
	if hand.CountCards() != 7 {
		return 0
	}

	// Check for flush first (most restrictive)
	flushSuit := checkFlush(hand)
	if flushSuit >= 0 {
		// Check for straight flush
		flushCards := getFlushCards(hand, uint8(flushSuit))
		if straightRank := checkStraight(flushCards); straightRank > 0 {
			return StraightFlush | HandRank(straightRank)
		}
		// Regular flush - use top 5 cards
		return Flush | HandRank(getTopCards(flushCards, 5))
	}

	// Count ranks for pairs, trips, etc.
	rankCounts := countRanks(hand)

	// Check for four of a kind
	if quad := findNOfAKind(rankCounts, 4); quad >= 0 {
		kicker := findKicker(rankCounts, []uint8{uint8(quad)})
		return FourOfAKind | (HandRank(quad) << 4) | HandRank(kicker)
	}

	// Check for full house
	trips := findNOfAKind(rankCounts, 3)
	if trips >= 0 {
		pair := findNOfAKindExcept(rankCounts, 2, uint8(trips))
		if pair >= 0 {
			return FullHouse | (HandRank(trips) << 4) | HandRank(pair)
		}
	}

	// Check for straight
	if straightRank := checkStraight(hand); straightRank > 0 {
		return Straight | HandRank(straightRank)
	}

	// Check for three of a kind
	if trips >= 0 {
		kickers := findKickers(rankCounts, []uint8{uint8(trips)}, 2)
		return ThreeOfAKind | (HandRank(trips) << 8) | HandRank(kickers)
	}

	// Check for two pair
	pair1 := findNOfAKind(rankCounts, 2)
	if pair1 >= 0 {
		pair2 := findNOfAKindExcept(rankCounts, 2, uint8(pair1))
		if pair2 >= 0 {
			kicker := findKicker(rankCounts, []uint8{uint8(pair1), uint8(pair2)})
			return TwoPair | (HandRank(pair1) << 8) | (HandRank(pair2) << 4) | HandRank(kicker)
		}
		// One pair
		kickers := findKickers(rankCounts, []uint8{uint8(pair1)}, 3)
		return Pair | (HandRank(pair1) << 12) | HandRank(kickers)
	}

	// High card
	kickers := findKickers(rankCounts, []uint8{}, 5)
	return HighCard | HandRank(kickers)
}

// countRanks counts how many of each rank we have
func countRanks(hand Hand) [13]uint8 {
	var counts [13]uint8
	// Check each possible card
	for suit := uint8(0); suit < 4; suit++ {
		suitMask := hand.GetSuitMask(suit)
		for rank := uint8(0); rank < 13; rank++ {
			if suitMask&(1<<rank) != 0 {
				counts[rank]++
			}
		}
	}
	return counts
}

// findNOfAKind finds the highest rank with exactly n cards
func findNOfAKind(counts [13]uint8, n uint8) int {
	for rank := 12; rank >= 0; rank-- {
		if counts[rank] == n {
			return rank
		}
	}
	return -1
}

// findNOfAKindExcept finds the highest rank with exactly n cards, excluding a specific rank
func findNOfAKindExcept(counts [13]uint8, n uint8, except uint8) int {
	for rank := 12; rank >= 0; rank-- {
		if uint8(rank) != except && counts[rank] == n {
			return rank
		}
	}
	return -1
}

// findKicker finds the highest kicker excluding used ranks
func findKicker(counts [13]uint8, used []uint8) uint8 {
	isUsed := make(map[uint8]bool)
	for _, r := range used {
		isUsed[r] = true
	}

	for rank := uint8(12); rank < 13; rank-- { // Will wrap around after 0
		if !isUsed[rank] && counts[rank] > 0 {
			return rank
		}
	}
	return 0
}

// findKickers finds the top n kickers excluding used ranks
func findKickers(counts [13]uint8, used []uint8, n int) uint16 {
	isUsed := make(map[uint8]bool)
	for _, r := range used {
		isUsed[r] = true
	}

	kickers := uint16(0)
	found := 0
	for rank := uint8(12); rank < 13 && found < n; rank-- { // Will wrap around after 0
		if !isUsed[rank] && counts[rank] > 0 {
			kickers |= uint16(1) << rank
			found++
		}
	}
	return kickers
}

// checkFlush returns the suit if there's a flush, -1 otherwise
func checkFlush(hand Hand) int {
	for suit := uint8(0); suit < 4; suit++ {
		suitMask := hand.GetSuitMask(suit)
		if bits.OnesCount16(suitMask) >= 5 {
			return int(suit)
		}
	}
	return -1
}

// getFlushCards returns a Hand containing only cards of the specified suit
func getFlushCards(hand Hand, suit uint8) Hand {
	suitMask := hand.GetSuitMask(suit)
	offset := suit * 13
	return Hand(uint64(suitMask) << offset)
}

// checkStraight checks for a straight and returns the high card rank
func checkStraight(hand Hand) uint8 {
	rankMask := hand.GetRankMask()

	// Check for ace-low straight (A-2-3-4-5)
	if rankMask&0x100F == 0x100F { // Ace + 2-3-4-5
		return 3 // 5-high straight
	}

	// Check for regular straights
	for high := uint8(12); high >= 4; high-- {
		straightMask := uint16(0x1F) << (high - 4)
		if rankMask&straightMask == straightMask {
			return high
		}
	}

	return 0
}

// getTopCards returns a bitmask of the top n cards by rank
func getTopCards(hand Hand, n int) uint16 {
	rankMask := hand.GetRankMask()
	result := uint16(0)
	found := 0

	for rank := uint8(12); rank < 13 && found < n; rank-- {
		if rankMask&(1<<rank) != 0 {
			result |= 1 << rank
			found++
		}
	}
	return result
}

// CompareHands compares two hands and returns 1 if a wins, -1 if b wins, 0 for tie
func CompareHands(a, b HandRank) int {
	if a > b {
		return 1
	} else if a < b {
		return -1
	}
	return 0
}
