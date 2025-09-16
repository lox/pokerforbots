package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
)

var (
	// Server flags
	spawnServer = flag.Bool("spawn-server", false, "Start a poker server")
	serverURL   = flag.String("server", "ws://localhost:8080", "Server WebSocket URL")
	serverPort  = flag.Int("port", 8080, "Server port (when spawning)")

	// Bot configuration
	numBots        = flag.Int("bots", 6, "Total number of bots")
	callingBots    = flag.Int("calling", 0, "Number of calling station bots (0 = auto)")
	randomBots     = flag.Int("random", 0, "Number of random bots (0 = auto)")
	aggressiveBots = flag.Int("aggressive", 0, "Number of aggressive bots (0 = auto)")

	// Game configuration
	hands   = flag.Int("hands", 0, "Number of hands to play (0 = forever)")
	seed    = flag.Int64("seed", 0, "Random seed (0 for current time)")
	verbose = flag.Bool("v", false, "Verbose output")
	quiet   = flag.Bool("q", false, "Quiet mode (only show summary)")

	// Statistics
	startTime time.Time
	rng       *rand.Rand
)

type BotOrchestrator struct {
	serverProcess *exec.Cmd
	bots          []*BotClient
	logger        *HandLogger
	targetHands   int
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
	flag.Parse()

	// Validate configuration
	if *callingBots == 0 && *randomBots == 0 && *aggressiveBots == 0 {
		// Auto distribution based on total bots
		third := *numBots / 3
		remainder := *numBots % 3
		*callingBots = third
		*randomBots = third
		*aggressiveBots = third

		// Distribute remainder
		if remainder >= 1 {
			*callingBots++
		}
		if remainder >= 2 {
			*randomBots++
		}
	} else if *callingBots+*randomBots+*aggressiveBots > 0 {
		// Custom bot distribution specified
		*numBots = *callingBots + *randomBots + *aggressiveBots
	}

	if *numBots < 2 || *numBots > 10 {
		log.Fatal("Number of bots must be between 2 and 10")
	}

	// Set seed
	if *seed == 0 {
		*seed = time.Now().UnixNano()
	}
	rng = rand.New(rand.NewSource(*seed))

	// Print configuration
	if !*quiet {
		fmt.Printf("=====================================\n")
		fmt.Printf("Poker Bot Orchestrator\n")
		fmt.Printf("=====================================\n")
		fmt.Printf("Seed: %d\n", *seed)
		fmt.Printf("Bots: %d total\n", *numBots)
		fmt.Printf("  - Calling Station: %d\n", *callingBots)
		fmt.Printf("  - Random: %d\n", *randomBots)
		fmt.Printf("  - Aggressive: %d\n", *aggressiveBots)
		if *hands > 0 {
			fmt.Printf("Target: %d hands\n", *hands)
		} else {
			fmt.Printf("Target: Run forever\n")
		}
		if *spawnServer {
			fmt.Printf("Server: Spawning on port %d\n", *serverPort)
		} else {
			fmt.Printf("Server: %s\n", *serverURL)
		}
		fmt.Printf("=====================================\n\n")
	}

	orchestrator := &BotOrchestrator{
		targetHands: *hands,
		logger:      &HandLogger{},
	}

	// Set up signal handling early
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server if requested
	if *spawnServer {
		if err := orchestrator.startServer(); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
		defer orchestrator.cleanup()

		// Update server URL to local
		*serverURL = fmt.Sprintf("ws://localhost:%d", *serverPort)
	} else {
		defer orchestrator.cleanup()
	}

	// Verify server is reachable
	if !orchestrator.isServerReady() {
		log.Println("Server is not reachable")
		return
	}

	// Start time tracking
	startTime = time.Now()

	// Create and connect bots
	if err := orchestrator.createBots(); err != nil {
		log.Printf("Failed to create bots: %v", err)
		return
	}

	// Run bots
	done := orchestrator.run()

	// Wait for completion or interrupt
	select {
	case <-done:
		if !*quiet {
			fmt.Println("\nTarget reached!")
		}
	case <-sigChan:
		if !*quiet {
			fmt.Println("\nInterrupted - cleaning up...")
		}
	}

	// Show summary
	orchestrator.printSummary()
}

