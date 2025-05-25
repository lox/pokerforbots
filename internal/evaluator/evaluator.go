package evaluator

import (
	"sort"

	"github.com/lox/holdem-cli/internal/deck"
)

// EvaluateHand evaluates exactly 5 cards and returns the best hand
func EvaluateHand(cards []deck.Card) Hand {
	if len(cards) != 5 {
		panic("EvaluateHand requires exactly 5 cards")
	}

	// Sort cards by rank for easier analysis
	sortedCards := make([]deck.Card, len(cards))
	copy(sortedCards, cards)
	sort.Sort(cardsByRankDesc(sortedCards))

	// Check for flush
	isFlush := isFlush(sortedCards)

	// Check for straight
	isStraight, straightHigh := isStraight(sortedCards)

	// Count ranks
	rankCounts := countRanks(sortedCards)

	// Determine hand type
	if isFlush && isStraight {
		if straightHigh == deck.Ace && sortedCards[1].Rank == deck.King {
			// Royal flush (A, K, Q, J, T all same suit)
			return Hand{
				Rank:     RoyalFlush,
				Cards:    sortedCards,
				Kickers:  []deck.Rank{deck.Ace},
				HighCard: deck.Ace,
			}
		}
		// Straight flush
		return Hand{
			Rank:     StraightFlush,
			Cards:    sortedCards,
			Kickers:  []deck.Rank{straightHigh},
			HighCard: straightHigh,
		}
	}

	// Check for four of a kind
	if fourOfAKindRank := findNOfAKind(rankCounts, 4); fourOfAKindRank != 0 {
		kicker := findKicker(rankCounts, fourOfAKindRank)
		return Hand{
			Rank:     FourOfAKind,
			Cards:    sortedCards,
			Kickers:  []deck.Rank{fourOfAKindRank, kicker},
			HighCard: fourOfAKindRank,
		}
	}

	// Check for full house
	threeOfAKindRank := findNOfAKind(rankCounts, 3)
	pairRank := findNOfAKind(rankCounts, 2)
	if threeOfAKindRank != 0 && pairRank != 0 {
		return Hand{
			Rank:     FullHouse,
			Cards:    sortedCards,
			Kickers:  []deck.Rank{threeOfAKindRank, pairRank},
			HighCard: threeOfAKindRank,
		}
	}

	// Check for flush
	if isFlush {
		kickers := make([]deck.Rank, len(sortedCards))
		for i, card := range sortedCards {
			kickers[i] = card.Rank
		}
		return Hand{
			Rank:     Flush,
			Cards:    sortedCards,
			Kickers:  kickers,
			HighCard: sortedCards[0].Rank,
		}
	}

	// Check for straight
	if isStraight {
		return Hand{
			Rank:     Straight,
			Cards:    sortedCards,
			Kickers:  []deck.Rank{straightHigh},
			HighCard: straightHigh,
		}
	}

	// Check for three of a kind
	if threeOfAKindRank != 0 {
		kickers := findKickers(rankCounts, threeOfAKindRank, 2)
		return Hand{
			Rank:     ThreeOfAKind,
			Cards:    sortedCards,
			Kickers:  append([]deck.Rank{threeOfAKindRank}, kickers...),
			HighCard: threeOfAKindRank,
		}
	}

	// Check for pairs
	pairs := findAllPairs(rankCounts)
	if len(pairs) >= 2 {
		// Two pair - sort pairs by rank (highest first)
		sort.Sort(ranksByDesc(pairs))
		kicker := findKickers(rankCounts, pairs[0], 1)[0] // Exclude both pairs
		// Also exclude second pair from kickers
		if kicker == pairs[1] {
			for rank, count := range rankCounts {
				if count == 1 && rank != pairs[0] && rank != pairs[1] {
					kicker = rank
					break
				}
			}
		}
		return Hand{
			Rank:     TwoPair,
			Cards:    sortedCards,
			Kickers:  []deck.Rank{pairs[0], pairs[1], kicker},
			HighCard: pairs[0],
		}
	}

	if len(pairs) == 1 {
		// One pair
		kickers := findKickers(rankCounts, pairs[0], 3)
		return Hand{
			Rank:     OnePair,
			Cards:    sortedCards,
			Kickers:  append([]deck.Rank{pairs[0]}, kickers...),
			HighCard: pairs[0],
		}
	}

	// High card
	kickers := make([]deck.Rank, len(sortedCards))
	for i, card := range sortedCards {
		kickers[i] = card.Rank
	}
	return Hand{
		Rank:     HighCard,
		Cards:    sortedCards,
		Kickers:  kickers,
		HighCard: sortedCards[0].Rank,
	}
}

