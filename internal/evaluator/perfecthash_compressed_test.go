package evaluator

import (
	"math/rand"
	"testing"

	"github.com/lox/pokerforbots/internal/deck"
)

// BenchmarkEvaluate7_Compressed benchmarks the compressed CHD implementation
func BenchmarkEvaluate7_Compressed_RandomHands(b *testing.B) {
	// Generate random 7-card hands for benchmarking
	rng := rand.New(rand.NewSource(42))
	hands := make([][7]deck.Card, 1000)

	for i := range hands {
		// Create a deck and shuffle it
		var cards [52]deck.Card
		idx := 0
		for s := 0; s < 4; s++ {
			for r := 2; r <= 14; r++ {
				cards[idx] = deck.NewCard(s, r)
				idx++
			}
		}

		// Shuffle using Fisher-Yates
		for j := len(cards) - 1; j > 0; j-- {
			k := rng.Intn(j + 1)
			cards[j], cards[k] = cards[k], cards[j]
		}

		// Take first 7 cards
		copy(hands[i][:], cards[:7])
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		hand := hands[i%len(hands)]
		_ = Evaluate7Compressed(hand[:])
	}
}

// BenchmarkEvaluate7_Compressed_TortureCases benchmarks specific hand types
func BenchmarkEvaluate7_Compressed_TortureCases(b *testing.B) {
	testCases := []struct {
		name  string
		cards [7]deck.Card
	}{
		{
			name: "RoyalFlush",
			cards: [7]deck.Card{
				deck.NewCard(0, 14), // Ace of spades
				deck.NewCard(0, 13), // King of spades
				deck.NewCard(0, 12), // Queen of spades
				deck.NewCard(0, 11), // Jack of spades
				deck.NewCard(0, 10), // Ten of spades
				deck.NewCard(1, 2),  // Two of hearts
				deck.NewCard(2, 3),  // Three of diamonds
			},
		},
		{
			name: "StraightFlush",
			cards: [7]deck.Card{
				deck.NewCard(0, 9), // Nine of spades
				deck.NewCard(0, 8), // Eight of spades
				deck.NewCard(0, 7), // Seven of spades
				deck.NewCard(0, 6), // Six of spades
				deck.NewCard(0, 5), // Five of spades
				deck.NewCard(1, 2), // Two of hearts
				deck.NewCard(2, 3), // Three of diamonds
			},
		},
		{
			name: "FourOfAKind",
			cards: [7]deck.Card{
				deck.NewCard(0, 14), // Ace of spades
				deck.NewCard(1, 14), // Ace of hearts
				deck.NewCard(2, 14), // Ace of diamonds
				deck.NewCard(3, 14), // Ace of clubs
				deck.NewCard(0, 2),  // Two of spades
				deck.NewCard(1, 3),  // Three of hearts
				deck.NewCard(2, 4),  // Four of diamonds
			},
		},
		{
			name: "FullHouse",
			cards: [7]deck.Card{
				deck.NewCard(0, 14), // Ace of spades
				deck.NewCard(1, 14), // Ace of hearts
				deck.NewCard(2, 14), // Ace of diamonds
				deck.NewCard(0, 13), // King of spades
				deck.NewCard(1, 13), // King of hearts
				deck.NewCard(2, 5),  // Five of diamonds
				deck.NewCard(3, 6),  // Six of clubs
			},
		},
		{
			name: "Flush",
			cards: [7]deck.Card{
				deck.NewCard(0, 14), // Ace of spades
				deck.NewCard(0, 12), // Queen of spades
				deck.NewCard(0, 10), // Ten of spades
				deck.NewCard(0, 8),  // Eight of spades
				deck.NewCard(0, 6),  // Six of spades
				deck.NewCard(1, 2),  // Two of hearts
				deck.NewCard(2, 3),  // Three of diamonds
			},
		},
		{
			name: "Straight",
			cards: [7]deck.Card{
				deck.NewCard(0, 10), // Ten of spades
				deck.NewCard(1, 9),  // Nine of hearts
				deck.NewCard(2, 8),  // Eight of diamonds
				deck.NewCard(3, 7),  // Seven of clubs
				deck.NewCard(0, 6),  // Six of spades
				deck.NewCard(1, 2),  // Two of hearts
				deck.NewCard(2, 4),  // Four of diamonds
			},
		},
		{
			name: "WheelStraight",
			cards: [7]deck.Card{
				deck.NewCard(0, 14), // Ace of spades (plays as 1)
				deck.NewCard(1, 5),  // Five of hearts
				deck.NewCard(2, 4),  // Four of diamonds
				deck.NewCard(3, 3),  // Three of clubs
				deck.NewCard(0, 2),  // Two of spades
				deck.NewCard(1, 7),  // Seven of hearts
				deck.NewCard(2, 9),  // Nine of diamonds
			},
		},
		{
			name: "ThreeOfAKind",
			cards: [7]deck.Card{
				deck.NewCard(0, 14), // Ace of spades
				deck.NewCard(1, 14), // Ace of hearts
				deck.NewCard(2, 14), // Ace of diamonds
				deck.NewCard(0, 13), // King of spades
				deck.NewCard(1, 12), // Queen of hearts
				deck.NewCard(2, 5),  // Five of diamonds
				deck.NewCard(3, 6),  // Six of clubs
			},
		},
		{
			name: "TwoPair",
			cards: [7]deck.Card{
				deck.NewCard(0, 14), // Ace of spades
				deck.NewCard(1, 14), // Ace of hearts
				deck.NewCard(2, 13), // King of diamonds
				deck.NewCard(3, 13), // King of clubs
				deck.NewCard(0, 12), // Queen of spades
				deck.NewCard(1, 5),  // Five of hearts
				deck.NewCard(2, 6),  // Six of diamonds
			},
		},
		{
			name: "OnePair",
			cards: [7]deck.Card{
				deck.NewCard(0, 14), // Ace of spades
				deck.NewCard(1, 14), // Ace of hearts
				deck.NewCard(2, 13), // King of diamonds
				deck.NewCard(3, 12), // Queen of clubs
				deck.NewCard(0, 11), // Jack of spades
				deck.NewCard(1, 5),  // Five of hearts
				deck.NewCard(2, 6),  // Six of diamonds
			},
		},
		{
			name: "HighCard",
			cards: [7]deck.Card{
				deck.NewCard(0, 14), // Ace of spades
				deck.NewCard(1, 13), // King of hearts
				deck.NewCard(2, 12), // Queen of diamonds
				deck.NewCard(3, 11), // Jack of clubs
				deck.NewCard(0, 9),  // Nine of spades
				deck.NewCard(1, 5),  // Five of hearts
				deck.NewCard(2, 6),  // Six of diamonds
			},
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = Evaluate7Compressed(tc.cards[:])
			}
		})
	}
}

// TestEvaluate7_Compressed_Correctness validates compressed implementation against original
func TestEvaluate7_Compressed_Correctness(t *testing.T) {
	// Test a variety of hands to ensure compressed implementation matches original
	rng := rand.New(rand.NewSource(12345))

	for i := 0; i < 10000; i++ {
		// Generate random 7-card hand
		var cards [52]deck.Card
		idx := 0
		for s := 0; s < 4; s++ {
			for r := 2; r <= 14; r++ {
				cards[idx] = deck.NewCard(s, r)
				idx++
			}
		}

		// Shuffle using Fisher-Yates
		for j := len(cards) - 1; j > 0; j-- {
			k := rng.Intn(j + 1)
			cards[j], cards[k] = cards[k], cards[j]
		}

		hand := cards[:7]

		// Compare results
		original := evaluate7Basic(hand)
		compressed := Evaluate7Compressed(hand)

		if original != compressed {
			t.Errorf("Hand %d: compressed result %d != original result %d", i, compressed, original)
			t.Errorf("Cards: %v", hand)
			return
		}
	}
}
