// Package classification provides poker classification tools including
// board texture analysis, draw detection, and hand strength categorization.
//
// This implementation uses efficient bit-packed poker.Hand representations
// and is ported from the proven Zig poker evaluation library.
package classification

import (
	"math/bits"

	"github.com/lox/pokerforbots/poker"
)

// BoardTexture represents the "wetness" of a poker board from dry to very wet
type BoardTexture int

const (
	Dry BoardTexture = iota
	SemiWet
	Wet
	VeryWet
)

func (bt BoardTexture) String() string {
	switch bt {
	case Dry:
		return "dry"
	case SemiWet:
		return "semi-wet"
	case Wet:
		return "wet"
	case VeryWet:
		return "very wet"
	default:
		return "unknown"
	}
}

// FlushInfo contains information about flush potential on a board
type FlushInfo struct {
	MaxSuitCount int
	DominantSuit *uint8
	IsMonotone   bool // Single suit (3+ cards)
	IsRainbow    bool // All different suits
}

// StraightInfo contains information about straight potential on a board
type StraightInfo struct {
	ConnectedCards int // Longest sequence of connected ranks
	Gaps           int // Number of gaps in sequences
	HasAce         bool
	BroadwayCards  int // Number of T, J, Q, K, A cards
}

// AnalyzeBoardTexture analyzes how coordinated/dangerous a board is
// Uses efficient bit-packed poker.Hand representation
func AnalyzeBoardTexture(board poker.Hand) BoardTexture {
	if board.CountCards() < 3 {
		return Dry
	}

	var wetness int

	// Check flush possibilities
	flushInfo := AnalyzeFlushPotential(board)
	switch {
	case flushInfo.IsMonotone && board.CountCards() >= 3:
		wetness += 4
	case flushInfo.MaxSuitCount >= 4:
		wetness += 4
	case flushInfo.MaxSuitCount == 3:
		wetness += 3
	case flushInfo.MaxSuitCount == 2:
		wetness += 1
	}

	// Check straight possibilities
	straightInfo := AnalyzeStraightPotential(board)
	switch {
	case straightInfo.ConnectedCards >= 4:
		wetness += 4
	case straightInfo.ConnectedCards == 3:
		wetness += 3
	case straightInfo.ConnectedCards == 2:
		wetness += 1
	}

	// Check for pairs on board
	pairCount := countBoardPairs(board)
	if pairCount >= 1 {
		wetness += 1 // Paired board
	}

	// High card concentration (multiple high cards = more dangerous)
	highCardCount := countHighCards(board)
	if highCardCount >= 3 {
		wetness += 1
	}

	switch {
	case wetness <= 0:
		return Dry
	case wetness <= 3:
		return SemiWet
	case wetness <= 5:
		return Wet
	default:
		return VeryWet
	}
}

// AnalyzeFlushPotential analyzes flush potential on the board using bit operations
func AnalyzeFlushPotential(board poker.Hand) FlushInfo {
	var suitCounts [4]int
	var suitMasks [4]uint16

	// Count cards of each suit using efficient bitmasks and keep the
	// per-suit masks for tie-breaking logic later.
	for suit := uint8(0); suit < 4; suit++ {
		suitMask := board.GetSuitMask(suit)
		suitCounts[suit] = bits.OnesCount16(suitMask)
		suitMasks[suit] = suitMask
	}

	var maxCount int
	var dominantSuit *uint8
	bestRankForSuit := -1
	nonZeroSuits := 0

	// Iterate suits in reverse order so tied counts prefer higher suits when
	// ranks are identical (mirrors legacy behaviour expected by tests).
	for suit := len(suitCounts) - 1; suit >= 0; suit-- {
		count := suitCounts[suit]
		if count == 0 {
			continue
		}

		nonZeroSuits++

		highestRank := bits.Len16(suitMasks[suit]) - 1
		if highestRank < 0 {
			highestRank = -1
		}

		if count > maxCount || (count == maxCount && highestRank > bestRankForSuit) {
			maxCount = count
			bestRankForSuit = highestRank
			suitCopy := uint8(suit)
			dominantSuit = &suitCopy
		}
	}

	cardCount := board.CountCards()

	return FlushInfo{
		MaxSuitCount: maxCount,
		DominantSuit: dominantSuit,
		IsMonotone:   nonZeroSuits == 1 && cardCount >= 3,
		IsRainbow:    nonZeroSuits == cardCount && cardCount >= 3,
	}
}

