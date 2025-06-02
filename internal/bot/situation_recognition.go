package bot

import (
	"fmt"

	"github.com/lox/pokerforbots/internal/deck"
	"github.com/lox/pokerforbots/internal/game"
)

// SituationContext captures the key elements of a poker decision
type SituationContext struct {
	Player         game.PlayerState
	TableState     game.TableState
	HandStrength   HandStrength
	Equity         float64
	PotOdds        float64
	IsInPosition   bool
	IsMultiway     bool
	ActionSequence ActionSequenceType
	BoardTexture   BoardTexture
}

// ActionSequenceType describes the betting action leading to this decision
type ActionSequenceType int

const (
	Unopened ActionSequenceType = iota
	SingleRaise
	ThreeBet
	FourBet
	Multiway
	PostflopBet
	PostflopRaise
)

// SituationRule represents a fundamental poker concept
type SituationRule struct {
	Name       string
	Condition  func(SituationContext) bool
	Adjustment ActionAdjustment
	Reasoning  string
	Priority   int // Higher priority rules override lower ones
}

// ActionAdjustment modifies action probabilities
type ActionAdjustment struct {
	FoldMultiplier  float64
	CallMultiplier  float64
	RaiseMultiplier float64
	Description     string
}

// SituationRecognizer evaluates poker situations and applies fundamental concepts
type SituationRecognizer struct {
	rules []SituationRule
}

// NewSituationRecognizer creates a recognizer with fundamental poker rules
func NewSituationRecognizer() *SituationRecognizer {
	return &SituationRecognizer{
		rules: getFundamentalPokerRules(),
	}
}

// EvaluateSituation applies all relevant situation rules
func (sr *SituationRecognizer) EvaluateSituation(ctx SituationContext) (ActionAdjustment, string) {
	var appliedRules []string
	totalAdjustment := ActionAdjustment{
		FoldMultiplier:  1.0,
		CallMultiplier:  1.0,
		RaiseMultiplier: 1.0,
		Description:     "Base action probabilities",
	}

	// Apply rules in priority order (highest first)
	for _, rule := range sr.rules {
		if rule.Condition(ctx) {
			// Multiply adjustments (this way multiple rules compound)
			totalAdjustment.FoldMultiplier *= rule.Adjustment.FoldMultiplier
			totalAdjustment.CallMultiplier *= rule.Adjustment.CallMultiplier
			totalAdjustment.RaiseMultiplier *= rule.Adjustment.RaiseMultiplier

			appliedRules = append(appliedRules, rule.Name)
		}
	}

	reasoning := fmt.Sprintf("Applied rules: %v", appliedRules)
	totalAdjustment.Description = reasoning

	return totalAdjustment, reasoning
}

// getFundamentalPokerRules returns the core situation recognition rules
func getFundamentalPokerRules() []SituationRule {
	return []SituationRule{
		// Rule 1: Don't 3-bet junk from blinds
		{
			Name: "Blind vs Late Position Raise - Weak Hand",
			Condition: func(ctx SituationContext) bool {
				return (ctx.Player.Position == game.SmallBlind || ctx.Player.Position == game.BigBlind) &&
					ctx.ActionSequence == SingleRaise &&
					ctx.HandStrength <= Medium &&
					ctx.TableState.CurrentBet > 0
			},
			Adjustment: ActionAdjustment{
				FoldMultiplier:  1.3, // Moderately more likely to fold
				CallMultiplier:  0.9, // Slightly less likely to call
				RaiseMultiplier: 0.3, // Less likely to 3-bet
			},
			Reasoning: "Weak hand in blinds vs late position raise - avoid 3-betting",
			Priority:  100,
		},

		// Rule 2: Don't bluff-jam draws out of position
		{
			Name: "Draw Out of Position vs Aggression",
			Condition: func(ctx SituationContext) bool {
				return ctx.TableState.CurrentRound != game.PreFlop &&
					!ctx.IsInPosition &&
					ctx.HandStrength <= Medium &&
					ctx.TableState.CurrentBet > 0 &&
					ctx.ActionSequence == PostflopBet
			},
			Adjustment: ActionAdjustment{
				FoldMultiplier:  1.2, // Moderately more likely to fold
				CallMultiplier:  1.0, // Same likelihood to call
				RaiseMultiplier: 0.4, // Less likely to raise/jam
			},
			Reasoning: "Draw out of position vs aggression - play cautiously",
			Priority:  90,
		},

		// Rule 3: Small speculative hands need good odds
		{
			Name: "Speculative Hand - Poor Odds",
			Condition: func(ctx SituationContext) bool {
				return ctx.HandStrength <= Weak &&
					ctx.PotOdds < 3.0 && // Need good odds for speculative hands
					ctx.TableState.CurrentBet > 0
			},
			Adjustment: ActionAdjustment{
				FoldMultiplier:  1.4,
				CallMultiplier:  0.7,
				RaiseMultiplier: 0.3,
			},
			Reasoning: "Speculative hand without good pot odds",
			Priority:  80,
		},

		// Rule 4: Don't c-bet weak hands multiway
		{
			Name: "Weak Hand Multiway",
			Condition: func(ctx SituationContext) bool {
				return ctx.IsMultiway &&
					ctx.HandStrength <= Medium &&
					ctx.TableState.CurrentBet == 0 // Considering betting
			},
			Adjustment: ActionAdjustment{
				FoldMultiplier:  1.0, // Check instead of fold
				CallMultiplier:  1.0, // Check instead of call
				RaiseMultiplier: 0.4, // Much less likely to bet
			},
			Reasoning: "Weak hand in multiway pot - avoid betting",
			Priority:  70,
		},

		// Rule 5: Strong hands in position - be aggressive
		{
			Name: "Strong Hand In Position",
			Condition: func(ctx SituationContext) bool {
				return ctx.IsInPosition &&
					ctx.HandStrength >= Strong &&
					ctx.TableState.CurrentRound != game.PreFlop
			},
			Adjustment: ActionAdjustment{
				FoldMultiplier:  0.5, // Less likely to fold
				CallMultiplier:  0.8, // Less likely to just call
				RaiseMultiplier: 1.5, // More likely to bet/raise
			},
			Reasoning: "Strong hand in position - value betting",
			Priority:  60,
		},

		// Rule 6: Very wet boards - tighten up without nuts
		{
			Name: "Wet Board Without Nuts",
			Condition: func(ctx SituationContext) bool {
				return (ctx.BoardTexture == WetBoard || ctx.BoardTexture == VeryWetBoard) &&
					ctx.HandStrength <= Strong &&
					ctx.TableState.CurrentRound != game.PreFlop
			},
			Adjustment: ActionAdjustment{
				FoldMultiplier:  1.2,
				CallMultiplier:  0.9,
				RaiseMultiplier: 0.7,
			},
			Reasoning: "Wet board without the nuts - play carefully",
			Priority:  50,
		},
	}
}

