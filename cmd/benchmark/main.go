package main

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/signal"
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
	Bots      int    `kong:"default='6',help='Number of WebSocket bots'"`
	Hands     int    `kong:"default='50000',help='Number of hands to benchmark'"`
	Port      string `kong:"default='0',help='Server port to use (0 for random port)'"`
	ServerURL string `kong:"help='External server URL (if set, uses external server instead of starting embedded one)'"`
	TimeoutMs int    `kong:"default='5',help='Decision timeout in milliseconds'"`
	Debug     bool   `kong:"default='false',help='Show debug logs'"`
}

func main() {
	var cli CLI
	kong.Parse(&cli,
		kong.Name("pokerforbots-benchmark"),
		kong.Description("WebSocket benchmark client for PokerForBots server"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
	)

	fmt.Printf("PokerForBots Benchmark\n")

	// Set up logging
	level := zerolog.Disabled // Default to no logging for maximum performance
	if cli.Debug {
		level = zerolog.DebugLevel // Enable verbose logging
	}
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).Level(level)

	var serverURL string
	var srv *server.Server

	if cli.ServerURL != "" {
		// Use external server
		serverURL = cli.ServerURL
		fmt.Printf("Using external server: %s\n", serverURL)
	} else {
		// Start embedded server
		fmt.Print("Starting embedded server...")

		// Create optimized server configuration for benchmarking
		seed := time.Now().UnixNano()
		rng := rand.New(rand.NewSource(seed))
		config := server.Config{
			SmallBlind:       5,
			BigBlind:         10,
			StartChips:       1000,
			Timeout:          time.Duration(cli.TimeoutMs) * time.Millisecond,
			MinPlayers:       cli.Bots,
			MaxPlayers:       cli.Bots,
			RequirePlayer:    false,             // Don't require player role for benchmark
			InfiniteBankroll: true,              // Prevent bots from running out of chips
			HandLimit:        uint64(cli.Hands), // Set server to stop after target hands
			Seed:             seed,
			EnableStats:      false, // Disable for maximum performance
		}

		srv = server.NewServerWithConfig(logger, rng, config)

		// Create listener to get actual assigned port (supports port 0 for random)
		listener, err := net.Listen("tcp", ":"+cli.Port)
		if err != nil {
			fmt.Printf("Failed to create listener: %v\n", err)
			return
		}

		// Get the actual assigned port (important when using port 0)
		actualPort := listener.Addr().(*net.TCPAddr).Port

		// Start server in background using the listener
		serverErr := make(chan error, 1)
		go func() {
			serverErr <- srv.Serve(listener)
		}()

		// Wait for server to be healthy
		baseURL := fmt.Sprintf("http://localhost:%d", actualPort)
		healthCtx, healthCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer healthCancel()

		select {
		case err := <-serverErr:
			fmt.Printf("Failed to start server: %v\n", err)
			return
		case <-time.After(100 * time.Millisecond):
			// Give server a moment to start, then check health
			if err := server.WaitForHealthy(healthCtx, baseURL); err != nil {
				fmt.Printf("Server not healthy: %v\n", err)
				return
			}
		}

		// Set up server cleanup
		defer func() {
			if srv != nil {
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				srv.Shutdown(shutdownCtx)
			}
		}()

		serverURL = fmt.Sprintf("ws://localhost:%d/ws", actualPort)
		fmt.Printf(" ✓ (port %d)\n", actualPort)
	}

	fmt.Printf("Config: Bots: %d, Hands: %d, Timeout: %dms\n\n", cli.Bots, cli.Hands, cli.TimeoutMs)

	// Shared counter for completed hands
	var handCount int64
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create bots
	fmt.Printf("Starting %d bots...\n", cli.Bots)
	bots := make([]*benchBot, cli.Bots)
	var wg sync.WaitGroup

	for i := 0; i < cli.Bots; i++ {
		id := fmt.Sprintf("bench-%03d", i)
		bot, err := newBenchBot(id, serverURL, logger, &handCount)
		if err != nil {
			fmt.Printf("Failed to create bot %s: %v\n", id, err)
			return
		}
		bots[i] = bot

		wg.Add(1)
		go func(b *benchBot) {
			defer wg.Done()
			b.run(ctx)
		}(bot)
	}

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Monitor progress
	startTime := time.Now()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	fmt.Printf("Benchmarking...\n")
	target := int64(cli.Hands)

	for {
		select {
		case <-sigChan:
			fmt.Println("\nInterrupted")
			cancel()
			goto done

		case <-ticker.C:
			completed := atomic.LoadInt64(&handCount)
			elapsed := time.Since(startTime).Seconds()

			if elapsed > 0 && completed > 0 {
				rate := float64(completed) / elapsed
				progress := float64(completed) * 100.0 / float64(target)
				fmt.Printf("  %d/%d hands (%.1f%%) - %.1f h/s\n",
					completed, target, progress, rate)
			}

			if completed >= target {
				cancel() // Signal all bots to stop
				goto done
			}
		}
	}

done:
	cancel()

	// Final stats
	finalCount := atomic.LoadInt64(&handCount)
	totalTime := time.Since(startTime)

	if finalCount > 0 {
		rate := float64(finalCount) / totalTime.Seconds()
		fmt.Printf("\nBenchmark complete:\n")
		fmt.Printf("  Hands: %d\n", finalCount)
		fmt.Printf("  Time: %.2fs\n", totalTime.Seconds())
		fmt.Printf("  Rate: %.1f hands/second\n", rate)
	}

	wg.Wait()
}

