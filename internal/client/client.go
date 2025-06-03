package client

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/server" // Reuse message types
)

// Client represents a WebSocket client for the poker game
type Client struct {
	serverURL  string
	conn       *websocket.Conn
	send       chan *server.Message
	receive    chan *server.Message
	logger     *log.Logger
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.RWMutex
	connected  bool
	playerName string
	tableID    string
	closeOnce  sync.Once

	// Event handlers
	eventHandlers map[string][]EventHandler
}

// EventHandler is a function that handles incoming events
type EventHandler func(*server.Message)

// NewClient creates a new WebSocket client
func NewClient(serverURL string, logger *log.Logger) *Client {
	ctx, cancel := context.WithCancel(context.Background())

	return &Client{
		serverURL:     serverURL,
		send:          make(chan *server.Message, 256),
		receive:       make(chan *server.Message, 256),
		logger:        logger.WithPrefix("client"),
		ctx:           ctx,
		cancel:        cancel,
		eventHandlers: make(map[string][]EventHandler),
	}
}

// Connect establishes a WebSocket connection to the server
func (c *Client) Connect() error {
	c.logger.Info("Connecting to server", "url", c.serverURL)

	u, err := url.Parse(c.serverURL)
	if err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}

	// Convert http/https to ws/wss
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}

	// Add WebSocket path
	u.Path = "/ws"

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.mu.Unlock()

	go c.readPump()
	go c.writePump()
	go c.eventProcessor()

	c.logger.Info("Connected to server")
	return nil
}

// Disconnect closes the WebSocket connection
func (c *Client) Disconnect() error {
	c.closeOnce.Do(func() {
		c.cancel()

		c.mu.Lock()
		defer c.mu.Unlock()

		if c.conn != nil {
			_ = c.conn.Close() // Ignore close errors during shutdown
			c.connected = false
		}

		close(c.send)
		close(c.receive)

		c.logger.Info("Disconnected from server")
	})
	return nil
}

// IsConnected returns whether the client is connected
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// SendMessage sends a message to the server
func (c *Client) SendMessage(msg *server.Message) error {
	select {
	case c.send <- msg:
		return nil
	case <-c.ctx.Done():
		return c.ctx.Err()
	default:
		return fmt.Errorf("send buffer full")
	}
}

// readPump handles incoming messages from the server
func (c *Client) readPump() {
	defer func() {
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
	}()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		var msg server.Message
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.Error("WebSocket error", "error", err)
			}
			break
		}

		c.logger.Debug("Received message", "type", msg.Type)

		select {
		case c.receive <- &msg:
		case <-c.ctx.Done():
			return
		}
	}
}

// writePump handles outgoing messages to the server
func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second) // Ping interval
	defer func() {
		ticker.Stop()
		_ = c.conn.Close() // Ignore close errors during cleanup
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteJSON(message); err != nil {
				c.logger.Error("Failed to write message", "error", err)
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		case <-c.ctx.Done():
			return
		}
	}
}

// eventProcessor processes incoming messages and dispatches to handlers
func (c *Client) eventProcessor() {
	for {
		select {
		case msg := <-c.receive:
			c.handleMessage(msg)
		case <-c.ctx.Done():
			return
		}
	}
}

// handleMessage dispatches messages to registered handlers
func (c *Client) handleMessage(msg *server.Message) {
	c.mu.RLock()
	handlers, exists := c.eventHandlers[msg.Type]
	c.mu.RUnlock()

	if exists {
		for _, handler := range handlers {
			go handler(msg) // Handle asynchronously
		}
	} else {
		c.logger.Debug("No handler for message type", "type", msg.Type)
	}
}

// AddEventHandler adds an event handler for a specific message type
func (c *Client) AddEventHandler(messageType string, handler EventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.eventHandlers[messageType] = append(c.eventHandlers[messageType], handler)
}

// Auth performs authentication with the server
func (c *Client) Auth(playerName string) error {
	c.playerName = playerName

	authMsg, err := server.NewMessage("auth", server.AuthData{
		PlayerName: playerName,
	})
	if err != nil {
		return err
	}

	return c.SendMessage(authMsg)
}

// JoinTable joins a poker table
func (c *Client) JoinTable(tableID string, buyIn int) error {
	joinMsg, err := server.NewMessage("join_table", server.JoinTableData{
		TableID: tableID,
		BuyIn:   buyIn,
	})
	if err != nil {
		return err
	}

	return c.SendMessage(joinMsg)
}

// LeaveTable leaves the current poker table
func (c *Client) LeaveTable(tableID string) error {
	leaveMsg, err := server.NewMessage("leave_table", server.LeaveTableData{
		TableID: tableID,
	})
	if err != nil {
		return err
	}

	return c.SendMessage(leaveMsg)
}

// ListTables requests a list of available tables
func (c *Client) ListTables() error {
	listMsg, err := server.NewMessage("list_tables", map[string]interface{}{})
	if err != nil {
		return err
	}

	return c.SendMessage(listMsg)
}

// SendDecision sends a player decision to the server
func (c *Client) SendDecision(action string, amount int, reasoning string) error {
	decisionMsg, err := server.NewMessage("player_decision", server.PlayerDecisionData{
		TableID:   c.tableID,
		Action:    action,
		Amount:    amount,
		Reasoning: reasoning,
	})
	if err != nil {
		return err
	}

	return c.SendMessage(decisionMsg)
}

// AddBots adds bots to the current table
func (c *Client) AddBots(tableID string, count int) error {
	addBotMsg, err := server.NewMessage("add_bot", server.AddBotData{
		TableID: tableID,
		Count:   count,
	})
	if err != nil {
		return err
	}

	return c.SendMessage(addBotMsg)
}

// KickBot removes a bot from the current table
func (c *Client) KickBot(tableID string, botName string) error {
	kickBotMsg, err := server.NewMessage("kick_bot", server.KickBotData{
		TableID: tableID,
		BotName: botName,
	})
	if err != nil {
		return err
	}

	return c.SendMessage(kickBotMsg)
}

// SetTableID sets the current table ID
func (c *Client) SetTableID(tableID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tableID = tableID
}

// GetTableID returns the current table ID
func (c *Client) GetTableID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tableID
}

// GetPlayerName returns the player name
func (c *Client) GetPlayerName() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.playerName
}

// WaitForMessage waits for a specific message type with timeout
func (c *Client) WaitForMessage(messageType string, timeout time.Duration) (*server.Message, error) {
	responseChan := make(chan *server.Message, 1)

	// Add temporary handler
	handler := func(msg *server.Message) {
		select {
		case responseChan <- msg:
		default:
		}
	}

	c.AddEventHandler(messageType, handler)

	// Wait for response or timeout
	select {
	case msg := <-responseChan:
		return msg, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout waiting for %s", messageType)
	case <-c.ctx.Done():
		return nil, c.ctx.Err()
	}
}
