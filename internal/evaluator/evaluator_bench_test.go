package evaluator

import (
	"math/rand"
	"testing"

	"github.com/lox/holdem-cli/internal/deck"
)

// Generate5CardHands creates N random 5-card hands using a fixed seed
func Generate5CardHands(n int, seed int64) [][]deck.Card {
	rng := rand.New(rand.NewSource(seed))
	hands := make([][]deck.Card, n)
	
	for i := 0; i < n; i++ {
		hands[i] = generate5CardHand(rng)
	}
	
	return hands
}

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
			allCards = append(allCards, deck.Card{Suit: suit, Rank: rank})
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
		{
			name: "RoyalFlush",
			cards: []deck.Card{
				{Suit: deck.Spades, Rank: deck.Ace},
				{Suit: deck.Spades, Rank: deck.King},
				{Suit: deck.Spades, Rank: deck.Queen},
				{Suit: deck.Spades, Rank: deck.Jack},
				{Suit: deck.Spades, Rank: deck.Ten},
			},
		},
		{
			name: "StraightFlush",
			cards: []deck.Card{
				{Suit: deck.Hearts, Rank: deck.Nine},
				{Suit: deck.Hearts, Rank: deck.Eight},
				{Suit: deck.Hearts, Rank: deck.Seven},
				{Suit: deck.Hearts, Rank: deck.Six},
				{Suit: deck.Hearts, Rank: deck.Five},
			},
		},
		{
			name: "FourOfAKind",
			cards: []deck.Card{
				{Suit: deck.Spades, Rank: deck.Ace},
				{Suit: deck.Hearts, Rank: deck.Ace},
				{Suit: deck.Diamonds, Rank: deck.Ace},
				{Suit: deck.Clubs, Rank: deck.Ace},
				{Suit: deck.Spades, Rank: deck.King},
			},
		},
		{
			name: "FullHouse",
			cards: []deck.Card{
				{Suit: deck.Spades, Rank: deck.King},
				{Suit: deck.Hearts, Rank: deck.King},
				{Suit: deck.Diamonds, Rank: deck.King},
				{Suit: deck.Clubs, Rank: deck.Queen},
				{Suit: deck.Spades, Rank: deck.Queen},
			},
		},
		{
			name: "Flush",
			cards: []deck.Card{
				{Suit: deck.Clubs, Rank: deck.Ace},
				{Suit: deck.Clubs, Rank: deck.Jack},
				{Suit: deck.Clubs, Rank: deck.Nine},
				{Suit: deck.Clubs, Rank: deck.Seven},
				{Suit: deck.Clubs, Rank: deck.Five},
			},
		},
		{
			name: "Straight",
			cards: []deck.Card{
				{Suit: deck.Spades, Rank: deck.Ten},
				{Suit: deck.Hearts, Rank: deck.Nine},
				{Suit: deck.Diamonds, Rank: deck.Eight},
				{Suit: deck.Clubs, Rank: deck.Seven},
				{Suit: deck.Spades, Rank: deck.Six},
			},
		},
		{
			name: "WheelStraight",
			cards: []deck.Card{
				{Suit: deck.Spades, Rank: deck.Ace},
				{Suit: deck.Hearts, Rank: deck.Five},
				{Suit: deck.Diamonds, Rank: deck.Four},
				{Suit: deck.Clubs, Rank: deck.Three},
				{Suit: deck.Spades, Rank: deck.Two},
			},
		},
		{
			name: "ThreeOfAKind",
			cards: []deck.Card{
				{Suit: deck.Spades, Rank: deck.Jack},
				{Suit: deck.Hearts, Rank: deck.Jack},
				{Suit: deck.Diamonds, Rank: deck.Jack},
				{Suit: deck.Clubs, Rank: deck.Nine},
				{Suit: deck.Spades, Rank: deck.Seven},
			},
		},
		{
			name: "TwoPair",
			cards: []deck.Card{
				{Suit: deck.Spades, Rank: deck.King},
				{Suit: deck.Hearts, Rank: deck.King},
				{Suit: deck.Diamonds, Rank: deck.Ten},
				{Suit: deck.Clubs, Rank: deck.Ten},
				{Suit: deck.Spades, Rank: deck.Five},
			},
		},
		{
			name: "OnePair",
			cards: []deck.Card{
				{Suit: deck.Spades, Rank: deck.Eight},
				{Suit: deck.Hearts, Rank: deck.Eight},
				{Suit: deck.Diamonds, Rank: deck.Ace},
				{Suit: deck.Clubs, Rank: deck.King},
				{Suit: deck.Spades, Rank: deck.Four},
			},
		},
		{
			name: "HighCard",
			cards: []deck.Card{
				{Suit: deck.Spades, Rank: deck.Ace},
				{Suit: deck.Hearts, Rank: deck.King},
				{Suit: deck.Diamonds, Rank: deck.Queen},
				{Suit: deck.Clubs, Rank: deck.Jack},
				{Suit: deck.Spades, Rank: deck.Nine},
			},
		},
	}
}

// BenchmarkEvaluate5_RandomHands benchmarks 5-card evaluation with random hands
func BenchmarkEvaluate5_RandomHands(b *testing.B) {
	hands := Generate5CardHands(10000, 42) // Fixed seed for repeatability
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_ = Evaluate5(hands[i%len(hands)])
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

// BenchmarkEvaluate5_TortureCases benchmarks 5-card evaluation with edge cases
func BenchmarkEvaluate5_TortureCases(b *testing.B) {
	tortureCases := GenerateTortureCases()

	for _, testCase := range tortureCases {
		b.Run(testCase.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = Evaluate5(testCase.cards)
			}
		})
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
				deck.Card{Suit: deck.Hearts, Rank: deck.Two},
				deck.Card{Suit: deck.Diamonds, Rank: deck.Three},
			)
			
			for i := 0; i < b.N; i++ {
				_ = Evaluate7(extendedCards)
			}
		})
	}
}

// BenchmarkHandStrengthComparison benchmarks HandStrength comparison operations
func BenchmarkHandStrengthComparison(b *testing.B) {
	strength1 := HandStrength{
		Category: OnePair,
		Tiebreak: []int{int(deck.Ace), int(deck.King), int(deck.Queen)},
	}
	strength2 := HandStrength{
		Category: OnePair,
		Tiebreak: []int{int(deck.Ace), int(deck.King), int(deck.Jack)},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = strength1.Compare(strength2)
	}
}