package main

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/log"
	"github.com/lox/holdem-cli/internal/bot"
	"github.com/lox/holdem-cli/internal/game"
)

type CLI struct {
	Hands      int           `default:"50000" help:"Number of hands to simulate"`
	Opponent   string        `default:"fold" help:"Opponent type: fold, call, rand, chart, maniac, tag, mixed"`
	Seed       int64         `default:"0" help:"RNG seed (0 for random)"`
	Timeout    time.Duration `default:"5s" help:"Timeout per hand to detect hangs"`
	Verbose    bool          `short:"v" help:"Verbose logging"`
	CPUProfile string        `help:"Write CPU profile to file"`
	MemProfile string        `help:"Write memory profile to file"`
}

type HandResult struct {
	NetBB          float64 // Net big blinds won/lost for our bot
	Seed           int64   // RNG seed for this hand (for replay)
	Position       int     // OurBot's position (1-6)
	WentToShowdown bool    // Did hand go to showdown?
	FinalPotSize   int     // Final pot size in chips
	StreetReached  string  // Furthest street reached (Pre-flop, Flop, Turn, River)
}

type Statistics struct {
	Hands  int
	SumBB  float64
	SumBB2 float64   // Sum of squares for variance calculation
	Values []float64 // Store all values for median/percentile calculation

	// Detailed analytics - track ALL results, not just wins
	ShowdownWins    int     // Hands won at showdown
	NonShowdownWins int     // Hands won without showdown (fold equity)
	ShowdownBB      float64 // BB from showdown (wins AND losses)
	NonShowdownBB   float64 // BB from fold equity (wins AND losses)
	AllBB           float64 // Total BB for sanity check

	// Position analytics
	PositionResults [7]PositionStats // Index 0 unused, 1-6 for positions

	// Pot size analytics
	MaxPotChips int     // Largest pot observed (in chips)
	MaxPotBB    float64 // Largest pot observed (in bb)
	BigPots     int     // Pots >= 50bb (high action hands)
	BigPotsBB   float64 // BB from big pots
}

type PositionStats struct {
	Hands  int
	SumBB  float64
	SumBB2 float64
}

func (s *Statistics) Mean() float64 {
	if s.Hands == 0 {
		return 0
	}
	return s.SumBB / float64(s.Hands)
}

func (s *Statistics) Variance() float64 {
	if s.Hands < 2 {
		return 0
	}
	mean := s.Mean()
	return (s.SumBB2 - float64(s.Hands)*mean*mean) / float64(s.Hands-1)
}

func (s *Statistics) StdDev() float64 {
	return math.Sqrt(s.Variance())
}

func (s *Statistics) StdError() float64 {
	if s.Hands == 0 {
		return 0
	}
	return s.StdDev() / math.Sqrt(float64(s.Hands))
}

func (s *Statistics) ConfidenceInterval95() (float64, float64) {
	mean := s.Mean()
	se := s.StdError()
	margin := 1.96 * se // 95% confidence
	return mean - margin, mean + margin
}

func (s *Statistics) Add(result HandResult) {
	netBB := result.NetBB
	s.Hands++
	s.SumBB += netBB
	s.SumBB2 += netBB * netBB
	s.Values = append(s.Values, netBB)

	// Track showdown vs non-showdown (for now, assume all hands are non-showdown)
	// TODO: Get actual showdown info from engine
	if netBB > 0 {
		s.NonShowdownWins++
	}

	// Track ALL results (wins and losses) in appropriate buckets
	if result.WentToShowdown {
		s.ShowdownBB += netBB
	} else {
		s.NonShowdownBB += netBB
	}
	s.AllBB += netBB // Total for sanity check

	// Track by position
	pos := result.Position
	if pos >= 1 && pos <= 6 {
		s.PositionResults[pos].Hands++
		s.PositionResults[pos].SumBB += netBB
		s.PositionResults[pos].SumBB2 += netBB * netBB
	}

	// Track pot sizes and limits
	potChips := result.FinalPotSize
	potBB := float64(potChips) / 2.0 // 2 chips = 1 big blind

	// Update max pot if this is largest seen
	if potChips > s.MaxPotChips {
		s.MaxPotChips = potChips
		s.MaxPotBB = potBB
	}

	// Track big pots (>= 50bb)
	if potBB >= 50 {
		s.BigPots++
		s.BigPotsBB += netBB
	}
}

