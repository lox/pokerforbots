package main

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
	"github.com/lox/pokerforbots/internal/server"
	"github.com/rs/zerolog"
)

type CLI struct {
	// Server flags
	SpawnServer bool   `kong:"help='Start an embedded poker server',short='s'"`
	Server      string `kong:"default='ws://localhost:8080',help='Server WebSocket URL'"`
	Port        int    `kong:"default='8080',help='Server port when spawning embedded server'"`

	// Bot configuration
	Bots       int `kong:"default='6',help='Total number of bots',short='b'"`
	Calling    int `kong:"default='0',help='Number of calling station bots (0 = auto)',short='c'"`
	Random     int `kong:"default='0',help='Number of random bots (0 = auto)',short='r'"`
	Aggressive int `kong:"default='0',help='Number of aggressive bots (0 = auto)',short='a'"`

	// Game configuration
	Hands   int   `kong:"default='0',help='Number of hands to play (0 = forever)',short='n'"`
	Seed    int64 `kong:"default='0',help='Random seed (0 for current time)'"`
	Verbose bool  `kong:"help='Enable verbose output',short='v'"`
	Quiet   bool  `kong:"help='Quiet mode - only show summary',short='q'"`
}

var (
	// Global variables that were previously flags
	cli CLI

	// Statistics
	startTime time.Time
	rng       *rand.Rand
)

type BotOrchestrator struct {
	server      *server.Server
	serverAddr  string
	bots        []*BotClient
	handLogger  *HandLogger
	targetHands int
	logger      zerolog.Logger
}

type HandLogger struct {
	handID      string
	players     []PlayerInfo
	streets     []string
	actions     []ActionLog
	board       []string
	winners     []protocol.Winner
	mu          sync.Mutex
	handsLogged uint64
}

type PlayerInfo struct {
	Name  string
	Seat  int
	Chips int
}

type ActionLog struct {
	Street string
	Player string
	Seat   int
	Action string
	Amount int
	Pot    int
}

func main() {
	kctx := kong.Parse(&cli,
		kong.Name("spawn-bots"),
		kong.Description("Spawn poker bots to test the server"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
	)

	// Configure zerolog for beautiful console output
	level := zerolog.InfoLevel
	if cli.Verbose {
		level = zerolog.DebugLevel
	}
	if cli.Quiet {
		level = zerolog.WarnLevel
	}

	// Create logger with beautiful formatting
	logger := zerolog.New(zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: "15:04:05",
		NoColor:    false,
	}).Level(level).With().Timestamp().Str("component", "orchestrator").Logger()

	// Validate configuration
	if cli.Calling == 0 && cli.Random == 0 && cli.Aggressive == 0 {
		// Auto distribution based on total bots
		third := cli.Bots / 3
		remainder := cli.Bots % 3
		cli.Calling = third
		cli.Random = third
		cli.Aggressive = third

		// Distribute remainder
		if remainder >= 1 {
			cli.Calling++
		}
		if remainder >= 2 {
			cli.Random++
		}
	} else if cli.Calling+cli.Random+cli.Aggressive > 0 {
		// Custom bot distribution specified
		cli.Bots = cli.Calling + cli.Random + cli.Aggressive
	}

	if cli.Bots < 2 || cli.Bots > 10 {
		kctx.Fatalf("Number of bots must be between 2 and 10 (got %d)", cli.Bots)
	}

	// Set seed
	if cli.Seed == 0 {
		cli.Seed = time.Now().UnixNano()
	}
	rng = rand.New(rand.NewSource(cli.Seed))

	// Log configuration
	logEvent := logger.Info().Int64("seed", cli.Seed).Int("total_bots", cli.Bots).
		Int("calling_bots", cli.Calling).Int("random_bots", cli.Random).Int("aggressive_bots", cli.Aggressive)

	if cli.Hands > 0 {
		logEvent = logEvent.Int("target_hands", cli.Hands)
	} else {
		logEvent = logEvent.Str("target_hands", "unlimited")
	}

	if cli.SpawnServer {
		logEvent = logEvent.Int("server_port", cli.Port).Bool("spawn_server", true)
	} else {
		logEvent = logEvent.Str("server_url", cli.Server).Bool("spawn_server", false)
	}

	logEvent.Msg("Starting Poker Bot Orchestrator")

	// Print reproduce arguments
	reproduceArgs := fmt.Sprintf("--seed %d --bots %d --calling %d --random %d --aggressive %d",
		cli.Seed, cli.Bots, cli.Calling, cli.Random, cli.Aggressive)

	if cli.Hands > 0 {
		reproduceArgs += fmt.Sprintf(" --hands %d", cli.Hands)
	}

	if cli.SpawnServer {
		reproduceArgs += fmt.Sprintf(" --spawn-server --port %d", cli.Port)
	} else {
		reproduceArgs += fmt.Sprintf(" --server %s", cli.Server)
	}

	logger.Info().Str("reproduce_args", reproduceArgs).Msg("To reproduce this exact run")

	orchestrator := &BotOrchestrator{
		targetHands: cli.Hands,
		handLogger:  &HandLogger{},
		logger:      logger,
	}

	// Set up signal handling early
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server if requested
	if cli.SpawnServer {
		if err := orchestrator.startServer(); err != nil {
			logger.Fatal().Err(err).Msg("Failed to start server")
		}
		defer orchestrator.cleanup()

		// Update server URL to local
		cli.Server = fmt.Sprintf("ws://localhost:%d", cli.Port)
	} else {
		defer orchestrator.cleanup()
	}

	// Verify server is reachable
	if !orchestrator.isServerReady() {
		logger.Fatal().Str("server_url", cli.Server).Msg("Server is not reachable")
		return
	}
	logger.Info().Str("server_url", cli.Server).Msg("Server connection verified")

	// Start time tracking
	startTime = time.Now()

	// Create and connect bots
	if err := orchestrator.createBots(); err != nil {
		logger.Fatal().Err(err).Msg("Failed to create bots")
		return
	}
	logger.Info().Int("bot_count", len(orchestrator.bots)).Msg("All bots connected successfully")

	// Run bots
	done := orchestrator.run()

	// Wait for completion or interrupt
	select {
	case <-done:
		logger.Info().Msg("Target hands reached - shutting down")
	case <-sigChan:
		logger.Info().Msg("Interrupt signal received - cleaning up")
	}

	// Show summary
	orchestrator.printSummary()
}

