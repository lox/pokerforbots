package evaluator

import (
	"testing"

	"github.com/lox/pokerforbots/internal/deck"
)

func TestEvaluate7(t *testing.T) {
	tests := []struct {
		name     string
		cards    string // Card notation like "AsKsQsJsTs9h8h"
		expected int    // hand type for comparison
	}{
		{
			name:     "Royal Flush",
			cards:    "AsKsQsJsTs9h8h", // A♠ K♠ Q♠ J♠ T♠ 9♥ 8♥
			expected: RoyalFlushType,
		},
		{
			name:     "Straight Flush",
			cards:    "9s8s7s6s5s4h3h", // 9♠ 8♠ 7♠ 6♠ 5♠ 4♥ 3♥
			expected: StraightFlushType,
		},
		{
			name:     "Four of a Kind",
			cards:    "AsAhAdAcKs2h3h", // A♠ A♥ A♦ A♣ K♠ 2♥ 3♥
			expected: FourOfAKindType,
		},
		{
			name:     "Full House",
			cards:    "AsAhAdKsKh2h3h", // A♠ A♥ A♦ K♠ K♥ 2♥ 3♥
			expected: FullHouseType,
		},
		{
			name:     "Flush",
			cards:    "AsKsQs8s6s4h3h", // A♠ K♠ Q♠ 8♠ 6♠ 4♥ 3♥
			expected: FlushType,
		},
		{
			name:     "Straight",
			cards:    "AsKhQdJcTs9h8h", // A♠ K♥ Q♦ J♣ T♠ 9♥ 8♥
			expected: StraightType,
		},
		{
			name:     "Three of a Kind",
			cards:    "AsAhAdKs9c7h5h", // A♠ A♥ A♦ K♠ 9♣ 7♥ 5♥
			expected: ThreeOfAKindType,
		},
		{
			name:     "Two Pair",
			cards:    "AsAhKdKs9c7h5h", // A♠ A♥ K♦ K♠ 9♣ 7♥ 5♥
			expected: TwoPairType,
		},
		{
			name:     "One Pair",
			cards:    "AsAhKdQs9c7h5h", // A♠ A♥ K♦ Q♠ 9♣ 7♥ 5♥
			expected: OnePairType,
		},
		{
			name:     "High Card",
			cards:    "AsKhQd9s7c5h3h", // A♠ K♥ Q♦ 9♠ 7♣ 5♥ 3♥
			expected: HighCardType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cards := deck.MustParseCards(tt.cards)
			result := Evaluate7(cards)
			handType := result.Type()

			if handType != tt.expected {
				t.Errorf("Expected hand type %d, got %d", tt.expected, handType)
			}
		})
	}
}

func TestHandComparison(t *testing.T) {
	// Test that stronger hands get lower scores
	royalFlush := deck.MustParseCards("AsKsQsJsTs9h8h")
	fourOfAKind := deck.MustParseCards("AsAhAdAcKs2h3h")
	highCard := deck.MustParseCards("AsKhQd9s7c5h3h")

	royalScore := Evaluate7(royalFlush)
	fourScore := Evaluate7(fourOfAKind)
	highScore := Evaluate7(highCard)

	if royalScore.Compare(fourScore) <= 0 {
		t.Errorf("Royal flush should be stronger than four of a kind: %s vs %s", royalScore.String(), fourScore.String())
	}
	if fourScore.Compare(highScore) <= 0 {
		t.Errorf("Four of a kind should be stronger than high card: %s vs %s", fourScore.String(), highScore.String())
	}
}

func TestKickerComparison(t *testing.T) {
	// Test that higher kickers get lower scores within same hand type
	aceHigh := deck.MustParseCards("AsKhQd9s7c5h3h")  // A high
	kingHigh := deck.MustParseCards("KsQhJd9s7c5h3h") // K high

	aceScore := Evaluate7(aceHigh)
	kingScore := Evaluate7(kingHigh)

	aceHandType := aceScore.Type()
	kingHandType := kingScore.Type()

	if aceHandType != kingHandType {
		t.Errorf("Both should be same hand type, got %d vs %d", aceHandType, kingHandType)
	}

	// For high card, stronger hand should have positive Compare result
	if aceScore.Compare(kingScore) <= 0 {
		t.Errorf("Ace high should be stronger than king high: %s vs %s", aceScore.String(), kingScore.String())
	}
}

func TestPairComparison(t *testing.T) {
	// Test that higher pairs get lower scores
	acesPair := deck.MustParseCards("AsAhKdQs9c7h5h")  // Pair of Aces
	ninesPair := deck.MustParseCards("9s9hKdQsAc7h5h") // Pair of Nines

	acesScore := Evaluate7(acesPair)
	ninesScore := Evaluate7(ninesPair)

	acesHandType := acesScore.Type()
	ninesHandType := ninesScore.Type()

	if acesHandType != OnePairType || ninesHandType != OnePairType {
		t.Errorf("Both should be one pair, got %d and %d", acesHandType, ninesHandType)
	}

	// Pair of Aces should be stronger than pair of Nines
	if acesScore.Compare(ninesScore) <= 0 {
		t.Errorf("Pair of Aces should be stronger than pair of Nines: %s vs %s", acesScore.String(), ninesScore.String())
	}
}
