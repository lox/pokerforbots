package poker

import (
	"math/bits"
)

// HandRank represents the strength of a poker hand
type HandRank uint32

// Bit layout to prevent overlap:
// Bits 28-31: Hand type (4 bits)
// Bits 24-27: Primary rank (4 bits) - quad/trip/top pair/straight high
// Bits 20-23: Secondary rank (4 bits) - pair in full house/second pair
// Bits 16-19: Tertiary rank (4 bits) - kicker 1
// Bits 12-15: Quaternary rank (4 bits) - kicker 2
// Bits 8-11:  Quinary rank (4 bits) - kicker 3
// Bits 4-7:   Senary rank (4 bits) - kicker 4
// Bits 0-3:   Septenary rank (4 bits) - kicker 5

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

	// Check for flush first (most restrictive) - check ALL suits for best flush
	bestFlushRank := HandRank(0)
	for suit := range uint8(4) {
		suitMask := hand.GetSuitMask(suit)
		if bits.OnesCount16(suitMask) >= 5 {
			if straightRank := straightHighMask(suitMask); straightRank > 0 {
				rank := StraightFlush | (HandRank(straightRank) << 24)
				if rank > bestFlushRank {
					bestFlushRank = rank
				}
			} else {
				flushCards := Hand(uint64(suitMask) << (suit * 13))
				// Regular flush - use top 5 cards
				topCards := getTopCardsOrdered(flushCards, 5)
				rank := Flush | (HandRank(topCards[0]) << 24) | (HandRank(topCards[1]) << 20) |
					(HandRank(topCards[2]) << 16) | (HandRank(topCards[3]) << 12) | (HandRank(topCards[4]) << 8)
				if rank > bestFlushRank {
					bestFlushRank = rank
				}
			}
		}
	}
	if bestFlushRank > 0 {
		return bestFlushRank
	}

	// Count ranks for pairs, trips, etc.
	rankCounts, rankMask := countRanks(hand)

	// Check for four of a kind
	if quad := findNOfAKind(rankCounts, 4); quad >= 0 {
		kicker := findKicker(rankCounts, rankMask, []uint8{uint8(quad)})
		return FourOfAKind | (HandRank(quad) << 24) | (HandRank(kicker) << 20)
	}

	// Check for full house - FIXED: accept trips as pair component
	trips := findNOfAKind(rankCounts, 3)
	if trips >= 0 {
		// Look for another set (3+ cards) or pair (2+ cards)
		pair := findNOfAKindAtLeast(rankCounts, 2, uint8(trips))
		if pair >= 0 {
			return FullHouse | (HandRank(trips) << 24) | (HandRank(pair) << 20)
		}
	}

	// Check for straight
	if straightRank := straightHighMask(rankMask); straightRank > 0 {
		return Straight | (HandRank(straightRank) << 24)
	}

	// Check for three of a kind
	if trips >= 0 {
		kickers := findOrderedKickers(rankCounts, rankMask, []uint8{uint8(trips)}, 2)
		return ThreeOfAKind | (HandRank(trips) << 24) | (HandRank(kickers[0]) << 20) | (HandRank(kickers[1]) << 16)
	}

	// Check for two pair
	pair1 := findNOfAKind(rankCounts, 2)
	if pair1 >= 0 {
		pair2 := findNOfAKindExcept(rankCounts, 2, uint8(pair1))
		if pair2 >= 0 {
			// Ensure pair1 is higher than pair2
			if pair2 > pair1 {
				pair1, pair2 = pair2, pair1
			}
			kicker := findKicker(rankCounts, rankMask, []uint8{uint8(pair1), uint8(pair2)})
			return TwoPair | (HandRank(pair1) << 24) | (HandRank(pair2) << 20) | (HandRank(kicker) << 16)
		}
		// One pair
		kickers := findOrderedKickers(rankCounts, rankMask, []uint8{uint8(pair1)}, 3)
		return Pair | (HandRank(pair1) << 24) | (HandRank(kickers[0]) << 20) | (HandRank(kickers[1]) << 16) | (HandRank(kickers[2]) << 12)
	}

	// High card
	kickers := findOrderedKickers(rankCounts, rankMask, []uint8{}, 5)
	return HighCard | (HandRank(kickers[0]) << 24) | (HandRank(kickers[1]) << 20) | (HandRank(kickers[2]) << 16) | (HandRank(kickers[3]) << 12) | (HandRank(kickers[4]) << 8)
}

// countRanks returns per-rank card counts along with a presence mask.
func countRanks(hand Hand) ([13]uint8, uint16) {
	var counts [13]uint8
	var mask uint16

	remaining := uint64(hand)
	for remaining != 0 {
		lowBit := remaining & -remaining
		index := bits.TrailingZeros64(remaining)
		rank := uint8(index % 13)
		counts[rank]++
		mask |= 1 << rank
		remaining ^= lowBit
	}

	return counts, mask
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

// findNOfAKindAtLeast finds the highest rank with at least n cards, excluding a specific rank
func findNOfAKindAtLeast(counts [13]uint8, n uint8, except uint8) int {
	for rank := 12; rank >= 0; rank-- {
		if uint8(rank) != except && counts[rank] >= n {
			return rank
		}
	}
	return -1
}

// findKicker finds the highest kicker excluding used ranks
func findKicker(_ [13]uint8, mask uint16, used []uint8) uint8 {
	available := mask &^ ranksMask(used)
	if available == 0 {
		return 0
	}
	return uint8(bits.Len16(available) - 1)
}

// findOrderedKickers finds the top n kickers in descending order, excluding used ranks
func findOrderedKickers(_ [13]uint8, mask uint16, used []uint8, n int) []uint8 {
	available := mask &^ ranksMask(used)
	kickers := make([]uint8, 0, n)
	for len(kickers) < n {
		if available == 0 {
			kickers = append(kickers, 0)
			continue
		}
		top := uint8(bits.Len16(available) - 1)
		kickers = append(kickers, top)
		available &^= 1 << top
	}
	return kickers
}

func ranksMask(ranks []uint8) uint16 {
	var mask uint16
	for _, r := range ranks {
		mask |= 1 << r
	}
	return mask
}

// straightHighMask returns the high-card rank of the best straight present in the mask (0 if none).
// The mask is expected to use rank bits (0-12 for deuce through ace) with an optional extra ace bit.
func straightHighMask(mask uint16) uint8 {
	const wheelMask = 0x100F // Ace + 2-3-4-5
	mask &= 0x1FFF           // Ignore any bits above rank twelve

	if mask&wheelMask == wheelMask {
		return 3
	}

	// Bitwise cascade identifies consecutive sequences in one pass.
	seq := mask & (mask >> 1) & (mask >> 2) & (mask >> 3) & (mask >> 4)
	if seq == 0 {
		return 0
	}

	low := uint8(bits.Len16(seq) - 1)
	return low + 4
}

// getTopCardsOrdered returns the top n card ranks in descending order
func getTopCardsOrdered(hand Hand, n int) []uint8 {
	rankMask := hand.GetRankMask()
	result := make([]uint8, 0, n)

	for rank := 12; rank >= 0 && len(result) < n; rank-- {
		if rankMask&(1<<rank) != 0 {
			result = append(result, uint8(rank))
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
