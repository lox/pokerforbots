package evaluator

import (
	"math/rand"
	"testing"

	"github.com/lox/pokerforbots/internal/deck"
)

// Helper function to create cards
func card(suit int, rank int) deck.Card {
	return deck.Card{Suit: suit, Rank: rank}
}

// Benchmark equity estimation on different board states
func BenchmarkEquityPreflop(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	hole := []deck.Card{
		card(deck.Spades, deck.Ace),
		card(deck.Hearts, deck.King),
	}
	board := []deck.Card{} // Empty board (preflop)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateEquity(hole, board, RandomRange{}, 1000, rng)
	}
}

func BenchmarkEquityFlop(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	hole := []deck.Card{
		card(deck.Spades, deck.Ace),
		card(deck.Hearts, deck.King),
	}
	board := []deck.Card{
		card(deck.Diamonds, deck.Ace),
		card(deck.Clubs, deck.Seven),
		card(deck.Spades, deck.Two),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateEquity(hole, board, RandomRange{}, 1000, rng)
	}
}

func BenchmarkEquityTurn(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	hole := []deck.Card{
		card(deck.Spades, deck.Ace),
		card(deck.Hearts, deck.King),
	}
	board := []deck.Card{
		card(deck.Diamonds, deck.Ace),
		card(deck.Clubs, deck.Seven),
		card(deck.Spades, deck.Two),
		card(deck.Hearts, deck.Queen),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateEquity(hole, board, RandomRange{}, 1000, rng)
	}
}

func BenchmarkEquityRiver(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	hole := []deck.Card{
		card(deck.Spades, deck.Ace),
		card(deck.Hearts, deck.King),
	}
	board := []deck.Card{
		card(deck.Diamonds, deck.Ace),
		card(deck.Clubs, deck.Seven),
		card(deck.Spades, deck.Two),
		card(deck.Hearts, deck.Queen),
		card(deck.Diamonds, deck.Jack),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateEquity(hole, board, RandomRange{}, 1000, rng)
	}
}

// Benchmark parallel vs sequential implementations
func BenchmarkEquitySequential(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	hole := []deck.Card{
		card(deck.Spades, deck.Ace),
		card(deck.Hearts, deck.King),
	}
	board := []deck.Card{
		card(deck.Diamonds, deck.Ace),
		card(deck.Clubs, deck.Seven),
		card(deck.Spades, deck.Two),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateEquitySequential(hole, board, RandomRange{}, 10000, rng)
	}
}

func BenchmarkEquityParallel(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	hole := []deck.Card{
		card(deck.Spades, deck.Ace),
		card(deck.Hearts, deck.King),
	}
	board := []deck.Card{
		card(deck.Diamonds, deck.Ace),
		card(deck.Clubs, deck.Seven),
		card(deck.Spades, deck.Two),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateEquityParallel(hole, board, RandomRange{}, 10000, rng)
	}
}

// Benchmark different opponent ranges
func BenchmarkEquityVsRandomRange(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	hole := []deck.Card{
		card(deck.Spades, deck.Ace),
		card(deck.Hearts, deck.King),
	}
	board := []deck.Card{
		card(deck.Diamonds, deck.Ace),
		card(deck.Clubs, deck.Seven),
		card(deck.Spades, deck.Two),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateEquity(hole, board, RandomRange{}, 1000, rng)
	}
}

func BenchmarkEquityVsTightRange(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	hole := []deck.Card{
		card(deck.Spades, deck.Ace),
		card(deck.Hearts, deck.King),
	}
	board := []deck.Card{
		card(deck.Diamonds, deck.Ace),
		card(deck.Clubs, deck.Seven),
		card(deck.Spades, deck.Two),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateEquity(hole, board, TightRange{}, 1000, rng)
	}
}

func BenchmarkEquityVsMediumRange(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	hole := []deck.Card{
		card(deck.Spades, deck.Ace),
		card(deck.Hearts, deck.King),
	}
	board := []deck.Card{
		card(deck.Diamonds, deck.Ace),
		card(deck.Clubs, deck.Seven),
		card(deck.Spades, deck.Two),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateEquity(hole, board, MediumRange{}, 1000, rng)
	}
}

func BenchmarkEquityVsLooseRange(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	hole := []deck.Card{
		card(deck.Spades, deck.Ace),
		card(deck.Hearts, deck.King),
	}
	board := []deck.Card{
		card(deck.Diamonds, deck.Ace),
		card(deck.Clubs, deck.Seven),
		card(deck.Spades, deck.Two),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateEquity(hole, board, LooseRange{}, 1000, rng)
	}
}

// Benchmark different sample sizes
func BenchmarkEquitySamples100(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	hole := []deck.Card{
		card(deck.Spades, deck.Ace),
		card(deck.Hearts, deck.King),
	}
	board := []deck.Card{
		card(deck.Diamonds, deck.Ace),
		card(deck.Clubs, deck.Seven),
		card(deck.Spades, deck.Two),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateEquity(hole, board, RandomRange{}, 100, rng)
	}
}

func BenchmarkEquitySamples1000(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	hole := []deck.Card{
		card(deck.Spades, deck.Ace),
		card(deck.Hearts, deck.King),
	}
	board := []deck.Card{
		card(deck.Diamonds, deck.Ace),
		card(deck.Clubs, deck.Seven),
		card(deck.Spades, deck.Two),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateEquity(hole, board, RandomRange{}, 1000, rng)
	}
}

func BenchmarkEquitySamples10000(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	hole := []deck.Card{
		card(deck.Spades, deck.Ace),
		card(deck.Hearts, deck.King),
	}
	board := []deck.Card{
		card(deck.Diamonds, deck.Ace),
		card(deck.Clubs, deck.Seven),
		card(deck.Spades, deck.Two),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EstimateEquity(hole, board, RandomRange{}, 10000, rng)
	}
}

// Benchmark hand strength evaluation
func BenchmarkEvaluateHandStrength(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	hole := []deck.Card{
		card(deck.Spades, deck.Ace),
		card(deck.Hearts, deck.King),
	}
	board := []deck.Card{
		card(deck.Diamonds, deck.Ace),
		card(deck.Clubs, deck.Seven),
		card(deck.Spades, deck.Two),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EvaluateHandStrength(hole, board, rng)
	}
}

// Benchmark CardSet operations
func BenchmarkCardSetOperations(b *testing.B) {
	var cardSet CardSet
	cards := []deck.Card{
		card(deck.Spades, deck.Ace),
		card(deck.Hearts, deck.King),
		card(deck.Diamonds, deck.Queen),
		card(deck.Clubs, deck.Jack),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cardSet = 0
		for _, c := range cards {
			cardSet.Add(c)
		}
		for _, c := range cards {
			_ = cardSet.Contains(c)
		}
	}
}

// Benchmark range sampling
func BenchmarkRandomRangeSampling(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	availableCards := make([]deck.Card, 0, 52)
	for suit := deck.Spades; suit <= deck.Clubs; suit++ {
		for rank := deck.Two; rank <= deck.Ace; rank++ {
			availableCards = append(availableCards, deck.Card{Suit: suit, Rank: rank})
		}
	}

	r := RandomRange{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.SampleHand(availableCards, rng)
	}
}

func BenchmarkTightRangeSampling(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	availableCards := make([]deck.Card, 0, 52)
	for suit := deck.Spades; suit <= deck.Clubs; suit++ {
		for rank := deck.Two; rank <= deck.Ace; rank++ {
			availableCards = append(availableCards, deck.Card{Suit: suit, Rank: rank})
		}
	}

	r := TightRange{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = r.SampleHand(availableCards, rng)
	}
}
