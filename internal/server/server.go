package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	maxMessageSize = 512
)

var (
	ErrSendTimeout = errors.New("send timeout")
)

// Server represents the poker server
type Server struct {
	pool     *BotPool
	upgrader websocket.Upgrader
	botCount atomic.Int64
}

// NewServer creates a new poker server
func NewServer() *Server {
	return &Server{
		pool: NewBotPool(2, 9), // 2-9 players per hand
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				// Allow connections from any origin for demo
				return true
			},
		},
	}
}

// Start starts the server on the given address
func (s *Server) Start(addr string) error {
	// Start bot pool manager
	go s.pool.Run()

	// Set up HTTP routes
	http.HandleFunc("/ws", s.handleWebSocket)
	http.HandleFunc("/health", s.handleHealth)
	http.HandleFunc("/stats", s.handleStats)

	log.Printf("Server starting on %s", addr)
	return http.ListenAndServe(addr, nil)
}

// handleWebSocket handles WebSocket connections from bots
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	// Generate unique bot ID
	botID := uuid.New().String()
	s.botCount.Add(1)

	// Create bot instance
	bot := NewBot(botID, conn, s.pool)

	// Register with pool
	s.pool.Register(bot)

	// Start bot message pumps
	go bot.WritePump()
	go bot.ReadPump()

	log.Printf("Bot connected: %s (total: %d)", botID, s.botCount.Load())
}

// handleHealth returns server health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK\n")
}

// handleStats returns server statistics
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	botCount := s.pool.BotCount()
	fmt.Fprintf(w, "Connected bots: %d\n", botCount)
}