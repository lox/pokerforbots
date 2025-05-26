package game

// BotDecision represents a bot's decision with reasoning
type BotDecision struct {
	Action    Action
	Amount    int    // For raises, the total bet amount
	Reasoning string // Human-readable explanation
}

// Bot represents a poker bot that can make decisions and execute actions
type Bot interface {
	// MakeDecision analyzes the game state and returns a decision with reasoning
	MakeDecision(player *Player, table *Table) BotDecision

	// ExecuteAction executes the bot's decision and updates game state
	ExecuteAction(player *Player, table *Table) string // returns reasoning
}
