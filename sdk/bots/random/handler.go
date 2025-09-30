package random

import (
	rand "math/rand/v2"
	"time"

	"github.com/lox/pokerforbots/protocol"
	"github.com/lox/pokerforbots/sdk/client"
)

// Handler implements a random strategy that makes random valid actions
type Handler struct {
	rng *rand.Rand
}

func NewHandler() *Handler {
	return &Handler{
		rng: rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0)),
	}
}

func (*Handler) OnHandStart(*client.GameState, protocol.HandStart) error         { return nil }
func (*Handler) OnGameUpdate(*client.GameState, protocol.GameUpdate) error       { return nil }
func (*Handler) OnPlayerAction(*client.GameState, protocol.PlayerAction) error   { return nil }
func (*Handler) OnStreetChange(*client.GameState, protocol.StreetChange) error   { return nil }
func (*Handler) OnHandResult(*client.GameState, protocol.HandResult) error       { return nil }
func (*Handler) OnGameCompleted(*client.GameState, protocol.GameCompleted) error { return nil }

func (h *Handler) OnActionRequest(_ *client.GameState, req protocol.ActionRequest) (string, int, error) {
	action := req.ValidActions[h.rng.IntN(len(req.ValidActions))]
	amount := 0
	// Protocol v2: Only "raise" exists (no "bet" in simplified protocol)
	if action == "raise" {
		amount = req.MinBet
	}
	return action, amount, nil
}

// Check it implements the client.Handler interface
var _ client.Handler = (*Handler)(nil)
