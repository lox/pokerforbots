package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/gorilla/websocket"
)

// Server represents the WebSocket server
type Server struct {
	addr        string
	upgrader    websocket.Upgrader
	connections map[*Connection]bool
	register    chan *Connection
	unregister  chan *Connection
	logger      *log.Logger
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	gameService *GameService
}

// NewServer creates a new WebSocket server
func NewServer(addr string, logger *log.Logger) *Server {
	ctx, cancel := context.WithCancel(context.Background())

	return &Server{
		addr: addr,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// For development, allow all origins
				// In production, implement proper origin checking
				return true
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		connections: make(map[*Connection]bool),
		register:    make(chan *Connection),
		unregister:  make(chan *Connection),
		logger:      logger.WithPrefix("server"),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start starts the WebSocket server
func (s *Server) Start() error {
	go s.run()

	// Create a dedicated mux for this server instance
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/health", s.handleHealth)

	s.logger.Info("Starting WebSocket server", "addr", s.addr)
	return http.ListenAndServe(s.addr, mux)
}

// Stop stops the WebSocket server
func (s *Server) Stop() error {
	s.cancel()

	// Close all connections
	s.mu.Lock()
	for conn := range s.connections {
		_ = conn.Close() // Ignore close errors during shutdown
	}
	s.mu.Unlock()

	return nil
}

// run handles connection lifecycle
func (s *Server) run() {
	for {
		select {
		case conn := <-s.register:
			s.mu.Lock()
			s.connections[conn] = true
			s.mu.Unlock()
			s.logger.Info("Client connected", "total", len(s.connections))

		case conn := <-s.unregister:
			s.mu.Lock()
			if _, ok := s.connections[conn]; ok {
				delete(s.connections, conn)

				// Clean up player from any tables they were in
				playerID := conn.GetPlayer()
				tableID := conn.GetTable()
				if playerID != "" && tableID != "" && s.gameService != nil {
					s.logger.Info("Cleaning up disconnected player", "player", playerID, "table", tableID)
					_ = s.gameService.LeaveTable(tableID, playerID) // Ignore errors during cleanup
				}

				_ = conn.Close() // Ignore close errors during unregistration
			}
			s.mu.Unlock()
			s.logger.Info("Client disconnected", "total", len(s.connections))

		case <-s.ctx.Done():
			return
		}
	}
}

// handleWebSocket handles WebSocket upgrade requests
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("Failed to upgrade connection", "error", err)
		return
	}

	client := NewConnection(conn, s.logger, s.gameService)
	s.register <- client
	client.Start()

	// Connection cleanup is handled by the connection itself
	go func() {
		<-client.ctx.Done()
		s.unregister <- client
	}()
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(w, "OK") // Ignore write errors for health check
}

// BroadcastToTable sends a message to all connections at a specific table
func (s *Server) BroadcastToTable(tableID string, msg *Message) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for conn := range s.connections {
		if conn.GetTable() == tableID {
			if err := conn.SendMessage(msg); err != nil {
				s.logger.Error("Failed to send message to client", "error", err, "player", conn.GetPlayer())
			} else {
				count++
			}
		}
	}

	s.logger.Debug("Broadcasted message to table", "tableId", tableID, "type", msg.Type, "recipients", count)
}

// SendToPlayer sends a message to a specific player
func (s *Server) SendToPlayer(playerID string, msg *Message) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for conn := range s.connections {
		if conn.GetPlayer() == playerID {
			return conn.SendMessage(msg)
		}
	}

	return fmt.Errorf("player not found: %s", playerID)
}

// GetConnectedPlayers returns a list of connected player IDs
func (s *Server) GetConnectedPlayers() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var players []string
	for conn := range s.connections {
		if playerID := conn.GetPlayer(); playerID != "" {
			players = append(players, playerID)
		}
	}

	return players
}

// GetTablePlayers returns a list of player IDs connected to a specific table
func (s *Server) GetTablePlayers(tableID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var players []string
	for conn := range s.connections {
		if conn.GetTable() == tableID && conn.GetPlayer() != "" {
			players = append(players, conn.GetPlayer())
		}
	}

	return players
}

// SetGameService sets the game service for the server
func (s *Server) SetGameService(gameService *GameService) {
	s.gameService = gameService
}
