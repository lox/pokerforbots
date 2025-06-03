package evaluator

import (
	"context"
	"math/rand"
	"runtime"
	"sync"

	"github.com/lox/pokerforbots/internal/deck"
	"golang.org/x/sync/errgroup"
)

// workerResult holds the results from a Monte Carlo worker
type workerResult struct {
	wins         int
	ties         int
	validSamples int
}

// CardSet represents a set of cards using a bitset for fast operations
// Each card maps to a bit: index = (rank-2)*4 + suit
type CardSet uint64

// cardIndex converts a card to its bit index (0-51)
func cardIndex(card deck.Card) int {
	return (card.Rank-deck.Two)*4 + card.Suit
}

// Add adds a card to the set
func (cs *CardSet) Add(card deck.Card) {
	*cs |= 1 << cardIndex(card)
}

// Contains checks if a card is in the set
func (cs CardSet) Contains(card deck.Card) bool {
	return cs&(1<<cardIndex(card)) != 0
}

// NewCardSet creates a CardSet from a slice of cards
func NewCardSet(cards []deck.Card) CardSet {
	var cs CardSet
	for _, card := range cards {
		cs.Add(card)
	}
	return cs
}

// Slice pool for reusable boardCandidates allocation
var boardCandidatesPool = sync.Pool{
	New: func() interface{} {
		return make([]deck.Card, 0, 52)
	},
}

// Range represents a range of possible opponent hands
type Range interface {
	SampleHand(availableCards []deck.Card, rng *rand.Rand) ([]deck.Card, bool)
}

// RandomRange represents any random two cards
type RandomRange struct{}

func (r RandomRange) SampleHand(availableCards []deck.Card, rng *rand.Rand) ([]deck.Card, bool) {
	if len(availableCards) < 2 {
		return nil, false
	}

	// Pick 2 random cards without creating full permutation
	idx1 := rng.Intn(len(availableCards))
	idx2 := rng.Intn(len(availableCards) - 1)
	if idx2 >= idx1 {
		idx2++
	}

	return []deck.Card{availableCards[idx1], availableCards[idx2]}, true
}

// TightRange represents a tight opponent (good hands only)
type TightRange struct{}

func (r TightRange) SampleHand(availableCards []deck.Card, rng *rand.Rand) ([]deck.Card, bool) {
	if len(availableCards) < 2 {
		return nil, false
	}

	attempts := 0
	for attempts < 200 { // More attempts for better tight range
		// Pick 2 random cards without creating full permutation
		idx1 := rng.Intn(len(availableCards))
		idx2 := rng.Intn(len(availableCards) - 1)
		if idx2 >= idx1 {
			idx2++
		}
		hand := []deck.Card{availableCards[idx1], availableCards[idx2]}

		// Check if it's a tight range hand (pairs, suited connectors, high cards)
		if isTightHand(hand) {
			return hand, true
		}
		attempts++
	}

	// Fallback to medium range if we can't find a tight hand (not random)
	return MediumRange{}.SampleHand(availableCards, rng)
}

// MediumRange represents a medium opponent (moderate range between tight and loose)
type MediumRange struct{}

func (r MediumRange) SampleHand(availableCards []deck.Card, rng *rand.Rand) ([]deck.Card, bool) {
	// Medium range: looser than tight, tighter than random
	// Accept medium hands with some probability
	maxAttempts := 50
	attempts := 0

	for attempts < maxAttempts {
		hand, ok := RandomRange{}.SampleHand(availableCards, rng)
		if !ok {
			return hand, false
		}

		// Accept tight hands always
		if isTightHand(hand) {
			return hand, true
		}

		// Accept medium hands with 60% probability
		if isMediumHand(hand) && rng.Float64() < 0.6 {
			return hand, true
		}

		attempts++
	}

	// Fallback to random if we can't find a suitable hand
	return RandomRange{}.SampleHand(availableCards, rng)
}

// LooseRange represents a loose opponent (wider range)
type LooseRange struct{}