func (o *BotOrchestrator) startServer() error {
	if !*quiet {
		fmt.Printf("Starting poker server on port %d...\n", *serverPort)
	}

	// Build the server first
	buildCmd := exec.Command("go", "build", "-o", "dist/server", "cmd/server/main.go")
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build server: %v", err)
	}

	// Start the server
	o.serverProcess = exec.Command("./dist/server")

	// Optionally capture server output for debugging
	if *verbose {
		o.serverProcess.Stdout = os.Stdout
		o.serverProcess.Stderr = os.Stderr
	}

	if err := o.serverProcess.Start(); err != nil {
		return fmt.Errorf("failed to start server: %v", err)
	}

	// Wait for server to be ready
	time.Sleep(2 * time.Second)

	return nil
}

func (o *BotOrchestrator) stopServer() {
	if o.serverProcess != nil {
		// Try graceful shutdown first
		o.serverProcess.Process.Signal(syscall.SIGTERM)

		// Give it a moment to shut down
		done := make(chan error, 1)
		go func() {
			done <- o.serverProcess.Wait()
		}()

		select {
		case <-done:
			// Process exited gracefully
		case <-time.After(2 * time.Second):
			// Force kill if it doesn't exit
			o.serverProcess.Process.Kill()
			o.serverProcess.Wait()
		}
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
	if o.serverProcess != nil {
		o.stopServer()
	}
}

func (o *BotOrchestrator) isServerReady() bool {
	// Try to connect to stats endpoint
	url := strings.Replace(*serverURL, "ws://", "http://", 1)
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
	o.bots = make([]*BotClient, *numBots)
	botIdx := 0

	// Create calling station bots
	for i := 0; i < *callingBots; i++ {
		o.bots[botIdx] = NewBotClient(botIdx, "calling-station", o.logger)
		botIdx++
	}

	// Create random bots
	for i := 0; i < *randomBots; i++ {
		o.bots[botIdx] = NewBotClient(botIdx, "random", o.logger)
		botIdx++
	}

	// Create aggressive bots
	for i := 0; i < *aggressiveBots; i++ {
		o.bots[botIdx] = NewBotClient(botIdx, "aggressive", o.logger)
		botIdx++
	}

	// Connect all bots
	for _, bot := range o.bots {
		if err := bot.Connect(*serverURL); err != nil {
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
				currentHands := atomic.LoadUint64(&o.logger.handsLogged)

				if !*quiet && !*verbose {
					// Show progress line
					elapsed := time.Since(startTime)
					rate := float64(currentHands) / elapsed.Seconds() * 60

					if o.targetHands > 0 {
						fmt.Printf("\rHands: %d/%d | Rate: %.0f/min | Elapsed: %s",
							currentHands, o.targetHands, rate, elapsed.Round(time.Second))
					} else {
						fmt.Printf("\rHands: %d | Rate: %.0f/min | Elapsed: %s",
							currentHands, rate, elapsed.Round(time.Second))
					}
				}

				// Check if we reached target
				if o.targetHands > 0 && currentHands >= uint64(o.targetHands) {
					close(done)
					return
				}
			}
		}
	}()

	return done
}

func (o *BotOrchestrator) printSummary() {
	hands := atomic.LoadUint64(&o.logger.handsLogged)
	elapsed := time.Since(startTime)
	rate := float64(hands) / elapsed.Seconds() * 60

	fmt.Printf("\n\n=====================================\n")
	fmt.Printf("Summary\n")
	fmt.Printf("=====================================\n")
	fmt.Printf("Hands played: %d\n", hands)
	fmt.Printf("Time elapsed: %s\n", elapsed.Round(time.Second))
	fmt.Printf("Hands/minute: %.0f\n", rate)
	fmt.Printf("=====================================\n")

	if *seed != 0 {
		fmt.Printf("To reproduce: -seed %d -bots %d", *seed, *numBots)
		if hands > 0 {
			fmt.Printf(" -hands %d", hands)
		}
		if *spawnServer {
			fmt.Printf(" -spawn-server")
		}
		fmt.Printf("\n")
	}
}

