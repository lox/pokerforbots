package server

import (
	"github.com/lox/pokerforbots/v2/internal/auth"
	"github.com/lox/pokerforbots/v2/internal/randutil"
	handhistory "github.com/lox/pokerforbots/v2/internal/server/hand_history"

	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	rand "math/rand/v2"
	"net"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/v2/protocol"
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
	SmallBlind            int
	BigBlind              int
	StartChips            int
	Timeout               time.Duration
	MinActionTime         time.Duration // Minimum time to wait before processing action (prevents timing tells)
	MinPlayers            int
	MaxPlayers            int
	Seed                  int64
	EnableStats           bool // Collect detailed statistics
	MaxStatsHands         int  // Maximum hands to track for stats (default 10000)
	EnableLatencyTracking bool // Collect per-action response latency
	AuthRequired          bool // Fail closed on auth unavailable (default: fail open)

	// Legacy fields (deprecated - will be removed)
	HandLimit              uint64 // Deprecated: Use spawner for hand limits
	InfiniteBankroll       bool   // Deprecated: Use spawner for bankroll management
	StopOnInsufficientBots bool   // Deprecated: Use spawner for bot management

	// Hand history recording
	EnableHandHistory           bool
	HandHistoryDir              string
	HandHistoryFlushSecs        int
	HandHistoryFlushHands       int
	HandHistoryIncludeHoleCards bool
}

// serverConfig holds the configuration for building a server
type serverConfig struct {
	config        Config
	pool          *BotPool      // Custom pool for testing
	botIDGen      func() string // Custom ID generator for testing
	authValidator AuthValidator // Auth validator (defaults to NoopValidator)
}

// AuthValidator validates authentication tokens.
// This interface is defined here to avoid circular imports with internal/auth.
type AuthValidator interface {
	Validate(ctx context.Context, token string) (identity any, err error)
}

// ServerOption configures how we create a server
type ServerOption func(*serverConfig)

// WithConfig sets the full server configuration.
// This replaces the entire config, so it should be used before
// other options that modify specific config fields (like WithHandLimit).
func WithConfig(config Config) ServerOption {
	return func(c *serverConfig) {
		c.config = config
	}
}

// WithBotPool sets a custom bot pool (for testing)
func WithBotPool(pool *BotPool) ServerOption {
	return func(c *serverConfig) {
		c.pool = pool
	}
}

// WithBotIDGen sets a custom bot ID generator (for testing)
func WithBotIDGen(gen func() string) ServerOption {
	return func(c *serverConfig) {
		c.botIDGen = gen
	}
}

// WithHandLimit sets the hand limit.
// Note: If using WithBotPool, the pool's hand limit won't be updated.
// Use WithConfig for full control when providing a custom pool.
func WithHandLimit(limit uint64) ServerOption {
	return func(c *serverConfig) {
		c.config.HandLimit = limit
		// If a custom pool was provided, update its limit too
		if c.pool != nil {
			c.pool.handLimit = limit
			c.pool.config.HandLimit = limit
		}
	}
}

// WithAuthValidator sets the authentication validator.
func WithAuthValidator(validator AuthValidator) ServerOption {
	return func(c *serverConfig) {
		c.authValidator = validator
	}
}

// Server represents the poker server
type Server struct {
	pool               *BotPool
	manager            *GameManager
	handHistoryManager *handhistory.Manager
	defaultGameID      string
	upgrader           websocket.Upgrader
	botCount           atomic.Int64
	mux                *http.ServeMux
	logger             zerolog.Logger
	httpServer         *http.Server
	botIDGen           func() string // Function to generate bot IDs
	config             Config
	authValidator      AuthValidator // Auth validator
	// NPC support removed - use spawner for bot orchestration
	routesOnce sync.Once
}

