package evaluator

import (
	"testing"

	"github.com/lox/holdem-cli/internal/deck"
)

func TestEstimateEquity(t *testing.T) {
	tests := []struct {
		name          string
		hole          string
		board         string
		opponentRange Range
		expectedMin   float64 // Minimum expected equity
		expectedMax   float64 // Maximum expected equity
	}{
		{
			name:          "Pocket Aces vs Random",
			hole:          "AsAd",
			board:         "",
			opponentRange: RandomRange{},
			expectedMin:   0.70, // AA should have very high equity pre-flop (allow for Monte Carlo variance)
			expectedMax:   1.00,
		},
		{
			name:          "72o vs Random",
			hole:          "7h2c",
			board:         "",
			opponentRange: RandomRange{},
			expectedMin:   0.20, // 72o is weak but not terrible
			expectedMax:   0.40,
		},
		{
			name:          "Strong Draw vs Random",
			hole:          "AsKs",
			board:         "QsJs2h",
			opponentRange: RandomRange{},
			expectedMin:   0.65, // Royal flush + straight + flush draws
			expectedMax:   0.80,
		},
		{
			name:          "Weak Hand vs Random",
			hole:          "2h3c",
			board:         "AsKdQh",
			opponentRange: RandomRange{},
			expectedMin:   0.10,
			expectedMax:   0.25, // Very weak ace high
		},
		{
			name:          "Made Hand vs Tight",
			hole:          "AhAc",
			board:         "Ad7s2c",
			opponentRange: TightRange{},
			expectedMin:   0.85, // Trip aces vs tight range
			expectedMax:   1.00,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hole := deck.MustParseCards(tt.hole)
			var board []deck.Card
			if tt.board != "" {
				board = deck.MustParseCards(tt.board)
			}

			equity := EstimateEquity(hole, board, tt.opponentRange, 1000)

			if equity < tt.expectedMin || equity > tt.expectedMax {
				t.Errorf("Equity %.3f outside expected range [%.3f, %.3f]",
					equity, tt.expectedMin, tt.expectedMax)
			}
		})
	}
}

func TestEstimateEquityInvalidInputs(t *testing.T) {
	tests := []struct {
		name     string
		hole     []deck.Card
		board    []deck.Card
		expected float64
	}{
		{
			name:     "Empty hole cards",
			hole:     []deck.Card{},
			board:    []deck.Card{},
			expected: 0.0,
		},
		{
			name:     "One hole card",
			hole:     deck.MustParseCards("As"),
			board:    []deck.Card{},
			expected: 0.0,
		},
		{
			name:     "Too many board cards",
			hole:     deck.MustParseCards("AsKs"),
			board:    deck.MustParseCards("2h3h4h5h6h7h"), // 6 cards
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			equity := EstimateEquity(tt.hole, tt.board, RandomRange{}, 100)
			if equity != tt.expected {
				t.Errorf("Expected equity %.3f, got %.3f", tt.expected, equity)
			}
		})
	}
}

func TestRangeComparison(t *testing.T) {
	// Test that pocket aces perform differently against different ranges
	hole := deck.MustParseCards("AsAh")
	board := []deck.Card{} // Pre-flop

	randomEquity := EstimateEquity(hole, board, RandomRange{}, 1000)
	tightEquity := EstimateEquity(hole, board, TightRange{}, 1000)

	// AA should perform worse against tight opponents (who have better hands)
	if tightEquity >= randomEquity {
		t.Errorf("AA should perform worse vs tight range (%.3f) than random range (%.3f)",
			tightEquity, randomEquity)
	}

	// But both should still be quite high
	if randomEquity < 0.75 {
		t.Errorf("AA vs random should have high equity, got %.3f", randomEquity)
	}
	if tightEquity < 0.70 {
		t.Errorf("AA vs tight should still have good equity, got %.3f", tightEquity)
	}
}

