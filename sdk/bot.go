package sdk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
	"github.com/rs/zerolog"
)

// Handler defines the interface for bot decision-making
type Handler interface {
	// OnHandStart is called when a new hand begins
	OnHandStart(state *GameState, start protocol.HandStart) error

	// OnActionRequest is called when the bot needs to make a decision
	OnActionRequest(state *GameState, req protocol.ActionRequest) (string, int, error)

	// OnGameUpdate is called when the game state changes
	OnGameUpdate(state *GameState, update protocol.GameUpdate) error

	// OnPlayerAction is called when another player acts
	OnPlayerAction(state *GameState, action protocol.PlayerAction) error

	// OnStreetChange is called when the betting round changes
	OnStreetChange(state *GameState, street protocol.StreetChange) error

	// OnHandResult is called when a hand completes
	OnHandResult(state *GameState, result protocol.HandResult) error

	// OnGameCompleted is called when the game finishes (optional, return io.EOF to exit)
	OnGameCompleted(state *GameState, completed protocol.GameCompleted) error
}

// GameState holds the current table state
type GameState struct {
	HandID        string
	Seat          int
	Pot           int
	Chips         int
	StartingChips int
	Players       []protocol.Player
	LastAction    protocol.PlayerAction
	HoleCards     []string
	Board         []string
	Street        string
	Button        int
	ActiveCount   int
}

// Bot provides a simple framework for poker bot implementations
type Bot struct {
	id      string
	conn    *websocket.Conn
	logger  zerolog.Logger
	handler Handler
	state   *GameState
}

// New creates a new bot with the given handler
func New(id string, handler Handler, logger zerolog.Logger) *Bot {
	return &Bot{
		id:      id,
		logger:  logger.With().Str("bot_id", id).Logger(),
		handler: handler,
		state:   &GameState{},
	}
}

// Connect establishes a websocket connection and sends the connect message
func (b *Bot) Connect(serverURL string) error {
	u, err := url.Parse(serverURL)
	if err != nil {
		return err
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	b.conn = conn

	connect := &protocol.Connect{
		Type: protocol.TypeConnect,
		Name: b.id,
		Role: "player",
	}
	payload, err := protocol.Marshal(connect)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, payload)
}

// Run starts the bot's main loop with context support
func (b *Bot) Run(ctx context.Context) error {
	if b.conn == nil {
		return errors.New("not connected")
	}
	defer b.conn.Close()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Set read timeout to avoid hanging on shutdown
		b.conn.SetReadDeadline(time.Now().Add(2 * time.Second))

		msgType, data, err := b.conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				return nil
			}
			return err
		}
		if msgType != websocket.BinaryMessage {
			continue
		}

		if err := b.handle(data); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			b.logger.Error().Err(err).Msg("handler error")
		}
	}
}

// ID returns the bot's ID
func (b *Bot) ID() string {
	return b.id
}

// State returns the current game state
func (b *Bot) State() *GameState {
	return b.state
}

func (b *Bot) handle(data []byte) error {
	// Try each message type in order of likelihood
	if b.tryActionRequest(data) {
		return nil
	}
	if b.tryHandStart(data) {
		return nil
	}
	if b.tryGameUpdate(data) {
		return nil
	}
	if b.tryPlayerAction(data) {
		return nil
	}
	if b.tryStreetChange(data) {
		return nil
	}
	if b.tryHandResult(data) {
		return nil
	}
	return b.tryGameCompleted(data)
}

func (b *Bot) tryHandStart(data []byte) bool {
	var start protocol.HandStart
	if err := protocol.Unmarshal(data, &start); err != nil || start.Type != protocol.TypeHandStart {
		return false
	}

	b.state.HandID = start.HandID
	b.state.Seat = start.YourSeat
	b.state.Players = start.Players
	b.state.Chips = start.Players[start.YourSeat].Chips
	b.state.StartingChips = b.state.Chips
	b.state.HoleCards = start.HoleCards
	b.state.Board = nil
	b.state.Street = "preflop"
	b.state.Button = start.Button
	b.updateActiveCount()

	if err := b.handler.OnHandStart(b.state, start); err != nil {
		b.logger.Error().Err(err).Msg("OnHandStart error")
	}
	return true
}