func (s *Statistics) Median() float64 {
	if len(s.Values) == 0 {
		return 0
	}
	sorted := make([]float64, len(s.Values))
	copy(sorted, s.Values)
	sort.Float64s(sorted)

	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

func (s *Statistics) Percentile(p float64) float64 {
	if len(s.Values) == 0 {
		return 0
	}
	sorted := make([]float64, len(s.Values))
	copy(sorted, s.Values)
	sort.Float64s(sorted)

	index := p * float64(len(sorted)-1)
	lower := int(index)
	upper := lower + 1

	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}

	weight := index - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

func main() {
	var cli CLI
	ctx := kong.Parse(&cli)

	// Setup CPU profiling if requested
	var cpuFile *os.File
	if cli.CPUProfile != "" {
		f, err := os.Create(cli.CPUProfile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating CPU profile: %v\n", err)
			os.Exit(1)
		}
		cpuFile = f

		if err := pprof.StartCPUProfile(f); err != nil {
			fmt.Fprintf(os.Stderr, "Error starting CPU profile: %v\n", err)
			f.Close()
			os.Exit(1)
		}
		fmt.Printf("CPU profiling enabled, writing to %s\n", cli.CPUProfile)
	}

	// Setup RNG seed
	if cli.Seed == 0 {
		cli.Seed = time.Now().UnixNano()
	}

	// Setup logging
	var logger *log.Logger
	if cli.Verbose {
		logger = log.NewWithOptions(os.Stderr, log.Options{Level: log.DebugLevel})
	} else {
		logger = log.NewWithOptions(os.Stderr, log.Options{Level: log.WarnLevel})
	}

	fmt.Printf("Starting simulation: %d hands vs %s-bot (seed: %d)\n",
		cli.Hands, cli.Opponent, cli.Seed)

	startTime := time.Now()
	stats, opponentInfo := runSimulation(cli.Hands, cli.Opponent, cli.Seed, cli.Timeout, logger)
	duration := time.Since(startTime)
	printResults(stats, opponentInfo, duration)

	// Stop CPU profiling before exit
	if cpuFile != nil {
		pprof.StopCPUProfile()
		cpuFile.Close()
		fmt.Printf("CPU profile written to %s\n", cli.CPUProfile)
	}

	// Write memory profile if requested
	if cli.MemProfile != "" {
		f, err := os.Create(cli.MemProfile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating memory profile: %v\n", err)
			os.Exit(1)
		}

		runtime.GC()
		if err := pprof.WriteHeapProfile(f); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing memory profile: %v\n", err)
			f.Close()
			os.Exit(1)
		}
		f.Close()
		fmt.Printf("Memory profile written to %s\n", cli.MemProfile)
	}

	ctx.Exit(0)
}

