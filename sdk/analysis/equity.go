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

// CalculateEquity performs Monte Carlo simulation to calculate equity
// heroHand: Hero's complete hand (2 hole cards)
// board: Community cards (0-5 cards)
// opponents: Number of opponents (each gets 2 random hole cards)
// simulations: Number of simulations to run
func CalculateEquity(heroHand poker.Hand, board poker.Hand, opponents int, simulations int, rng *rand.Rand) EquityResult {
	// Validate hero hand has exactly 2 cards
	if heroHand.CountCards() != 2 {
		return EquityResult{} // Invalid input
	}

	// Validate board has at most 5 cards
	if board.CountCards() > 5 {
		return EquityResult{} // Invalid input
	}

	// Validate simulations is positive
	if simulations <= 0 {
		return EquityResult{} // Invalid input
	}

	// Validate we have enough cards in deck
	cardsNeeded := 2 + board.CountCards() + (5 - board.CountCards()) + (opponents * 2)
	if cardsNeeded > 52 {
		return EquityResult{} // Not enough cards in deck
	}

	// Validate hero hand and board don't overlap
	if (heroHand & board) != 0 {
		return EquityResult{} // Overlapping cards
	}

	if opponents < 1 {
		opponents = 1 // At least one opponent
	}

	var wins, ties uint32

	// Pre-allocate deck for reuse
	deck := poker.NewDeck(rng)

	for sim := 0; sim < simulations; sim++ {
		deck.Shuffle()

		// Declare variables at top to avoid goto issues
		var heroWins = true
		var heroTies = false
		var heroFullHand poker.Hand
		var heroRank poker.HandRank

		// Create used cards mask to avoid dealing duplicates
		usedCards := heroHand | board

		// Deal remaining board cards if needed
		finalBoard := board
		cardsNeeded := 5 - board.CountCards()
		for i := 0; i < cardsNeeded; i++ {
			for {
				card := deck.DealOne()
				if card == 0 {
					// Deck exhausted - abort this simulation
					goto nextSimulation
				}
				if !usedCards.HasCard(card) {
					finalBoard.AddCard(card)
					usedCards.AddCard(card)
					break
				}
			}
		}

		// Deal opponent hole cards and evaluate
		heroFullHand = heroHand | finalBoard
		heroRank = poker.Evaluate7Cards(heroFullHand)

		// Compare against all opponents

		for opp := 0; opp < opponents; opp++ {
			// Deal 2 hole cards for this opponent
			var oppHand poker.Hand
			for i := 0; i < 2; i++ {
				for {
					card := deck.DealOne()
					if card == 0 {
						// Deck exhausted - abort this simulation
						goto nextSimulation
					}
					if !usedCards.HasCard(card) {
						oppHand.AddCard(card)
						usedCards.AddCard(card)
						break
					}
				}
			}

			// Evaluate opponent's hand
			oppFullHand := oppHand | finalBoard
			oppRank := poker.Evaluate7Cards(oppFullHand)
			comparison := poker.CompareHands(heroRank, oppRank)

			if comparison < 0 {
				heroWins = false
				break
			} else if comparison == 0 {
				heroTies = true
			}
		}

		if heroWins {
			if heroTies {
				ties++
			} else {
				wins++
			}
		}
	nextSimulation:
	}

	return EquityResult{
		Wins:             wins,
		Ties:             ties,
		TotalSimulations: uint32(simulations),
	}
}