// FindBestHand finds the best 5-card hand from 7 cards (2 hole + 5 community)
func FindBestHand(cards []deck.Card) Hand {
	if len(cards) < 5 {
		panic("FindBestHand requires at least 5 cards")
	}

	if len(cards) == 5 {
		return EvaluateHand(cards)
	}

	// Generate all possible 5-card combinations
	var bestHand Hand
	combinations := generateCombinations(cards, 5)

	for i, combo := range combinations {
		hand := EvaluateHand(combo)
		if i == 0 || hand.IsStrongerThan(bestHand) {
			bestHand = hand
		}
	}

	return bestHand
}

// Helper functions

func isFlush(cards []deck.Card) bool {
	suit := cards[0].Suit
	for _, card := range cards[1:] {
		if card.Suit != suit {
			return false
		}
	}
	return true
}

func isStraight(cards []deck.Card) (bool, deck.Rank) {
	// Sort cards by rank
	sorted := make([]deck.Card, len(cards))
	copy(sorted, cards)
	sort.Sort(cardsByRankDesc(sorted))

	// Check for A-2-3-4-5 straight (wheel)
	if sorted[0].Rank == deck.Ace && sorted[1].Rank == deck.Five &&
		sorted[2].Rank == deck.Four && sorted[3].Rank == deck.Three &&
		sorted[4].Rank == deck.Two {
		return true, deck.Five // In wheel straight, 5 is the high card
	}

	// Check for regular straight
	for i := 1; i < len(sorted); i++ {
		if int(sorted[i-1].Rank)-int(sorted[i].Rank) != 1 {
			return false, 0
		}
	}

	return true, sorted[0].Rank
}

func countRanks(cards []deck.Card) map[deck.Rank]int {
	counts := make(map[deck.Rank]int)
	for _, card := range cards {
		counts[card.Rank]++
	}
	return counts
}

func findNOfAKind(rankCounts map[deck.Rank]int, n int) deck.Rank {
	for rank, count := range rankCounts {
		if count == n {
			return rank
		}
	}
	return 0
}

func findAllPairs(rankCounts map[deck.Rank]int) []deck.Rank {
	var pairs []deck.Rank
	for rank, count := range rankCounts {
		if count == 2 {
			pairs = append(pairs, rank)
		}
	}
	return pairs
}

func findKicker(rankCounts map[deck.Rank]int, excludeRank deck.Rank) deck.Rank {
	var bestKicker deck.Rank
	for rank, count := range rankCounts {
		if rank != excludeRank && count == 1 && rank > bestKicker {
			bestKicker = rank
		}
	}
	return bestKicker
}

func findKickers(rankCounts map[deck.Rank]int, excludeRank deck.Rank, numKickers int) []deck.Rank {
	var kickers []deck.Rank
	for rank, count := range rankCounts {
		if rank != excludeRank && count == 1 {
			kickers = append(kickers, rank)
		}
	}
	sort.Sort(ranksByDesc(kickers))

	if len(kickers) > numKickers {
		kickers = kickers[:numKickers]
	}

	return kickers
}

// generateCombinations generates all possible combinations of k cards from the given slice
func generateCombinations(cards []deck.Card, k int) [][]deck.Card {
	var result [][]deck.Card

	var generate func(start int, current []deck.Card)
	generate = func(start int, current []deck.Card) {
		if len(current) == k {
			combo := make([]deck.Card, len(current))
			copy(combo, current)
			result = append(result, combo)
			return
		}

		for i := start; i < len(cards); i++ {
			generate(i+1, append(current, cards[i]))
		}
	}

	generate(0, []deck.Card{})
	return result
}