func runSimulation(numHands int, opponentType string, seed int64, timeout time.Duration, logger *log.Logger) (*Statistics, string) {
	stats := &Statistics{}

	// Determine opponent info string
	opponentInfo := opponentType
	var opponentMix []string
	if opponentType == "mixed" {
		opponentMix = createMixedOpponentTypes()
		opponentInfo = fmt.Sprintf("mixed(%s)", strings.Join(opponentMix, ","))
	}

	fmt.Printf("Target: <2ms per hand for optimal performance\n\n")

	for hand := 0; hand < numHands; hand++ {
		// Generate independent seed for this hand
		handSeed := seed + int64(hand)

		// Rotate OurBot's position (1-6) to eliminate positional bias
		ourPosition := (hand % 6) + 1

		result, err := playHandWithTimeout(opponentType, opponentMix, handSeed, ourPosition, timeout, logger)
		if err != nil {
			fmt.Printf("\n❌ HANG DETECTED on hand %d!\n", hand+1)
			fmt.Printf("Reproduction command: go run ./cmd/simulate --hands=1 --seed=%d --opponent=%s --timeout=10s\n", handSeed, opponentType)
			fmt.Printf("Hand seed: %d, Position: %d\n", handSeed, ourPosition)
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		stats.Add(result)
	}

	return stats, opponentInfo
}

func playHand(opponentType string, opponentMix []string, handSeed int64, ourPosition int, logger *log.Logger) HandResult {
	handRng := rand.New(rand.NewSource(handSeed))

	// Setup 6-max table with controlled RNG
	const STARTING_CHIPS = 200 // 100bb at $1/$2
	table := game.NewTable(handRng, game.TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
		Seed:       handSeed,
	})

	// Use default deck for now - the table's Rand will	control randomness

	// Add our bot at specified position
	ourBot := game.NewPlayer(ourPosition, "OurBot", game.AI, STARTING_CHIPS)
	table.AddPlayer(ourBot)

	// Add opponent bots to remaining positions
	for i := 1; i <= 6; i++ {
		if i != ourPosition {
			opponent := game.NewPlayer(i, fmt.Sprintf("Opp%d", i), game.AI, STARTING_CHIPS)
			table.AddPlayer(opponent)
		}
	}

	// Create agents with controlled RNG
	agents := make(map[string]game.Agent)
	agents["OurBot"] = bot.NewBot(handRng, logger)

	// Create opponent agents based on type
	typeIndex := 0
	for i := 1; i <= 6; i++ {
		if i != ourPosition {
			oppName := fmt.Sprintf("Opp%d", i)
			if opponentType == "mixed" {
				agents[oppName] = createOpponent(opponentMix[typeIndex], handRng, logger)
				typeIndex++
			} else {
				agents[oppName] = createOpponent(opponentType, handRng, logger)
			}
		}
	}

	// Create game engine and play hand (defaultAgent used as fallback only)
	defaultAgent := bot.NewBotWithRNG(logger, bot.DefaultBotConfig(), handRng)
	engine := game.NewGameEngine(table, defaultAgent, logger)

	// Record initial chips
	initialChips := ourBot.Chips
	startChips := make(map[string]int)
	for _, player := range table.GetActivePlayers() {
		startChips[player.Name] = player.Chips
	}

	// Play the hand
	engine.StartNewHand()
	handResult, err := engine.PlayHand(agents)
	if err != nil {
		logger.Error("Failed to play hand", "error", err, "seed", handSeed)
		// Return a losing result to continue simulation
		return HandResult{
			NetBB:          -1.0, // Fold/error = lose 1BB
			Seed:           handSeed,
			Position:       ourPosition,
			WentToShowdown: false,
			FinalPotSize:   2, // Just the blinds
			StreetReached:  "Pre-flop",
		}
	}

	// Calculate net BB for our bot
	finalChips := ourBot.Chips
	netChips := finalChips - initialChips
	netBB := float64(netChips) / float64(table.BigBlind())

	// Calculate final pot size from the largest chip delta (winner's gain)
	maxDelta := 0
	for _, player := range table.GetActivePlayers() {
		delta := player.Chips - startChips[player.Name]
		if delta > maxDelta {
			maxDelta = delta
		}
	}

	// Extract information from engine result if available
	wentToShowdown := false
	streetReached := "Pre-flop"
	if handResult != nil {
		wentToShowdown = (handResult.ShowdownType == "showdown")
		// Street reached can be inferred from the current round when hand ended
		if table.CurrentRound() == game.Flop {
			streetReached = "Flop"
		} else if table.CurrentRound() == game.Turn {
			streetReached = "Turn"
		} else if table.CurrentRound() == game.River {
			streetReached = "River"
		}
	}

	return HandResult{
		NetBB:          netBB,
		Seed:           handSeed,
		Position:       ourPosition,
		WentToShowdown: wentToShowdown,
		FinalPotSize:   maxDelta, // Winner's chip gain = pot size
		StreetReached:  streetReached,
	}
}

