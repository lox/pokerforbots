package server

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
	"github.com/rs/zerolog"
)

// Bot represents a connected bot client
type Bot struct {
	ID           string
	conn         *websocket.Conn
	send         chan []byte
	pool         *BotPool
	inHand       bool
	mu           sync.RWMutex
	lastPing     time.Time
	closed       bool                // Track if bot is closed
	done         chan struct{}       // Signal channel closure
	actionChan   chan ActionEnvelope // Channel to send actions to hand runner with bot ID
	handRunnerMu sync.RWMutex
	bankroll     int // Total chips the bot has
	logger       zerolog.Logger
	displayName  string
	gameID       string
	role         BotRole
}

func (b *Bot) close() {
	b.mu.Lock()
	if !b.closed {
		b.closed = true
		close(b.done)
	}
	b.mu.Unlock()
}

// Done returns a channel that is closed when the bot connection shuts down.
func (b *Bot) Done() <-chan struct{} {
	return b.done
}

// IsClosed reports whether the bot connection has been closed.
func (b *Bot) IsClosed() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.closed
}

const (
	defaultMaxBuyIn   = 1000 // 100 big blinds at 5/10
	defaultBankrollBB = 100  // bots keep 100 buy-ins by default
)

// NewBot creates a new bot instance
func NewBot(logger zerolog.Logger, id string, conn *websocket.Conn, pool *BotPool) *Bot {
	maxBuyIn := defaultMaxBuyIn
	if pool != nil && pool.config.StartChips > 0 {
		maxBuyIn = pool.config.StartChips
	}

	bankroll := maxBuyIn * defaultBankrollBB

	return &Bot{
		ID:       id,
		conn:     conn,
		send:     make(chan []byte, 256),
		pool:     pool,
		lastPing: time.Now(),
		done:     make(chan struct{}),
		bankroll: bankroll,
		logger:   logger.With().Str("component", "bot").Str("bot_id", id).Logger(),
		role:     BotRoleNPC,
	}
}

// SetDisplayName stores the bot's preferred display name from the connect message.
func (b *Bot) SetDisplayName(name string) {
	b.mu.Lock()
	b.displayName = name
	b.mu.Unlock()
}

// DisplayName returns the last recorded display name for the bot.
func (b *Bot) DisplayName() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.displayName
}

// SetGameID records the game identifier the bot is currently assigned to.
func (b *Bot) SetGameID(gameID string) {
	b.mu.Lock()
	b.gameID = gameID
	b.mu.Unlock()
}

// GameID returns the identifier of the game the bot is assigned to.
func (b *Bot) GameID() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.gameID
}

// SetRole sets the semantic role for the bot (player/npc).
func (b *Bot) SetRole(role BotRole) {
	b.mu.Lock()
	b.role = role
	b.mu.Unlock()
}

// Role returns the bot's role.
func (b *Bot) Role() BotRole {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.role
}

// SendMessage sends a protocol message to the bot
func (b *Bot) SendMessage(msg any) error {
	// Check if bot is closed
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return ErrBotClosed
	}
	b.mu.RUnlock()

	data, err := protocol.Marshal(msg)
	if err != nil {
		return err
	}

	select {
	case b.send <- data:
		return nil
	case <-b.done:
		return ErrBotClosed
	case <-time.After(time.Second):
		return ErrSendTimeout
	}
}

// SetInHand marks the bot as being in a hand or not
func (b *Bot) SetInHand(inHand bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.inHand = inHand
}

// IsInHand returns whether the bot is currently in a hand
func (b *Bot) IsInHand() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.inHand
}

// SetActionChannel sets the channel for sending actions to hand runner
func (b *Bot) SetActionChannel(ch chan ActionEnvelope) {
	b.handRunnerMu.Lock()
	defer b.handRunnerMu.Unlock()
	b.actionChan = ch
}

// GetBuyIn returns the buy-in amount for this bot (capped at the table's starting stack)
func (b *Bot) GetBuyIn() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	maxBuyIn := defaultMaxBuyIn
	if b.pool != nil && b.pool.config.StartChips > 0 {
		maxBuyIn = b.pool.config.StartChips
	}

	if b.bankroll >= maxBuyIn {
		return maxBuyIn
	}
	return b.bankroll // Return remaining bankroll if less than max buy-in
}

// ApplyResult applies the P&L delta to the bot's bankroll
func (b *Bot) ApplyResult(delta int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Apply the delta (can be positive or negative)
	b.bankroll += delta

	// Ensure bankroll doesn't go negative
	if b.bankroll < 0 {
		b.bankroll = 0
	}
}

// HasChips returns true if the bot has chips to play
func (b *Bot) HasChips() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.bankroll > 0
}

// ClearActionChannel clears the action channel
func (b *Bot) ClearActionChannel() {
	b.handRunnerMu.Lock()
	defer b.handRunnerMu.Unlock()
	b.actionChan = nil
}

// ReadPump reads messages from the websocket connection
func (b *Bot) ReadPump() {
	defer func() {
		b.close()
		b.pool.Unregister(b)
		_ = b.conn.Close()
	}()

	_ = b.conn.SetReadDeadline(time.Now().Add(pongWait))
	b.conn.SetPongHandler(func(string) error {
		_ = b.conn.SetReadDeadline(time.Now().Add(pongWait))
		b.mu.Lock()
		b.lastPing = time.Now()
		b.mu.Unlock()
		return nil
	})

	for {
		_, message, err := b.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				b.logger.Error().Err(err).Msg("Unexpected WebSocket close error")
			}
			break
		}

		// Parse message
		var action protocol.Action
		if err := protocol.Unmarshal(message, &action); err != nil {
			// Send error response
			continue
		}

		// Handle action if bot is in a hand
		if b.IsInHand() {
			// Wrap action in envelope with bot ID for verification
			envelope := ActionEnvelope{
				BotID:  b.ID,
				Action: action,
			}

			// Forward to hand runner via action channel
			b.handRunnerMu.RLock()
			if b.actionChan != nil {
				select {
				case b.actionChan <- envelope:
					// Action sent successfully
				default:
					// Channel full or closed, ignore
				}
			}
			b.handRunnerMu.RUnlock()
		}
	}
}

// WritePump writes messages to the websocket connection
func (b *Bot) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = b.conn.Close()
		b.close()
	}()

	for {
		select {
		case message, ok := <-b.send:
			b.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				b.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := b.conn.WriteMessage(websocket.BinaryMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			b.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := b.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