func (o *BotOrchestrator) startServer() error {
	o.logger.Info().Int("port", cli.Port).Msg("Starting embedded poker server")

	// Create server logger - use debug level if verbose, otherwise info
	serverLevel := zerolog.InfoLevel
	if cli.Verbose {
		serverLevel = zerolog.DebugLevel
	}

	serverLogger := zerolog.New(zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: "15:04:05",
		NoColor:    false,
	}).Level(serverLevel).With().Timestamp().Str("component", "server").Logger()

	// Create RNG instance from seed at main level for dependency injection
	serverRNG := rand.New(rand.NewSource(cli.Seed))

	// Create server instance with optional hand limit and provided RNG
	if o.targetHands > 0 {
		o.server = server.NewServerWithHandLimit(serverLogger, serverRNG, uint64(o.targetHands))
		o.logger.Info().Int("hand_limit", o.targetHands).Int64("seed", cli.Seed).Msg("Server created with hand limit and deterministic RNG")
	} else {
		o.server = server.NewServer(serverLogger, serverRNG)
		o.logger.Info().Int64("seed", cli.Seed).Msg("Server created with unlimited hands and deterministic RNG")
	}
	o.serverAddr = fmt.Sprintf(":%d", cli.Port)

	// Start server in background
	go func() {
		if err := o.server.Start(o.serverAddr); err != nil && err != http.ErrServerClosed {
			o.logger.Error().Err(err).Msg("Server failed to start")
		}
	}()

	// Wait for server to be ready by checking health endpoint
	healthURL := fmt.Sprintf("http://localhost:%d/health", cli.Port)
	for i := 0; i < 20; i++ { // Try for 10 seconds
		resp, err := http.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			o.logger.Debug().Msg("Server startup completed successfully")
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("server did not become ready within 10 seconds")
}

func (o *BotOrchestrator) stopServer() {
	if o.server != nil {
		o.logger.Info().Msg("Stopping embedded server")

		// Create a context with timeout for graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Attempt graceful shutdown
		if err := o.server.Shutdown(ctx); err != nil {
			o.logger.Error().Err(err).Msg("Error during server shutdown")
		} else {
			o.logger.Info().Msg("Server stopped gracefully")
		}

		o.server = nil
	}
}

func (o *BotOrchestrator) cleanup() {
	// Close all bot connections
	for _, bot := range o.bots {
		if bot != nil && bot.conn != nil {
			bot.conn.Close()
		}
	}

	// Stop server if we spawned it
	if o.server != nil {
		o.stopServer()
	}
}

