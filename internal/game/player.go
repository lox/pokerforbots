package game

import (
	"github.com/lox/pokerforbots/v2/poker"
)

// Player represents a player in a hand
type Player struct {
	Seat      int
	Name      string
	Chips     int
	HoleCards poker.Hand
	Folded    bool
	AllInFlag bool
	Bet       int // Current bet in this round
	TotalBet  int // Total bet in the hand
}

// IsActive returns true if the player can still act
func (p *Player) IsActive() bool {
	return !p.Folded && !p.AllInFlag && p.Chips > 0
}
