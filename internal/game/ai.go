package game

import (
	"math/rand"
	"time"

	"github.com/charmbracelet/log"
	"github.com/lox/holdem-cli/internal/deck"
	"github.com/lox/holdem-cli/internal/evaluator"
)

// AIEngine handles AI decision making
type AIEngine struct {
	rng    *rand.Rand
	logger *log.Logger
}

// NewAIEngine creates a new AI engine
func NewAIEngine(logger *log.Logger) *AIEngine {
	return &AIEngine{
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
		logger: logger,
	}
}

// HandStrength represents the relative strength of a hand
type HandStrength int

const (
	VeryWeak HandStrength = iota
	Weak
	Medium
	Strong
	VeryStrong
)

// String returns the string representation of hand strength
func (hs HandStrength) String() string {
	switch hs {
	case VeryWeak:
		return "Very Weak"
	case Weak:
		return "Weak"
	case Medium:
		return "Medium"
	case Strong:
		return "Strong"
	case VeryStrong:
		return "Very Strong"
	default:
		return "Unknown"
	}
}

// MakeDecision makes an AI decision for the given player
func (ai *AIEngine) MakeDecision(player *Player, table *Table) Action {
	// Evaluate hand strength
	strength := ai.evaluateHandStrength(player, table)

	// Get position factor (early position plays tighter)
	positionFactor := ai.getPositionFactor(player.Position)

	// Calculate pot odds if there's a bet to call
	potOdds := ai.calculatePotOdds(player, table)

	// Log decision factors
	holeCardsStr := ""
	if len(player.HoleCards) >= 2 {
		holeCardsStr = player.HoleCards[0].String() + " " + player.HoleCards[1].String()
	}

	ai.logger.Info("AI decision analysis",
		"player", player.Name,
		"round", table.CurrentRound.String(),
		"holeCards", holeCardsStr,
		"handStrength", strength.String(),
		"position", player.Position.String(),
		"positionFactor", positionFactor,
		"currentBet", table.CurrentBet,
		"playerBetThisRound", player.BetThisRound,
		"playerChips", player.Chips,
		"pot", table.Pot,
		"potOdds", potOdds)

	// Make decision based on hand strength, position, and pot odds
	action := ai.makeDecisionBasedOnFactors(player, table, strength, positionFactor, potOdds)

	ai.logger.Info("AI decision made",
		"player", player.Name,
		"decision", action.String(),
		"reasoning", ai.getDecisionReasoning(action, strength, positionFactor, potOdds, table))

	return action
}

// evaluateHandStrength evaluates the current hand strength
func (ai *AIEngine) evaluateHandStrength(player *Player, table *Table) HandStrength {
	if len(player.HoleCards) != 2 {
		return VeryWeak
	}

	// Pre-flop hand strength evaluation
	if table.CurrentRound == PreFlop {
		return ai.evaluatePreFlopStrength(player.HoleCards)
	}

	// Post-flop evaluation with community cards
	if len(table.CommunityCards) >= 3 {
		return ai.evaluatePostFlopStrength(player, table.CommunityCards)
	}

	return Medium
}

// evaluatePreFlopStrength evaluates pre-flop hand strength
func (ai *AIEngine) evaluatePreFlopStrength(holeCards []deck.Card) HandStrength {
	card1, card2 := holeCards[0], holeCards[1]

	// Pocket pairs
	if card1.Rank == card2.Rank {
		switch {
		case card1.Rank >= deck.Jack: // JJ, QQ, KK, AA
			return VeryStrong
		case card1.Rank >= deck.Nine: // 99, TT
			return Strong
		case card1.Rank >= deck.Six: // 66, 77, 88
			return Medium
		default: // 22-55
			return Weak
		}
	}

	// Suited cards
	suited := card1.Suit == card2.Suit

	// High cards
	highCard := card1.Rank
	lowCard := card2.Rank
	if lowCard > highCard {
		highCard, lowCard = lowCard, highCard
	}

	// Premium hands
	if (highCard == deck.Ace && lowCard >= deck.King) ||
		(highCard == deck.King && lowCard == deck.Queen) {
		if suited {
			return VeryStrong
		}
		return Strong
	}

	// Good hands
	if (highCard == deck.Ace && lowCard >= deck.Ten) ||
		(highCard == deck.King && lowCard >= deck.Jack) ||
		(highCard == deck.Queen && lowCard >= deck.Jack) {
		if suited {
			return Strong
		}
		return Medium
	}

	// Suited connectors and one-gappers
	if suited {
		gap := int(highCard) - int(lowCard)
		if gap <= 2 && highCard >= deck.Seven { // 7-8s, 8-9s, etc.
			return Medium
		}
	}

	// Face cards
	if highCard >= deck.Jack {
		return Weak
	}

	return VeryWeak
}

