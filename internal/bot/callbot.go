package bot

import (
	"github.com/charmbracelet/log"
	"github.com/lox/pokerforbots/internal/game"
)

// CallBot is a simple bot that checks/calls to river, then folds river
// Now enhanced with betting analysis using the new clean architecture
type CallBot struct {
	logger *log.Logger
}

// NewCallBot creates a new CallBot instance
func NewCallBot(logger *log.Logger) *CallBot {
	return &CallBot{logger: logger}
}

// MakeDecision uses the clean architecture with immutable state
func (c *CallBot) MakeDecision(tableState game.TableState, validActions []game.ValidAction) game.Decision {
	actingPlayer := tableState.Players[tableState.ActingPlayerIdx]

	// Analyze betting action using hand history
	roundSummary := tableState.HandHistory.GetBettingRoundSummary(tableState.CurrentRound)

	// Enhanced logic: fold river only if facing heavy action
	if tableState.CurrentRound == game.River {
		// Check if there was aggressive betting (multiple raises)
		if roundSummary.NumRaises >= 2 {
			return c.findAction(game.Fold, validActions, "folding river to aggressive betting")
		}

		// Check bet sizing - fold to large bets on river
		betSizing := tableState.HandHistory.GetBetSizingInfo(tableState.CurrentRound)
		if len(betSizing) > 0 {
			lastBet := betSizing[len(betSizing)-1]
			if lastBet.Ratio > 0.8 { // Large bet relative to pot
				return c.findAction(game.Fold, validActions, "folding river to large bet")
			}
		}
	}

	// Position-aware calling: be more selective from early position
	if actingPlayer.Position == game.UnderTheGun || actingPlayer.Position == game.EarlyPosition {
		// In early position, fold to 3-bets+ preflop
		if tableState.CurrentRound == game.PreFlop && roundSummary.NumRaises >= 2 {
			return c.findAction(game.Fold, validActions, "folding to 3-bet from early position")
		}
	}

	// Stack size consideration
	stackToBBRatio := float64(actingPlayer.Chips) / float64(tableState.BigBlind)
	if stackToBBRatio < 10 { // Short stack
		// Be more aggressive when short-stacked
		if c.hasAction(game.AllIn, validActions) && roundSummary.NumRaises == 0 {
			return c.findAction(game.AllIn, validActions, "shoving with short stack")
		}
	}

	// Default call-bot behavior: call/check when possible
	if c.hasAction(game.Check, validActions) {
		return c.findAction(game.Check, validActions, "call-bot checking")
	}

	if c.hasAction(game.Call, validActions) {
		return c.findAction(game.Call, validActions, "call-bot calling")
	}

	// Fallback to fold
	return c.findAction(game.Fold, validActions, "call-bot forced fold")
}

// Helper methods for the new architecture
func (c *CallBot) hasAction(action game.Action, validActions []game.ValidAction) bool {
	for _, validAction := range validActions {
		if validAction.Action == action {
			return true
		}
	}
	return false
}

func (c *CallBot) findAction(preferredAction game.Action, validActions []game.ValidAction, reasoning string) game.Decision {
	for _, validAction := range validActions {
		if validAction.Action == preferredAction {
			return game.Decision{
				Action:    preferredAction,
				Amount:    validAction.MinAmount,
				Reasoning: reasoning,
			}
		}
	}

	// Fallback to first available action
	if len(validActions) > 0 {
		return game.Decision{
			Action:    validActions[0].Action,
			Amount:    validActions[0].MinAmount,
			Reasoning: "fallback: " + reasoning,
		}
	}

	// Should never happen with correct ValidActions
	return game.Decision{Action: game.Fold, Amount: 0, Reasoning: "emergency fold"}
}
