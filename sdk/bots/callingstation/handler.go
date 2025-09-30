package callingstation

import (
	"slices"

	"github.com/lox/pokerforbots/protocol"
	"github.com/lox/pokerforbots/sdk/client"
)

// Handler implements a calling station strategy that always calls or checks
type Handler struct{}

func (Handler) OnHandStart(*client.GameState, protocol.HandStart) error         { return nil }
func (Handler) OnGameUpdate(*client.GameState, protocol.GameUpdate) error       { return nil }
func (Handler) OnPlayerAction(*client.GameState, protocol.PlayerAction) error   { return nil }
func (Handler) OnStreetChange(*client.GameState, protocol.StreetChange) error   { return nil }
func (Handler) OnHandResult(*client.GameState, protocol.HandResult) error       { return nil }
func (Handler) OnGameCompleted(*client.GameState, protocol.GameCompleted) error { return nil }

func (Handler) OnActionRequest(_ *client.GameState, req protocol.ActionRequest) (string, int, error) {
	// Calling station strategy: always check or call, never raise
	if slices.Contains(req.ValidActions, "check") {
		return "check", 0, nil
	}
	if slices.Contains(req.ValidActions, "call") {
		return "call", 0, nil
	}
	return "fold", 0, nil
}

// Check it implements the client.Handler interface
var _ client.Handler = (*Handler)(nil)
