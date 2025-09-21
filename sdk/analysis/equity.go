// Package analysis provides poker analysis tools including equity evaluation,
// range analysis, and statistical calculations.
//
// This implementation uses efficient bit-packed poker.Hand representations
// and is ported from the proven Zig poker evaluation library.
package analysis

import (
	"math"
	"math/rand"

	"github.com/lox/pokerforbots/poker"
)

// EquityResult represents the result of an equity calculation
type EquityResult struct {
	Wins             uint32
	Ties             uint32
	TotalSimulations uint32
}

// WinRate returns the win rate as a percentage (0.0 to 1.0)
func (e EquityResult) WinRate() float64 {
	if e.TotalSimulations == 0 {
		return 0.0
	}
	return float64(e.Wins) / float64(e.TotalSimulations)
}

// TieRate returns the tie rate as a percentage (0.0 to 1.0)
func (e EquityResult) TieRate() float64 {
	if e.TotalSimulations == 0 {
		return 0.0
	}
	return float64(e.Ties) / float64(e.TotalSimulations)
}

// LossRate returns the loss rate as a percentage (0.0 to 1.0)
func (e EquityResult) LossRate() float64 {
	if e.TotalSimulations == 0 {
		return 0.0
	}
	losses := e.TotalSimulations - e.Wins - e.Ties
	return float64(losses) / float64(e.TotalSimulations)
}

// Equity returns the overall equity (0.0 to 1.0)
// Wins count as 1.0, ties count as 0.5
func (e EquityResult) Equity() float64 {
	if e.TotalSimulations == 0 {
		return 0.0
	}
	winEquity := float64(e.Wins)
	tieEquity := float64(e.Ties) * 0.5
	return (winEquity + tieEquity) / float64(e.TotalSimulations)
}

// ConfidenceInterval returns the 95% confidence interval for equity
func (e EquityResult) ConfidenceInterval() (lower, upper float64) {
	equity := e.Equity()
	n := float64(e.TotalSimulations)

	if n == 0 {
		return 0.0, 0.0
	}

	// Standard error for binomial proportion
	se := math.Sqrt((equity * (1.0 - equity)) / n)

	// 95% confidence interval (±1.96 * SE)
	margin := 1.96 * se

	lower = math.Max(0.0, equity-margin)
	upper = math.Min(1.0, equity+margin)

	return lower, upper
}

// parseCards converts card strings to a poker.Hand using efficient parsing
func parseCards(cardStrs []string) (poker.Hand, error) {
	var hand poker.Hand
	for _, cardStr := range cardStrs {
		card, err := poker.ParseCard(cardStr)
		if err != nil {
			return 0, err
		}
		hand.AddCard(card)
	}
	return hand, nil
}

// CalculateEquity performs Monte Carlo simulation to calculate equity using efficient poker.Hand types
// heroHoles: Hero's hole cards (exactly 2 cards)
// board: Community cards (0-5 cards)
// opponents: Number of opponents (each gets 2 random hole cards)
// simulations: Number of simulations to run
func CalculateEquity(heroHoles []string, board []string, opponents int, simulations int, rng *rand.Rand) EquityResult {
	if len(heroHoles) != 2 {
		return EquityResult{} // Invalid input
	}

	if opponents < 1 {
		opponents = 1 // At least one opponent
	}

	// Parse hole cards and board using efficient poker.Hand types
	heroCards, err := parseCards(heroHoles)
	if err != nil {
		return EquityResult{} // Invalid card format
	}

	boardCards, err := parseCards(board)
	if err != nil {
		return EquityResult{} // Invalid card format
	}

	usedCards := heroCards | boardCards

	var wins, ties uint32

	for sim := 0; sim < simulations; sim++ {
		// Create a deck and remove used cards
		deck := poker.NewDeck(rng)

		// Remove used cards from deck by dealing until we skip them
		// This is inefficient but simple - could be optimized
		availableCards := make([]poker.Card, 0, 52-usedCards.CountCards())
		for deck.CardsRemaining() > 0 {
			card := deck.DealOne()
			if !usedCards.HasCard(card) {
				availableCards = append(availableCards, card)
			}
		}

		// Shuffle available cards
		for i := len(availableCards) - 1; i > 0; i-- {
			j := rng.Intn(i + 1)
			availableCards[i], availableCards[j] = availableCards[j], availableCards[i]
		}

		cardIndex := 0

		// Complete the board if needed
		cardsNeeded := 5 - len(board)
		finalBoard := boardCards

		for i := 0; i < cardsNeeded && cardIndex < len(availableCards); i++ {
			finalBoard.AddCard(availableCards[cardIndex])
			cardIndex++
		}

		// Sample opponent hole cards
		opponentHands := make([]poker.Hand, opponents)
		for i := 0; i < opponents && cardIndex+1 < len(availableCards); i++ {
			opponentHands[i] = poker.NewHand(availableCards[cardIndex], availableCards[cardIndex+1])
			cardIndex += 2
		}

		// Evaluate hands using proper poker evaluator
		heroFinalHand := heroCards | finalBoard
		heroStrength := poker.Evaluate7Cards(heroFinalHand)

		// Compare against opponents
		heroWins := true
		tied := false

		for i, oppHand := range opponentHands {
			if i >= opponents {
				break
			}
			oppFinalHand := oppHand | finalBoard
			oppStrength := poker.Evaluate7Cards(oppFinalHand)

			cmp := poker.CompareHands(heroStrength, oppStrength)
			if cmp < 0 {
				heroWins = false
				break
			} else if cmp == 0 {
				tied = true
			}
		}

		if heroWins {
			if tied {
				ties++
			} else {
				wins++
			}
		}
	}

	return EquityResult{
		Wins:             wins,
		Ties:             ties,
		TotalSimulations: uint32(simulations),
	}
}

// QuickEquity provides a fast equity estimate with default parameters
func QuickEquity(heroHoles []string, board []string, opponents int) float64 {
	rng := rand.New(rand.NewSource(42)) // Fixed seed for deterministic testing
	result := CalculateEquity(heroHoles, board, opponents, 10000, rng)
	return result.Equity()
}
