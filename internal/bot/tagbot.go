package bot

import (
	"math/rand"

	"github.com/charmbracelet/log"
	"github.com/lox/holdem-cli/internal/game"
)

// TAGBot is a Tight Aggressive bot that plays premium hands aggressively
type TAGBot struct {
	rng    *rand.Rand
	logger *log.Logger
}

// NewTAGBot creates a new TAGBot instance
func NewTAGBot(rng *rand.Rand, logger *log.Logger) *TAGBot {
	return &TAGBot{rng: rng, logger: logger}
}

func (t *TAGBot) MakeDecision(tableState game.TableState, validActions []game.ValidAction) game.Decision {
	actingPlayer := tableState.Players[tableState.ActingPlayerIdx]

	// Simplified TAG logic for now - play tight ranges
	if tableState.CurrentRound == game.PreFlop {
		if len(actingPlayer.HoleCards) == 2 {
			card1, card2 := actingPlayer.HoleCards[0], actingPlayer.HoleCards[1]

			// Play premium hands only
			isPremium := false
			if card1.Rank == card2.Rank && card1.Rank >= 10 { // TT+
				isPremium = true
			} else if (card1.Rank == 14 && card2.Rank >= 12) || (card2.Rank == 14 && card1.Rank >= 12) { // AK, AQ
				isPremium = true
			}

			if isPremium {
				for _, action := range validActions {
					if action.Action == game.Raise {
						return game.Decision{Action: game.Raise, Amount: action.MinAmount + (action.MaxAmount-action.MinAmount)/4, Reasoning: "TAG raise premium"}
					}
				}
			}
		}
	}

	// Default tight behavior - check/call, rarely raise
	for _, action := range validActions {
		if action.Action == game.Check {
			return game.Decision{Action: game.Check, Amount: 0, Reasoning: "TAG check"}
		}
	}

	if t.rng.Float64() < 0.3 { // 30% call rate
		for _, action := range validActions {
			if action.Action == game.Call {
				return game.Decision{Action: game.Call, Amount: 0, Reasoning: "TAG call"}
			}
		}
	}

	return game.Decision{Action: game.Fold, Amount: 0, Reasoning: "TAG fold"}
}