// GameMetrics summarizes runtime performance for a game instance.
type GameMetrics struct {
	HandsCompleted uint64
	HandLimit      uint64
	HandsPerSecond float64
	StartTime      time.Time
	EndTime        time.Time
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
			for i := range 16 {
				uuid[i] = byte(r.IntN(256))
			}
		})

		// Set version (4) and variant bits according to RFC 4122
		uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
		uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant bits

		return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
			uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
	}
}

// NewServer creates a new poker server with provided random source.
//
// Example usage:
//
//	// Basic server with defaults
//	server := NewServer(logger, rng)
//
//	// Server with custom config
//	server := NewServer(logger, rng, WithConfig(myConfig))
//
//	// Server with hand limit
//	server := NewServer(logger, rng, WithHandLimit(1000))
//
//	// Testing - with custom pool and ID generator
//	server := NewServer(logger, rng, WithBotPool(pool), WithBotIDGen(gen))
func NewServer(logger zerolog.Logger, rng *rand.Rand, opts ...ServerOption) *Server {
	// Default configuration
	cfg := serverConfig{
		config: Config{
			SmallBlind:                  5,
			BigBlind:                    10,
			StartChips:                  1000,
			Timeout:                     100 * time.Millisecond,
			MinPlayers:                  2,
			MaxPlayers:                  9,
			HandLimit:                   0,
			Seed:                        0,
			HandHistoryDir:              "hands",
			HandHistoryFlushSecs:        10,
			HandHistoryFlushHands:       100,
			HandHistoryIncludeHoleCards: false,
		},
	}

	// Apply options
	for _, opt := range opts {
		opt(&cfg)
	}

	// Create or use provided pool
	var pool *BotPool
	if cfg.pool != nil {
		pool = cfg.pool
	} else {
		pool = NewBotPool(logger, rng, cfg.config)
	}

	// Create or use provided bot ID generator
	var botIDGen func() string
	switch {
	case cfg.botIDGen != nil:
		botIDGen = cfg.botIDGen
	case cfg.pool != nil:
		// If custom pool provided but no ID gen, use UUID
		botIDGen = func() string { return uuid.New().String() }
	default:
		// Default deterministic ID gen with the pool's RNG
		botIDGen = createDeterministicBotIDGen(rng, pool.WithRNG)
	}

	// Create game manager and register default game
	manager := NewGameManager(logger)
	defaultGameID := "default"
	manager.RegisterGame(defaultGameID, pool, cfg.config)

	// Optional hand history manager
	var hhManager *handhistory.Manager
	if cfg.config.EnableHandHistory {
		hhLogger := logger.With().Str("component", "hand_history_manager").Logger()
		flushSecs := cfg.config.HandHistoryFlushSecs
		if flushSecs <= 0 {
			flushSecs = 10
		}
		flushHands := cfg.config.HandHistoryFlushHands
		if flushHands <= 0 {
			flushHands = 100
		}
		hhManager = handhistory.NewManager(hhLogger, handhistory.ManagerConfig{
			BaseDir:          cfg.config.HandHistoryDir,
			FlushInterval:    time.Duration(flushSecs) * time.Second,
			FlushHands:       flushHands,
			IncludeHoleCards: cfg.config.HandHistoryIncludeHoleCards,
		})
	}

	// Use provided auth validator or default to noop
	authValidator := cfg.authValidator
	if authValidator == nil {
		authValidator = &noopAuthValidator{}
	}

	srv := &Server{
		pool:               pool,
		manager:            manager,
		handHistoryManager: hhManager,
		defaultGameID:      defaultGameID,
		botIDGen:           botIDGen,
		config:             cfg.config,
		authValidator:      authValidator,
		upgrader: websocket.Upgrader{
			// Increased buffer sizes from 1024 to 4096 for better throughput
			// Profiling showed 28.5% of time spent in read/write syscalls
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				// Allow connections from any origin for demo
				return true
			},
		},
		mux:    http.NewServeMux(),
		logger: logger,
	}

	if hhManager != nil {
		srv.attachHandHistoryMonitor(defaultGameID, pool, cfg.config)
	}

	return srv
}