// BuildSituationContext creates context from game state
func BuildSituationContext(player game.PlayerState, tableState game.TableState, handStrength HandStrength, equity, potOdds float64) SituationContext {
	return SituationContext{
		Player:         player,
		TableState:     tableState,
		HandStrength:   handStrength,
		Equity:         equity,
		PotOdds:        potOdds,
		IsInPosition:   isInPosition(player.Position),
		IsMultiway:     isMultiway(tableState),
		ActionSequence: determineActionSequence(tableState),
		BoardTexture:   determineBoardTexture(tableState.CommunityCards),
	}
}

// Helper functions
func isInPosition(position game.Position) bool {
	return position == game.Button || position == game.Cutoff || position == game.LatePosition
}

func isMultiway(tableState game.TableState) bool {
	activePlayers := 0
	for _, player := range tableState.Players {
		if player.IsActive && !player.IsFolded {
			activePlayers++
		}
	}
	return activePlayers > 2
}

func determineActionSequence(tableState game.TableState) ActionSequenceType {
	if tableState.HandHistory == nil {
		return Unopened
	}

	preflopRaises := 0
	postflopBets := 0

	for _, action := range tableState.HandHistory.Actions {
		if action.Round == game.PreFlop && action.Action == game.Raise {
			preflopRaises++
		} else if action.Round != game.PreFlop && action.Action == game.Raise {
			postflopBets++
		}
	}

	// Determine sequence based on current round
	if tableState.CurrentRound == game.PreFlop {
		switch preflopRaises {
		case 0:
			return Unopened
		case 1:
			return SingleRaise
		case 2:
			return ThreeBet
		default:
			return FourBet
		}
	} else {
		// Postflop
		if postflopBets > 0 {
			if postflopBets == 1 {
				return PostflopBet
			}
			return PostflopRaise
		}
		return Unopened
	}
}

func determineBoardTexture(communityCards []deck.Card) BoardTexture {
	if len(communityCards) < 3 {
		return DryBoard
	}

	// Simple board texture analysis
	wetness := 0

	// Check for flush possibilities
	suitCounts := make(map[int]int)
	for _, card := range communityCards {
		suitCounts[card.Suit]++
	}

	maxSuitCount := 0
	for _, count := range suitCounts {
		if count > maxSuitCount {
			maxSuitCount = count
		}
	}

	if maxSuitCount >= 3 {
		wetness += 2 // Flush draw possible
	}

	// Check for straight possibilities
	ranks := make([]int, len(communityCards))
	for i, card := range communityCards {
		ranks[i] = card.Rank
	}

	// Sort ranks
	for i := 0; i < len(ranks); i++ {
		for j := i + 1; j < len(ranks); j++ {
			if ranks[i] > ranks[j] {
				ranks[i], ranks[j] = ranks[j], ranks[i]
			}
		}
	}

	// Check connectivity
	connectedCards := 1
	for i := 1; i < len(ranks); i++ {
		if ranks[i]-ranks[i-1] <= 2 {
			connectedCards++
		}
	}

	if connectedCards >= 3 {
		wetness += 2 // Straight draws possible
	}

	// Classify based on wetness
	switch {
	case wetness >= 4:
		return VeryWetBoard
	case wetness >= 2:
		return WetBoard
	case wetness >= 1:
		return SemiWetBoard
	default:
		return DryBoard
	}
}
