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

	return evaluate7CardsUnchecked(hand)
}

// Evaluate7CardsBatch evaluates multiple 7-card hands and writes results into out.
// If out is nil or smaller than hands, a new slice is allocated and returned.
// Each hand is assumed to contain exactly seven cards; behavior is undefined otherwise.
func Evaluate7CardsBatch(hands []Hand, out []HandRank) []HandRank {
	if len(out) < len(hands) {
		out = make([]HandRank, len(hands))
	} else {
		out = out[:len(hands)]
	}

	for i, hand := range hands {
		out[i] = evaluate7CardsUnchecked(hand)
	}

	return out
}

func evaluate7CardsUnchecked(hand Hand) HandRank {
	var suitMasks [4]uint16
	var rankMask uint16
	for suit := uint8(0); suit < 4; suit++ {
		mask := hand.GetSuitMask(suit)
		suitMasks[suit] = mask
		rankMask |= mask
	}

	return rankFromMasks(suitMasks, rankMask)
}

func rankFromMasks(suitMasks [4]uint16, rankMask uint16) HandRank {
	// Check for flush first (most restrictive) - check ALL suits for best flush
	bestFlushRank := HandRank(0)
	for _, suitMask := range suitMasks {
		if bits.OnesCount16(suitMask) >= 5 {
			if straightRank := straightHighMask(suitMask); straightRank > 0 {
				rank := StraightFlush | (HandRank(straightRank) << 24)
				if rank > bestFlushRank {
					bestFlushRank = rank
				}
			} else {
				topCards := topRanksFromMask(suitMask, 5)
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

	s0, s1, s2, s3 := suitMasks[0], suitMasks[1], suitMasks[2], suitMasks[3]

	quadsMask := s0 & s1 & s2 & s3
	tripCandidates := (s0 & s1 & s2) | (s0 & s1 & s3) | (s0 & s2 & s3) | (s1 & s2 & s3)
	tripsMask := tripCandidates &^ quadsMask
	pairsMask := ((s0 & s1) | (s0 & s2) | (s0 & s3) | (s1 & s2) | (s1 & s3) | (s2 & s3)) &^ tripCandidates

	if quad := highestRank(quadsMask); quad >= 0 {
		kicker := findKicker(rankMask, []uint8{uint8(quad)})
		return FourOfAKind | (HandRank(quad) << 24) | (HandRank(kicker) << 20)
	}

	if tripRank := highestRank(tripsMask); tripRank >= 0 {
		pairCandidates := pairsMask | (tripsMask &^ (1 << tripRank))
		if pairRank := highestRank(pairCandidates); pairRank >= 0 {
			return FullHouse | (HandRank(tripRank) << 24) | (HandRank(pairRank) << 20)
		}
	}

	if straightRank := straightHighMask(rankMask); straightRank > 0 {
		return Straight | (HandRank(straightRank) << 24)
	}

	if tripRank := highestRank(tripsMask); tripRank >= 0 {
		kickers := findOrderedKickers(rankMask, []uint8{uint8(tripRank)}, 2)
		return ThreeOfAKind | (HandRank(tripRank) << 24) | (HandRank(kickers[0]) << 20) | (HandRank(kickers[1]) << 16)
	}

	if pair1 := highestRank(pairsMask); pair1 >= 0 {
		if pair2 := highestRank(pairsMask &^ (1 << pair1)); pair2 >= 0 {
			kicker := findKicker(rankMask, []uint8{uint8(pair1), uint8(pair2)})
			return TwoPair | (HandRank(pair1) << 24) | (HandRank(pair2) << 20) | (HandRank(kicker) << 16)
		}
		kickers := findOrderedKickers(rankMask, []uint8{uint8(pair1)}, 3)
		return Pair | (HandRank(pair1) << 24) | (HandRank(kickers[0]) << 20) | (HandRank(kickers[1]) << 16) | (HandRank(kickers[2]) << 12)
	}

	kickers := findOrderedKickers(rankMask, nil, 5)
	return HighCard | (HandRank(kickers[0]) << 24) | (HandRank(kickers[1]) << 20) | (HandRank(kickers[2]) << 16) | (HandRank(kickers[3]) << 12) | (HandRank(kickers[4]) << 8)
}

// highestRank returns the highest rank present in the bitmask (or -1 when empty).
func highestRank(mask uint16) int {
	if mask == 0 {
		return -1
	}
	return bits.Len16(mask) - 1
}

// findKicker finds the highest kicker excluding used ranks.
func findKicker(mask uint16, used []uint8) uint8 {
	available := mask &^ ranksMask(used)
	if available == 0 {
		return 0
	}
	return uint8(bits.Len16(available) - 1)
}

// findOrderedKickers finds the top n kickers in descending order, excluding used ranks
func findOrderedKickers(mask uint16, used []uint8, n int) []uint8 {
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

func topRanksFromMask(mask uint16, n int) []uint8 {
	return findOrderedKickers(mask, nil, n)
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

// CompareHands compares two hands and returns 1 if a wins, -1 if b wins, 0 for tie
func CompareHands(a, b HandRank) int {
	if a > b {
		return 1
	} else if a < b {
		return -1
	}
	return 0
}