// BotClient represents a single bot connection
type BotClient struct {
	id            int
	name          string
	strategy      string
	conn          *websocket.Conn
	logger        *HandLogger
	seat          int
	chips         int
	holeCards     []string
	currentStreet string
}

func NewBotClient(id int, strategy string, logger *HandLogger) *BotClient {
	return &BotClient{
		id:       id,
		name:     fmt.Sprintf("Bot%d_%s", id, strategy),
		strategy: strategy,
		logger:   logger,
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
	b.logger.mu.Lock()
	if b.seat == 0 { // Only first bot logs hand info
		b.logger.handID = msg.HandID
		b.logger.players = nil
		b.logger.streets = nil
		b.logger.actions = nil
		b.logger.board = nil
		b.logger.winners = nil

		for _, p := range msg.Players {
			b.logger.players = append(b.logger.players, PlayerInfo{
				Name:  p.Name,
				Seat:  p.Seat,
				Chips: p.Chips,
			})
		}

		if *verbose && !*quiet {
			fmt.Printf("\n=== HAND %s ===\n", msg.HandID)
			fmt.Printf("Button: Seat %d\n", msg.Button)
			fmt.Printf("Blinds: %d/%d\n", msg.SmallBlind, msg.BigBlind)
			fmt.Printf("Players:\n")
			for _, p := range msg.Players {
				fmt.Printf("  Seat %d: %s (%d chips)\n", p.Seat, p.Name, p.Chips)
			}
			fmt.Printf("\n")
		}
	}
	b.logger.mu.Unlock()
}

func (b *BotClient) handleActionRequest(msg *protocol.ActionRequest) {
	// Determine action based on strategy
	action, amount := b.selectAction(msg)

	// Log action
	if *verbose && !*quiet {
		actionStr := action
		if action == "raise" {
			actionStr = fmt.Sprintf("raises to %d", amount)
		} else if action == "call" && msg.ToCall > 0 {
			actionStr = fmt.Sprintf("calls %d", msg.ToCall)
		}

		b.logger.mu.Lock()
		fmt.Printf("[%s] %s: %s\n", strings.ToUpper(b.currentStreet), b.name, actionStr)
		b.logger.mu.Unlock()
	}

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
					if amount < req.MinRaise {
						amount = req.MinRaise
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
				amount := req.MinRaise + rng.Intn(req.Pot+1)
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

	b.logger.mu.Lock()
	b.logger.streets = append(b.logger.streets, msg.Street)
	b.logger.board = msg.Board

	if *verbose && !*quiet {
		fmt.Printf("\n=== %s ===\n", strings.ToUpper(msg.Street))
		if len(msg.Board) > 0 {
			fmt.Printf("Board: %s\n", strings.Join(msg.Board, " "))
		}
	}

	b.logger.mu.Unlock()
}

func (b *BotClient) handleHandResult(msg *protocol.HandResult) {
	b.logger.mu.Lock()
	defer b.logger.mu.Unlock()

	b.logger.winners = msg.Winners
	b.logger.board = msg.Board

	// Increment hands counter
	atomic.AddUint64(&b.logger.handsLogged, 1)

	if *verbose && !*quiet {
		fmt.Printf("\n=== SHOWDOWN ===\n")
		fmt.Printf("Final Board: %s\n", strings.Join(msg.Board, " "))
		fmt.Printf("Winners:\n")
		for _, winner := range msg.Winners {
			fmt.Printf("  %s wins %d\n", winner.Name, winner.Amount)
		}
		fmt.Printf("\n")
	}
}
