package server

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period
	pingPeriod = (pongWait * 9) / 10
)

var (
	ErrSendTimeout = errors.New("send timeout")
	ErrBotClosed   = errors.New("bot connection closed")
)

// Server represents the poker server
type Server struct {
	pool       *BotPool
	upgrader   websocket.Upgrader
	botCount   atomic.Int64
	mux        *http.ServeMux
	logger     zerolog.Logger
	httpServer *http.Server
	botIDGen   func() string // Function to generate bot IDs
}

// createDeterministicBotIDGen creates a deterministic bot ID generator using the provided RNG
func createDeterministicBotIDGen(rng *rand.Rand) func() string {
	var mu sync.Mutex
	return func() string {
		mu.Lock()
		defer mu.Unlock()

		// Generate deterministic UUID using the provided RNG
		var uuid [16]byte
		for i := 0; i < 16; i++ {
			uuid[i] = byte(rng.Intn(256))
		}

		// Set version (4) and variant bits according to RFC 4122
		uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
		uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant bits

		return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
			uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
	}
}

// NewServer creates a new poker server with provided random source
func NewServer(logger zerolog.Logger, rng *rand.Rand) *Server {
	pool := NewBotPool(logger, 2, 9, rng)
	return NewServerWithBotIDGen(logger, pool, createDeterministicBotIDGen(rng))
}

// NewServerWithHandLimit creates a new poker server with a hand limit
func NewServerWithHandLimit(logger zerolog.Logger, rng *rand.Rand, handLimit uint64) *Server {
	pool := NewBotPoolWithLimit(logger, 2, 9, rng, handLimit)
	return NewServerWithBotIDGen(logger, pool, createDeterministicBotIDGen(rng))
}

// NewServerWithPool creates a new poker server with a custom bot pool (for testing)
func NewServerWithPool(logger zerolog.Logger, pool *BotPool) *Server {
	return NewServerWithBotIDGen(logger, pool, func() string { return uuid.New().String() })
}

// NewServerWithBotIDGen creates a new poker server with custom bot pool and ID generator (for testing)
func NewServerWithBotIDGen(logger zerolog.Logger, pool *BotPool, botIDGen func() string) *Server {
	return &Server{
		pool:     pool,
		botIDGen: botIDGen,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				// Allow connections from any origin for demo
				return true
			},
		},
		mux:    http.NewServeMux(),
		logger: logger,
	}
}

// Start starts the server on the given address
func (s *Server) Start(addr string) error {
	// Start bot pool manager
	go s.pool.Run()

	// Set up HTTP routes
	s.mux.HandleFunc("/ws", s.handleWebSocket)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/stats", s.handleStats)

	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.mux,
	}

	s.logger.Info().Str("addr", addr).Msg("Server starting")
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info().Msg("Starting graceful server shutdown")

	// Stop the bot pool first
	s.pool.Stop()

	// Shutdown the HTTP server
	if s.httpServer != nil {
		if err := s.httpServer.Shutdown(ctx); err != nil {
			s.logger.Error().Err(err).Msg("Error during HTTP server shutdown")
			return err
		}
	}

	s.logger.Info().Msg("Server shutdown completed")
	return nil
}

// handleWebSocket handles WebSocket connections from bots
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error().Err(err).Msg("WebSocket upgrade error")
		return
	}

	// Generate unique bot ID
	botID := s.botIDGen()
	s.botCount.Add(1)

	// Create bot instance
	bot := NewBot(s.logger, botID, conn, s.pool)

	// Register with pool
	s.pool.Register(bot)

	// Start bot message pumps
	go bot.WritePump()
	go bot.ReadPump()

	s.logger.Info().Str("bot_id", botID).Int64("total_bots", s.botCount.Load()).Msg("Bot connected")
}

// handleHealth returns server health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK\n")
}

// handleStats returns server statistics
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	botCount := s.pool.BotCount()
	handCount := s.pool.HandCount()
	handLimit := s.pool.HandLimit()
	handsRemaining := s.pool.HandsRemaining()

	fmt.Fprintf(w, "Connected bots: %d\n", botCount)
	fmt.Fprintf(w, "Hands completed: %d\n", handCount)

	if handLimit > 0 {
		fmt.Fprintf(w, "Hand limit: %d\n", handLimit)
		fmt.Fprintf(w, "Hands remaining: %d\n", handsRemaining)
	} else {
		fmt.Fprintf(w, "Hand limit: unlimited\n")
	}
}
