package game

import (
	"fmt"
)

// TUIBridgeAgent is a special agent that bridges the gap between the old TUI interface
// and the new Agent pattern. This allows us to use the game engine while keeping
// the existing TUI interface unchanged.
type TUIBridgeAgent struct {
	promptFunc func() (bool, error) // The existing TUI prompt function
	showFunc   func(*Player, string) // Function to show player action
}

// NewTUIBridgeAgent creates a bridge agent for the existing TUI interface
func NewTUIBridgeAgent(promptFunc func() (bool, error), showFunc func(*Player, string)) *TUIBridgeAgent {
	return &TUIBridgeAgent{
		promptFunc: promptFunc,
		showFunc:   showFunc,
	}
}

// MakeDecision prompts through the existing TUI but returns a generic decision
func (t *TUIBridgeAgent) MakeDecision(player *Player, table *Table) Decision {
	if t.promptFunc == nil {
		return Decision{
			Action:    Fold,
			Amount:    0,
			Reasoning: "No TUI interface available",
		}
	}

	shouldContinue, err := t.promptFunc()
	if err != nil || !shouldContinue {
		return Decision{
			Action:    Fold,
			Amount:    0,
			Reasoning: fmt.Sprintf("Player quit or error: %v", err),
		}
	}

	// The TUI has handled the interaction and updated the player state
	// We just return a success decision
	return Decision{
		Action:    Check, // Generic action - the real action was handled by TUI
		Amount:    0,
		Reasoning: "Human decision via TUI",
	}
}

// ExecuteAction works with the existing TUI that handles actions directly
func (t *TUIBridgeAgent) ExecuteAction(player *Player, table *Table) string {
	if !player.CanAct() {
		return "Player cannot act"
	}

	// For human players using the TUI bridge, the TUI has already handled the action
	// We just need to call the decision and let the TUI do its work
	decision := t.MakeDecision(player, table)
	
	// Show the action if we have a show function
	if t.showFunc != nil {
		t.showFunc(player, decision.Reasoning)
	}

	return decision.Reasoning
}