func (r LooseRange) SampleHand(availableCards []deck.Card, rng *rand.Rand) ([]deck.Card, bool) {
	// For now, same as random - could be more sophisticated
	return RandomRange{}.SampleHand(availableCards, rng)
}

func isTightHand(hand []deck.Card) bool {
	if len(hand) != 2 {
		return false
	}

	card1, card2 := hand[0], hand[1]

	// Pocket pairs (TT+)
	if card1.Rank == card2.Rank && card1.Rank >= deck.Ten {
		return true
	}

	// High cards (both Jack+)
	if card1.Rank >= deck.Jack && card2.Rank >= deck.Jack {
		return true
	}

	// Premium suited connectors (T9s+ only)
	if card1.Suit == card2.Suit {
		gap := abs(card1.Rank - card2.Rank)
		if gap <= 1 && (card1.Rank >= deck.Ten && card2.Rank >= deck.Nine) ||
			(card2.Rank >= deck.Ten && card1.Rank >= deck.Nine) {
			return true
		}
	}

	// Ace with good kicker (AT+)
	if (card1.Rank == deck.Ace && card2.Rank >= deck.Ten) ||
		(card2.Rank == deck.Ace && card1.Rank >= deck.Ten) {
		return true
	}

	return false
}

func isMediumHand(hand []deck.Card) bool {
	if len(hand) != 2 {
		return false
	}

	// If it's already a tight hand, don't double count
	if isTightHand(hand) {
		return false
	}

	card1, card2 := hand[0], hand[1]

	// Medium pocket pairs (66-99)
	if card1.Rank == card2.Rank && card1.Rank >= 6 && card1.Rank <= 9 {
		return true
	}

	// One high card (8+) with decent kicker
	if (card1.Rank >= 8 && card2.Rank >= 6) || (card2.Rank >= 8 && card1.Rank >= 6) {
		return true
	}

	// Suited hands with one medium card
	if card1.Suit == card2.Suit {
		if card1.Rank >= 7 || card2.Rank >= 7 {
			return true
		}
	}

	// Ace with any kicker (not covered by tight)
	if card1.Rank == deck.Ace || card2.Rank == deck.Ace {
		return true
	}

	return false
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// EstimateEquity calculates win percentage using parallel Monte Carlo simulation
func EstimateEquity(hole []deck.Card, board []deck.Card, opponentRange Range, numSamples int, rng *rand.Rand) float64 {
	// Use parallel version for larger sample sizes where overhead is worth it
	if numSamples >= 500 {
		return EstimateEquityParallel(hole, board, opponentRange, numSamples, rng)
	}
	// Use sequential version for small sample sizes
	return EstimateEquitySequential(hole, board, opponentRange, numSamples, rng)
}

// EstimateEquitySequential is the original sequential implementation
func EstimateEquitySequential(hole []deck.Card, board []deck.Card, opponentRange Range, numSamples int, rng *rand.Rand) float64 {
	if len(hole) != 2 {
		return 0.0
	}
	if len(board) > 5 {
		return 0.0
	}

	wins := 0
	ties := 0
	validSamples := 0

	// Create bitset of used cards (much faster than map)
	var usedCards CardSet
	for _, card := range hole {
		usedCards.Add(card)
	}
	for _, card := range board {
		usedCards.Add(card)
	}

	// Pre-build available cards slice once
	var availableCards []deck.Card
	for suit := deck.Spades; suit <= deck.Clubs; suit++ {
		for rank := deck.Two; rank <= deck.Ace; rank++ {
			card := deck.Card{Suit: suit, Rank: rank}
			if !usedCards.Contains(card) {
				availableCards = append(availableCards, card)
			}
		}
	}

	// Pre-allocate reusable slices
	finalBoard := make([]deck.Card, 5)
	heroHand := make([]deck.Card, 7)
	oppHand := make([]deck.Card, 7)

	for i := 0; i < numSamples; i++ {
		// Sample opponent hand directly from available cards
		oppHole, ok := opponentRange.SampleHand(availableCards, rng)
		if !ok {
			continue
		}

		// Create temporary used cards bitset for this sample (much faster than map)
		tempUsed := usedCards // Copy the base set
		for _, card := range oppHole {
			tempUsed.Add(card)
		}

		// Complete the board
		copy(finalBoard[:len(board)], board)
		boardNeeded := 5 - len(board)
		filled := 0

		// Get reusable boardCandidates slice from pool
		boardCandidates := boardCandidatesPool.Get().([]deck.Card)
		boardCandidates = boardCandidates[:0] // Reset length but keep capacity

		// Collect available cards for board completion (fast bitset lookup)
		for _, card := range availableCards {
			if !tempUsed.Contains(card) {
				boardCandidates = append(boardCandidates, card)
			}
		}

		// Randomly sample from candidates
		for filled < boardNeeded && filled < len(boardCandidates) {
			idx := rng.Intn(len(boardCandidates) - filled)
			finalBoard[len(board)+filled] = boardCandidates[idx]
			// Swap used card to end to avoid reselection
			boardCandidates[idx], boardCandidates[len(boardCandidates)-1-filled] =
				boardCandidates[len(boardCandidates)-1-filled], boardCandidates[idx]
			filled++
		}

		// Return slice to pool for reuse
		boardCandidatesPool.Put(&boardCandidates)

		if len(finalBoard) != 5 {
			continue
		}

		// Evaluate both hands using pre-allocated slices
		copy(heroHand[:2], hole)
		copy(heroHand[2:], finalBoard)

		copy(oppHand[:2], oppHole)
		copy(oppHand[2:], finalBoard)

		// Ensure both hands have exactly 7 cards
		if len(heroHand) != 7 || len(oppHand) != 7 {
			continue
		}

		heroScore := Evaluate7(heroHand)
		oppScore := Evaluate7(oppHand)

		// Compare results using HandRank.Compare method
		comparison := heroScore.Compare(oppScore)
		if comparison > 0 {
			wins++
		} else if comparison == 0 {
			ties++
		}

		validSamples++
	}

	if validSamples == 0 {
		return 0.0
	}

	return (float64(wins) + float64(ties)/2.0) / float64(validSamples)
}

// EstimateEquityParallel calculates win percentage using parallel Monte Carlo simulation
func EstimateEquityParallel(hole []deck.Card, board []deck.Card, opponentRange Range, numSamples int, rng *rand.Rand) float64 {
	if len(hole) != 2 {
		return 0.0
	}
	if len(board) > 5 {
		return 0.0
	}

	// Determine optimal worker count (don't exceed CPU cores)
	workers := runtime.NumCPU()
	if workers > 8 {
		workers = 8 // Cap at 8 for diminishing returns
	}

	// Divide samples among workers
	samplesPerWorker := numSamples / workers
	remainder := numSamples % workers

	// Pre-build available cards once (shared by all workers)
	var usedCards CardSet
	for _, card := range hole {
		usedCards.Add(card)
	}
	for _, card := range board {
		usedCards.Add(card)
	}

	var availableCards []deck.Card
	for suit := deck.Spades; suit <= deck.Clubs; suit++ {
		for rank := deck.Two; rank <= deck.Ace; rank++ {
			card := deck.Card{Suit: suit, Rank: rank}
			if !usedCards.Contains(card) {
				availableCards = append(availableCards, card)
			}
		}
	}

	// Use errgroup to manage workers
	g, ctx := errgroup.WithContext(context.Background())
	results := make(chan workerResult, workers)

	// Launch workers
	for w := 0; w < workers; w++ {
		workerSamples := samplesPerWorker
		if w < remainder {
			workerSamples++ // Distribute remainder samples
		}

		// Create independent RNG for each worker to avoid contention
		workerSeed := rng.Int63()

		g.Go(func() error {
			workerRng := rand.New(rand.NewSource(workerSeed))
			result := runEquityWorker(hole, board, availableCards, opponentRange, workerSamples, workerRng)

			select {
			case results <- result:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}

	// Collect results
	totalWins := 0
	totalTies := 0
	totalValidSamples := 0

	go func() {
		defer close(results)
		g.Wait()
	}()

	for result := range results {
		totalWins += result.wins
		totalTies += result.ties
		totalValidSamples += result.validSamples
	}

	if err := g.Wait(); err != nil {
		// Fallback to sequential if parallel fails
		return EstimateEquitySequential(hole, board, opponentRange, numSamples, rng)
	}

	if totalValidSamples == 0 {
		return 0.0
	}

	return (float64(totalWins) + float64(totalTies)/2.0) / float64(totalValidSamples)
}

// runEquityWorker runs Monte Carlo simulation for a worker
func runEquityWorker(hole []deck.Card, board []deck.Card, availableCards []deck.Card,
	opponentRange Range, numSamples int, rng *rand.Rand) workerResult {

	wins := 0
	ties := 0
	validSamples := 0

	// Pre-allocate reusable slices for this worker
	finalBoard := make([]deck.Card, 5)
	heroHand := make([]deck.Card, 7)
	oppHand := make([]deck.Card, 7)

	// Create base used cards for this worker
	var baseUsedCards CardSet
	for _, card := range hole {
		baseUsedCards.Add(card)
	}
	for _, card := range board {
		baseUsedCards.Add(card)
	}

	for i := 0; i < numSamples; i++ {
		// Sample opponent hand
		oppHole, ok := opponentRange.SampleHand(availableCards, rng)
		if !ok {
			continue
		}

		// Create temporary used cards bitset for this sample
		tempUsed := baseUsedCards
		for _, card := range oppHole {
			tempUsed.Add(card)
		}

		// Complete the board
		copy(finalBoard[:len(board)], board)
		boardNeeded := 5 - len(board)
		filled := 0

		// Get reusable boardCandidates slice from pool
		boardCandidates := boardCandidatesPool.Get().([]deck.Card)
		boardCandidates = boardCandidates[:0]

		for _, card := range availableCards {
			if !tempUsed.Contains(card) {
				boardCandidates = append(boardCandidates, card)
			}
		}

		// Randomly sample from candidates
		for filled < boardNeeded && filled < len(boardCandidates) {
			idx := rng.Intn(len(boardCandidates) - filled)
			finalBoard[len(board)+filled] = boardCandidates[idx]
			boardCandidates[idx], boardCandidates[len(boardCandidates)-1-filled] =
				boardCandidates[len(boardCandidates)-1-filled], boardCandidates[idx]
			filled++
		}

		boardCandidatesPool.Put(&boardCandidates)

		if len(finalBoard) != 5 {
			continue
		}

		// Evaluate both hands
		copy(heroHand[:2], hole)
		copy(heroHand[2:], finalBoard)
		copy(oppHand[:2], oppHole)
		copy(oppHand[2:], finalBoard)

		if len(heroHand) != 7 || len(oppHand) != 7 {
			continue
		}

		heroScore := Evaluate7(heroHand)
		oppScore := Evaluate7(oppHand)

		comparison := heroScore.Compare(oppScore)
		if comparison > 0 {
			wins++
		} else if comparison == 0 {
			ties++
		}

		validSamples++
	}

	return workerResult{wins: wins, ties: ties, validSamples: validSamples}
}

// EvaluateHandStrength converts equity to a score for AI decision making
func EvaluateHandStrength(hole []deck.Card, board []deck.Card, rng *rand.Rand) int {
	if len(hole) == 2 && len(board) >= 0 && len(board) <= 5 {
		// Calculate equity against random opponent
		equity := EstimateEquity(hole, board, RandomRange{}, 1000, rng)

		// Convert equity to a score where lower = better
		// 100% equity = score 1,000,000 (very strong)
		// 50% equity = score 5,000,000 (medium)
		// 0% equity = score 10,000,000 (very weak)
		score := int((1.0-equity)*9000000) + 1000000

		return score
	}

	// Fallback for invalid input
	return (HighCardType << 20) | 0xFFFFF
}
