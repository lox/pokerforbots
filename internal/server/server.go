package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
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

// Config holds server configuration
type Config struct {
	SmallBlind    int
	BigBlind      int
	StartChips    int
	Timeout       time.Duration
	MinPlayers    int
	MaxPlayers    int
	RequirePlayer bool
	HandLimit     uint64
	Seed          int64
	MatchInterval time.Duration
}

// Server represents the poker server
type Server struct {
	pool          *BotPool
	manager       *GameManager
	defaultGameID string
	upgrader      websocket.Upgrader
	botCount      atomic.Int64
	mux           *http.ServeMux
	logger        zerolog.Logger
	httpServer    *http.Server
	botIDGen      func() string // Function to generate bot IDs
	config        Config
	bootstrapNPCs map[string][]NPCSpec
	routesOnce    sync.Once
}

// createDeterministicBotIDGen creates a deterministic bot ID generator using the provided RNG accessor.
// If no accessor is supplied, a local mutex is used to guard the RNG.
func createDeterministicBotIDGen(rng *rand.Rand, withRNG func(func(*rand.Rand))) func() string {
	if withRNG == nil {
		var fallback sync.Mutex
		withRNG = func(fn func(*rand.Rand)) {
			fallback.Lock()
			defer fallback.Unlock()
			fn(rng)
		}
	}

	return func() string {
		var uuid [16]byte
		withRNG(func(r *rand.Rand) {
			for i := 0; i < 16; i++ {
				uuid[i] = byte(r.Intn(256))
			}
		})

		// Set version (4) and variant bits according to RFC 4122
		uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
		uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant bits

		return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
			uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
	}
}

// NewServer creates a new poker server with provided random source and default config
func NewServer(logger zerolog.Logger, rng *rand.Rand) *Server {
	config := Config{
		SmallBlind:    5,
		BigBlind:      10,
		StartChips:    1000,
		Timeout:       100 * time.Millisecond,
		MinPlayers:    2,
		MaxPlayers:    9,
		RequirePlayer: true,
		HandLimit:     0,
		Seed:          0,
		MatchInterval: 100 * time.Millisecond,
	}
	return NewServerWithConfig(logger, rng, config)
}

// NewServerWithConfig creates a new poker server with provided random source and config
func NewServerWithConfig(logger zerolog.Logger, rng *rand.Rand, config Config) *Server {
	pool := NewBotPoolWithLimitAndConfig(logger, config.MinPlayers, config.MaxPlayers, rng, config.HandLimit, config)
	return NewServerWithBotIDGenAndConfig(logger, pool, createDeterministicBotIDGen(rng, pool.WithRNG), config)
}

// NewServerWithHandLimit creates a new poker server with a hand limit
func NewServerWithHandLimit(logger zerolog.Logger, rng *rand.Rand, handLimit uint64) *Server {
	pool := NewBotPoolWithLimit(logger, 2, 9, rng, handLimit)
	return NewServerWithBotIDGen(logger, pool, createDeterministicBotIDGen(rng, pool.WithRNG))
}

// NewServerWithPool creates a new poker server with a custom bot pool (for testing)
func NewServerWithPool(logger zerolog.Logger, pool *BotPool) *Server {
	return NewServerWithBotIDGen(logger, pool, func() string { return uuid.New().String() })
}

// NewServerWithBotIDGen creates a new poker server with custom bot pool and ID generator (for testing)
func NewServerWithBotIDGen(logger zerolog.Logger, pool *BotPool, botIDGen func() string) *Server {
	config := Config{
		SmallBlind:    5,
		BigBlind:      10,
		StartChips:    1000,
		Timeout:       100 * time.Millisecond,
		MinPlayers:    2,
		MaxPlayers:    9,
		RequirePlayer: true,
		HandLimit:     0,
		Seed:          0,
		MatchInterval: 100 * time.Millisecond,
	}
	return NewServerWithBotIDGenAndConfig(logger, pool, botIDGen, config)
}