func playHandWithTimeout(opponentType string, opponentMix []string, handSeed int64, ourPosition int, timeout time.Duration, logger *log.Logger) (HandResult, error) {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Channel to receive the result
	resultCh := make(chan HandResult, 1)
	errorCh := make(chan error, 1)

	// Run playHand in a goroutine
	go func() {
		result := playHand(opponentType, opponentMix, handSeed, ourPosition, logger)
		resultCh <- result
	}()

	// Wait for either completion or timeout
	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errorCh:
		return HandResult{}, err
	case <-ctx.Done():
		return HandResult{}, fmt.Errorf("hand timed out after %v (seed: %d, position: %d)", timeout, handSeed, ourPosition)
	}
}

func createMixedOpponentTypes() []string {
	// Fixed realistic opponent mix for consistent testing
	return []string{"tag", "rand", "tag", "maniac", "call"}
}

func createOpponent(opponentType string, rng *rand.Rand, logger *log.Logger) game.Agent {
	switch opponentType {
	case "fold":
		return bot.NewFoldBot(logger)
	case "call":
		return bot.NewCallBot(logger)
	case "rand":
		return bot.NewRandBot(rng, logger)
	case "chart":
		return bot.NewChartBot(logger)
	case "maniac":
		return bot.NewManiacBot(rng, logger)
	case "tag":
		return bot.NewTAGBot(rng, logger)
	default:
		logger.Fatal("Unknown opponent type", "type", opponentType)
		return nil
	}
}

