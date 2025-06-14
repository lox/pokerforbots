package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/lox/pokerforbots/internal/deck"
	"github.com/lox/pokerforbots/internal/evaluator"
)

func main() {
	BenchmarkMinimal()
}

// generate7CardHands creates N random 7-card hands using a fixed seed
func generate7CardHands(source rand.Source, n int) [][]deck.Card {
	rng := rand.New(source)
	hands := make([][]deck.Card, n)

	for i := 0; i < n; i++ {
		hands[i] = generate7CardHand(rng)
	}

	return hands
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

// BenchmarkMinimal runs a minimal benchmark comparable across languages
func BenchmarkMinimal() {
	// Generate 10,000 hands with seed 42 (matching Zig)
	fmt.Println("Generating 10,000 hands with seed 42...")
	hands := generate7CardHands(rand.NewSource(42), 10000)

	// Warm-up run through all hands once
	fmt.Println("Warming up...")
	dummy := 0
	for _, hand := range hands {
		result := evaluator.Evaluate7(hand)
		dummy += int(result) // Prevent optimization
	}

	// 3 measurement runs
	const numRuns = 3
	results := make([]time.Duration, numRuns)
	totalOps := make([]int, numRuns)

	fmt.Printf("Running %d measurement runs...\n", numRuns)

	for run := 0; run < numRuns; run++ {
		start := time.Now()
		ops := 0

		// Run for approximately 1 second
		runStart := time.Now()
		for time.Since(runStart) < time.Second {
			for _, hand := range hands {
				result := evaluator.Evaluate7(hand)
				dummy += int(result) // Prevent optimization
				ops++
			}
		}

		elapsed := time.Since(start)
		results[run] = elapsed
		totalOps[run] = ops

		nsPerOp := float64(elapsed.Nanoseconds()) / float64(ops)
		fmt.Printf("Run %d: %d ops in %v (%.2f ns/op)\n",
			run+1, ops, elapsed, nsPerOp)
	}

	// Calculate and print average
	var totalTime time.Duration
	var totalOperations int

	for i := 0; i < numRuns; i++ {
		totalTime += results[i]
		totalOperations += totalOps[i]
	}

	avgNsPerOp := float64(totalTime.Nanoseconds()) / float64(totalOperations)
	avgTime := totalTime / time.Duration(numRuns)
	avgOps := totalOperations / numRuns

	fmt.Printf("\nAverage: %d ops in %v (%.2f ns/op)\n",
		avgOps, avgTime, avgNsPerOp)

	// Performance summary
	evaluationsPerSecond := 1_000_000_000.0 / avgNsPerOp / 1_000_000.0 // Convert to millions
	fmt.Println("\n=== Performance Summary ===")
	fmt.Printf("%.2f ns/op (average across %d runs)\n", avgNsPerOp, numRuns)
	fmt.Printf("%.1fM evaluations/second\n", evaluationsPerSecond)

	// Use dummy to prevent optimization
	if dummy == 0 {
		fmt.Println("Unexpected dummy value")
	}
}
