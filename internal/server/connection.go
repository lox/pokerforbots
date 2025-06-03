package server

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/gorilla/websocket"
)

// Connection represents a WebSocket connection to a client
type Connection struct {
	conn      *websocket.Conn
	send      chan *Message
	playerID  string
	tableID   string
	logger    *log.Logger
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.RWMutex
	closeOnce sync.Once
}

// NewConnection creates a new connection wrapper
func NewConnection(conn *websocket.Conn, logger *log.Logger) *Connection {
	ctx, cancel := context.WithCancel(context.Background())

	return &Connection{
		conn:   conn,
		send:   make(chan *Message, 256),
		logger: logger.WithPrefix("conn"),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start begins handling the connection
func (c *Connection) Start() {
	go c.writePump()
	go c.readPump()
}

// Close closes the connection
func (c *Connection) Close() error {
	var err error
	c.closeOnce.Do(func() {
		c.cancel()
		close(c.send)
		err = c.conn.Close()
	})
	return err
}

// SendMessage sends a message to the client
func (c *Connection) SendMessage(msg *Message) error {
	select {
	case c.send <- msg:
		return nil
	case <-c.ctx.Done():
		return c.ctx.Err()
	default:
		c.logger.Warn("Connection send buffer full, closing connection")
		_ = c.Close() // Ignore close errors
		return ErrConnectionClosed
	}
}

// SetPlayer associates this connection with a player
func (c *Connection) SetPlayer(playerID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.playerID = playerID
}

// GetPlayer returns the associated player ID
func (c *Connection) GetPlayer() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.playerID
}

// SetTable associates this connection with a table
func (c *Connection) SetTable(tableID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tableID = tableID
}

// GetTable returns the associated table ID
func (c *Connection) GetTable() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tableID
}

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 8192
)

var (
	ErrConnectionClosed = websocket.ErrCloseSent
)

// readPump handles incoming messages from the client
func (c *Connection) readPump() {
	defer func() { _ = c.Close() }() // Ignore close errors during cleanup

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		var msg Message
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.Error("WebSocket error", "error", err)
			}
			break
		}

		c.handleMessage(&msg)
	}
}

// writePump handles outgoing messages to the client
func (c *Connection) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close() // Ignore close errors during cleanup
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteJSON(message); err != nil {
				c.logger.Error("Failed to write message", "error", err)
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}

		case <-c.ctx.Done():
			return
		}
	}
}

// handleMessage processes incoming messages from the client
func (c *Connection) handleMessage(msg *Message) {
	c.logger.Debug("Received message", "type", msg.Type, "player", c.GetPlayer())

	switch msg.Type {
	case "auth":
		var data AuthData
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			c.sendError("invalid_message", "Failed to parse auth data")
			return
		}
		c.handleAuth(data)

	case "join_table":
		var data JoinTableData
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			c.sendError("invalid_message", "Failed to parse join table data")
			return
		}
		c.handleJoinTable(data)

	case "leave_table":
		var data LeaveTableData
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			c.sendError("invalid_message", "Failed to parse leave table data")
			return
		}
		c.handleLeaveTable(data)

	case "list_tables":
		c.handleListTables()

	case "player_decision":
		var data PlayerDecisionData
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			c.sendError("invalid_message", "Failed to parse player decision data")
			return
		}
		c.handlePlayerDecision(data)

	default:
		c.sendError("unknown_message_type", "Unknown message type: "+msg.Type)
	}
}

// sendError sends an error message to the client
func (c *Connection) sendError(code, message string) {
	errorMsg, err := NewMessage("error", ErrorData{
		Code:    code,
		Message: message,
	})
	if err != nil {
		c.logger.Error("Failed to create error message", "error", err)
		return
	}

	_ = c.SendMessage(errorMsg) // Ignore send errors during error handling
}

// GameService reference - set by server
var gameService *GameService

// SetGameService sets the game service for handling game operations
func SetGameService(gs *GameService) {
	gameService = gs
}

func (c *Connection) handleAuth(data AuthData) {
	c.logger.Info("Auth request", "playerName", data.PlayerName)

	// Simple authentication - just accept any player name
	if data.PlayerName == "" {
		c.sendError("invalid_auth", "Player name required")
		return
	}

	c.SetPlayer(data.PlayerName)

	response, _ := NewMessage("auth_response", AuthResponseData{
		Success:  true,
		PlayerID: data.PlayerName,
	})
	_ = c.SendMessage(response) // Ignore send errors
}

func (c *Connection) handleJoinTable(data JoinTableData) {
	c.logger.Info("Join table request", "tableId", data.TableID, "player", c.GetPlayer())

	if gameService == nil {
		c.sendError("service_unavailable", "Game service not available")
		return
	}

	playerName := c.GetPlayer()
	if playerName == "" {
		c.sendError("not_authenticated", "Must authenticate first")
		return
	}

	err := gameService.JoinTable(data.TableID, playerName, data.BuyIn)
	if err != nil {
		c.sendError("join_failed", err.Error())
		return
	}

	// Set table association
	c.SetTable(data.TableID)

	// Get table info for response
	table := gameService.GetTable(data.TableID)
	if table == nil {
		c.sendError("table_not_found", "Table not found after join")
		return
	}

	// Create player list for response
	players := make([]PlayerState, 0, len(table.players))
	for _, player := range table.players {
		players = append(players, PlayerStateFromGame(player, false))
	}

	response, _ := NewMessage("table_joined", TableJoinedData{
		TableID:    data.TableID,
		SeatNumber: table.players[playerName].SeatNumber,
		Players:    players,
	})
	_ = c.SendMessage(response) // Ignore send errors
}

func (c *Connection) handleLeaveTable(data LeaveTableData) {
	c.logger.Info("Leave table request", "tableId", data.TableID, "player", c.GetPlayer())

	if gameService == nil {
		c.sendError("service_unavailable", "Game service not available")
		return
	}

	playerName := c.GetPlayer()
	if playerName == "" {
		c.sendError("not_authenticated", "Must authenticate first")
		return
	}

	err := gameService.LeaveTable(data.TableID, playerName)
	if err != nil {
		c.sendError("leave_failed", err.Error())
		return
	}

	// Clear table association
	c.SetTable("")

	response, _ := NewMessage("table_left", map[string]string{"tableId": data.TableID})
	_ = c.SendMessage(response) // Ignore send errors
}

func (c *Connection) handleListTables() {
	c.logger.Info("List tables request", "player", c.GetPlayer())

	if gameService == nil {
		c.sendError("service_unavailable", "Game service not available")
		return
	}

	tables := gameService.ListTables()
	response, _ := NewMessage("table_list", TableListData{
		Tables: tables,
	})
	_ = c.SendMessage(response) // Ignore send errors
}

func (c *Connection) handlePlayerDecision(data PlayerDecisionData) {
	c.logger.Info("Player decision", "player", c.GetPlayer(), "action", data.Action, "amount", data.Amount)

	if gameService == nil {
		c.sendError("service_unavailable", "Game service not available")
		return
	}

	playerName := c.GetPlayer()
	if playerName == "" {
		c.sendError("not_authenticated", "Must authenticate first")
		return
	}

	err := gameService.HandlePlayerDecision(playerName, data)
	if err != nil {
		c.sendError("decision_failed", err.Error())
		return
	}

	// No response needed - the game engine will publish events
}