// noopAuthValidator is a no-op validator that allows all connections.
type noopAuthValidator struct{}

func (v *noopAuthValidator) Validate(ctx context.Context, token string) (any, error) {
	return nil, nil
}

func isNoopValidator(v AuthValidator) bool {
	_, ok := v.(*noopAuthValidator)
	return ok
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
			if s.handHistoryManager != nil {
				s.handHistoryManager.Shutdown()
			}
			return err
		}
	}

	if s.handHistoryManager != nil {
		s.handHistoryManager.Shutdown()
	}

	s.logger.Info().Msg("Server shutdown completed")
	return nil
}

// DefaultGameDone returns a channel that will be closed when the default game completes.
// Returns nil if no default game exists.
// Deprecated: Use SetHandMonitor for progress tracking instead
func (s *Server) DefaultGameDone() <-chan struct{} {
	if s.pool != nil {
		return s.pool.Done()
	}
	return nil
}

// DefaultGameMetrics returns aggregate metrics for the default game, when present.
func (s *Server) DefaultGameMetrics() (GameMetrics, bool) {
	if s.pool == nil {
		return GameMetrics{}, false
	}
	metrics := GameMetrics{
		HandsCompleted: s.pool.HandCount(),
		HandLimit:      s.pool.HandLimit(),
		HandsPerSecond: s.pool.HandsPerSecond(),
		StartTime:      s.pool.StartTime(),
		EndTime:        s.pool.EndTime(),
	}
	return metrics, true
}

// SetHandMonitor sets a monitor for the default game's hand progress.
// Returns an error if no default game exists.
func (s *Server) SetHandMonitor(monitor HandMonitor) error {
	if s.pool == nil {
		return fmt.Errorf("no default game pool")
	}
	s.pool.SetHandMonitor(monitor)
	return nil
}

// WaitForCompletion returns a channel that closes when the game completes.
// This is a convenience wrapper that uses HandMonitor internally.
func (s *Server) WaitForCompletion() <-chan struct{} {
	done := make(chan struct{})

	monitor := &completionMonitor{
		done: done,
	}

	if err := s.SetHandMonitor(monitor); err != nil {
		// No default game, close immediately
		close(done)
		return done
	}

	return done
}

// completionMonitor is a simple HandMonitor that signals completion
type completionMonitor struct {
	done chan struct{}
	once sync.Once
}

