package chart

import (
	"github.com/lox/pokerforbots/sdk"
	"github.com/lox/pokerforbots/sdk/deck"
)

// PreflopChartBot uses a simple preflop chart strategy
type PreflopChartBot struct {
	name      string
	evaluator *sdk.Evaluator
}

// NewPreflopChartBot creates a new chart-based bot
func NewPreflopChartBot(name string) *PreflopChartBot {
	return &PreflopChartBot{
		name:      name,
		evaluator: sdk.NewEvaluator(),
	}
}

// MakeDecision implements the Agent interface using preflop charts
func (pcb *PreflopChartBot) MakeDecision(tableState sdk.TableState, validActions []sdk.ValidAction) sdk.Decision {
	botPlayer := tableState.GetBotPlayer()
	if botPlayer == nil {
		return sdk.NewFoldDecision("Cannot find bot player")
	}

	// Preflop strategy
	if tableState.IsPreflop() {
		return pcb.makePreflopDecision(botPlayer.HoleCards, tableState, validActions)
	}

	// Post-flop strategy (simplified)
	return pcb.makePostflopDecision(botPlayer.HoleCards, tableState, validActions)
}

func (pcb *PreflopChartBot) makePreflopDecision(holeCards []deck.Card, tableState sdk.TableState, validActions []sdk.ValidAction) sdk.Decision {
	if len(holeCards) != 2 {
		return sdk.NewFoldDecision("Invalid hole cards")
	}

	// Premium hands - always raise/call
	if pcb.evaluator.IsPremiumPreflop(holeCards) {
		// Try to raise
		for _, action := range validActions {
			if action.Action == sdk.ActionRaise {
				// Raise to 3x big blind or minimum raise
				raiseAmount := max(action.MinAmount, tableState.Pot/2)
				if raiseAmount <= action.MaxAmount {
					return sdk.NewRaiseDecision(raiseAmount, "Premium preflop hand")
				}
			}
		}

		// If can't raise, call
		for _, action := range validActions {
			if action.Action == sdk.ActionCall {
				return sdk.NewCallDecision("Premium hand - call")
			}
		}
	}

	// Playable hands - call or check
	if pcb.evaluator.IsPlayablePreflop(holeCards) {
		// Check for cheap entry
		for _, action := range validActions {
			if action.Action == sdk.ActionCheck {
				return sdk.NewCheckDecision("Playable hand - check for free")
			}
		}

		// Call if the price is reasonable (less than 5% of stack)
		botPlayer := tableState.GetBotPlayer()
		if botPlayer != nil {
			for _, action := range validActions {
				if action.Action == sdk.ActionCall {
					callAmount := tableState.CurrentBet - botPlayer.BetThisRound
					if callAmount <= botPlayer.Chips/20 { // 5% of stack
						return sdk.NewCallDecision("Playable hand - small call")
					}
				}
			}
		}
	}

	// Default: fold weak hands
	return sdk.NewFoldDecision("Weak preflop hand")
}

func (pcb *PreflopChartBot) makePostflopDecision(holeCards []deck.Card, tableState sdk.TableState, validActions []sdk.ValidAction) sdk.Decision {
	// Evaluate hand strength
	handStrength := pcb.evaluator.EvaluateHand(holeCards, tableState.CommunityCards)

	// Strong hand (top 20%) - bet/raise aggressively
	if handStrength.Percentile >= 80 {
		for _, action := range validActions {
			if action.Action == sdk.ActionRaise {
				// Bet about 2/3 pot
				betAmount := (tableState.Pot * 2) / 3
				betAmount = max(betAmount, action.MinAmount)
				betAmount = min(betAmount, action.MaxAmount)
				return sdk.NewRaiseDecision(betAmount, "Strong hand - value bet")
			}
		}

		for _, action := range validActions {
			if action.Action == sdk.ActionCall {
				return sdk.NewCallDecision("Strong hand - call")
			}
		}
	}

	// Medium hand (40-80%) - call or check
	if handStrength.Percentile >= 40 {
		for _, action := range validActions {
			if action.Action == sdk.ActionCheck {
				return sdk.NewCheckDecision("Medium hand - check")
			}
		}

		// Call small bets
		botPlayer := tableState.GetBotPlayer()
		if botPlayer != nil {
			for _, action := range validActions {
				if action.Action == sdk.ActionCall {
					callAmount := tableState.CurrentBet - botPlayer.BetThisRound
					if callAmount <= tableState.Pot/3 { // 1/3 pot or less
						return sdk.NewCallDecision("Medium hand - call small bet")
					}
				}
			}
		}
	}

	// Weak hand - check/fold
	for _, action := range validActions {
		if action.Action == sdk.ActionCheck {
			return sdk.NewCheckDecision("Weak hand - check")
		}
	}

	return sdk.NewFoldDecision("Weak hand - fold")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