// AnalyzeStraightPotential analyzes straight potential using efficient rank bitmask
func AnalyzeStraightPotential(board poker.Hand) StraightInfo {
	cardCount := board.CountCards()
	if cardCount == 0 {
		return StraightInfo{}
	}

	if cardCount == 1 {
		ranks := board.GetRankMask()
		hasAce := (ranks & (1 << poker.Ace)) != 0
		broadwayCount := 0
		if hasAce {
			broadwayCount = 1
		}
		return StraightInfo{
			ConnectedCards: 1,
			Gaps:           0,
			HasAce:         hasAce,
			BroadwayCards:  broadwayCount,
		}
	}

	// Build a rank mask without the duplicated ace-low bit so we can work with
	// strictly increasing rank indices.
	var rankMask uint16
	for suit := uint8(0); suit < 4; suit++ {
		rankMask |= board.GetSuitMask(suit)
	}

	hasAce := (rankMask & (1 << poker.Ace)) != 0

	broadwayCount := 0
	for rank := poker.Ten; rank <= poker.Ace; rank++ {
		if rankMask&(1<<rank) != 0 {
			broadwayCount++
		}
	}

	ranks := make([]int, 0, cardCount)
	for rank := 0; rank < 13; rank++ {
		if rankMask&(1<<rank) != 0 {
			ranks = append(ranks, rank)
		}
	}

	if len(ranks) == 0 {
		return StraightInfo{}
	}

	maxConnected := 1
	currentConnected := 1
	totalGaps := 0

	for i := 1; i < len(ranks); i++ {
		gap := ranks[i] - ranks[i-1] - 1
		if gap == 0 {
			currentConnected++
		} else {
			if currentConnected > maxConnected {
				maxConnected = currentConnected
			}
			currentConnected = 1
			if gap > 0 {
				totalGaps += gap
			}
		}
	}

	if currentConnected > maxConnected {
		maxConnected = currentConnected
	}

	// Consider wheel connectivity (A-2-3-4-5) by treating the ace as rank -1
	// when low ranks are present. This keeps the computation deterministic and
	// avoids double-counting for other scenarios.
	if hasAce {
		var lowRanks []int
		for _, rank := range ranks {
			if rank <= 3 {
				lowRanks = append(lowRanks, rank)
			}
		}

		if len(lowRanks) >= 2 {
			wheelRanks := append([]int{-1}, lowRanks...)
			wheelConnected := 1
			wheelMax := 1
			for i := 1; i < len(wheelRanks); i++ {
				if wheelRanks[i]-wheelRanks[i-1] == 1 {
					wheelConnected++
				} else {
					if wheelConnected > wheelMax {
						wheelMax = wheelConnected
					}
					wheelConnected = 1
				}
			}
			if wheelConnected > wheelMax {
				wheelMax = wheelConnected
			}
			if wheelMax > maxConnected {
				maxConnected = wheelMax
			}
		}
	}

	return StraightInfo{
		ConnectedCards: maxConnected,
		Gaps:           totalGaps,
		HasAce:         hasAce,
		BroadwayCards:  broadwayCount,
	}
}

// countBoardPairs counts pairs on the board using rank frequency analysis
func countBoardPairs(board poker.Hand) int {
	var rankCounts [13]int

	// Count each rank across all suits
	for suit := uint8(0); suit < 4; suit++ {
		suitMask := board.GetSuitMask(suit)
		for rank := uint8(0); rank < 13; rank++ {
			if suitMask&(1<<rank) != 0 {
				rankCounts[rank]++
			}
		}
	}

	pairs := 0
	for _, count := range rankCounts {
		if count >= 2 {
			pairs++
		}
	}

	return pairs
}

// countHighCards counts high cards (T, J, Q, K, A) on the board
func countHighCards(board poker.Hand) int {
	highCardCount := 0

	// Check each suit for high cards (ranks 8-12 = T-A)
	for suit := uint8(0); suit < 4; suit++ {
		suitMask := board.GetSuitMask(suit)
		highMask := suitMask & 0x1F00 // Bits 8-12 (T-A)
		highCardCount += bits.OnesCount16(highMask)
	}

	return highCardCount
}
