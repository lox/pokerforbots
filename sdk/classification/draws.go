// Package classification provides draw detection for poker hands.
//
// This implementation uses efficient bit-packed poker.Hand representations
// and is ported from the proven Zig poker evaluation library.
package classification

import (
	"math/bits"
	"slices"

	"github.com/lox/pokerforbots/v2/poker"
)

// DrawType represents the types of draws a hand can have
type DrawType int

const (
	FlushDraw DrawType = iota
	NutFlushDraw
	OpenEndedStraightDraw
	Gutshot
	DoubleGutshot
	ComboDraw // Multiple draws
	BackdoorFlush
	BackdoorStraight
	Overcards
	NoDraw
)

func (dt DrawType) String() string {
	switch dt {
	case FlushDraw:
		return "flush draw"
	case NutFlushDraw:
		return "nut flush draw"
	case OpenEndedStraightDraw:
		return "open-ended straight draw"
	case Gutshot:
		return "gutshot"
	case DoubleGutshot:
		return "double gutshot"
	case ComboDraw:
		return "combo draw"
	case BackdoorFlush:
		return "backdoor flush"
	case BackdoorStraight:
		return "backdoor straight"
	case Overcards:
		return "overcards"
	case NoDraw:
		return "no draw"
	default:
		return "unknown"
	}
}

// DrawInfo contains information about draws in a hand
type DrawInfo struct {
	Draws   []DrawType
	Outs    int
	NutOuts int
}

// HasStrongDraw returns true if the hand has a strong draw
func (d DrawInfo) HasStrongDraw() bool {
	for _, draw := range d.Draws {
		switch draw {
		case FlushDraw, NutFlushDraw, OpenEndedStraightDraw, ComboDraw:
			return true
		}
	}
	return false
}

// HasWeakDraw returns true if the hand has a weak draw
func (d DrawInfo) HasWeakDraw() bool {
	for _, draw := range d.Draws {
		switch draw {
		case Gutshot, BackdoorFlush, BackdoorStraight, Overcards:
			return true
		}
	}
	return false
}

// IsComboDraw returns true if the hand has multiple draws with many outs
func (d DrawInfo) IsComboDraw() bool {
	return len(d.Draws) >= 2 && d.Outs >= 12
}

// DetectDraws analyzes a hand for all possible draws using efficient bit operations
func DetectDraws(holeCards, board poker.Hand) DrawInfo {
	if board.CountCards() < 3 {
		return DrawInfo{Draws: []DrawType{NoDraw}, Outs: 0, NutOuts: 0}
	}

	var draws []DrawType
	var outsMask poker.Hand // Bitmask of all outs to avoid double-counting
	var nutOutsMask poker.Hand

	allCards := holeCards | board

	// Check flush draws
	flushInfo := detectFlushDraw(holeCards, board)
	if flushInfo.HasFlushDraw {
		if flushInfo.IsNutFlushDraw {
			draws = append(draws, NutFlushDraw)
			nutOutsMask |= flushInfo.OutsMask
		} else {
			draws = append(draws, FlushDraw)
		}
		outsMask |= flushInfo.OutsMask
	}

	// Check straight draws
	straightInfo := detectStraightDraws(holeCards, board)
	if straightInfo.HasOESD {
		draws = append(draws, OpenEndedStraightDraw)
		outsMask |= straightInfo.OESDOutsMask
	}
	if straightInfo.HasGutshot {
		draws = append(draws, Gutshot)
		outsMask |= straightInfo.GutshotOutsMask
	}
	if straightInfo.HasDoubleGutshot {
		draws = append(draws, DoubleGutshot)
		outsMask |= straightInfo.DoubleGutshotOutsMask
	}

	// Check for backdoor draws (only on flop)
	if board.CountCards() == 3 {
		backdoorFlush := detectBackdoorFlush(holeCards, board)
		if backdoorFlush.HasBackdoorFlush {
			draws = append(draws, BackdoorFlush)
			// Backdoor draws don't count as immediate outs
		}

		backdoorStraight := detectBackdoorStraight(holeCards, board)
		if backdoorStraight.HasBackdoorStraight {
			draws = append(draws, BackdoorStraight)
			// Backdoor draws don't count as immediate outs
		}
	}

	// Check for overcards (only if no stronger draws)
	if !flushInfo.HasFlushDraw && !straightInfo.HasOESD {
		overcardsInfo := detectOvercards(holeCards, board, allCards)
		if overcardsInfo.HasOvercards {
			draws = append(draws, Overcards)
			outsMask |= overcardsInfo.OutsMask
		}
	}

	// Count total outs (avoiding double-counting with bitmask)
	totalOuts := outsMask.CountCards()
	nutOuts := nutOutsMask.CountCards()

	// Detect combo draws (preserve individual draw types, also add combo)
	if len(draws) >= 2 && totalOuts >= 12 {
		draws = append(draws, ComboDraw)
	}

	if len(draws) == 0 {
		draws = []DrawType{NoDraw}
	}

	return DrawInfo{
		Draws:   draws,
		Outs:    totalOuts,
		NutOuts: nutOuts,
	}
}

