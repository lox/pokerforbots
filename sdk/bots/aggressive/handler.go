package aggressive

import (
	rand "math/rand/v2"
	"slices"
	"time"

	"github.com/lox/pokerforbots/protocol"
	"github.com/lox/pokerforbots/sdk/client"
)

// Handler implements an aggressive strategy that raises 70% of the time when possible
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
	// Raise 70% of the time when possible
	if (slices.Contains(req.ValidActions, "raise") || slices.Contains(req.ValidActions, "bet")) && h.rng.Float64() < 0.7 {
		// MinBet is the minimum total bet/raise amount (not the increment)
		if slices.Contains(req.ValidActions, "raise") {
			return "raise", req.MinBet, nil
		}
		return "bet", req.MinBet, nil
	}
	if slices.Contains(req.ValidActions, "call") {
		return "call", 0, nil
	}
	if slices.Contains(req.ValidActions, "check") {
		return "check", 0, nil
	}
	return "fold", 0, nil
}

// Check it implements the client.Handler interface
var _ client.Handler = (*Handler)(nil)
