package game

import "github.com/lox/pokerforbots/internal/deck"

// Decision represents a player's decision with reasoning
type Decision struct {
	Action    Action
	Amount    int    // For raises, the total bet amount
	Reasoning string // Human-readable explanation
}

// ValidAction represents an action that a player can legally take
type ValidAction struct {
	Action    Action
	MinAmount int // For raises: minimum total bet
	MaxAmount int // For raises: maximum (all-in)
}

// PlayerState represents the read-only state of a player for decision making
type PlayerState struct {
	Name         string
	Chips        int
	Position     Position
	BetThisRound int
	TotalBet     int
	HoleCards    []deck.Card // Only populated for the acting player
	IsActive     bool
	IsFolded     bool
	IsAllIn      bool
	LastAction   Action
}

// TableState represents the read-only state of the table for decision making
type TableState struct {
	CurrentBet      int
	Pot             int
	CurrentRound    BettingRound
	CommunityCards  []deck.Card
	SmallBlind      int
	BigBlind        int
	Players         []PlayerState // ALL players with appropriate visibility
	ActingPlayerIdx int           // Which player in the slice is making the decision

	// Betting context from hand history
	HandHistory *HandHistory // Full hand history for analysis
}

// Agent represents any entity (human or AI) that can make decisions for a player
// Agents receive immutable game state and return decisions - no state mutation allowed
type Agent interface {
	// MakeDecision analyzes immutable game state and returns a decision
	MakeDecision(tableState TableState, validActions []ValidAction) Decision
}
