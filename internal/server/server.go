package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	statistics "github.com/lox/pokerforbots/internal/server/statistics"
	"github.com/lox/pokerforbots/protocol"
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
	SmallBlind       int
	BigBlind         int
	StartChips       int
	Timeout          time.Duration
	MinPlayers       int
	MaxPlayers       int
	RequirePlayer    bool
	HandLimit        uint64
	Seed             int64
	InfiniteBankroll bool // When true, bots never run out of chips
	EnableStats      bool // Collect detailed statistics
	MaxStatsHands    int  // Maximum hands to track for stats (default 10000)
}

// serverConfig holds the configuration for building a server
type serverConfig struct {
	config   Config
	pool     *BotPool      // Custom pool for testing
	botIDGen func() string // Custom ID generator for testing
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
			SmallBlind:    5,
			BigBlind:      10,
			StartChips:    1000,
			Timeout:       100 * time.Millisecond,
			MinPlayers:    2,
			MaxPlayers:    9,
			RequirePlayer: true,
			HandLimit:     0,
			Seed:          0,
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

	return &Server{
		pool:          pool,
		manager:       manager,
		defaultGameID: defaultGameID,
		botIDGen:      botIDGen,
		config:        cfg.config,
		bootstrapNPCs: make(map[string][]NPCSpec),
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

// DefaultGameDone returns a channel that will be closed when the default game completes.
// Returns nil if no default game exists.
func (s *Server) DefaultGameDone() <-chan struct{} {
	if s.pool != nil {
		return s.pool.Done()
	}
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

	s.logger.Debug().
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
	ID               string    `json:"id"`
	SmallBlind       int       `json:"small_blind"`
	BigBlind         int       `json:"big_blind"`
	StartChips       int       `json:"start_chips"`
	TimeoutMs        int       `json:"timeout_ms"`
	MinPlayers       int       `json:"min_players"`
	MaxPlayers       int       `json:"max_players"`
	RequirePlayer    *bool     `json:"require_player"`
	InfiniteBankroll *bool     `json:"infinite_bankroll"`
	NPCs             []NPCSpec `json:"npcs"`
	Hands            *uint64   `json:"hands,omitempty"`
	Seed             *int64    `json:"seed,omitempty"`
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
		SmallBlind:       req.SmallBlind,
		BigBlind:         req.BigBlind,
		StartChips:       req.StartChips,
		Timeout:          time.Duration(req.TimeoutMs) * time.Millisecond,
		MinPlayers:       req.MinPlayers,
		MaxPlayers:       req.MaxPlayers,
		RequirePlayer:    true,
		InfiniteBankroll: false,
		HandLimit:        0,
	}
	if req.RequirePlayer != nil {
		config.RequirePlayer = *req.RequirePlayer
	}
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
	rng := rand.New(rand.NewSource(seed))
	pool := NewBotPool(s.logger, rng, config)
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

	instance.StopNPCs()
	instance.Pool.Stop()
	s.logger.Info().Str("game_id", id).Msg("Admin deleted game")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) serveAdminGameGet(w http.ResponseWriter, id, sub string) {
	if sub == "stats.md" {
		s.serveAdminGameStatsMarkdown(w, id)
		return
	}
	if sub == "stats.txt" {
		s.serveAdminGameStatsText(w, id)
		return
	}
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

func (s *Server) serveAdminGameStatsText(w http.ResponseWriter, id string) {
	instance, ok := s.manager.GetGame(id)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("game not found"))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	players := instance.Pool.PlayerStats()
	for _, ps := range players {
		_, _ = w.Write([]byte("=== PLAYER: " + ps.DisplayName + " (" + ps.Role + ") ===\n"))
		if instance.Pool.statsCollector != nil && instance.Pool.statsCollector.IsEnabled() {
			if d, ok := instance.Pool.statsCollector.(*DetailedStatsCollector); ok {
				d.mu.RLock()
				stat := d.stats[ps.BotID]
				d.mu.RUnlock()
				if stat != nil {
					_, _ = w.Write([]byte(stat.Summary()))
					_, _ = w.Write([]byte("\n"))
					continue
				}
			}
		}
		_, _ = w.Write([]byte("Hands: "))
		_, _ = fmt.Fprintf(w, "%d\n", ps.Hands)
		_, _ = w.Write([]byte("Net chips: "))
		_, _ = fmt.Fprintf(w, "%d\n\n", ps.NetChips)
	}
}

func (s *Server) serveAdminGameStatsMarkdown(w http.ResponseWriter, id string) {
	instance, ok := s.manager.GetGame(id)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("game not found"))
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = fmt.Fprintf(w, "# Game %s Statistics\n\n", instance.ID)
	// Game overview
	gs, _ := s.manager.GameStats(id)
	_, _ = fmt.Fprintf(w, "## Game overview\n\n")
	_, _ = fmt.Fprintf(w, "- Blinds: %d/%d\n", gs.SmallBlind, gs.BigBlind)
	_, _ = fmt.Fprintf(w, "- Start chips: %d\n", gs.StartChips)
	_, _ = fmt.Fprintf(w, "- Timeout: %d ms\n", gs.TimeoutMs)
	_, _ = fmt.Fprintf(w, "- Players: %d-%d\n", gs.MinPlayers, gs.MaxPlayers)
	if gs.HandLimit > 0 {
		_, _ = fmt.Fprintf(w, "- Hand limit: %d (remaining: %d)\n", gs.HandLimit, gs.HandsRemaining)
	} else {
		_, _ = fmt.Fprintf(w, "- Hand limit: unlimited\n")
	}
	_, _ = fmt.Fprintf(w, "- Hands completed: %d\n", gs.HandsCompleted)
	_, _ = fmt.Fprintf(w, "- Hands per second: %.2f\n", gs.HandsPerSecond)
	// Start/end/duration
	if !gs.StartTime.IsZero() {
		_, _ = fmt.Fprintf(w, "- Start time: %s\n", gs.StartTime.Format(time.RFC3339))
	}
	if !gs.EndTime.IsZero() {
		_, _ = fmt.Fprintf(w, "- End time: %s\n", gs.EndTime.Format(time.RFC3339))
	}
	if gs.DurationSeconds > 0 {
		_, _ = fmt.Fprintf(w, "- Duration: %.2f s\n", gs.DurationSeconds)
	}
	_, _ = fmt.Fprintf(w, "- Timeouts: %d\n", gs.Timeouts)
	_, _ = fmt.Fprintf(w, "- Seed: %d\n", gs.Seed)
	_, _ = fmt.Fprintf(w, "- Active bots: %d\n\n", instance.Pool.BotCount())
	// Leaderboard
	_, _ = fmt.Fprintf(w, "## Leaderboard\n\n")
	players := instance.Pool.PlayerStats()
	sort.Slice(players, func(i, j int) bool { return players[i].NetChips > players[j].NetChips })
	_, _ = fmt.Fprintf(w, "| Player | Role | Hands | Net Chips | BB/100 |\n|---|---:|---:|---:|---:|\n")
	for _, p := range players {
		bb100 := 0.0
		if gs.BigBlind > 0 {
			bb100 = (p.AvgPerHand / float64(gs.BigBlind)) * 100
		}
		_, _ = fmt.Fprintf(w, "| %s | %s | %d | %d | %.2f |\n", p.DisplayName, p.Role, p.Hands, p.NetChips, bb100)
	}
	_, _ = fmt.Fprintf(w, "\n")
	// Aggregate analyses (when stats enabled) omitted intentionally
	for _, ps := range players {
		_, _ = fmt.Fprintf(w, "## Player: %s (%s)\n\n", ps.DisplayName, ps.Role)
		if instance.Pool.statsCollector != nil && instance.Pool.statsCollector.IsEnabled() {
			if d, ok := instance.Pool.statsCollector.(*DetailedStatsCollector); ok {
				d.mu.RLock()
				stat := d.stats[ps.BotID]
				d.mu.RUnlock()
				if stat != nil {
					hands, sumBB, winningHands, losingHands, showdownWins, nonShowdownWins, showdownLosses, showdownBB, nonShowdownBB, _, _, _, _, _ := stat.GetStats()
					mean := stat.Mean()
					bb100 := stat.BB100()
					median := stat.Median()
					stdDev := stat.StdDev()
					low, high := stat.ConfidenceInterval95()
					_, _ = fmt.Fprintf(w, "### Results summary\n\n")
					_, _ = fmt.Fprintf(w, "- Hands played: %d\n", hands)
					_, _ = fmt.Fprintf(w, "- Net result: %.2f BB (%.2f BB/100)\n", sumBB, bb100)
					_, _ = fmt.Fprintf(w, "- Mean: %.4f BB/hand | Median: %.4f BB/hand\n", mean, median)
					_, _ = fmt.Fprintf(w, "- Std Dev: %.4f BB | 95%% CI: [%.4f, %.4f]\n\n", stdDev, low, high)

					winRate := 0.0
					if hands > 0 {
						winRate = float64(winningHands) / float64(hands) * 100
					}
					_, _ = fmt.Fprintf(w, "### Win/Loss breakdown\n\n")
					_, _ = fmt.Fprintf(w, "- Winning hands: %d (%.1f%%)\n", winningHands, winRate)
					_, _ = fmt.Fprintf(w, "  - Showdown wins: %d | Non-showdown wins: %d\n", showdownWins, nonShowdownWins)
					_, _ = fmt.Fprintf(w, "- Losing hands: %d (%.1f%%)\n", losingHands, 100-winRate)
					_, _ = fmt.Fprintf(w, "  - Showdown losses: %d\n\n", showdownLosses)

					totalShowdowns := showdownWins + showdownLosses
					showdownRate := 0.0
					if totalShowdowns > 0 {
						showdownRate = float64(showdownWins) / float64(totalShowdowns) * 100
					}
					_, _ = fmt.Fprintf(w, "### Showdown analysis\n\n")
					_, _ = fmt.Fprintf(w, "- Went to showdown: %d hands\n", totalShowdowns)
					_, _ = fmt.Fprintf(w, "- Showdown win rate: %.1f%%%%\n", showdownRate)
					_, _ = fmt.Fprintf(w, "- Showdown BB: %.2f | Non-showdown BB: %.2f\n\n", showdownBB, nonShowdownBB)

					_, _ = fmt.Fprintf(w, "### Position analysis\n\n")
					bd := stat.ButtonDistanceResults()
					for dist := 0; dist < 6; dist++ {
						pbd := bd[dist]
						if pbd.Hands > 0 {
							posMean := stat.ButtonDistanceMean(dist)
							posName := statistics.GetPositionName(dist)
							_, _ = fmt.Fprintf(w, "- %s: %d hands, %.3f BB/hand (%.1f BB/100)\n", posName, pbd.Hands, posMean, posMean*100)
						}
					}
					_, _ = fmt.Fprintf(w, "\n")

					_, _ = fmt.Fprintf(w, "### Street analysis\n\n")
					order := []struct{ key, label string }{
						{"preflop", "Preflop"}, {"flop", "Flop"}, {"turn", "Turn"}, {"river", "River"},
					}
					ssMap := stat.StreetStats()
					for _, o := range order {
						if ss, ok := ssMap[o.key]; ok && ss.HandsReached > 0 {
							avg := ss.NetBB / float64(ss.HandsReached)
							_, _ = fmt.Fprintf(w, "- %s: %d hands ended, %.3f BB/hand\n", o.label, ss.HandsReached, avg)
						}
					}
					_, _ = fmt.Fprintf(w, "\n")
					continue
				}
			}
		}
		_, _ = fmt.Fprintf(w, "### Results summary\n\n")
		_, _ = fmt.Fprintf(w, "- Hands: %d\n", ps.Hands)
		_, _ = fmt.Fprintf(w, "- Net chips: %d\n\n", ps.NetChips)
		// No per-player street analysis available without detailed stats
		_, _ = fmt.Fprintf(w, "### Street analysis\n\n")
		_, _ = fmt.Fprintf(w, "- No street-level data. Enable --enable-stats with --stats-depth=detailed or full.\n\n")
	}
}