// NewServerWithBotIDGenAndConfig creates a new poker server with custom bot pool, ID generator and config
func NewServerWithBotIDGenAndConfig(logger zerolog.Logger, pool *BotPool, botIDGen func() string, config Config) *Server {
	manager := NewGameManager(logger)
	defaultGameID := "default"
	manager.RegisterGame(defaultGameID, pool, config)

	return &Server{
		pool:          pool,
		manager:       manager,
		defaultGameID: defaultGameID,
		botIDGen:      botIDGen,
		config:        config,
		bootstrapNPCs: make(map[string][]NPCSpec),
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
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.Serve(listener)
}

// Serve starts the server using an existing listener.
func (s *Server) Serve(listener net.Listener) error {
	// Start bot pools for all registered games
	s.manager.StartAll()

	// Bootstrap NPCs after pools are running
	for gameID, specs := range s.bootstrapNPCs {
		if len(specs) == 0 {
			continue
		}
		if instance, ok := s.manager.GetGame(gameID); ok {
			instance.AddNPCs(s.logger, specs)
		} else {
			s.logger.Warn().Str("game_id", gameID).Msg("Bootstrap NPC target game not found")
		}
	}
	s.bootstrapNPCs = nil

	s.ensureRoutes()

	s.httpServer = &http.Server{
		Handler: s.mux,
	}

	s.logger.Info().Str("addr", listener.Addr().String()).Msg("Server starting")

	return s.httpServer.Serve(listener)
}

func (s *Server) ensureRoutes() {
	s.routesOnce.Do(func() {
		s.mux.HandleFunc("/ws", s.handleWebSocket)
		s.mux.HandleFunc("/health", s.handleHealth)
		s.mux.HandleFunc("/stats", s.handleStats)
		s.mux.HandleFunc("/games", s.handleGames)
		s.mux.HandleFunc("/admin/games", s.handleAdminGames)
		s.mux.HandleFunc("/admin/games/", s.handleAdminGame)
	})
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info().Msg("Starting graceful server shutdown")

	// Stop all game pools first
	s.manager.StopAll()

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

	msgType, payload, err := conn.ReadMessage()
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to read connect message")
		_ = conn.Close()
		return
	}

	if msgType != websocket.BinaryMessage {
		s.logger.Error().Msg("Connect message must be binary")
		_ = conn.Close()
		return
	}

	var connectMsg protocol.Connect
	if err := protocol.Unmarshal(payload, &connectMsg); err != nil || connectMsg.Type != protocol.TypeConnect {
		if err != nil {
			s.logger.Error().Err(err).Msg("Invalid connect payload")
		} else {
			s.logger.Error().Msg("First message was not a connect request")
		}
		_ = conn.Close()
		return
	}

	requestedGame := connectMsg.Game
	if requestedGame == "" {
		requestedGame = s.defaultGameID
	}

	game, ok := s.manager.GetGame(requestedGame)
	if !ok {
		s.logger.Warn().Str("requested_game", requestedGame).Msg("Unknown game requested, falling back to default")
		var fallback bool
		game, fallback = s.manager.GetGame(s.defaultGameID)
		if !fallback {
			s.logger.Error().Msg("No default game available; closing connection")
			_ = conn.Close()
			return
		}
	}

	// Generate unique bot ID
	botID := s.botIDGen()

	// Create bot instance tied to the selected game
	bot := NewBot(s.logger, botID, conn, game.Pool)
	bot.SetDisplayName(connectMsg.Name)
	bot.SetGameID(game.ID)
	bot.SetRole(normalizeRole(connectMsg.Role))

	// Register with game pool
	game.Pool.Register(bot)

	s.botCount.Add(1)

	// Start bot message pumps
	go bot.WritePump()
	go bot.ReadPump()

	s.logger.Info().
		Str("bot_id", botID).
		Str("game_id", game.ID).
		Str("name", bot.DisplayName()).
		Int64("total_bots", s.botCount.Load()).
		Msg("Bot connected")
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
	timeoutCount := s.pool.TimeoutCount()
	handsPerSecond := s.pool.HandsPerSecond()

	fmt.Fprintf(w, "Connected bots: %d\n", botCount)
	fmt.Fprintf(w, "Hands completed: %d\n", handCount)
	fmt.Fprintf(w, "Hands per second: %.2f\n", handsPerSecond)
	fmt.Fprintf(w, "Timeouts: %d\n", timeoutCount)

	if handLimit > 0 {
		fmt.Fprintf(w, "Hand limit: %d\n", handLimit)
		fmt.Fprintf(w, "Hands remaining: %d\n", handsRemaining)
	} else {
		fmt.Fprintf(w, "Hand limit: unlimited\n")
	}
}

