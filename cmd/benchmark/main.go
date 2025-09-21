package main

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/lox/pokerforbots/internal/server"
	"github.com/lox/pokerforbots/protocol"
	"github.com/lox/pokerforbots/sdk/client"
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

		srv = server.NewServer(logger, rng, server.WithConfig(config))

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
	bots := make([]*client.Bot, cli.Bots)
	var wg sync.WaitGroup

	for i := 0; i < cli.Bots; i++ {
		id := fmt.Sprintf("bench-%03d", i)
		benchBot := newBenchBot(id, logger, &handCount)
		bot := client.New(id, benchBot, logger)

		if err := bot.Connect(serverURL); err != nil {
			fmt.Printf("Failed to connect bot %s: %v\n", id, err)
			return
		}
		bots[i] = bot

		wg.Add(1)
		go func(b *client.Bot) {
			defer wg.Done()
			b.Run(ctx)
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
	logger  zerolog.Logger
	rng     *rand.Rand
	counter *int64 // Shared counter for hands completed
}

func newBenchBot(id string, logger zerolog.Logger, counter *int64) *benchBot {
	return &benchBot{
		id:      id,
		logger:  logger.With().Str("bot_id", id).Logger(),
		rng:     rand.New(rand.NewSource(rand.Int63())),
		counter: counter,
	}
}

// SDK Handler interface implementation
func (b *benchBot) OnHandStart(*client.GameState, protocol.HandStart) error       { return nil }
func (b *benchBot) OnGameUpdate(*client.GameState, protocol.GameUpdate) error     { return nil }
func (b *benchBot) OnPlayerAction(*client.GameState, protocol.PlayerAction) error { return nil }
func (b *benchBot) OnStreetChange(*client.GameState, protocol.StreetChange) error { return nil }

func (b *benchBot) OnHandResult(state *client.GameState, result protocol.HandResult) error {
	// Only the first bot increments to avoid double-counting
	// The server counts hands, not hand-participations
	if b.id == "bench-000" {
		atomic.AddInt64(b.counter, 1)
	}
	return nil
}

func (b *benchBot) OnGameCompleted(state *client.GameState, completed protocol.GameCompleted) error {
	// When the server hits its hand limit, it sends GameCompleted
	// Signal the benchmark to exit by setting counter to match target
	if b.id == "bench-000" {
		atomic.StoreInt64(b.counter, int64(completed.HandsCompleted))
	}
	return nil
}

func (b *benchBot) OnActionRequest(state *client.GameState, req protocol.ActionRequest) (string, int, error) {
	if len(req.ValidActions) == 0 {
		return "fold", 0, nil
	}

	// Simple strategy: mostly call/check, occasionally fold/raise
	r := b.rng.Float32()

	// 70% call/check, 20% fold, 10% raise
	switch {
	case r < 0.7:
		if slices.Contains(req.ValidActions, "check") {
			return "check", 0, nil
		}
		if slices.Contains(req.ValidActions, "call") {
			return "call", 0, nil
		}
	case r < 0.9:
		if slices.Contains(req.ValidActions, "fold") {
			return "fold", 0, nil
		}
	default:
		if slices.Contains(req.ValidActions, "raise") {
			amount := req.MinBet
			if req.Pot > 0 && b.rng.Float32() < 0.3 {
				amount = req.Pot / 2 // Half pot bet
			}
			if amount < req.MinBet {
				amount = req.MinBet
			}
			return "raise", amount, nil
		}
	}

	// Fallback
	return req.ValidActions[0], 0, nil
}
