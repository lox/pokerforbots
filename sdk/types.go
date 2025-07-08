package sdk

import "github.com/lox/pokerforbots/sdk/deck"

// TableState represents the current state of the poker table from a bot's perspective
type TableState struct {
	CurrentBet      int              `json:"currentBet"`      // Current bet amount to call
	Pot             int              `json:"pot"`             // Total pot size
	CurrentRound    string           `json:"currentRound"`    // preflop, flop, turn, river, showdown
	CommunityCards  []deck.Card      `json:"communityCards"`  // Visible community cards
	Players         []BotPlayerState `json:"players"`         // All players at the table
	ActingPlayerIdx int              `json:"actingPlayerIdx"` // Index of player who needs to act
}

// BotPlayerState represents the state of a player at the table from a bot's perspective
type BotPlayerState struct {
	Name         string      `json:"name"`         // Player name
	Chips        int         `json:"chips"`        // Current chip count
	Position     string      `json:"position"`     // Button, Small Blind, Big Blind, etc.
	SeatNumber   int         `json:"seatNumber"`   // Seat number (1-6)
	HoleCards    []deck.Card `json:"holeCards"`    // Hole cards (only visible for bot's own cards)
	BetThisRound int         `json:"betThisRound"` // Amount bet in current betting round
	TotalBet     int         `json:"totalBet"`     // Total amount bet in current hand
	IsActive     bool        `json:"isActive"`     // Whether player is active in the hand
	IsFolded     bool        `json:"isFolded"`     // Whether player has folded
	IsAllIn      bool        `json:"isAllIn"`      // Whether player is all-in
	LastAction   string      `json:"lastAction"`   // Last action taken (fold, call, raise, etc.)
}

// Betting rounds
const (
	RoundPreflop  = "preflop"
	RoundFlop     = "flop"
	RoundTurn     = "turn"
	RoundRiver    = "river"
	RoundShowdown = "showdown"
)

// Player positions
const (
	PositionButton     = "Button"
	PositionSmallBlind = "Small Blind"
	PositionBigBlind   = "Big Blind"
	PositionUTG        = "UTG"
	PositionUTG1       = "UTG+1"
	PositionHijack     = "Hijack"
	PositionCutoff     = "Cutoff"
)

// GetBotPlayer returns the player state for the bot (identified by having hole cards)
func (ts TableState) GetBotPlayer() *BotPlayerState {
	for i, player := range ts.Players {
		if len(player.HoleCards) > 0 {
			return &ts.Players[i]
		}
	}
	return nil
}

// GetActingPlayer returns the player who needs to make a decision
func (ts TableState) GetActingPlayer() *BotPlayerState {
	if ts.ActingPlayerIdx >= 0 && ts.ActingPlayerIdx < len(ts.Players) {
		return &ts.Players[ts.ActingPlayerIdx]
	}
	return nil
}

// GetPlayerByName returns the player with the specified name
func (ts TableState) GetPlayerByName(name string) *BotPlayerState {
	for i, player := range ts.Players {
		if player.Name == name {
			return &ts.Players[i]
		}
	}
	return nil
}

// GetActivePlayers returns all players who are still active in the hand
func (ts TableState) GetActivePlayers() []BotPlayerState {
	var active []BotPlayerState
	for _, player := range ts.Players {
		if player.IsActive && !player.IsFolded {
			active = append(active, player)
		}
	}
	return active
}

// GetCommunityCardCount returns the number of community cards revealed
func (ts TableState) GetCommunityCardCount() int {
	return len(ts.CommunityCards)
}

// IsPreflop returns true if it's the preflop betting round
func (ts TableState) IsPreflop() bool {
	return ts.CurrentRound == RoundPreflop
}

// IsFlop returns true if it's the flop betting round
func (ts TableState) IsFlop() bool {
	return ts.CurrentRound == RoundFlop
}

// IsTurn returns true if it's the turn betting round
func (ts TableState) IsTurn() bool {
	return ts.CurrentRound == RoundTurn
}

// IsRiver returns true if it's the river betting round
func (ts TableState) IsRiver() bool {
	return ts.CurrentRound == RoundRiver
}