func (b *Bot) tryGameUpdate(data []byte) bool {
	var update protocol.GameUpdate
	if err := protocol.Unmarshal(data, &update); err != nil || update.Type != protocol.TypeGameUpdate {
		return false
	}

	b.state.Pot = update.Pot
	b.state.Players = update.Players
	if b.state.Seat >= 0 && b.state.Seat < len(update.Players) {
		b.state.Chips = update.Players[b.state.Seat].Chips
	}
	b.updateActiveCount()

	if err := b.handler.OnGameUpdate(b.state, update); err != nil {
		b.logger.Error().Err(err).Msg("OnGameUpdate error")
	}
	return true
}

func (b *Bot) tryPlayerAction(data []byte) bool {
	var action protocol.PlayerAction
	if err := protocol.Unmarshal(data, &action); err != nil || action.Type != protocol.TypePlayerAction {
		return false
	}

	b.state.LastAction = action

	if err := b.handler.OnPlayerAction(b.state, action); err != nil {
		b.logger.Error().Err(err).Msg("OnPlayerAction error")
	}
	return true
}

func (b *Bot) tryStreetChange(data []byte) bool {
	var street protocol.StreetChange
	if err := protocol.Unmarshal(data, &street); err != nil || street.Type != protocol.TypeStreetChange {
		return false
	}

	b.state.Street = street.Street
	b.state.Board = street.Board

	if err := b.handler.OnStreetChange(b.state, street); err != nil {
		b.logger.Error().Err(err).Msg("OnStreetChange error")
	}
	return true
}

func (b *Bot) tryHandResult(data []byte) bool {
	var result protocol.HandResult
	if err := protocol.Unmarshal(data, &result); err != nil || result.Type != protocol.TypeHandResult {
		return false
	}

	// Adjust our chip count with payout from winners, since no post-award GameUpdate is sent.
	// Determine our displayed name as seen in this message
	labels := []string{b.id}
	if len(b.id) >= 8 {
		labels = append(labels, b.id[:8])
	}
	labels = append(labels, fmt.Sprintf("player-%d", b.state.Seat+1))
	labels = append(labels, fmt.Sprintf("bot-%d", b.state.Seat+1))

	payout := 0
	for _, w := range result.Winners {
		// Prefer matching by hole cards when available
		if len(w.HoleCards) == 2 && len(b.state.HoleCards) == 2 {
			wc1, wc2 := w.HoleCards[0], w.HoleCards[1]
			mc1, mc2 := b.state.HoleCards[0], b.state.HoleCards[1]
			if (wc1 == mc1 && wc2 == mc2) || (wc1 == mc2 && wc2 == mc1) {
				payout += w.Amount
				continue
			}
		}
		for _, lab := range labels {
			if w.Name == lab {
				payout += w.Amount
				break
			}
		}
	}
	if payout > 0 {
		b.state.Chips += payout
	}

	if err := b.handler.OnHandResult(b.state, result); err != nil {
		b.logger.Error().Err(err).Msg("OnHandResult error")
	}
	return true
}

func (b *Bot) tryActionRequest(data []byte) bool {
	var req protocol.ActionRequest
	if err := protocol.Unmarshal(data, &req); err != nil || req.Type != protocol.TypeActionRequest {
		return false
	}

	action, amount, err := b.handler.OnActionRequest(b.state, req)
	if err != nil {
		b.logger.Error().Err(err).Msg("OnActionRequest error")
		action, amount = "fold", 0 // Fallback to fold
	}

	act := protocol.Action{
		Type:   protocol.TypeAction,
		Action: action,
		Amount: amount,
	}

	payload, err := protocol.Marshal(&act)
	if err != nil {
		b.logger.Error().Err(err).Msg("marshal action error")
		return true
	}

	if err := b.conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		b.logger.Error().Err(err).Msg("send action error")
	}
	return true
}

func (b *Bot) tryGameCompleted(data []byte) error {
	var completed protocol.GameCompleted
	if err := protocol.Unmarshal(data, &completed); err != nil || completed.Type != protocol.TypeGameCompleted {
		return nil
	}

	return b.handler.OnGameCompleted(b.state, completed)
}

func (b *Bot) updateActiveCount() {
	active := 0
	for _, p := range b.state.Players {
		if !p.Folded {
			active++
		}
	}
	b.state.ActiveCount = active
}
