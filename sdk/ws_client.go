package sdk

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gorilla/websocket"
)

// EventHandler is a function that handles incoming messages
type EventHandler func(*Message)

// WSClient provides a WebSocket client for connecting to the poker server
type WSClient struct {
	serverURL     string
	conn          *websocket.Conn
	logger        *log.Logger
	mu            sync.RWMutex
	eventHandlers map[MessageType][]EventHandler
	connected     bool
	stopChan      chan struct{}
}

// NewWSClient creates a new WebSocket client
func NewWSClient(serverURL string, logger *log.Logger) *WSClient {
	return &WSClient{
		serverURL:     serverURL,
		logger:        logger,
		eventHandlers: make(map[MessageType][]EventHandler),
		stopChan:      make(chan struct{}),
	}
}

// Connect establishes a WebSocket connection to the server
func (c *WSClient) Connect() error {
	u, err := url.Parse(c.serverURL)
	if err != nil {
		return fmt.Errorf("invalid server URL: %w", err)
	}

	// Ensure WebSocket scheme
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
		// Already correct
	default:
		u.Scheme = "ws"
	}

	c.logger.Info("Connecting to server", "url", u.String())

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.connected = true
	c.mu.Unlock()

	// Start message reader
	go c.readMessages()

	return nil
}

// Disconnect closes the WebSocket connection
func (c *WSClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	c.connected = false
	close(c.stopChan)

	if c.conn != nil {
		// Send close message
		_ = c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		return c.conn.Close()
	}

	return nil
}

// SendMessage sends a message to the server
func (c *WSClient) SendMessage(msg *Message) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected || c.conn == nil {
		return fmt.Errorf("not connected")
	}

	return c.conn.WriteJSON(msg)
}

// SendRawMessage sends a raw JSON message to the server
func (c *WSClient) SendRawMessage(msgType MessageType, data json.RawMessage) error {
	msg := &Message{
		Type:      msgType,
		Data:      data,
		Timestamp: time.Now().UTC(),
	}
	return c.SendMessage(msg)
}

// AddEventHandler registers a handler for a specific message type
func (c *WSClient) AddEventHandler(msgType MessageType, handler EventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.eventHandlers[msgType] = append(c.eventHandlers[msgType], handler)
}

// RemoveEventHandlers removes all handlers for a specific message type
func (c *WSClient) RemoveEventHandlers(msgType MessageType) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.eventHandlers, msgType)
}

// readMessages continuously reads messages from the WebSocket connection
func (c *WSClient) readMessages() {
	defer func() {
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
	}()

	for {
		select {
		case <-c.stopChan:
			return
		default:
			var msg Message
			err := c.conn.ReadJSON(&msg)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					c.logger.Error("WebSocket error", "error", err)
				}
				return
			}

			// Dispatch to handlers
			c.dispatchMessage(&msg)
		}
	}
}

// dispatchMessage sends a message to all registered handlers
func (c *WSClient) dispatchMessage(msg *Message) {
	c.mu.RLock()
	handlers := c.eventHandlers[msg.Type]
	c.mu.RUnlock()

	for _, handler := range handlers {
		// Run handlers in goroutines to prevent blocking
		go handler(msg)
	}
}

// IsConnected returns whether the client is connected
func (c *WSClient) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// Auth sends an authentication message
func (c *WSClient) Auth(playerName string) error {
	c.logger.Info("Sending auth message", "playerName", playerName)
	data := AuthData{
		PlayerName: playerName,
	}
	msg, err := NewMessage(MessageTypeAuth, data)
	if err != nil {
		return err
	}
	return c.SendMessage(msg)
}

// JoinTable sends a join table message
func (c *WSClient) JoinTable(tableID string, buyIn int) error {
	data := JoinTableData{
		TableID: tableID,
		BuyIn:   buyIn,
	}
	msg, err := NewMessage(MessageTypeJoinTable, data)
	if err != nil {
		return err
	}
	return c.SendMessage(msg)
}

// LeaveTable sends a leave table message
func (c *WSClient) LeaveTable(tableID string) error {
	data := LeaveTableData{
		TableID: tableID,
	}
	msg, err := NewMessage(MessageTypeLeaveTable, data)
	if err != nil {
		return err
	}
	return c.SendMessage(msg)
}

// SendDecision sends a player decision
func (c *WSClient) SendDecision(tableID string, action string, amount int, reasoning string) error {
	data := PlayerDecisionData{
		TableID:   tableID,
		Action:    action,
		Amount:    amount,
		Reasoning: reasoning,
	}
	msg, err := NewMessage(MessageTypePlayerDecision, data)
	if err != nil {
		return err
	}
	return c.SendMessage(msg)
}

// ListTables sends a list tables message
func (c *WSClient) ListTables() error {
	msg, err := NewMessage(MessageTypeListTables, nil)
	if err != nil {
		return err
	}
	return c.SendMessage(msg)
}