// handleGames returns the list of configured games as JSON.
func (s *Server) handleGames(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	summaries := s.manager.ListGames()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(summaries); err != nil {
		s.logger.Error().Err(err).Msg("failed to encode games response")
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// AddBootstrapNPCs schedules NPC bots to be attached to the given game when Start is invoked.
func (s *Server) AddBootstrapNPCs(gameID string, specs []NPCSpec) {
	if len(specs) == 0 {
		return
	}
	s.bootstrapNPCs[gameID] = append(s.bootstrapNPCs[gameID], specs...)
}

type adminGameRequest struct {
	ID            string    `json:"id"`
	SmallBlind    int       `json:"small_blind"`
	BigBlind      int       `json:"big_blind"`
	StartChips    int       `json:"start_chips"`
	TimeoutMs     int       `json:"timeout_ms"`
	MinPlayers    int       `json:"min_players"`
	MaxPlayers    int       `json:"max_players"`
	RequirePlayer *bool     `json:"require_player"`
	NPCs          []NPCSpec `json:"npcs"`
	Hands         *uint64   `json:"hands,omitempty"`
	Seed          *int64    `json:"seed,omitempty"`
}

func (s *Server) handleAdminGames(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	// TODO: add admin authentication (shared secret or mTLS) once operational requirements are finalized.

	var req adminGameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid JSON payload"))
		return
	}

	if req.ID == "" || req.SmallBlind <= 0 || req.BigBlind <= 0 || req.StartChips <= 0 || req.TimeoutMs <= 0 || req.MinPlayers <= 0 || req.MaxPlayers < req.MinPlayers {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid game parameters"))
		return
	}
	for _, spec := range req.NPCs {
		if spec.Count < 0 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("npc count must be non-negative"))
			return
		}
	}

	if _, exists := s.manager.GetGame(req.ID); exists {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte("game already exists"))
		return
	}

	config := Config{
		SmallBlind:    req.SmallBlind,
		BigBlind:      req.BigBlind,
		StartChips:    req.StartChips,
		Timeout:       time.Duration(req.TimeoutMs) * time.Millisecond,
		MinPlayers:    req.MinPlayers,
		MaxPlayers:    req.MaxPlayers,
		RequirePlayer: true,
		HandLimit:     0,
		MatchInterval: 100 * time.Millisecond,
	}
	if req.RequirePlayer != nil {
		config.RequirePlayer = *req.RequirePlayer
	}

	if req.Hands != nil {
		config.HandLimit = *req.Hands
	}

	seed := time.Now().UnixNano()
	if req.Seed != nil {
		seed = *req.Seed
	}

	config.Seed = seed
	rng := rand.New(rand.NewSource(seed))
	pool := NewBotPoolWithLimitAndConfig(s.logger, config.MinPlayers, config.MaxPlayers, rng, config.HandLimit, config)
	instance := s.manager.RegisterGame(req.ID, pool, config)
	go pool.Run()

	if len(req.NPCs) > 0 {
		instance.AddNPCs(s.logger, req.NPCs)
	}

	s.logger.Info().
		Str("game_id", req.ID).
		Int("npc_groups", len(req.NPCs)).
		Msg("Admin created game")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	summary := s.manager.ListGames()
	if err := json.NewEncoder(w).Encode(summary); err != nil {
		s.logger.Error().Err(err).Msg("failed to encode admin create response")
	}
}

func (s *Server) handleAdminGame(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/games/")
	if path == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("missing game id"))
		return
	}

	parts := strings.Split(path, "/")
	id := parts[0]

	switch r.Method {
	case http.MethodDelete:
		if len(parts) != 1 {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("game not found"))
			return
		}

		instance, ok := s.manager.DeleteGame(id)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("game not found"))
			return
		}

		instance.StopNPCs()
		instance.Pool.Stop()
		s.logger.Info().Str("game_id", id).Msg("Admin deleted game")
		w.WriteHeader(http.StatusNoContent)
	case http.MethodGet:
		if len(parts) != 2 || parts[1] != "stats" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("endpoint not found"))
			return
		}

		stats, ok := s.manager.GameStats(id)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("game not found"))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(stats); err != nil {
			s.logger.Error().Err(err).Msg("failed to encode game stats response")
			w.WriteHeader(http.StatusInternalServerError)
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