func TestIsTightHand(t *testing.T) {
	tests := []struct {
		name     string
		cards    string
		expected bool
	}{
		// Pocket pairs
		{
			name:     "Pocket Aces",
			cards:    "AsAh",
			expected: true,
		},
		{
			name:     "Pocket Deuces",
			cards:    "2h2c",
			expected: true,
		},

		// High cards
		{
			name:     "AK offsuit",
			cards:    "AsKh",
			expected: true,
		},
		{
			name:     "QJ offsuit",
			cards:    "QsJh",
			expected: true,
		},

		// Suited connectors
		{
			name:     "Suited connector T9s",
			cards:    "Ts9s",
			expected: true,
		},
		{
			name:     "Low suited connector 87s",
			cards:    "8h7h",
			expected: true,
		},

		// Ace with good kicker
		{
			name:     "AT offsuit",
			cards:    "AsTs",
			expected: true,
		},
		{
			name:     "A9 offsuit",
			cards:    "As9h",
			expected: true,
		},

		// Weak hands
		{
			name:     "72 offsuit",
			cards:    "7h2c",
			expected: false,
		},
		{
			name:     "J3 offsuit",
			cards:    "Js3h",
			expected: false,
		},
		{
			name:     "A8 offsuit",
			cards:    "As8h",
			expected: false,
		},
		{
			name:     "Low unsuited cards",
			cards:    "6h4c",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cards := deck.MustParseCards(tt.cards)
			result := isTightHand(cards)
			if result != tt.expected {
				t.Errorf("isTightHand(%s) = %v, expected %v", tt.cards, result, tt.expected)
			}
		})
	}
}

func TestRandomRangeSampleHand(t *testing.T) {
	availableCards := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Hearts, Rank: deck.King},
		{Suit: deck.Diamonds, Rank: deck.Queen},
		{Suit: deck.Clubs, Rank: deck.Jack},
	}

	range_ := RandomRange{}
	hand, ok := range_.SampleHand(availableCards)

	if !ok {
		t.Fatal("Should be able to sample hand from available cards")
	}

	if len(hand) != 2 {
		t.Errorf("Expected 2 cards, got %d", len(hand))
	}

	// Check that sampled cards are from available cards
	for _, card := range hand {
		found := false
		for _, available := range availableCards {
			if card == available {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Sampled card %v not in available cards", card)
		}
	}
}

func TestTightRangeSampleHand(t *testing.T) {
	// Create a deck with mostly tight hands
	availableCards := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Hearts, Rank: deck.Ace}, // AA available
		{Suit: deck.Diamonds, Rank: deck.King},
		{Suit: deck.Clubs, Rank: deck.Queen},
		{Suit: deck.Spades, Rank: deck.Jack},
		{Suit: deck.Hearts, Rank: deck.Ten},
	}

	range_ := TightRange{}
	hand, ok := range_.SampleHand(availableCards)

	if !ok {
		t.Fatal("Should be able to sample tight hand")
	}

	if len(hand) != 2 {
		t.Errorf("Expected 2 cards, got %d", len(hand))
	}

	// The sampled hand should be considered tight
	if !isTightHand(hand) {
		t.Errorf("TightRange sampled non-tight hand: %v", hand)
	}
}

func TestTightRangeInsufficientCards(t *testing.T) {
	// Only one card available
	availableCards := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
	}

	range_ := TightRange{}
	_, ok := range_.SampleHand(availableCards)

	if ok {
		t.Error("Should not be able to sample hand with insufficient cards")
	}
}

func TestEquityConsistency(t *testing.T) {
	// Test that the same hand gives consistent results (within reasonable variance)
	hole := deck.MustParseCards("AsKs")
	board := deck.MustParseCards("QsJs")

	equity1 := EstimateEquity(hole, board, RandomRange{}, 5000)
	equity2 := EstimateEquity(hole, board, RandomRange{}, 5000)

	// With 5000 samples, results should be quite consistent
	variance := abs_float(equity1 - equity2)
	if variance > 0.05 { // Allow 5% variance
		t.Errorf("Equity results too inconsistent: %.3f vs %.3f (variance %.3f)",
			equity1, equity2, variance)
	}
}

func TestBoardProgression(t *testing.T) {
	// Test that equity changes logically as board develops
	hole := deck.MustParseCards("AsKs")

	preflopEquity := EstimateEquity(hole, []deck.Card{}, RandomRange{}, 100)

	// Flop gives us royal flush draw
	flop := deck.MustParseCards("QsJs2h")
	flopEquity := EstimateEquity(hole, flop, RandomRange{}, 1000)

	// Turn completes the royal flush
	turn := deck.MustParseCards("QsJs2hTs")
	turnEquity := EstimateEquity(hole, turn, RandomRange{}, 1000)

	// Equity should increase from pre-flop to flop (we picked up draws)
	if flopEquity <= preflopEquity {
		t.Errorf("Flop equity (%.3f) should be higher than preflop (%.3f)",
			flopEquity, preflopEquity)
	}

	// Turn equity should be very high (royal flush)
	if turnEquity < 0.98 {
		t.Errorf("Turn equity with royal flush should be ~100%%, got %.3f", turnEquity)
	}
}

func abs_float(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
