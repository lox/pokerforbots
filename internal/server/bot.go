package server

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
	"github.com/rs/zerolog/log"
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
	closed       bool                 // Track if bot is closed
	done         chan struct{}        // Signal channel closure
	actionChan   chan protocol.Action // Channel to send actions to hand runner
	handRunnerMu sync.RWMutex
}

// NewBot creates a new bot instance
func NewBot(id string, conn *websocket.Conn, pool *BotPool) *Bot {
	return &Bot{
		ID:       id,
		conn:     conn,
		send:     make(chan []byte, 256),
		pool:     pool,
		lastPing: time.Now(),
		done:     make(chan struct{}),
	}
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
func (b *Bot) SetActionChannel(ch chan protocol.Action) {
	b.handRunnerMu.Lock()
	defer b.handRunnerMu.Unlock()
	b.actionChan = ch
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
		b.pool.Unregister(b)
		_ = b.conn.Close()
		b.mu.Lock()
		if !b.closed {
			b.closed = true
			close(b.done)
		}
		b.mu.Unlock()
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
				log.Error().Err(err).Str("bot_id", b.ID).Msg("Unexpected WebSocket close error")
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
			// Forward to hand runner via action channel
			b.handRunnerMu.RLock()
			if b.actionChan != nil {
				select {
				case b.actionChan <- action:
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
		b.mu.Lock()
		if !b.closed {
			b.closed = true
			close(b.done)
		}
		b.mu.Unlock()
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
