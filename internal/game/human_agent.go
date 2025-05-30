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
func (h *HumanAgent) MakeDecision(tableState TableState, validActions []ValidAction) Decision {
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