func printResults(stats *Statistics, opponentType string, duration time.Duration) {
	mean := stats.Mean()
	median := stats.Median()
	stdDev := stats.StdDev()
	stdErr := stats.StdError()
	low, high := stats.ConfidenceInterval95()
	p25 := stats.Percentile(0.25)
	p75 := stats.Percentile(0.75)
	p95 := stats.Percentile(0.95)
	p05 := stats.Percentile(0.05)

	// Performance metrics
	handsPerSec := float64(stats.Hands) / duration.Seconds()
	msPerHand := duration.Seconds() * 1000 / float64(stats.Hands)

	fmt.Printf("\n=== FINAL RESULTS vs %s-bot ===\n", opponentType)
	fmt.Printf("Hands played: %d\n", stats.Hands)
	fmt.Printf("Total time: %v\n", duration.Round(time.Millisecond))
	fmt.Printf("Performance: %.1f hands/sec, %.2fms/hand\n", handsPerSec, msPerHand)

	fmt.Printf("\n=== STATISTICAL RESULTS ===\n")
	fmt.Printf("Mean: %.4f bb/hand\n", mean)
	fmt.Printf("Median: %.4f bb/hand\n", median)
	fmt.Printf("Std Dev: %.4f bb\n", stdDev)
	fmt.Printf("Std Error: %.4f bb\n", stdErr)
	fmt.Printf("95%% CI: [%.4f, %.4f] bb/hand\n", low, high)
	fmt.Printf("Percentiles: P5=%.3f, P25=%.3f, P75=%.3f, P95=%.3f\n", p05, p25, p75, p95)

	// Sanity check - ledger must balance
	if math.Abs(stats.AllBB-stats.ShowdownBB-stats.NonShowdownBB) > 1e-6 {
		fmt.Printf("❌ LEDGER MISMATCH! AllBB: %.6f, ShowdownBB: %.6f, NonShowdownBB: %.6f\n",
			stats.AllBB, stats.ShowdownBB, stats.NonShowdownBB)
	}

	// Detailed analytics
	fmt.Printf("\n=== PROFIT SOURCE ANALYSIS ===\n")
	totalWins := stats.ShowdownWins + stats.NonShowdownWins
	if totalWins > 0 {
		showdownPct := float64(stats.ShowdownWins) / float64(totalWins) * 100
		nonShowdownPct := float64(stats.NonShowdownWins) / float64(totalWins) * 100
		fmt.Printf("Winning hands: %d showdown (%.1f%%), %d fold equity (%.1f%%)\n",
			stats.ShowdownWins, showdownPct, stats.NonShowdownWins, nonShowdownPct)
	}

	// Fixed reporting: mean per ALL hands, not just wins
	meanNSD := stats.NonShowdownBB / float64(stats.Hands)
	meanSD := stats.ShowdownBB / float64(stats.Hands)
	fmt.Printf("Non-showdown: %.2f bb/hand avg (all hands)\n", meanNSD)
	fmt.Printf("Showdown: %.2f bb/hand avg (all hands)\n", meanSD)
	fmt.Printf("Sanity check: %.2f + %.2f = %.2f (should equal %.2f)\n",
		meanNSD, meanSD, meanNSD+meanSD, mean)

	fmt.Printf("\n=== POT SIZE ANALYSIS ===\n")
	fmt.Printf("Max pot observed: %d chips (%.1f bb)\n", stats.MaxPotChips, stats.MaxPotBB)
	fmt.Printf("Stack cap check: %.1f bb / 600 bb max = %.1f%% of total chips\n",
		stats.MaxPotBB, stats.MaxPotBB/600.0*100)
	fmt.Printf("Big pots (>=50bb): %d hands (%.1f%%), %.2f bb total\n",
		stats.BigPots, float64(stats.BigPots)/float64(stats.Hands)*100, stats.BigPotsBB)

	fmt.Printf("\n=== POSITION ANALYSIS ===\n")
	for pos := 1; pos <= 6; pos++ {
		ps := stats.PositionResults[pos]
		if ps.Hands > 0 {
			posMean := ps.SumBB / float64(ps.Hands)
			fmt.Printf("Position %d: %d hands, %.3f bb/hand\n", pos, ps.Hands, posMean)
		}
	}

	if msPerHand > 2.0 {
		fmt.Printf("\n⚠️  Performance: %.2fms/hand (above 2ms target)\n", msPerHand)
	} else {
		fmt.Printf("\n✅ Performance: %.2fms/hand (meets <2ms target)\n", msPerHand)
	}

	// Phase 4 pass/fail gates
	fmt.Printf("\n=== PASS/FAIL GATES ===\n")
	switch opponentType {
	case "fold":
		passed := mean >= 1.0
		fmt.Printf("fold-bot gate: mean >= 1.0 bb/hand: %.4f %s\n",
			mean, passFailString(passed))
	case "call", "rand", "maniac":
		passed := (mean - 1.96*stdErr) > 0
		fmt.Printf("%s-bot gate: (mean - 1.96*se) > 0: %.4f %s\n",
			opponentType, mean-1.96*stdErr, passFailString(passed))
	case "chart", "tag", "mixed":
		passed := (mean - 1.96*stdErr) >= 0
		fmt.Printf("%s-bot gate: (mean - 1.96*se) >= 0: %.4f %s\n",
			opponentType, mean-1.96*stdErr, passFailString(passed))
	default:
		if strings.HasPrefix(opponentType, "mixed") {
			passed := (mean - 1.96*stdErr) >= 0
			fmt.Printf("%s-bot gate: (mean - 1.96*se) >= 0: %.4f %s\n",
				opponentType, mean-1.96*stdErr, passFailString(passed))
		} else {
			fmt.Printf("Unknown opponent type: %s\n", opponentType)
		}
	}
}

func passFailString(passed bool) string {
	if passed {
		return "✅ PASS"
	}
	return "❌ FAIL"
}