// Helper types and functions

type flushDrawInfo struct {
	HasFlushDraw   bool
	IsNutFlushDraw bool
	Suit           uint8
	OutsMask       poker.Hand
}

type straightDrawInfo struct {
	HasOESD               bool
	HasGutshot            bool
	HasDoubleGutshot      bool
	OESDOutsMask          poker.Hand
	GutshotOutsMask       poker.Hand
	DoubleGutshotOutsMask poker.Hand
}

type backdoorFlushInfo struct {
	HasBackdoorFlush bool
	Suit             uint8
}

type backdoorStraightInfo struct {
	HasBackdoorStraight bool
}

type overcardsInfo struct {
	HasOvercards bool
	OutsMask     poker.Hand
}

func detectFlushDraw(holeCards, board poker.Hand) flushDrawInfo {
	// Check each suit for flush draw potential
	for suit := range uint8(4) {
		holeSuitMask := holeCards.GetSuitMask(suit)
		boardSuitMask := board.GetSuitMask(suit)

		holeCount := bits.OnesCount16(holeSuitMask)
		boardCount := bits.OnesCount16(boardSuitMask)
		totalCount := holeCount + boardCount

		// Treat three or more cards of the same suit as a flush draw when at
		// least one of them comes from the player's hole cards. This mirrors
		// the behaviour expected by the previous string-based implementation
		// and keeps compatibility with existing bot heuristics.
		if totalCount >= 3 && holeCount > 0 {
			// Calculate outs mask (remaining cards of this suit)
			usedMask := holeSuitMask | boardSuitMask
			availableMask := uint16(0x1FFF) &^ usedMask // All ranks minus used ones

			// Convert to full hand bitmask for this suit
			outsMask := poker.Hand(availableMask) << (suit * 13)

			// Check if it's nut flush draw (we have ace of this suit)
			isNutFlush := (holeSuitMask & (1 << poker.Ace)) != 0

			return flushDrawInfo{
				HasFlushDraw:   true,
				IsNutFlushDraw: isNutFlush,
				Suit:           suit,
				OutsMask:       outsMask,
			}
		}
	}

	return flushDrawInfo{HasFlushDraw: false}
}