// evaluatePostFlopStrength evaluates post-flop hand strength
func (ai *AIEngine) evaluatePostFlopStrength(player *Player, communityCards []deck.Card) HandStrength {
	// Combine hole cards with community cards
	allCards := make([]deck.Card, 0, 7)
	allCards = append(allCards, player.HoleCards...)
	allCards = append(allCards, communityCards...)

	// Get best hand
	bestHand := evaluator.FindBestHand(allCards)

	// Evaluate based on hand rank
	switch bestHand.Rank {
	case evaluator.RoyalFlush, evaluator.StraightFlush:
		return VeryStrong
	case evaluator.FourOfAKind, evaluator.FullHouse:
		return VeryStrong
	case evaluator.Flush, evaluator.Straight:
		return Strong
	case evaluator.ThreeOfAKind:
		return Strong
	case evaluator.TwoPair:
		return Medium
	case evaluator.OnePair:
		// Evaluate pair strength
		if len(bestHand.Kickers) > 0 {
			pairRank := bestHand.Kickers[0]
			if pairRank >= deck.Jack {
				return Medium
			}
			return Weak
		}
		return Weak
	case evaluator.HighCard:
		// High card hands are generally weak
		if len(bestHand.Kickers) > 0 && bestHand.Kickers[0] == deck.Ace {
			return Weak
		}
		return VeryWeak
	}

	return Medium
}

// getPositionFactor returns a factor based on position (lower = tighter play)
func (ai *AIEngine) getPositionFactor(position Position) float64 {
	switch position {
	case SmallBlind, BigBlind:
		return 0.8 // Play tighter in blinds
	case UnderTheGun, EarlyPosition:
		return 0.7 // Play very tight in early position
	case MiddlePosition:
		return 0.9 // Standard play
	case LatePosition, Cutoff:
		return 1.1 // Play looser in late position
	case Button:
		return 1.2 // Play loosest on the button
	default:
		return 1.0
	}
}

// calculatePotOdds calculates the pot odds for calling
func (ai *AIEngine) calculatePotOdds(player *Player, table *Table) float64 {
	if table.CurrentBet <= player.BetThisRound {
		return 0 // No bet to call
	}

	callAmount := table.CurrentBet - player.BetThisRound
	if callAmount >= player.Chips {
		callAmount = player.Chips // All-in scenario
	}

	if callAmount == 0 {
		return 100 // Free to call (check)
	}

	return float64(table.Pot) / float64(callAmount)
}

// makeDecisionBasedOnFactors makes the final decision
func (ai *AIEngine) makeDecisionBasedOnFactors(player *Player, table *Table, strength HandStrength, positionFactor, potOdds float64) Action {
	// Base probabilities for each action based on hand strength
	var foldProb, callProb, raiseProb float64

	switch strength {
	case VeryWeak:
		foldProb, callProb, raiseProb = 0.85, 0.15, 0.0
	case Weak:
		foldProb, callProb, raiseProb = 0.60, 0.35, 0.05
	case Medium:
		foldProb, callProb, raiseProb = 0.15, 0.70, 0.15 // Less foldy with medium hands
	case Strong:
		foldProb, callProb, raiseProb = 0.05, 0.40, 0.55
	case VeryStrong:
		foldProb, callProb, raiseProb = 0.0, 0.20, 0.80
	}

	ai.logger.Debug("Base probabilities",
		"player", player.Name,
		"strength", strength.String(),
		"foldProb", foldProb,
		"callProb", callProb,
		"raiseProb", raiseProb)

	// Adjust for position
	if positionFactor < 1.0 {
		// Tighter play - increase fold probability
		foldProb = foldProb * (2.0 - positionFactor)
		callProb = callProb * positionFactor
		raiseProb = raiseProb * positionFactor
	} else {
		// Looser play - decrease fold probability
		foldProb = foldProb / positionFactor
		callProb = callProb * positionFactor
		raiseProb = raiseProb * positionFactor
	}

	// Adjust for pot odds
	if potOdds > 2.0 && strength >= Weak {
		// Good pot odds, more likely to call
		potOddsBonus := (potOdds - 2.0) * 0.15 // Scale bonus with better odds
		callProb += potOddsBonus
		foldProb -= potOddsBonus * 0.8
		raiseProb += potOddsBonus * 0.2
	}

	// Normalize probabilities
	total := foldProb + callProb + raiseProb
	if total > 0 {
		foldProb /= total
		callProb /= total
		raiseProb /= total
	}

	ai.logger.Debug("Final probabilities after adjustments",
		"player", player.Name,
		"positionFactor", positionFactor,
		"potOdds", potOdds,
		"finalFoldProb", foldProb,
		"finalCallProb", callProb,
		"finalRaiseProb", raiseProb)

	// Special cases
	if table.CurrentBet == 0 {
		// No bet to call, can check
		if foldProb > 0.5 {
			return Check // Check instead of fold when possible
		}
		if raiseProb > callProb {
			return Raise
		}
		return Check
	}

	// If can't afford to call, must fold or go all-in
	callAmount := table.CurrentBet - player.BetThisRound
	if callAmount >= player.Chips {
		if strength >= Strong && ai.rng.Float64() < 0.3 {
			return AllIn
		}
		return Fold
	}

	// Make random decision based on probabilities
	r := ai.rng.Float64()
	if r < foldProb {
		return Fold
	} else if r < foldProb+callProb {
		return Call
	} else {
		return Raise
	}
}

