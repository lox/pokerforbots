package evaluator

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

// HandRank represents a poker hand ranking with embedded score
type HandRank int

// Compare returns -1 if h is weaker, 0 if equal, 1 if h is stronger
func (h HandRank) Compare(other HandRank) int {
	if h < other {
		return 1 // Lower score = stronger hand
	} else if h > other {
		return -1
	}
	return 0
}

// String returns the readable name of the hand
func (h HandRank) String() string {
	handType := int(h) >> 20
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

// Type returns the hand type (pair, flush, etc.)
func (h HandRank) Type() int {
	return int(h) >> 20
}

// PairRank returns the pair rank for one pair hands
func (h HandRank) PairRank() int {
	if h.Type() != OnePairType {
		return 0
	}
	tiebreakData := int(h) & 0xFFFFF
	encodedPairRank := (tiebreakData >> 12) & 0xF
	return 15 - encodedPairRank
}

// HighCardRank returns the high card rank for high card hands
func (h HandRank) HighCardRank() int {
	if h.Type() != HighCardType {
		return 0
	}
	tiebreakData := int(h) & 0xFFFFF
	encodedHighRank := tiebreakData & 0xF
	return 15 - encodedHighRank
}
