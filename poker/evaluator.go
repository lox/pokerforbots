package poker

import (
	"math/bits"
)

// HandRank represents the strength of a poker hand. Lower values are stronger.
type HandRank uint16

// HandType enumerates the categories of poker hands ordered from weakest to strongest.
type HandType uint8

const (
	HighCard HandType = iota
	Pair
	TwoPair
	ThreeOfAKind
	Straight
	Flush
	FullHouse
	FourOfAKind
	StraightFlush
)

const (
	straightFlushCount = 10
	fourOfAKindCount   = 13 * 12
	fullHouseCount     = 13 * 12
	flushCount         = 1277
	straightCount      = 10
	threeOfAKindCount  = 13 * 66
	twoPairCount       = 78 * 11
	onePairCount       = 13 * 220
	highCardCount      = 1277
)

const (
	baseStraightFlush = 0
	baseFourOfAKind   = baseStraightFlush + straightFlushCount
	baseFullHouse     = baseFourOfAKind + fourOfAKindCount
	baseFlush         = baseFullHouse + fullHouseCount
	baseStraight      = baseFlush + flushCount
	baseThreeOfAKind  = baseStraight + straightCount
	baseTwoPair       = baseThreeOfAKind + threeOfAKindCount
	baseOnePair       = baseTwoPair + twoPairCount
	baseHighCard      = baseOnePair + onePairCount
)

// boundaries mark the exclusive upper bound for each category in ascending strength order.
var handTypeBoundaries = [...]HandRank{
	HandRank(baseFourOfAKind),
	HandRank(baseFullHouse),
	HandRank(baseFlush),
	HandRank(baseStraight),
	HandRank(baseThreeOfAKind),
	HandRank(baseTwoPair),
	HandRank(baseOnePair),
	HandRank(baseHighCard),
	HandRank(baseHighCard + highCardCount),
}

// HandType returns the type of hand (pair, flush, etc.).
func (hr HandRank) Type() HandType {
	switch {
	case hr < handTypeBoundaries[0]:
		return StraightFlush
	case hr < handTypeBoundaries[1]:
		return FourOfAKind
	case hr < handTypeBoundaries[2]:
		return FullHouse
	case hr < handTypeBoundaries[3]:
		return Flush
	case hr < handTypeBoundaries[4]:
		return Straight
	case hr < handTypeBoundaries[5]:
		return ThreeOfAKind
	case hr < handTypeBoundaries[6]:
		return TwoPair
	case hr < handTypeBoundaries[7]:
		return Pair
	default:
		return HighCard
	}
}

// String returns a human-readable hand description.
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
	bestStrength := HandRank(baseHighCard + highCardCount) // sentinel larger than any strength
	flushFound := false
	for _, suitMask := range suitMasks {
		if bits.OnesCount16(suitMask) >= 5 {
			if straightRank := straightHighMask(suitMask); straightRank > 0 {
				idxAsc := straightIndex(straightRank)
				detail := uint16(straightFlushCount-1) - idxAsc
				strength := HandRank(baseStraightFlush + detail)
				if strength < bestStrength {
					return strength
				}
			} else {
				topCards := topRanksFromMask(suitMask, 5)
				mask := maskFromRanks(topCards)
				idxAsc := comboIndex13of5[mask]
				idxAdj := adjustFiveCardIndex(idxAsc)
				detail := uint16(flushCount-1) - idxAdj
				strength := HandRank(baseFlush + detail)
				if strength < bestStrength {
					bestStrength = strength
					flushFound = true
				}
			}
		}
	}
	if flushFound {
		return bestStrength
	}

	s0, s1, s2, s3 := suitMasks[0], suitMasks[1], suitMasks[2], suitMasks[3]

	quadsMask := s0 & s1 & s2 & s3
	tripCandidates := (s0 & s1 & s2) | (s0 & s1 & s3) | (s0 & s2 & s3) | (s1 & s2 & s3)
	tripsMask := tripCandidates &^ quadsMask
	pairsMask := ((s0 & s1) | (s0 & s2) | (s0 & s3) | (s1 & s2) | (s1 & s3) | (s2 & s3)) &^ tripCandidates

	if quad := highestRank(quadsMask); quad >= 0 {
		quadRank := uint8(quad)
		kicker := findKicker(rankMask, []uint8{quadRank})
		kickerOrd := uint16(rankOrdinalAsc(kicker, []uint8{quadRank}))
		idxAsc := uint16(quadRank)*12 + kickerOrd
		detail := uint16(fourOfAKindCount-1) - idxAsc
		return HandRank(baseFourOfAKind + detail)
	}

	if tripRank := highestRank(tripsMask); tripRank >= 0 {
		trip := uint8(tripRank)
		pairCandidates := pairsMask | (tripsMask &^ (1 << tripRank))
		if pairRank := highestRank(pairCandidates); pairRank >= 0 {
			pair := uint8(pairRank)
			pairOrd := uint16(rankOrdinalAsc(pair, []uint8{trip}))
			idxAsc := uint16(trip)*12 + pairOrd
			detail := uint16(fullHouseCount-1) - idxAsc
			return HandRank(baseFullHouse + detail)
		}
	}

	if straightRank := straightHighMask(rankMask); straightRank > 0 {
		idxAsc := straightIndex(straightRank)
		detail := uint16(straightCount-1) - idxAsc
		return HandRank(baseStraight + detail)
	}

	if tripRank := highestRank(tripsMask); tripRank >= 0 {
		trip := uint8(tripRank)
		kickers := findOrderedKickers(rankMask, []uint8{trip}, 2)
		mask := maskFromOrdinals(kickers, []uint8{trip})
		idxAsc := uint16(trip)*66 + comboIndex12of2[mask]
		detail := uint16(threeOfAKindCount-1) - idxAsc
		return HandRank(baseThreeOfAKind + detail)
	}

	if pair1 := highestRank(pairsMask); pair1 >= 0 {
		highPair := uint8(pair1)
		if pair2 := highestRank(pairsMask &^ (1 << pair1)); pair2 >= 0 {
			lowPair := uint8(pair2)
			if lowPair > highPair {
				highPair, lowPair = lowPair, highPair
			}
			pairMask := (1 << lowPair) | (1 << highPair)
			pairIdxAsc := comboIndex13of2[pairMask]
			kicker := findKicker(rankMask, []uint8{highPair, lowPair})
			kickerOrd := uint16(rankOrdinalAsc(kicker, []uint8{highPair, lowPair}))
			idxAsc := pairIdxAsc*11 + kickerOrd
			detail := uint16(twoPairCount-1) - idxAsc
			return HandRank(baseTwoPair + detail)
		}
		kickers := findOrderedKickers(rankMask, []uint8{highPair}, 3)
		mask := maskFromOrdinals(kickers, []uint8{highPair})
		idxAsc := uint16(highPair)*220 + comboIndex12of3[mask]
		detail := uint16(onePairCount-1) - idxAsc
		return HandRank(baseOnePair + detail)
	}

	kickers := findOrderedKickers(rankMask, nil, 5)
	mask := maskFromRanks(kickers)
	idxAsc := comboIndex13of5[mask]
	idxAdj := adjustFiveCardIndex(idxAsc)
	detail := uint16(highCardCount-1) - idxAdj
	return HandRank(baseHighCard + detail)
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