// GetRaiseAmount determines how much to raise
func (ai *AIEngine) GetRaiseAmount(player *Player, table *Table, strength HandStrength) int {
	potSize := table.Pot
	currentBet := table.CurrentBet

	// Base raise as a fraction of pot
	var raiseFactor float64
	switch strength {
	case Medium:
		raiseFactor = 0.5 + ai.rng.Float64()*0.3 // 0.5-0.8x pot
	case Strong:
		raiseFactor = 0.7 + ai.rng.Float64()*0.4 // 0.7-1.1x pot
	case VeryStrong:
		raiseFactor = 0.8 + ai.rng.Float64()*0.6 // 0.8-1.4x pot
	default:
		raiseFactor = 0.5 // Conservative
	}

	baseRaise := int(float64(potSize) * raiseFactor)

	// Minimum raise is current bet + big blind
	minRaise := currentBet + table.BigBlind
	if baseRaise < minRaise {
		baseRaise = minRaise
	}

	// Can't raise more than we have
	maxRaise := player.Chips + player.BetThisRound
	if baseRaise > maxRaise {
		baseRaise = maxRaise
	}

	return baseRaise
}

// ExecuteAIAction executes an AI player's action
func (ai *AIEngine) ExecuteAIAction(player *Player, table *Table) {
	if player.Type != AI || !player.CanAct() {
		return
	}

	action := ai.MakeDecision(player, table)

	switch action {
	case Fold:
		player.Fold()
	case Call:
		callAmount := table.CurrentBet - player.BetThisRound
		if callAmount > 0 && callAmount <= player.Chips {
			player.Call(callAmount)
			table.Pot += callAmount
		} else {
			player.Check() // Fall back to check if can't call
		}
	case Check:
		player.Check()
	case Raise:
		strength := ai.evaluateHandStrength(player, table)
		raiseAmount := ai.GetRaiseAmount(player, table, strength)
		totalNeeded := raiseAmount - player.BetThisRound

		if totalNeeded > 0 && totalNeeded <= player.Chips {
			player.Raise(totalNeeded)
			table.Pot += totalNeeded
			table.CurrentBet = raiseAmount
		} else {
			// Fall back to call or check
			callAmount := table.CurrentBet - player.BetThisRound
			if callAmount > 0 && callAmount <= player.Chips {
				player.Call(callAmount)
				table.Pot += callAmount
			} else {
				player.Check()
			}
		}
	case AllIn:
		allInAmount := player.Chips
		if player.AllIn() {
			table.Pot += allInAmount
			if player.TotalBet > table.CurrentBet {
				table.CurrentBet = player.TotalBet
			}
		}
	}
}

// getDecisionReasoning provides a human-readable explanation for the AI's decision
func (ai *AIEngine) getDecisionReasoning(action Action, strength HandStrength, positionFactor, potOdds float64, table *Table) string {
	switch action {
	case Fold:
		if strength == VeryWeak {
			return "Very weak hand, folding to avoid losses"
		} else if positionFactor < 1.0 {
			return "Tight play in early position"
		} else if potOdds < 2.0 {
			return "Poor pot odds, not worth calling"
		}
		return "Hand not strong enough to continue"
	case Check:
		if table.CurrentBet == 0 {
			return "No bet to call, checking to see next card"
		}
		return "Checking as safe option"
	case Call:
		if potOdds > 3.0 {
			return "Good pot odds, worth calling"
		} else if strength >= Medium {
			return "Decent hand strength, calling to see more cards"
		}
		return "Calling with marginal hand"
	case Raise:
		if strength >= Strong {
			return "Strong hand, raising for value"
		} else if positionFactor > 1.0 {
			return "Late position bluff/semi-bluff"
		}
		return "Raising with good hand"
	case AllIn:
		return "Strong hand, going all-in for maximum value"
	default:
		return "Unknown decision reason"
	}
}