func detectStraightDraws(holeCards, board poker.Hand) straightDrawInfo {
	allCards := holeCards | board
	rankMask := allCards.GetRankMask()

	var info straightDrawInfo

	// Check for open-ended straight draws (8 outs)
	// Scan for 4 consecutive ranks with gaps on both ends
	for start := 0; start <= 9; start++ { // A-2-3-4 through J-Q-K-A
		consecutive := 0
		for i := range 4 {
			if rankMask&(1<<(start+i)) != 0 {
				consecutive++
			}
		}

		if consecutive == 4 {
			// Check if both ends are available for OESD
			lowRank := start - 1
			highRank := start + 4

			if lowRank >= 0 && highRank <= 13 {
				lowAvailable := (rankMask & (1 << lowRank)) == 0
				highAvailable := (rankMask & (1 << highRank)) == 0

				if lowAvailable && highAvailable {
					info.HasOESD = true
					// Create outs mask for both ends (4 cards each)
					for suit := range uint8(4) {
						info.OESDOutsMask.AddCard(poker.NewCard(uint8(lowRank), suit))
						info.OESDOutsMask.AddCard(poker.NewCard(uint8(highRank), suit))
					}
				}
			}
		}
	}

	// Check for gutshots (4 outs)
	// Look for 4 out of 5 consecutive ranks with exactly one gap
	for start := 0; start <= 8; start++ { // Windows of five consecutive ranks
		var presentRanks []int
		for i := range 5 {
			if rankMask&(1<<(start+i)) != 0 {
				presentRanks = append(presentRanks, start+i)
			}
		}

		if len(presentRanks) == 4 {
			first := presentRanks[0]
			last := presentRanks[len(presentRanks)-1]

			// If the four ranks are consecutive and both outer cards are
			// available, this situation is already covered by the OESD logic
			// and should not be double-counted as a gutshot.
			if last-first == 3 {
				lowOut := first - 1
				highOut := last + 1

				if first == 0 {
					lowOut = int(poker.Ace)
				}

				hasLow := lowOut >= 0 && lowOut <= int(poker.Ace) && (rankMask&(1<<lowOut)) == 0
				hasHigh := highOut >= 0 && highOut <= int(poker.Ace) && (rankMask&(1<<highOut)) == 0

				if hasLow && hasHigh {
					continue
				}
			}

			// Find the missing rank
			allNeeded := make(map[int]bool)
			for i := range 5 {
				allNeeded[start+i] = true
			}

			var missingRank int
			for rank := range allNeeded {
				found := slices.Contains(presentRanks, rank)
				if !found {
					missingRank = rank
					break
				}
			}

			info.HasGutshot = true
			// Create outs mask for the missing rank (4 cards)
			for suit := range uint8(4) {
				info.GutshotOutsMask.AddCard(poker.NewCard(uint8(missingRank), suit))
			}
			break // Only count one gutshot
		}
	}

	return info
}

func detectBackdoorFlush(holeCards, board poker.Hand) backdoorFlushInfo {
	if board.CountCards() != 3 {
		return backdoorFlushInfo{HasBackdoorFlush: false}
	}

	// Need exactly two cards of the same suit total (classic backdoor case):
	// e.g. two suited hole cards or one suited card combined with the board.
	for suit := range uint8(4) {
		holeCount := bits.OnesCount16(holeCards.GetSuitMask(suit))
		boardCount := bits.OnesCount16(board.GetSuitMask(suit))

		if holeCount >= 1 && holeCount+boardCount == 2 {
			return backdoorFlushInfo{
				HasBackdoorFlush: true,
				Suit:             suit,
			}
		}
	}

	return backdoorFlushInfo{HasBackdoorFlush: false}
}

func detectBackdoorStraight(_, _ poker.Hand) backdoorStraightInfo {
	// Simplified implementation - would need complex analysis of
	// potential turn/river combinations for backdoor straights
	// For now, return false (conservative)
	return backdoorStraightInfo{HasBackdoorStraight: false}
}

func detectOvercards(holeCards, board, usedCards poker.Hand) overcardsInfo {
	// Find highest board rank
	boardRankMask := board.GetRankMask()
	var highestBoardRank uint8 = 0

	for rank := uint8(12); rank > 0; rank-- { // Start from ace, work down
		if boardRankMask&(1<<rank) != 0 {
			highestBoardRank = rank
			break
		}
	}

	// Count overcards in hole cards
	holeRankMask := holeCards.GetRankMask()
	var outsMask poker.Hand

	for rank := highestBoardRank + 1; rank <= 12; rank++ { // Check ranks higher than board
		if holeRankMask&(1<<rank) != 0 {
			// We have this rank, count remaining cards as outs
			for suit := range uint8(4) {
				card := poker.NewCard(rank, suit)
				if !usedCards.HasCard(card) {
					outsMask |= poker.Hand(card)
				}
			}
		}
	}

	return overcardsInfo{
		HasOvercards: outsMask.CountCards() > 0,
		OutsMask:     outsMask,
	}
}