var comboIndex13of5 = func() [1 << 13]uint16 {
	var table [1 << 13]uint16
	var idx uint16
	for a := 0; a <= 8; a++ {
		for b := a + 1; b <= 9; b++ {
			for c := b + 1; c <= 10; c++ {
				for d := c + 1; d <= 11; d++ {
					for e := d + 1; e <= 12; e++ {
						mask := (1 << a) | (1 << b) | (1 << c) | (1 << d) | (1 << e)
						table[mask] = idx
						idx++
					}
				}
			}
		}
	}
	return table
}()

var comboIndex13of2 = func() [1 << 13]uint16 {
	var table [1 << 13]uint16
	var idx uint16
	for a := 0; a <= 11; a++ {
		for b := a + 1; b <= 12; b++ {
			mask := (1 << a) | (1 << b)
			table[mask] = idx
			idx++
		}
	}
	return table
}()

var comboIndex12of2 = func() [1 << 12]uint16 {
	var table [1 << 12]uint16
	var idx uint16
	for a := 0; a <= 10; a++ {
		for b := a + 1; b <= 11; b++ {
			mask := (1 << a) | (1 << b)
			table[mask] = idx
			idx++
		}
	}
	return table
}()

var comboIndex12of3 = func() [1 << 12]uint16 {
	var table [1 << 12]uint16
	var idx uint16
	for a := 0; a <= 9; a++ {
		for b := a + 1; b <= 10; b++ {
			for c := b + 1; c <= 11; c++ {
				mask := (1 << a) | (1 << b) | (1 << c)
				table[mask] = idx
				idx++
			}
		}
	}
	return table
}()

var straightComboIndices = func() [10]uint16 {
	var arr [10]uint16
	idx := 0
	// Wheel (A-5)
	wheelMask := (1 << 0) | (1 << 1) | (1 << 2) | (1 << 3) | (1 << 12)
	arr[idx] = comboIndex13of5[wheelMask]
	idx++
	for high := 4; high <= 12; high++ {
		mask := uint16(0)
		for r := high - 4; r <= high; r++ {
			mask |= 1 << r
		}
		arr[idx] = comboIndex13of5[mask]
		idx++
	}
	sortSmallUint16(arr[:])
	return arr
}()

func straightIndex(high uint8) uint16 {
	if high == 3 { // wheel
		return 0
	}
	return uint16(high - 3)
}

func rankOrdinalAsc(rank uint8, excludes []uint8) uint8 {
	var offset uint8
	for _, ex := range excludes {
		if ex < rank {
			offset++
		}
	}
	return rank - offset
}

func maskFromRanks(ranks []uint8) uint16 {
	var mask uint16
	for _, r := range ranks {
		mask |= 1 << r
	}
	return mask
}

func maskFromOrdinals(ranks []uint8, excludes []uint8) uint16 {
	var mask uint16
	for _, r := range ranks {
		ord := rankOrdinalAsc(r, excludes)
		mask |= 1 << ord
	}
	return mask
}

func sortSmallUint16(vals []uint16) {
	for i := 1; i < len(vals); i++ {
		v := vals[i]
		j := i - 1
		for j >= 0 && vals[j] > v {
			vals[j+1] = vals[j]
			j--
		}
		vals[j+1] = v
	}
}

func adjustFiveCardIndex(idx uint16) uint16 {
	var adjust uint16
	for _, s := range straightComboIndices {
		if idx > s {
			adjust++
		} else {
			break
		}
	}
	return idx - adjust
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
	if a < b {
		return 1
	} else if a > b {
		return -1
	}
	return 0
}