func (o *BotOrchestrator) isServerReady() bool {
	// Try to connect to stats endpoint
	url := strings.Replace(cli.Server, "ws://", "http://", 1)
	url = strings.Replace(url, "wss://", "https://", 1)
	statsURL := url + "/stats"

	for i := 0; i < 10; i++ {
		resp, err := http.Get(statsURL)
		if err == nil {
			resp.Body.Close()
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func (o *BotOrchestrator) createBots() error {
	o.bots = make([]*BotClient, cli.Bots)
	botIdx := 0

	// Create calling station bots
	for i := 0; i < cli.Calling; i++ {
		o.bots[botIdx] = NewBotClient(botIdx, "calling-station", o.handLogger, o.logger)
		botIdx++
	}

	// Create random bots
	for i := 0; i < cli.Random; i++ {
		o.bots[botIdx] = NewBotClient(botIdx, "random", o.handLogger, o.logger)
		botIdx++
	}

	// Create aggressive bots
	for i := 0; i < cli.Aggressive; i++ {
		o.bots[botIdx] = NewBotClient(botIdx, "aggressive", o.handLogger, o.logger)
		botIdx++
	}

	// Connect all bots
	for _, bot := range o.bots {
		if err := bot.Connect(cli.Server); err != nil {
			return fmt.Errorf("bot %s failed to connect: %v", bot.name, err)
		}
	}

	return nil
}

func (o *BotOrchestrator) run() <-chan struct{} {
	done := make(chan struct{})

	// Start all bots
	var wg sync.WaitGroup
	for _, bot := range o.bots {
		wg.Add(1)
		go func(b *BotClient) {
			defer wg.Done()
			b.Run()
		}(bot)
	}

	// Monitor progress
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for { //nolint:staticcheck // explicit select for clarity
			select {
			case <-ticker.C:
				currentHands := atomic.LoadUint64(&o.handLogger.handsLogged)

				elapsed := time.Since(startTime)
				rate := float64(currentHands) / elapsed.Seconds() * 60

				// Log progress
				handsCompleted := currentHands
				logEvent := o.logger.Info().Uint64("hands_completed", handsCompleted).
					Float64("rate_per_minute", rate).
					Dur("elapsed", elapsed)

				if o.targetHands > 0 {
					handsRemaining := o.targetHands - int(handsCompleted)
					if handsRemaining < 0 {
						handsRemaining = 0
					}
					progress := float64(handsCompleted) / float64(o.targetHands) * 100
					if progress > 100 {
						progress = 100
					}
					logEvent = logEvent.Int("target_hands", o.targetHands).
						Int("hands_remaining", handsRemaining).
						Float64("progress_percent", progress)
				}

				logEvent.Msg("Session progress")

				// For limited hands, check if server has reached its limit
				if o.targetHands > 0 {
					// Poll server stats to see if hand limit reached
					if o.checkServerHandLimit() {
						o.logger.Info().Msg("Server hand limit reached - shutting down")
						close(done)
						return
					}
				}
			}
		}
	}()

	return done
}

// checkServerHandLimit polls the server's stats endpoint to see if hand limit is reached
func (o *BotOrchestrator) checkServerHandLimit() bool {
	if o.serverAddr == "" {
		return false // No server address available
	}

	statsURL := fmt.Sprintf("http://localhost%s/stats", o.serverAddr)
	resp, err := http.Get(statsURL)
	if err != nil {
		// Server might be down or unreachable, don't shutdown on this
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	statsText := string(body)

	// Look for "Hands remaining: 0" which indicates limit reached
	return strings.Contains(statsText, "Hands remaining: 0")
}

func (o *BotOrchestrator) printSummary() {
	hands := atomic.LoadUint64(&o.handLogger.handsLogged)
	elapsed := time.Since(startTime)
	rate := float64(hands) / elapsed.Seconds() * 60

	o.logger.Info().Uint64("hands_played", hands).
		Dur("total_elapsed", elapsed).
		Float64("final_rate_per_minute", rate).
		Msg("Session completed successfully")
}

// BotClient represents a single bot connection
type BotClient struct {
	id            int
	name          string
	strategy      string
	conn          *websocket.Conn
	handLogger    *HandLogger
	logger        zerolog.Logger
	seat          int
	chips         int
	holeCards     []string
	currentStreet string
}

func NewBotClient(id int, strategy string, handLogger *HandLogger, logger zerolog.Logger) *BotClient {
	botName := fmt.Sprintf("Bot%d_%s", id, strategy)
	return &BotClient{
		id:         id,
		name:       botName,
		strategy:   strategy,
		handLogger: handLogger,
		logger:     logger.With().Str("component", "bot").Str("bot_name", botName).Str("strategy", strategy).Logger(),
	}
}

func (b *BotClient) Connect(serverURL string) error {
	// Ensure we're connecting to the /ws endpoint
	if !strings.HasSuffix(serverURL, "/ws") {
		serverURL = strings.TrimRight(serverURL, "/") + "/ws"
	}

	u, err := url.Parse(serverURL)
	if err != nil {
		return err
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}

	b.conn = conn

	// Send connect message
	connectMsg := &protocol.Connect{
		Type: "connect",
		Name: b.name,
	}
	data, _ := protocol.Marshal(connectMsg)
	return conn.WriteMessage(websocket.BinaryMessage, data)
}

func (b *BotClient) Run() {
	for {
		_, data, err := b.conn.ReadMessage()
		if err != nil {
			return
		}

		b.handleMessage(data)
	}
}

func (b *BotClient) handleMessage(data []byte) {
	// Try HandStart
	var handStart protocol.HandStart
	if err := protocol.Unmarshal(data, &handStart); err == nil && handStart.Type == "hand_start" {
		b.handleHandStart(&handStart)
		return
	}

	// Try ActionRequest
	var actionReq protocol.ActionRequest
	if err := protocol.Unmarshal(data, &actionReq); err == nil && actionReq.Type == "action_request" {
		b.handleActionRequest(&actionReq)
		return
	}

	// Try GameUpdate
	var gameUpdate protocol.GameUpdate
	if err := protocol.Unmarshal(data, &gameUpdate); err == nil && gameUpdate.Type == "game_update" {
		b.handleGameUpdate(&gameUpdate)
		return
	}

	// Try StreetChange
	var streetChange protocol.StreetChange
	if err := protocol.Unmarshal(data, &streetChange); err == nil && streetChange.Type == "street_change" {
		b.handleStreetChange(&streetChange)
		return
	}

	// Try HandResult
	var handResult protocol.HandResult
	if err := protocol.Unmarshal(data, &handResult); err == nil && handResult.Type == "hand_result" {
		b.handleHandResult(&handResult)
		return
	}
}

func (b *BotClient) handleHandStart(msg *protocol.HandStart) {
	b.seat = msg.YourSeat
	b.holeCards = msg.HoleCards
	b.currentStreet = "preflop"

	// Find our chips
	for _, p := range msg.Players {
		if p.Seat == b.seat {
			b.chips = p.Chips
			break
		}
	}

	// Log hand start
	b.handLogger.mu.Lock()
	if b.seat == 0 { // Only first bot logs hand info
		b.handLogger.handID = msg.HandID
		b.handLogger.players = nil
		b.handLogger.streets = nil
		b.handLogger.actions = nil
		b.handLogger.board = nil
		b.handLogger.winners = nil

		for _, p := range msg.Players {
			b.handLogger.players = append(b.handLogger.players, PlayerInfo{
				Name:  p.Name,
				Seat:  p.Seat,
				Chips: p.Chips,
			})
		}

		// Log hand start with structured data
		playerNames := make([]string, len(msg.Players))
		playerChips := make([]int, len(msg.Players))
		for i, p := range msg.Players {
			playerNames[i] = p.Name
			playerChips[i] = p.Chips
		}

		b.logger.Debug().Str("hand_id", msg.HandID).
			Int("button", msg.Button).
			Int("small_blind", msg.SmallBlind).
			Int("big_blind", msg.BigBlind).
			Strs("player_names", playerNames).
			Ints("player_chips", playerChips).
			Strs("hole_cards", b.holeCards).
			Msg("Hand started")
	}
	b.handLogger.mu.Unlock()
}

func (b *BotClient) handleActionRequest(msg *protocol.ActionRequest) {
	// Determine action based on strategy
	action, amount := b.selectAction(msg)

	// Log action with structured data
	b.logger.Debug().Str("hand_id", msg.HandID).
		Str("street", b.currentStreet).
		Str("action", action).
		Int("amount", amount).
		Int("pot", msg.Pot).
		Int("to_call", msg.ToCall).
		Strs("valid_actions", msg.ValidActions).
		Msg("Player action")

	// Send action
	actionMsg := &protocol.Action{
		Type:   "action",
		Action: action,
		Amount: amount,
	}
	respData, _ := protocol.Marshal(actionMsg)
	b.conn.WriteMessage(websocket.BinaryMessage, respData)
}

func (b *BotClient) selectAction(req *protocol.ActionRequest) (string, int) {
	switch b.strategy {
	case "calling-station":
		// Always check or call
		for _, action := range req.ValidActions {
			if action == "check" {
				return "check", 0
			}
		}
		for _, action := range req.ValidActions {
			if action == "call" {
				return "call", 0
			}
		}
		return "fold", 0

	case "aggressive":
		// Raise/bet 70% of the time
		if rng.Float32() < 0.7 {
			for _, action := range req.ValidActions {
				if action == "raise" {
					// Raise 2-3x pot
					amount := req.Pot * (2 + rng.Intn(2))
					if amount < req.MinBet {
						amount = req.MinBet
					}
					// Cap at chips
					if amount > b.chips {
						amount = b.chips
					}
					return "raise", amount
				}
			}
			for _, action := range req.ValidActions {
				if action == "allin" {
					return "allin", 0
				}
			}
		}
		// Otherwise call/check
		for _, action := range req.ValidActions {
			if action == "call" {
				return "call", 0
			}
			if action == "check" {
				return "check", 0
			}
		}
		return "fold", 0

	default: // random
		// Pick random valid action
		if len(req.ValidActions) > 0 {
			action := req.ValidActions[rng.Intn(len(req.ValidActions))]
			if action == "raise" {
				amount := req.MinBet + rng.Intn(req.Pot+1)
				// Cap at chips
				if amount > b.chips {
					amount = b.chips
				}
				return "raise", amount
			}
			return action, 0
		}
		return "fold", 0
	}
}

func (b *BotClient) handleGameUpdate(msg *protocol.GameUpdate) {
	// Update our chips
	for _, p := range msg.Players {
		if p.Seat == b.seat {
			b.chips = p.Chips
			break
		}
	}
}

func (b *BotClient) handleStreetChange(msg *protocol.StreetChange) {
	b.currentStreet = msg.Street

	b.handLogger.mu.Lock()
	b.handLogger.streets = append(b.handLogger.streets, msg.Street)
	b.handLogger.board = msg.Board
	b.handLogger.mu.Unlock()

	// Log street change with structured data
	if b.seat == 0 { // Only first bot logs to avoid duplicates
		b.logger.Debug().Str("street", msg.Street).
			Strs("board", msg.Board).
			Msg("Street changed")
	}
}

func (b *BotClient) handleHandResult(msg *protocol.HandResult) {
	b.handLogger.mu.Lock()
	b.handLogger.winners = msg.Winners
	b.handLogger.board = msg.Board
	handNum := b.handLogger.handsLogged + 1 // Get hand number before incrementing
	if b.seat == 0 {
		atomic.AddUint64(&b.handLogger.handsLogged, 1)
	}
	b.handLogger.mu.Unlock()

	// Log hand result with structured data (only first bot to avoid duplicates)
	if b.seat == 0 {
		winnerNames := make([]string, len(msg.Winners))
		winnerAmounts := make([]int, len(msg.Winners))
		for i, winner := range msg.Winners {
			winnerNames[i] = winner.Name
			winnerAmounts[i] = winner.Amount
		}

		b.logger.Debug().Str("hand_id", msg.HandID).
			Strs("final_board", msg.Board).
			Strs("winner_names", winnerNames).
			Ints("winner_amounts", winnerAmounts).
			Msg("Hand completed")

		// Print reproduce arguments to recreate the session up to and including this hand
		// This will replay with the same seed and run exactly handNum hands
		reproduceArgs := fmt.Sprintf("--seed %d --bots %d --calling %d --random %d --aggressive %d --hands %d",
			cli.Seed, cli.Bots, cli.Calling, cli.Random, cli.Aggressive, handNum)

		if cli.SpawnServer {
			reproduceArgs += fmt.Sprintf(" --spawn-server --port %d", cli.Port)
		} else {
			reproduceArgs += fmt.Sprintf(" --server %s", cli.Server)
		}

		b.logger.Info().
			Uint64("hand_number", handNum).
			Str("hand_id", msg.HandID).
			Str("reproduce_args", reproduceArgs).
			Msg("To reproduce up to this hand")
	}
}