func (m *completionMonitor) OnHandComplete(HandOutcome)                    {}
func (m *completionMonitor) OnGameStart(handLimit uint64)                  {}
func (m *completionMonitor) OnHandStart(string, []HandPlayer, int, Blinds) {}
func (m *completionMonitor) OnPlayerAction(string, int, string, int, int)  {}
func (m *completionMonitor) OnStreetChange(string, string, []string)       {}
func (m *completionMonitor) OnGameComplete(handsCompleted uint64, reason string) {
	m.once.Do(func() {
		close(m.done)
	})
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

	// Validate authentication
	var authBotID, ownerID string
	authEnabled := s.authValidator != nil && !isNoopValidator(s.authValidator)

	// If auth is enabled and no token provided, reject
	if authEnabled && connectMsg.AuthToken == "" {
		s.logger.Warn().
			Str("bot_name", connectMsg.Name).
			Msg("authentication required but no token provided")
		_ = conn.Close()
		return
	}

	// Reject oversized tokens to prevent DoS (4KB limit)
	const maxTokenSize = 4096
	if len(connectMsg.AuthToken) > maxTokenSize {
		s.logger.Warn().
			Str("bot_name", connectMsg.Name).
			Int("token_size", len(connectMsg.AuthToken)).
			Msg("authentication token exceeds size limit")
		_ = conn.Close()
		return
	}

	if connectMsg.AuthToken != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		identity, err := s.authValidator.Validate(ctx, connectMsg.AuthToken)

		// Handle validation errors using errors.As to check error type
		switch {
		case err == nil && identity != nil:
			// Valid token - extract identity using type assertion
			// The adapter returns a struct with BotID, BotName, OwnerID fields
			// We use reflection.ValueOf to extract them without importing internal/auth
			v := reflect.ValueOf(identity)
			if v.Kind() == reflect.Ptr {
				v = v.Elem()
			}
			if v.Kind() == reflect.Struct {
				if botIDField := v.FieldByName("BotID"); botIDField.IsValid() && botIDField.Kind() == reflect.String {
					authBotID = botIDField.String()
				}
				if ownerIDField := v.FieldByName("OwnerID"); ownerIDField.IsValid() && ownerIDField.Kind() == reflect.String {
					ownerID = ownerIDField.String()
				}
				if authBotID != "" {
					s.logger.Info().
						Str("auth_bot_id", authBotID).
						Str("bot_name", connectMsg.Name).
						Str("owner_id", ownerID).
						Msg("bot authenticated")
				}
			}

		case errors.Is(err, auth.ErrInvalidToken):
			// Definitive rejection
			s.logger.Warn().
				Str("bot_name", connectMsg.Name).
				Msg("authentication failed: invalid token")
			_ = conn.Close()
			return

		case errors.Is(err, auth.ErrUnavailable):
			// Auth service unavailable - fail open or closed based on config
			if s.config.AuthRequired {
				s.logger.Error().
					Str("bot_name", connectMsg.Name).
					Err(err).
					Msg("authentication failed: service unavailable (fail closed)")
				_ = conn.Close()
				return
			}
			s.logger.Warn().
				Str("bot_name", connectMsg.Name).
				Err(err).
				Msg("authentication unavailable: allowing connection (fail open)")

		default:
			// Unexpected error - treat as unavailable
			if err != nil {
				if s.config.AuthRequired {
					s.logger.Error().
						Str("bot_name", connectMsg.Name).
						Err(err).
						Msg("authentication error (fail closed)")
					_ = conn.Close()
					return
				}
				s.logger.Warn().
					Str("bot_name", connectMsg.Name).
					Err(err).
					Msg("authentication error: allowing connection (fail open)")
			}
		}
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

	// Generate deterministic bot ID based on name (or auth token in future)
	// Use a simple hash of the name for a short, consistent ID
	var botID string
	if connectMsg.Name != "" {
		// Hash the name and take first 8 hex chars for a short ID
		h := fnv.New32a()
		h.Write([]byte(connectMsg.Name))
		botID = fmt.Sprintf("%08x", h.Sum32())
	} else {
		// Fallback to generated ID if no name provided
		botID = s.botIDGen()
	}

	// Determine protocol version (default to v1 for backward compatibility)
	protocolVersion := connectMsg.ProtocolVersion
	if protocolVersion == "" {
		protocolVersion = "1" // Default to v1 for backward compatibility with old bots
	}
	// Validate protocol version
	if protocolVersion != "1" && protocolVersion != "2" {
		s.logger.Warn().Str("version", protocolVersion).Msg("Unsupported protocol version, defaulting to v1")
		protocolVersion = "1"
	}

	// Create bot instance tied to the selected game
	bot := NewBot(s.logger, botID, conn, game.Pool)
	bot.SetDisplayName(connectMsg.Name)
	bot.SetGameID(game.ID)
	bot.ProtocolVersion = protocolVersion
	bot.AuthBotID = authBotID
	bot.OwnerID = ownerID

	// Register with game pool
	game.Pool.Register(bot)

	s.botCount.Add(1)

	// Start bot message pumps
	go bot.WritePump()
	go bot.ReadPump()

	s.logger.Debug().
		Str("bot_id", botID).
		Str("game_id", game.ID).
		Str("name", bot.DisplayName()).
		Str("protocol_version", protocolVersion).
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

type adminGameRequest struct {
	ID               string  `json:"id"`
	SmallBlind       int     `json:"small_blind"`
	BigBlind         int     `json:"big_blind"`
	StartChips       int     `json:"start_chips"`
	TimeoutMs        int     `json:"timeout_ms"`
	MinPlayers       int     `json:"min_players"`
	MaxPlayers       int     `json:"max_players"`
	InfiniteBankroll *bool   `json:"infinite_bankroll"`
	Hands            *uint64 `json:"hands,omitempty"`
	Seed             *int64  `json:"seed,omitempty"`
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

	if _, exists := s.manager.GetGame(req.ID); exists {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte("game already exists"))
		return
	}

	config := Config{
		SmallBlind:       req.SmallBlind,
		BigBlind:         req.BigBlind,
		StartChips:       req.StartChips,
		Timeout:          time.Duration(req.TimeoutMs) * time.Millisecond,
		MinPlayers:       req.MinPlayers,
		MaxPlayers:       req.MaxPlayers,
		InfiniteBankroll: false,
		HandLimit:        0,
	}
	config.EnableHandHistory = s.config.EnableHandHistory
	config.HandHistoryDir = s.config.HandHistoryDir
	config.HandHistoryFlushSecs = s.config.HandHistoryFlushSecs
	config.HandHistoryFlushHands = s.config.HandHistoryFlushHands
	config.HandHistoryIncludeHoleCards = s.config.HandHistoryIncludeHoleCards

	if req.InfiniteBankroll != nil {
		config.InfiniteBankroll = *req.InfiniteBankroll
	}

	if req.Hands != nil {
		config.HandLimit = *req.Hands
	}

	seed := time.Now().UnixNano()
	if req.Seed != nil {
		seed = *req.Seed
	}

	config.Seed = seed
	rng := randutil.New(seed)
	pool := NewBotPool(s.logger, rng, config)
	instance := s.manager.RegisterGame(req.ID, pool, config)
	s.attachHandHistoryMonitor(req.ID, pool, config)
	go pool.Run()

	_ = instance // Avoid unused variable warning

	s.logger.Info().
		Str("game_id", req.ID).
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
	sub := ""
	if len(parts) > 1 {
		sub = parts[1]
	}

	switch r.Method {
	case http.MethodDelete:
		s.serveAdminGameDelete(w, id, len(parts))
	case http.MethodGet:
		s.serveAdminGameGet(w, id, sub)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) serveAdminGameDelete(w http.ResponseWriter, id string, partsLen int) {
	if partsLen != 1 {
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

	instance.Pool.Stop()
	if s.handHistoryManager != nil {
		s.handHistoryManager.RemoveMonitor(id)
	}
	s.logger.Info().Str("game_id", id).Msg("Admin deleted game")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) serveAdminGameGet(w http.ResponseWriter, id, sub string) {
	if sub == "stats" {
		s.serveAdminGameStatsJSON(w, id)
		return
	}
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte("endpoint not found"))
}

func (s *Server) serveAdminGameStatsJSON(w http.ResponseWriter, id string) {
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
}

func (s *Server) attachHandHistoryMonitor(gameID string, pool *BotPool, config Config) {
	if s.handHistoryManager == nil || !config.EnableHandHistory {
		return
	}
	monitor, err := s.handHistoryManager.CreateMonitor(gameID)
	if err != nil {
		s.logger.Error().Err(err).Str("game_id", gameID).Msg("failed to create hand history monitor")
		return
	}
	pool.SetHandHistoryMonitor(newHandHistoryAdapter(monitor))
}

// WaitForHealthy polls the /health endpoint until it returns 200 OK or the context is cancelled.
// baseURL should be the server's base URL (e.g., "http://localhost:8080").
func WaitForHealthy(ctx context.Context, baseURL string) error {
	healthURL := baseURL + "/health"
	client := &http.Client{Timeout: 1 * time.Second}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			resp, err := client.Get(healthURL)
			if err == nil && resp.StatusCode == http.StatusOK {
				resp.Body.Close()
				return nil
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
	}
}
