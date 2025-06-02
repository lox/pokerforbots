package bot

import (
	"fmt"

	"github.com/lox/pokerforbots/internal/evaluator"
	"github.com/lox/pokerforbots/internal/game"
)

// SimpleRangeBuilder builds opponent ranges based on action context
type SimpleRangeBuilder struct{}

// NewSimpleRangeBuilder creates a new range builder
func NewSimpleRangeBuilder() *SimpleRangeBuilder {
	return &SimpleRangeBuilder{}
}

// BuildOpponentRange constructs a range based on the opponent's actions
func (rb *SimpleRangeBuilder) BuildOpponentRange(opponentName string, tableState game.TableState) (evaluator.Range, string) {
	// Find the opponent
	var opponent *game.PlayerState
	for i := range tableState.Players {
		if tableState.Players[i].Name == opponentName {
			opponent = &tableState.Players[i]
			break
		}
	}

	if opponent == nil {
		return evaluator.RandomRange{}, "Unknown opponent - using random range"
	}

	// Start with position-based range
	baseRange := rb.getPositionBasedRange(opponent.Position)
	description := fmt.Sprintf("Base %s range for %s", rb.getRangeDescription(baseRange), opponent.Position.String())

	// Apply action-based filters if we have hand history
	if tableState.HandHistory != nil {
		range_, desc := rb.applyActionFilters(baseRange, opponentName, tableState.HandHistory, tableState.CurrentRound)
		return range_, description + ". " + desc
	}

	return baseRange, description
}

// getPositionBasedRange returns starting range based on position
func (rb *SimpleRangeBuilder) getPositionBasedRange(position game.Position) evaluator.Range {
	switch position {
	case game.UnderTheGun, game.EarlyPosition:
		return evaluator.TightRange{} // 15-20% range
	case game.MiddlePosition:
		return evaluator.MediumRange{} // 20-25% range
	case game.LatePosition, game.Cutoff:
		return evaluator.MediumRange{} // 25-30% range
	case game.Button:
		return evaluator.LooseRange{} // 30-35% range
	case game.SmallBlind:
		return evaluator.TightRange{} // 15-20% (out of position)
	case game.BigBlind:
		return evaluator.MediumRange{} // 20-25% (getting odds)
	default:
		return evaluator.MediumRange{}
	}
}

// applyActionFilters narrows range based on betting actions
func (rb *SimpleRangeBuilder) applyActionFilters(baseRange evaluator.Range, opponentName string, handHistory *game.HandHistory, currentRound game.BettingRound) (evaluator.Range, string) {
	currentRange := baseRange
	var descriptions []string

	// Look for preflop aggression
	preflopRaised := false
	for _, action := range handHistory.Actions {
		if action.Round != game.PreFlop || action.PlayerName != opponentName {
			continue
		}

		if action.Action == game.Raise {
			currentRange = evaluator.TightRange{}
			descriptions = append(descriptions, "PF raise -> tight range")
			preflopRaised = true
			break
		}
	}

	// Look for postflop aggression (only if we're postflop)
	if currentRound != game.PreFlop {
		postflopBet := false
		for _, action := range handHistory.Actions {
			if action.Round == game.PreFlop || action.PlayerName != opponentName {
				continue
			}

			if action.Action == game.Raise {
				// Postflop betting after preflop aggression = very tight
				if preflopRaised {
					currentRange = evaluator.TightRange{}
					descriptions = append(descriptions, "PF raise + postflop bet -> very tight")
				} else {
					// Postflop betting without preflop aggression = medium tight
					currentRange = evaluator.MediumRange{}
					descriptions = append(descriptions, "Postflop bet -> medium range")
				}
				postflopBet = true
				break
			}
		}

		// If no postflop betting, they're checking/calling
		if !postflopBet && preflopRaised {
			descriptions = append(descriptions, "PF raiser checking postflop -> maintaining tight range")
		}
	}

	if len(descriptions) == 0 {
		descriptions = append(descriptions, "No significant actions -> maintaining position range")
	}

	return currentRange, descriptions[len(descriptions)-1]
}

// getRangeDescription returns a human readable range description
func (rb *SimpleRangeBuilder) getRangeDescription(range_ evaluator.Range) string {
	switch range_.(type) {
	case evaluator.TightRange:
		return "tight"
	case evaluator.MediumRange:
		return "medium"
	case evaluator.LooseRange:
		return "loose"
	default:
		return "random"
	}
}
