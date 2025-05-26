package evaluator

import (
	"math/rand"
	"github.com/lox/holdem-cli/internal/deck"
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
	
	// Pick 2 random cards
	indices := rand.Perm(len(availableCards))
	return []deck.Card{availableCards[indices[0]], availableCards[indices[1]]}, true
}

// TightRange represents a tight opponent (good hands only)
type TightRange struct{}

func (r TightRange) SampleHand(availableCards []deck.Card) ([]deck.Card, bool) {
	if len(availableCards) < 2 {
		return nil, false
	}
	
	attempts := 0
	for attempts < 100 {
		// Pick 2 random cards
		indices := rand.Perm(len(availableCards))
		hand := []deck.Card{availableCards[indices[0]], availableCards[indices[1]]}
		
		// Check if it's a tight range hand (pairs, suited connectors, high cards)
		if isTightHand(hand) {
			return hand, true
		}
		attempts++
	}
	
	// Fallback to random if we can't find a tight hand
	indices := rand.Perm(len(availableCards))
	return []deck.Card{availableCards[indices[0]], availableCards[indices[1]]}, true
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
	
	for i := 0; i < numSamples; i++ {
		// Shuffle available cards
		shuffled := make([]deck.Card, len(availableCards))
		copy(shuffled, availableCards)
		for i := len(shuffled) - 1; i > 0; i-- {
			j := rand.Intn(i + 1)
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		}
		
		cardIndex := 0
		
		// Sample opponent hand
		oppHole, ok := opponentRange.SampleHand(shuffled[cardIndex:])
		if !ok {
			continue
		}
		cardIndex += 2
		
		// Complete the board (5 - len(board) more cards needed)
		finalBoard := make([]deck.Card, len(board))
		copy(finalBoard, board)
		
		cardsNeeded := 5 - len(board)
		for j := 0; j < cardsNeeded; j++ {
			if cardIndex >= len(shuffled) {
				break
			}
			finalBoard = append(finalBoard, shuffled[cardIndex])
			cardIndex++
		}
		
		if len(finalBoard) != 5 {
			continue
		}
		
		// Evaluate both hands
		heroHand := make([]deck.Card, 0, 7)
		heroHand = append(heroHand, hole...)
		heroHand = append(heroHand, finalBoard...)
		
		oppHand := make([]deck.Card, 0, 7)
		oppHand = append(oppHand, oppHole...)
		oppHand = append(oppHand, finalBoard...)
		
		// Ensure both hands have exactly 7 cards
		if len(heroHand) != 7 || len(oppHand) != 7 {
			continue
		}
		
		heroScore := Evaluate7(heroHand)
		oppScore := Evaluate7(oppHand)
		
		// Compare results (lower score = better hand)
		if heroScore < oppScore {
			wins++
		} else if heroScore == oppScore {
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
		score := int((1.0 - equity) * 9000000) + 1000000
		
		return score
	}
	
	// Fallback for invalid input
	return (HighCardType << 20) | 0xFFFFF
}