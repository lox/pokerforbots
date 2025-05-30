package bot

import (
	"github.com/charmbracelet/log"
	"github.com/lox/holdem-cli/internal/game"
)

// ChartBot implements a simple push-fold pre-flop chart and check/call post-flop
type ChartBot struct {
	logger *log.Logger
}

// NewChartBot creates a new ChartBot instance
func NewChartBot(logger *log.Logger) *ChartBot {
	return &ChartBot{logger: logger}
}

func (c *ChartBot) MakeDecision(tableState game.TableState, validActions []game.ValidAction) game.Decision {
	actingPlayer := tableState.Players[tableState.ActingPlayerIdx]
	
	// Simple push-fold pre-flop chart, check/call post-flop
	if tableState.CurrentRound == game.PreFlop {
		// Very basic push-fold: premium hands only
		if len(actingPlayer.HoleCards) == 2 {
			card1, card2 := actingPlayer.HoleCards[0], actingPlayer.HoleCards[1]

			// Push with premium pairs and AK
			if (card1.Rank == card2.Rank && card1.Rank >= 10) || // TT+
				(card1.Rank >= 13 && card2.Rank >= 13) { // AK, AQ, AA, KK, QQ, etc.
				if actingPlayer.Chips <= 20*tableState.BigBlind { // Only if short stack
					for _, action := range validActions {
						if action.Action == game.Raise || action.Action == game.AllIn {
							return game.Decision{Action: action.Action, Amount: action.MaxAmount, Reasoning: "chart-bot push"}
						}
					}
				}
			}
		}

		// Otherwise fold to raises, check/call otherwise
		for _, action := range validActions {
			if action.Action == game.Check {
				return game.Decision{Action: game.Check, Amount: 0, Reasoning: "chart-bot checking"}
			}
		}
		for _, action := range validActions {
			if action.Action == game.Call {
				return game.Decision{Action: game.Call, Amount: 0, Reasoning: "chart-bot calling"}
			}
		}
		return game.Decision{Action: game.Fold, Amount: 0, Reasoning: "chart-bot folding"}
	}

	// Post-flop: check/call
	for _, action := range validActions {
		if action.Action == game.Check {
			return game.Decision{Action: game.Check, Amount: 0, Reasoning: "chart-bot checking"}
		}
	}
	for _, action := range validActions {
		if action.Action == game.Call {
			return game.Decision{Action: game.Call, Amount: 0, Reasoning: "chart-bot calling"}
		}
	}
	return game.Decision{Action: game.Fold, Amount: 0, Reasoning: "chart-bot folding"}
}


