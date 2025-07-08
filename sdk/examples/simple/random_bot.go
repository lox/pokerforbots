package simple

import (
	"math/rand"
	"time"

	"github.com/lox/pokerforbots/sdk"
)

// RandomBot makes random decisions from valid actions
type RandomBot struct {
	name string
	rng  *rand.Rand
}

// NewRandomBot creates a new RandomBot
func NewRandomBot(name string) *RandomBot {
	return &RandomBot{
		name: name,
		rng:  rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// MakeDecision implements the Agent interface - makes random valid decisions
func (rb *RandomBot) MakeDecision(tableState sdk.TableState, validActions []sdk.ValidAction) sdk.Decision {
	if len(validActions) == 0 {
		return sdk.NewFoldDecision("No valid actions")
	}

	// Pick a random action
	action := validActions[rb.rng.Intn(len(validActions))]

	// If it's a raise action, pick a random amount between min and max
	if action.Action == sdk.ActionRaise {
		amount := action.MinAmount
		if action.MaxAmount > action.MinAmount {
			amount = action.MinAmount + rb.rng.Intn(action.MaxAmount-action.MinAmount+1)
		}
		return sdk.NewRaiseDecision(amount, "RandomBot random raise")
	}

	// For other actions, use the convenience functions
	switch action.Action {
	case sdk.ActionFold:
		return sdk.NewFoldDecision("RandomBot random fold")
	case sdk.ActionCall:
		return sdk.NewCallDecision("RandomBot random call")
	case sdk.ActionCheck:
		return sdk.NewCheckDecision("RandomBot random check")
	case sdk.ActionAllIn:
		return sdk.NewAllInDecision("RandomBot random all-in")
	default:
		return sdk.NewFoldDecision("RandomBot unknown action")
	}
}
