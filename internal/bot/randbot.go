package bot

import (
	"math/rand"

	"github.com/charmbracelet/log"
	"github.com/lox/holdem-cli/internal/game"
)

// RandBot is a simple bot that makes uniform random legal actions
type RandBot struct {
	rng    *rand.Rand
	logger *log.Logger
}

// NewRandBot creates a new RandBot instance
func NewRandBot(rng *rand.Rand, logger *log.Logger) *RandBot {
	return &RandBot{rng: rng, logger: logger}
}

func (r *RandBot) MakeDecision(tableState game.TableState, validActions []game.ValidAction) game.Decision {
	if len(validActions) == 0 {
		return game.Decision{Action: game.Fold, Amount: 0, Reasoning: "rand-bot no valid actions"}
	}

	// Pick random valid action
	randomAction := validActions[r.rng.Intn(len(validActions))]

	// For raises, pick random amount between min and max
	amount := randomAction.MinAmount
	if randomAction.Action == game.Raise && randomAction.MaxAmount > randomAction.MinAmount {
		amount = randomAction.MinAmount + r.rng.Intn(randomAction.MaxAmount-randomAction.MinAmount+1)
	}

	return game.Decision{Action: randomAction.Action, Amount: amount, Reasoning: "rand-bot random action"}
}
