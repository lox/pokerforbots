package evaluator

import (
	"github.com/lox/holdem-cli/internal/deck"
	"math/rand"
)

// Range represents a range of possible opponent hands
type Range interface {
	SampleHand(availableCards []deck.Card) ([]deck.Card, bool)
}

// RandomRange represents any random two cards
type RandomRange struct{}

func (r RandomRange) SampleHand(availableCards []deck.Card) ([]deck.Card, bool) {
	if len(availableCards) < 2 {
		return nil, false
	}

	// Pick 2 random cards without creating full permutation
	idx1 := rand.Intn(len(availableCards))
	idx2 := rand.Intn(len(availableCards) - 1)
	if idx2 >= idx1 {
		idx2++
	}
	
	return []deck.Card{availableCards[idx1], availableCards[idx2]}, true
}

// TightRange represents a tight opponent (good hands only)
type TightRange struct{}

func (r TightRange) SampleHand(availableCards []deck.Card) ([]deck.Card, bool) {
	if len(availableCards) < 2 {
		return nil, false
	}

	attempts := 0
	for attempts < 100 {
		// Pick 2 random cards without creating full permutation  
		idx1 := rand.Intn(len(availableCards))
		idx2 := rand.Intn(len(availableCards) - 1)
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

	// Fallback to random if we can't find a tight hand
	return RandomRange{}.SampleHand(availableCards)
}

// LooseRange represents a loose opponent (wider range)
type LooseRange struct{}

func (r LooseRange) SampleHand(availableCards []deck.Card) ([]deck.Card, bool) {
	// For now, same as random - could be more sophisticated
	return RandomRange{}.SampleHand(availableCards)
}

func isTightHand(hand []deck.Card) bool {
	if len(hand) != 2 {
		return false
	}

	card1, card2 := hand[0], hand[1]

	// Pocket pairs
	if card1.Rank == card2.Rank {
		return true
	}

	// High cards (both > 9)
	if card1.Rank >= deck.Ten && card2.Rank >= deck.Ten {
		return true
	}

	// Suited connectors or one-gappers
	if card1.Suit == card2.Suit {
		gap := abs(card1.Rank - card2.Rank)
		if gap <= 2 && (card1.Rank >= deck.Seven || card2.Rank >= deck.Seven) {
			return true
		}
	}

	// Ace with decent kicker
	if (card1.Rank == deck.Ace && card2.Rank >= deck.Nine) ||
		(card2.Rank == deck.Ace && card1.Rank >= deck.Nine) {
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

// EstimateEquity calculates win percentage using Monte Carlo simulation
func EstimateEquity(hole []deck.Card, board []deck.Card, opponentRange Range, numSamples int) float64 {
	if len(hole) != 2 {
		return 0.0
	}
	if len(board) > 5 {
		return 0.0
	}

	wins := 0
	ties := 0
	validSamples := 0

	// Create deck of remaining cards
	usedCards := make(map[deck.Card]bool)
	for _, card := range hole {
		usedCards[card] = true
	}
	for _, card := range board {
		usedCards[card] = true
	}

	var availableCards []deck.Card
	for suit := deck.Spades; suit <= deck.Clubs; suit++ {
		for rank := deck.Two; rank <= deck.Ace; rank++ {
			card := deck.Card{Suit: suit, Rank: rank}
			if !usedCards[card] {
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
		oppHole, ok := opponentRange.SampleHand(availableCards)
		if !ok {
			continue
		}

		// Create temporary used cards map for this sample
		tempUsed := make(map[deck.Card]bool, len(hole)+len(board)+2)
		for _, card := range hole {
			tempUsed[card] = true
		}
		for _, card := range board {
			tempUsed[card] = true
		}
		for _, card := range oppHole {
			tempUsed[card] = true
		}

		// Complete the board
		copy(finalBoard[:len(board)], board)
		boardNeeded := 5 - len(board)
		filled := 0
		
		// Collect available cards for board completion
		boardCandidates := make([]deck.Card, 0, len(availableCards))
		for _, card := range availableCards {
			if !tempUsed[card] {
				boardCandidates = append(boardCandidates, card)
			}
		}
		
		// Randomly sample from candidates
		for filled < boardNeeded && filled < len(boardCandidates) {
			idx := rand.Intn(len(boardCandidates) - filled)
			finalBoard[len(board)+filled] = boardCandidates[idx]
			// Swap used card to end to avoid reselection
			boardCandidates[idx], boardCandidates[len(boardCandidates)-1-filled] = 
				boardCandidates[len(boardCandidates)-1-filled], boardCandidates[idx]
			filled++
		}

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

// EvaluateHandStrength converts equity to a score for AI decision making
func EvaluateHandStrength(hole []deck.Card, board []deck.Card) int {
	if len(hole) == 2 && len(board) >= 0 && len(board) <= 5 {
		// Calculate equity against random opponent
		equity := EstimateEquity(hole, board, RandomRange{}, 1000)

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
