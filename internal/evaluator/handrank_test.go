package evaluator

import (
	"testing"

	"github.com/lox/holdem-cli/internal/deck"
)

func TestHandRankCompare(t *testing.T) {
	// Test comparison between different hand types
	royalFlush := deck.MustParseCards("AsKsQsJsTs9h8h")
	fourOfAKind := deck.MustParseCards("AsAhAdAcKs2h3h")
	highCard := deck.MustParseCards("AsKhQd9s7c5h3h")

	royalScore := Evaluate7(royalFlush)
	fourScore := Evaluate7(fourOfAKind)
	highScore := Evaluate7(highCard)

	// Royal flush should beat four of a kind
	if royalScore.Compare(fourScore) <= 0 {
		t.Errorf("Royal flush should beat four of a kind")
	}

	// Four of a kind should beat high card
	if fourScore.Compare(highScore) <= 0 {
		t.Errorf("Four of a kind should beat high card")
	}

	// Same hand should tie
	if royalScore.Compare(royalScore) != 0 {
		t.Errorf("Same hand should tie")
	}
}

func TestHandRankString(t *testing.T) {
	tests := []struct {
		cards    string
		expected string
	}{
		{"AsKsQsJsTs9h8h", "Royal Flush"},
		{"9s8s7s6s5s4h3h", "Straight Flush"},
		{"AsAhAdAcKs2h3h", "Four of a Kind"},
		{"AsAhAdKsKh2h3h", "Full House"},
		{"AsKsQs9s7s4h3h", "Flush"},
		{"AsKhQdJsTs9h8h", "Straight"},
		{"AsAhAdKsQh2h3h", "Three of a Kind"},
		{"AsAhKdKsQh2h3h", "Two Pair"},
		{"AsAhKdQs9h2h3h", "One Pair"},
		{"AsKhQd9s7c5h3h", "High Card"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			cards := deck.MustParseCards(tt.cards)
			handRank := Evaluate7(cards)
			result := handRank.String()

			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestHandRankType(t *testing.T) {
	tests := []struct {
		cards    string
		expected int
	}{
		{"AsKsQsJsTs9h8h", RoyalFlushType},
		{"9s8s7s6s5s4h3h", StraightFlushType},
		{"AsAhAdAcKs2h3h", FourOfAKindType},
		{"AsAhAdKsKh2h3h", FullHouseType},
		{"AsKsQs9s7s4h3h", FlushType},
		{"AsKhQdJsTs9h8h", StraightType},
		{"AsAhAdKsQh2h3h", ThreeOfAKindType},
		{"AsAhKdKsQh2h3h", TwoPairType},
		{"AsAhKdQs9h2h3h", OnePairType},
		{"AsKhQd9s7c5h3h", HighCardType},
	}

	for _, tt := range tests {
		t.Run(HandRank(tt.expected<<20).String(), func(t *testing.T) {
			cards := deck.MustParseCards(tt.cards)
			handRank := Evaluate7(cards)
			result := handRank.Type()

			if result != tt.expected {
				t.Errorf("Expected type %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestHandRankPairRank(t *testing.T) {
	// Test pair rank extraction for one pair hands
	acesPair := deck.MustParseCards("AsAhKdQs9c7h5h")  // Pair of Aces
	kingsPair := deck.MustParseCards("KsKhAdQs9c7h5h") // Pair of Kings
	ninesPair := deck.MustParseCards("9s9hKdQsAc7h5h") // Pair of Nines

	acesScore := Evaluate7(acesPair)
	kingsScore := Evaluate7(kingsPair)
	ninesScore := Evaluate7(ninesPair)

	// Verify they are all one pair hands
	if acesScore.Type() != OnePairType {
		t.Errorf("Aces hand should be one pair")
	}
	if kingsScore.Type() != OnePairType {
		t.Errorf("Kings hand should be one pair")
	}
	if ninesScore.Type() != OnePairType {
		t.Errorf("Nines hand should be one pair")
	}

	// Test pair rank extraction
	if acesScore.PairRank() != 14 {
		t.Errorf("Aces pair rank should be 14, got %d", acesScore.PairRank())
	}
	if kingsScore.PairRank() != 13 {
		t.Errorf("Kings pair rank should be 13, got %d", kingsScore.PairRank())
	}
	if ninesScore.PairRank() != 9 {
		t.Errorf("Nines pair rank should be 9, got %d", ninesScore.PairRank())
	}

	// Test non-pair hands return 0
	highCard := deck.MustParseCards("AsKhQd9s7c5h3h")
	highScore := Evaluate7(highCard)
	if highScore.PairRank() != 0 {
		t.Errorf("High card hand should return 0 for pair rank, got %d", highScore.PairRank())
	}
}

func TestHandRankHighCardRank(t *testing.T) {
	// Test high card rank extraction
	aceHigh := deck.MustParseCards("AsKhQd9s7c5h3h")   // A high
	kingHigh := deck.MustParseCards("KsQhJd9s7c5h3h")  // K high
	queenHigh := deck.MustParseCards("QsJhTd9s7c5h3h") // Q high

	aceScore := Evaluate7(aceHigh)
	kingScore := Evaluate7(kingHigh)
	queenScore := Evaluate7(queenHigh)

	// Verify they are all high card hands
	if aceScore.Type() != HighCardType {
		t.Errorf("Ace high should be high card")
	}
	if kingScore.Type() != HighCardType {
		t.Errorf("King high should be high card")
	}
	if queenScore.Type() != HighCardType {
		t.Errorf("Queen high should be high card")
	}

	// Test high card rank extraction
	if aceScore.HighCardRank() != 14 {
		t.Errorf("Ace high card rank should be 14, got %d", aceScore.HighCardRank())
	}
	if kingScore.HighCardRank() != 13 {
		t.Errorf("King high card rank should be 13, got %d", kingScore.HighCardRank())
	}
	if queenScore.HighCardRank() != 12 {
		t.Errorf("Queen high card rank should be 12, got %d", queenScore.HighCardRank())
	}

	// Test non-high-card hands return 0
	pair := deck.MustParseCards("AsAhKdQs9c7h5h")
	pairScore := Evaluate7(pair)
	if pairScore.HighCardRank() != 0 {
		t.Errorf("Pair hand should return 0 for high card rank, got %d", pairScore.HighCardRank())
	}
}

func TestHandRankKickerComparison(t *testing.T) {
	// Test that kickers are properly compared within same hand type
	aceHighStrong := deck.MustParseCards("AsKhQd9s7c5h3h") // A-K-Q-9-7
	aceHighWeak := deck.MustParseCards("AsKhQd9s6c5h3h")   // A-K-Q-9-6

	strongScore := Evaluate7(aceHighStrong)
	weakScore := Evaluate7(aceHighWeak)

	// Both should be high card
	if strongScore.Type() != HighCardType || weakScore.Type() != HighCardType {
		t.Errorf("Both hands should be high card")
	}

	// Stronger kickers should win
	if strongScore.Compare(weakScore) <= 0 {
		t.Errorf("A-K-Q-9-7 should beat A-K-Q-9-6")
	}
}
