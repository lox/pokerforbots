package server

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
)

// Bot represents a connected bot client
type Bot struct {
	ID       string
	conn     *websocket.Conn
	send     chan []byte
	pool     *BotPool
	inHand   bool
	mu       sync.RWMutex
	lastPing time.Time
}

// NewBot creates a new bot instance
func NewBot(id string, conn *websocket.Conn, pool *BotPool) *Bot {
	return &Bot{
		ID:       id,
		conn:     conn,
		send:     make(chan []byte, 256),
		pool:     pool,
		lastPing: time.Now(),
	}
}

// SendMessage sends a protocol message to the bot
func (b *Bot) SendMessage(msg interface{}) error {
	data, err := protocol.Marshal(msg)
	if err != nil {
		return err
	}

	// Use defer to recover from panic if channel is closed
	defer func() {
		if r := recover(); r != nil {
			// Channel was closed, ignore
		}
	}()

	select {
	case b.send <- data:
		return nil
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

// ReadPump reads messages from the websocket connection
func (b *Bot) ReadPump() {
	defer func() {
		b.pool.Unregister(b)
		b.conn.Close()
	}()

	b.conn.SetReadDeadline(time.Now().Add(pongWait))
	b.conn.SetPongHandler(func(string) error {
		b.conn.SetReadDeadline(time.Now().Add(pongWait))
		b.mu.Lock()
		b.lastPing = time.Now()
		b.mu.Unlock()
		return nil
	})

	for {
		_, message, err := b.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				// Log error
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
			// Forward to hand runner (will be implemented)
			// For now, just acknowledge
		}
	}
}

// WritePump writes messages to the websocket connection
func (b *Bot) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		b.conn.Close()
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