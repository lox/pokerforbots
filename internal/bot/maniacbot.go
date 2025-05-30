package bot

import (
	"math/rand"

	"github.com/charmbracelet/log"
	"github.com/lox/holdem-cli/internal/game"
)

// ManiacBot is an extremely aggressive bot that shoves frequently
type ManiacBot struct {
	rng    *rand.Rand
	logger *log.Logger
}

// NewManiacBot creates a new ManiacBot instance
func NewManiacBot(rng *rand.Rand, logger *log.Logger) *ManiacBot {
	return &ManiacBot{rng: rng, logger: logger}
}

func (m *ManiacBot) MakeDecision(tableState game.TableState, validActions []game.ValidAction) game.Decision {
	actingPlayer := tableState.Players[tableState.ActingPlayerIdx]
	
	// Maniac strategy: raise/shove very frequently, call sometimes, rarely fold
	var hasCheck, hasCall, hasRaise, hasAllIn bool
	var raiseMin, raiseMax int
	
	for _, action := range validActions {
		switch action.Action {
		case game.Check:
			hasCheck = true
		case game.Call:
			hasCall = true
		case game.Raise:
			hasRaise = true
			raiseMin = action.MinAmount
			raiseMax = action.MaxAmount
		case game.AllIn:
			hasAllIn = true
		}
	}
	
	if hasCheck {
		// We can check - but maniacs prefer to bet
		if m.rng.Float64() < 0.85 { // 85% chance to bet
			if actingPlayer.Chips <= 20*tableState.BigBlind || m.rng.Float64() < 0.3 {
				// Shove if short stack or 30% of the time
				if hasAllIn {
					return game.Decision{Action: game.AllIn, Amount: 0, Reasoning: "maniac shove"}
				} else if hasRaise {
					return game.Decision{Action: game.Raise, Amount: raiseMax, Reasoning: "maniac max raise"}
				}
			} else if hasRaise {
				// Big raise
				raiseSize := raiseMin + (raiseMax-raiseMin)*3/4 // Use 75% of max range
				return game.Decision{Action: game.Raise, Amount: raiseSize, Reasoning: "maniac big raise"}
			}
		}
		return game.Decision{Action: game.Check, Amount: 0, Reasoning: "maniac checking"}
	} else {
		// Facing a bet
		randValue := m.rng.Float64()
		
		if randValue < 0.4 { // 40% chance to shove
			if hasAllIn {
				return game.Decision{Action: game.AllIn, Amount: 0, Reasoning: "maniac shove over bet"}
			} else if hasRaise {
				return game.Decision{Action: game.Raise, Amount: raiseMax, Reasoning: "maniac max raise over bet"}
			}
		} 
		
		if randValue < 0.8 && hasCall { // 40% chance to call (total 80%)
			return game.Decision{Action: game.Call, Amount: 0, Reasoning: "maniac call"}
		}
		
		// 20% chance to fold
		return game.Decision{Action: game.Fold, Amount: 0, Reasoning: "maniac fold"}
	}
}


