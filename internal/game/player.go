package game

import (
	"fmt"

	"github.com/lox/holdem-cli/internal/deck"
	"github.com/lox/holdem-cli/internal/evaluator"
)

// PlayerType represents the type of player
type PlayerType int

const (
	Human PlayerType = iota
	AI
)

// String returns the string representation of a player type
func (pt PlayerType) String() string {
	switch pt {
	case Human:
		return "Human"
	case AI:
		return "AI"
	default:
		return "Unknown"
	}
}

// Position represents a player's position at the table
type Position int

const (
	UnknownPosition Position = iota
	SmallBlind
	BigBlind
	UnderTheGun
	EarlyPosition
	MiddlePosition
	LatePosition
	Cutoff
	Button
)

// String returns the string representation of a position
func (p Position) String() string {
	switch p {
	case SmallBlind:
		return "Small Blind"
	case BigBlind:
		return "Big Blind"
	case UnderTheGun:
		return "Under the Gun"
	case EarlyPosition:
		return "Early Position"
	case MiddlePosition:
		return "Middle Position"
	case LatePosition:
		return "Late Position"
	case Cutoff:
		return "Cutoff"
	case Button:
		return "Button"
	default:
		return "Unknown"
	}
}

// Action represents a player's action
type Action int

const (
	NoAction Action = iota
	Fold
	Call
	Raise
	Check
	AllIn
)

// String returns the string representation of an action
func (a Action) String() string {
	switch a {
	case Fold:
		return "fold"
	case Call:
		return "call"
	case Raise:
		return "raise"
	case Check:
		return "check"
	case AllIn:
		return "all-in"
	default:
		return "no action"
	}
}

// Player represents a poker player
type Player struct {
	ID         int         // Unique identifier
	Name       string      // Player name
	Type       PlayerType  // Human or AI
	Chips      int         // Current chip count
	HoleCards  []deck.Card // Player's hole cards
	Position   Position    // Current position at table
	SeatNumber int         // Seat number (1-based)

	// Current hand state
	IsActive     bool // Still in the hand
	IsFolded     bool // Has folded this hand
	IsAllIn      bool // Is all-in
	BetThisRound int  // Amount bet in current betting round
	TotalBet     int  // Total amount bet this hand

	// Action tracking
	LastAction   Action // Last action taken
	ActionAmount int    // Amount of last action (for raises/calls)
}

// NewPlayer creates a new player
func NewPlayer(id int, name string, playerType PlayerType, startingChips int) *Player {
	return &Player{
		ID:           id,
		Name:         name,
		Type:         playerType,
		Chips:        startingChips,
		HoleCards:    make([]deck.Card, 0, 2),
		Position:     UnknownPosition,
		IsActive:     true,
		IsFolded:     false,
		IsAllIn:      false,
		BetThisRound: 0,
		TotalBet:     0,
		LastAction:   NoAction,
		ActionAmount: 0,
	}
}

// String returns a string representation of the player
func (p *Player) String() string {
	return fmt.Sprintf("%s (Seat %d) - $%d", p.Name, p.SeatNumber, p.Chips)
}

// DealHoleCards deals hole cards to the player
func (p *Player) DealHoleCards(cards []deck.Card) {
	p.HoleCards = make([]deck.Card, len(cards))
	copy(p.HoleCards, cards)
}

// Fold makes the player fold
func (p *Player) Fold() {
	p.IsFolded = true
	p.IsActive = false
	p.LastAction = Fold
	p.ActionAmount = 0
}

// Call makes the player call the current bet
func (p *Player) Call(amount int) bool {
	if amount > p.Chips {
		// Not enough chips to call, go all-in instead
		return p.AllIn()
	}

	p.Chips -= amount
	p.BetThisRound += amount
	p.TotalBet += amount
	p.LastAction = Call
	p.ActionAmount = amount
	
	// If player has no chips left after call, mark as all-in
	if p.Chips == 0 {
		p.IsAllIn = true
	}
	
	return true
}

// Raise makes the player raise
func (p *Player) Raise(totalAmount int) bool {
	if totalAmount > p.Chips {
		return false // Not enough chips
	}

	p.Chips -= totalAmount
	p.BetThisRound += totalAmount
	p.TotalBet += totalAmount
	p.LastAction = Raise
	p.ActionAmount = totalAmount
	
	// If player has no chips left after raise, mark as all-in
	if p.Chips == 0 {
		p.IsAllIn = true
	}
	
	return true
}

// Check makes the player check
func (p *Player) Check() {
	p.LastAction = Check
	p.ActionAmount = 0
}

// AllIn makes the player go all-in
func (p *Player) AllIn() bool {
	if p.Chips <= 0 {
		return false
	}

	amount := p.Chips
	p.Chips = 0
	p.BetThisRound += amount
	p.TotalBet += amount
	p.IsAllIn = true
	p.LastAction = AllIn
	p.ActionAmount = amount
	return true
}

// CanAct returns true if the player can take an action
func (p *Player) CanAct() bool {
	return p.IsActive && !p.IsFolded && !p.IsAllIn
}

// ResetForNewHand resets player state for a new hand
func (p *Player) ResetForNewHand() {
	p.HoleCards = p.HoleCards[:0]
	p.IsActive = true
	p.IsFolded = false
	p.IsAllIn = false
	p.BetThisRound = 0
	p.TotalBet = 0
	p.LastAction = NoAction
	p.ActionAmount = 0
}

// ResetForNewRound resets player state for a new betting round
func (p *Player) ResetForNewRound() {
	p.BetThisRound = 0
	p.LastAction = NoAction
	p.ActionAmount = 0
}

// GetBestHand returns the player's best hand rank given community cards
func (p *Player) GetBestHand(communityCards []deck.Card) evaluator.HandRank {
	if len(p.HoleCards) != 2 {
		panic("Player must have exactly 2 hole cards")
	}

	// Combine hole cards and community cards
	allCards := make([]deck.Card, 0, 7)
	allCards = append(allCards, p.HoleCards...)
	allCards = append(allCards, communityCards...)

	return evaluator.Evaluate7(allCards)
}

// IsInHand returns true if the player is still in the hand
func (p *Player) IsInHand() bool {
	return p.IsActive && !p.IsFolded
}

// HasActed returns true if the player has taken an action this round
func (p *Player) HasActed() bool {
	return p.LastAction != NoAction
}

// GetEffectiveStack returns the effective stack (current chips + any bets this hand)
func (p *Player) GetEffectiveStack() int {
	return p.Chips + p.TotalBet
}