type benchBot struct {
	id      string
	conn    *websocket.Conn
	logger  zerolog.Logger
	rng     *rand.Rand
	counter *int64 // Shared counter for hands completed
}

func newBenchBot(id string, serverURL string, logger zerolog.Logger, counter *int64) (*benchBot, error) {
	conn, _, err := websocket.DefaultDialer.Dial(serverURL, nil)
	if err != nil {
		return nil, err
	}

	bot := &benchBot{
		id:      id,
		conn:    conn,
		logger:  logger.With().Str("bot_id", id).Logger(),
		rng:     rand.New(rand.NewSource(rand.Int63())),
		counter: counter,
	}

	// Send connect message
	connect := &protocol.Connect{
		Type: protocol.TypeConnect,
		Name: id,
		Role: "player",
	}
	payload, err := protocol.Marshal(connect)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		conn.Close()
		return nil, err
	}

	return bot, nil
}

func (b *benchBot) run(ctx context.Context) {
	defer b.conn.Close()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Set read timeout to avoid hanging
		b.conn.SetReadDeadline(time.Now().Add(5 * time.Second))

		msgType, data, err := b.conn.ReadMessage()
		if err != nil {
			// Connection closed or timeout - server probably finished
			return
		}
		if msgType != websocket.BinaryMessage {
			continue
		}

		// Handle action requests
		var req protocol.ActionRequest
		if err := protocol.Unmarshal(data, &req); err == nil && req.Type == protocol.TypeActionRequest {
			action := b.pickAction(req)
			resp, err := protocol.Marshal(&action)
			if err != nil {
				continue
			}
			if err := b.conn.WriteMessage(websocket.BinaryMessage, resp); err != nil {
				return
			}
			continue
		}

		// Check for hand result to count completed hands
		var result protocol.HandResult
		if err := protocol.Unmarshal(data, &result); err == nil && result.Type == protocol.TypeHandResult {
			// Only the first bot increments to avoid double-counting
			if b.id == "bench-000" {
				atomic.AddInt64(b.counter, 1)
			}
		}

		// Check for game completion
		var gameComplete protocol.GameCompleted
		if err := protocol.Unmarshal(data, &gameComplete); err == nil && gameComplete.Type == protocol.TypeGameCompleted {
			return
		}
	}
}

func (b *benchBot) pickAction(req protocol.ActionRequest) protocol.Action {
	if len(req.ValidActions) == 0 {
		return protocol.Action{Type: protocol.TypeAction, Action: "fold"}
	}

	// Simple strategy: mostly call/check, occasionally fold/raise
	r := b.rng.Float32()

	// 70% call/check, 20% fold, 10% raise
	switch {
	case r < 0.7:
		for _, action := range req.ValidActions {
			if action == "check" {
				return protocol.Action{Type: protocol.TypeAction, Action: "check"}
			}
		}
		for _, action := range req.ValidActions {
			if action == "call" {
				return protocol.Action{Type: protocol.TypeAction, Action: "call"}
			}
		}
	case r < 0.9:
		for _, action := range req.ValidActions {
			if action == "fold" {
				return protocol.Action{Type: protocol.TypeAction, Action: "fold"}
			}
		}
	default:
		for _, action := range req.ValidActions {
			if action == "raise" {
				amount := req.MinBet
				if req.Pot > 0 && b.rng.Float32() < 0.3 {
					amount = req.Pot / 2 // Half pot bet
				}
				if amount < req.MinBet {
					amount = req.MinBet
				}
				return protocol.Action{Type: protocol.TypeAction, Action: "raise", Amount: amount}
			}
		}
	}

	// Fallback
	return protocol.Action{Type: protocol.TypeAction, Action: req.ValidActions[0]}
}
