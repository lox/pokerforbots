package evaluator

import (
	"math/rand"
	"testing"

	"github.com/lox/holdem-cli/internal/deck"
)

// Generate7CardHands creates N random 7-card hands using a fixed seed
func Generate7CardHands(n int, seed int64) [][]deck.Card {
	rng := rand.New(rand.NewSource(seed))
	hands := make([][]deck.Card, n)

	for i := 0; i < n; i++ {
		hands[i] = generate7CardHand(rng)
	}

	return hands
}

// generate5CardHand creates a single random 5-card hand
func generate5CardHand(rng *rand.Rand) []deck.Card {
	allCards := createFullDeck()
	shuffleDeck(allCards, rng)
	return allCards[:5]
}

// generate7CardHand creates a single random 7-card hand
func generate7CardHand(rng *rand.Rand) []deck.Card {
	allCards := createFullDeck()
	shuffleDeck(allCards, rng)
	return allCards[:7]
}

// createFullDeck creates a standard 52-card deck
func createFullDeck() []deck.Card {
	allCards := make([]deck.Card, 0, 52)
	for suit := deck.Spades; suit <= deck.Clubs; suit++ {
		for rank := deck.Two; rank <= deck.Ace; rank++ {
			allCards = append(allCards, deck.NewCard(suit, rank))
		}
	}
	return allCards
}

// shuffleDeck shuffles the deck using Fisher-Yates algorithm
func shuffleDeck(cards []deck.Card, rng *rand.Rand) {
	for i := len(cards) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		cards[i], cards[j] = cards[j], cards[i]
	}
}

// GenerateTortureCases creates edge case hands that stress different evaluation branches
func GenerateTortureCases() []struct {
	name  string
	cards []deck.Card
} {
	return []struct {
		name  string
		cards []deck.Card
	}{
		{"RoyalFlush", deck.MustParseCards("AsKsQsJsTs")},
		{"StraightFlush", deck.MustParseCards("9h8h7h6h5h")},
		{"FourOfAKind", deck.MustParseCards("AsAhAdAcKs")},
		{"FullHouse", deck.MustParseCards("KsKhKdQcQs")},
		{"Flush", deck.MustParseCards("AcJc9c7c5c")},
		{"Straight", deck.MustParseCards("Ts9h8d7c6s")},
		{"WheelStraight", deck.MustParseCards("As5h4d3c2s")},
		{"ThreeOfAKind", deck.MustParseCards("JsJhJd9c7s")},
		{"TwoPair", deck.MustParseCards("KsKhTdTc5s")},
		{"OnePair", deck.MustParseCards("8s8hAdKc4s")},
		{"HighCard", deck.MustParseCards("AsKhQdJc9s")},
	}
}

// BenchmarkEvaluate7_RandomHands benchmarks 7-card evaluation with random hands
func BenchmarkEvaluate7_RandomHands(b *testing.B) {
	hands := Generate7CardHands(10000, 42) // Fixed seed for repeatability
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = Evaluate7(hands[i%len(hands)])
	}
}

// BenchmarkEvaluate7_TortureCases benchmarks 7-card evaluation with edge cases
func BenchmarkEvaluate7_TortureCases(b *testing.B) {
	tortureCases := GenerateTortureCases()

	// Extend 5-card hands to 7 cards for testing
	for _, testCase := range tortureCases {
		b.Run(testCase.name, func(b *testing.B) {
			// Add two random cards to make it 7 cards
			extendedCards := make([]deck.Card, len(testCase.cards))
			copy(extendedCards, testCase.cards)
			extendedCards = append(extendedCards,
				deck.NewCard(deck.Hearts, deck.Two),
				deck.NewCard(deck.Diamonds, deck.Three),
			)

			for i := 0; i < b.N; i++ {
				_ = Evaluate7(extendedCards)
			}
		})
	}
}
