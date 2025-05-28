package game

import (
	"fmt"
)

// HumanAgent represents a human player that can interact through a user interface
type HumanAgent struct {
	promptFunc func() (Decision, error) // Function to prompt user for decision
}

// NewHumanAgent creates a new human agent with a prompt function
func NewHumanAgent(promptFunc func() (Decision, error)) *HumanAgent {
	return &HumanAgent{
		promptFunc: promptFunc,
	}
}

// MakeDecision prompts the human for a decision
func (h *HumanAgent) MakeDecision(player *Player, table *Table) Decision {
	if h.promptFunc == nil {
		// Fallback if no prompt function provided
		return Decision{
			Action:    Fold,
			Amount:    0,
			Reasoning: "No user interface available",
		}
	}

	decision, err := h.promptFunc()
	if err != nil {
		// If there's an error getting user input, fold by default
		return Decision{
			Action:    Fold,
			Amount:    0,
			Reasoning: fmt.Sprintf("Input error: %v", err),
		}
	}

	return decision
}

// ExecuteAction executes the human's decision and updates game state
func (h *HumanAgent) ExecuteAction(player *Player, table *Table) string {
	if !player.CanAct() {
		return "Player cannot act"
	}

	decision := h.MakeDecision(player, table)

	switch decision.Action {
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
		totalNeeded := decision.Amount - player.BetThisRound
		if totalNeeded > 0 && totalNeeded <= player.Chips {
			player.Raise(totalNeeded)
			table.Pot += totalNeeded
			table.CurrentBet = decision.Amount
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

	return decision.Reasoning
}