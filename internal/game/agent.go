package game

// Decision represents a player's decision with reasoning
type Decision struct {
	Action    Action
	Amount    int    // For raises, the total bet amount
	Reasoning string // Human-readable explanation
}

// Agent represents any entity (human or AI) that can make decisions for a player
type Agent interface {
	// MakeDecision analyzes the game state and returns a decision with reasoning
	MakeDecision(player *Player, table *Table) Decision

	// ExecuteAction executes the decision and updates game state
	ExecuteAction(player *Player, table *Table) string // returns reasoning
}

// Bot represents a poker bot that can make decisions and execute actions
// This interface is kept for backward compatibility but extends Agent
type Bot interface {
	Agent
}
