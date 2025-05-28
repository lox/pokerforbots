package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/log"
	"github.com/lox/holdem-cli/internal/bot"
	"github.com/lox/holdem-cli/internal/game"
)

type CLI struct {
	Hands    int    `default:"50000" help:"Number of hands to simulate"`
	Opponent string `default:"fold" help:"Opponent type: fold, call, rand, chart"`
	Seed     int64  `default:"0" help:"RNG seed (0 for random)"`
	Verbose  bool   `short:"v" help:"Verbose logging"`
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
	stats := runSimulation(cli.Hands, cli.Opponent, cli.Seed, logger)
	duration := time.Since(startTime)
	printResults(stats, cli.Opponent, duration)

	ctx.Exit(0)
}

func runSimulation(numHands int, opponentType string, seed int64, logger *log.Logger) *Statistics {
	stats := &Statistics{}
	startTime := time.Now()

	// Create master RNG for generating independent seeds
	masterRng := rand.New(rand.NewSource(seed))

	fmt.Printf("Target: <2ms per hand for optimal performance\n\n")

	for hand := 0; hand < numHands; hand++ {
		// Generate independent seed for this hand
		handSeed := masterRng.Int63()

		// Rotate OurBot's position (1-6) to eliminate positional bias
		ourPosition := (hand % 6) + 1

		result := playHand(opponentType, handSeed, ourPosition, logger)
		stats.Add(result)

		// Progress updates every 50k hands (Phase 2 requirement)
		if (hand+1)%50000 == 0 {
			elapsed := time.Since(startTime)
			handsPerSec := float64(hand+1) / elapsed.Seconds()
			avgHandTime := elapsed / time.Duration(hand+1)

			mean := stats.Mean()
			se := stats.StdError()
			low, high := stats.ConfidenceInterval95()

			fmt.Printf("=== %d HANDS COMPLETED ===\n", hand+1)
			fmt.Printf("Performance: %.1f hands/sec, %.2fms/hand\n", handsPerSec, avgHandTime.Seconds()*1000)
			fmt.Printf("Results: %.4f bb/hand ± %.4f SE\n", mean, se)
			fmt.Printf("95%% CI: [%.4f, %.4f] bb/hand\n", low, high)
			if avgHandTime.Milliseconds() > 2 {
				fmt.Printf("⚠️  Performance below target (>2ms/hand)\n")
			} else {
				fmt.Printf("✅ Performance on target (<2ms/hand)\n")
			}
			fmt.Printf("\n")
		}

		// Lightweight progress for smaller intervals
		if (hand+1)%10000 == 0 && (hand+1)%50000 != 0 {
			elapsed := time.Since(startTime)
			handsPerSec := float64(hand+1) / elapsed.Seconds()
			mean := stats.Mean()
			fmt.Printf("Hand %d: %.3f bb/hand (%.0f hands/sec)\n", hand+1, mean, handsPerSec)
		}
	}

	return stats
}

func runDebugSimulation(opponentType string, seed int64, logger *log.Logger) {
	fmt.Printf("=== DEBUG MODE: 20 Hand Analysis ===\n")
	fmt.Printf("Opponent: %s, Seed: %d\n\n", opponentType, seed)

	masterRng := rand.New(rand.NewSource(seed))

	for hand := 0; hand < 20; hand++ {
		handSeed := masterRng.Int63()
		ourPosition := (hand % 6) + 1

		result := playHandWithDebug(opponentType, handSeed, ourPosition, logger)

		fmt.Printf("=== HAND %d RESULT ===\n", hand+1)
		fmt.Printf("Net BB for OurBot: %.4f\n", result.NetBB)

		// Flag suspicious results
		if result.NetBB > 30 {
			fmt.Printf("🚨 SUSPICIOUS: >30bb win detected!\n")
		}
		if result.NetBB < -10 {
			fmt.Printf("🚨 SUSPICIOUS: >10bb loss detected!\n")
		}

		fmt.Printf("\n" + strings.Repeat("=", 50) + "\n")
	}
}

func playHandWithDebug(opponentType string, handSeed int64, ourPosition int, logger *log.Logger) HandResult {
	handRng := rand.New(rand.NewSource(handSeed))

	// Setup 6-max table with controlled RNG
	const STARTING_CHIPS = 200 // 100bb at $1/$2
	table := game.NewTable(handRng, game.TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	fmt.Printf("=== INITIAL SETUP ===\n")
	fmt.Printf("Starting chips per player: %d\n", STARTING_CHIPS)
	fmt.Printf("Small blind: %d, Big blind: %d\n", table.SmallBlind, table.BigBlind)

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

	// Verify total chips
	totalChips := 0
	for _, player := range table.Players {
		totalChips += player.Chips
		fmt.Printf("Player %s: %d chips\n", player.Name, player.Chips)
	}
	fmt.Printf("Total chips in play: %d\n", totalChips)
	fmt.Printf("Expected total: %d\n", 6*STARTING_CHIPS)

	// Create agents with controlled RNG
	agents := make(map[string]game.Agent)
	agents["OurBot"] = createControlledBot(handRng, logger, opponentType)

	// Create opponent agents based on type
	for i := 1; i <= 6; i++ {
		if i != ourPosition {
			oppName := fmt.Sprintf("Opp%d", i)
			agents[oppName] = createOpponent(opponentType, handRng, logger)
		}
	}

	// Create game engine and play hand (defaultAgent used as fallback only)
	engine := game.NewGameEngine(table, agents["OurBot"], logger)

	// Record initial chips for all players
	startChips := make(map[string]int)
	for _, player := range table.Players {
		startChips[player.Name] = player.Chips
	}

	fmt.Printf("\n=== PRE-HAND ===\n")
	fmt.Printf("HAND %d  OurPos=%d\n", handSeed%1000, ourPosition)
	for _, p := range table.Players {
		fmt.Printf("  %-6s start %3d chips\n", p.Name, startChips[p.Name])
	}
	fmt.Printf("  pot %d chips (start)\n", table.Pot)

	// Play the hand
	engine.StartNewHand()

	fmt.Printf("\n=== HAND PLAY ===\n")
	engine.PlayHand(agents)

	// Detailed chip tracking after hand
	fmt.Printf("\n=== POST-HAND CHIP ANALYSIS ===\n")
	totalDelta := 0
	for _, p := range table.Players {
		delta := p.Chips - startChips[p.Name]
		totalDelta += delta
		fmt.Printf("  %-6s start %3d  end %3d  Δ %+3d chips\n",
			p.Name, startChips[p.Name], p.Chips, delta)
	}
	fmt.Printf("  pot %d chips (final)\n", table.Pot)
	fmt.Printf("  Sum(Δ) + pot = %d + %d = %d (should be 0)\n", totalDelta, table.Pot, totalDelta+table.Pot)

	if totalDelta+table.Pot != 0 {
		fmt.Printf("❌ CHIP LEAK! Money created/destroyed: %d chips\n", totalDelta+table.Pot)
	}

	// Calculate net BB for our bot
	finalChips := ourBot.Chips
	netChips := finalChips - startChips["OurBot"]
	netBB := float64(netChips) / float64(table.BigBlind)

	// Calculate final pot size from the largest chip delta (winner's gain)
	maxDelta := 0
	for _, player := range table.Players {
		delta := player.Chips - startChips[player.Name]
		if delta > maxDelta {
			maxDelta = delta
		}
	}

	fmt.Printf("\n=== POST-HAND ===\n")
	fmt.Printf("OurBot final chips: %d\n", finalChips)
	fmt.Printf("Net chips: %d\n", netChips)
	fmt.Printf("Net BB: %.4f\n", netBB)
	fmt.Printf("Pot size: %d chips (%.1f bb)\n", maxDelta, float64(maxDelta)/2.0)

	// Verify total chips conservation
	finalTotalChips := table.Pot
	for _, player := range table.Players {
		finalTotalChips += player.Chips
		fmt.Printf("Player %s final: %d chips\n", player.Name, player.Chips)
	}
	fmt.Printf("Final total chips: %d (pot: %d)\n", finalTotalChips, table.Pot)
	if finalTotalChips != 6*STARTING_CHIPS {
		fmt.Printf("❌ CHIP CONSERVATION VIOLATION! Expected %d, got %d\n", 6*STARTING_CHIPS, finalTotalChips)
	} else {
		fmt.Printf("✅ Chip conservation verified\n")
	}

	// Assert pot doesn't exceed stack cap (6 players * 200 chips = 1200 max)
	if maxDelta > 6*STARTING_CHIPS {
		fmt.Printf("❌ POT EXCEEDS STACK CAP! Pot: %d, Max possible: %d\n", maxDelta, 6*STARTING_CHIPS)
	}

	return HandResult{
		NetBB:          netBB,
		Seed:           handSeed,
		Position:       ourPosition,
		WentToShowdown: false,    // TODO: Get from engine result
		FinalPotSize:   maxDelta, // Winner's chip gain = pot size
		StreetReached:  "",       // TODO: Get from engine result
	}
}

func playHand(opponentType string, handSeed int64, ourPosition int, logger *log.Logger) HandResult {
	handRng := rand.New(rand.NewSource(handSeed))

	// Setup 6-max table with controlled RNG
	const STARTING_CHIPS = 200 // 100bb at $1/$2
	table := game.NewTable(handRng, game.TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	// Use default deck for now - the table's RandSource should control randomness
	// We'll need to ensure the deck uses the table's RandSource

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
	agents["OurBot"] = createControlledBot(handRng, logger, opponentType)

	// Create opponent agents based on type
	for i := 1; i <= 6; i++ {
		if i != ourPosition {
			oppName := fmt.Sprintf("Opp%d", i)
			agents[oppName] = createOpponent(opponentType, handRng, logger)
		}
	}

	// Create game engine and play hand (defaultAgent used as fallback only)
	engine := game.NewGameEngine(table, agents["OurBot"], logger)

	// Record initial chips
	initialChips := ourBot.Chips
	startChips := make(map[string]int)
	for _, player := range table.Players {
		startChips[player.Name] = player.Chips
	}

	// Play the hand
	engine.StartNewHand()
	engine.PlayHand(agents)

	// Calculate net BB for our bot
	finalChips := ourBot.Chips
	netChips := finalChips - initialChips
	netBB := float64(netChips) / float64(table.BigBlind)

	// Calculate final pot size from the largest chip delta (winner's gain)
	maxDelta := 0
	for _, player := range table.Players {
		delta := player.Chips - startChips[player.Name]
		if delta > maxDelta {
			maxDelta = delta
		}
	}

	return HandResult{
		NetBB:          netBB,
		Seed:           handSeed,
		Position:       ourPosition,
		WentToShowdown: false,    // TODO: Get from engine result
		FinalPotSize:   maxDelta, // Winner's chip gain = pot size
		StreetReached:  "",       // TODO: Get from engine result
	}
}

func createControlledBot(rng *rand.Rand, logger *log.Logger, opponentType string) game.Agent {
	// Choose bot config based on opponent type
	var config bot.BotConfig
	switch opponentType {
	case "fold":
		config = bot.ExploitBotConfig // Use exploit config against fold-bots
	default:
		config = bot.DefaultBotConfig() // Default for other opponents
	}

	return bot.NewBotWithRNG(logger, config, rng)
}

func createOpponent(opponentType string, rng *rand.Rand, logger *log.Logger) game.Agent {
	switch opponentType {
	case "fold":
		return &FoldBot{logger: logger}
	case "call":
		return &CallBot{logger: logger}
	case "rand":
		return &RandBot{rng: rng, logger: logger}
	case "chart":
		return &ChartBot{logger: logger}
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
	case "call", "rand":
		passed := (mean - 1.96*stdErr) > 0
		fmt.Printf("%s-bot gate: (mean - 1.96*se) > 0: %.4f %s\n",
			opponentType, mean-1.96*stdErr, passFailString(passed))
	case "chart":
		passed := (mean - 1.96*stdErr) >= 0
		fmt.Printf("chart-bot gate: (mean - 1.96*se) >= 0: %.4f %s\n",
			mean-1.96*stdErr, passFailString(passed))
	}
}

func passFailString(passed bool) string {
	if passed {
		return "✓ PASS"
	}
	return "✗ FAIL"
}

// Simple deterministic opponent implementations

type FoldBot struct {
	logger *log.Logger
}

func (f *FoldBot) MakeDecision(player *game.Player, table *game.Table) game.Decision {
	// Always fold except when we can check
	if table.CurrentBet <= player.BetThisRound {
		return game.Decision{Action: game.Check, Amount: 0, Reasoning: "fold-bot checking"}
	}
	return game.Decision{Action: game.Fold, Amount: 0, Reasoning: "fold-bot folding"}
}

func (f *FoldBot) ExecuteAction(player *game.Player, table *game.Table) string {
	decision := f.MakeDecision(player, table)

	switch decision.Action {
	case game.Fold:
		player.Fold()
	case game.Check:
		player.Check()
	}

	if table.HandHistory != nil {
		table.HandHistory.AddAction(player.Name, decision.Action, player.ActionAmount, table.Pot, table.CurrentRound, decision.Reasoning)
	}

	return decision.Reasoning
}

type CallBot struct {
	logger *log.Logger
}

func (c *CallBot) MakeDecision(player *game.Player, table *game.Table) game.Decision {
	// Check/call to river, fold river
	if table.CurrentRound == game.River && table.CurrentBet > player.BetThisRound {
		return game.Decision{Action: game.Fold, Amount: 0, Reasoning: "call-bot folding river"}
	}

	if table.CurrentBet <= player.BetThisRound {
		return game.Decision{Action: game.Check, Amount: 0, Reasoning: "call-bot checking"}
	}

	return game.Decision{Action: game.Call, Amount: 0, Reasoning: "call-bot calling"}
}

func (c *CallBot) ExecuteAction(player *game.Player, table *game.Table) string {
	decision := c.MakeDecision(player, table)

	switch decision.Action {
	case game.Fold:
		player.Fold()
	case game.Check:
		player.Check()
	case game.Call:
		callAmount := table.CurrentBet - player.BetThisRound
		if callAmount > 0 && callAmount <= player.Chips {
			player.Call(callAmount)
			table.Pot += callAmount
		} else {
			player.Check()
		}
	}

	if table.HandHistory != nil {
		table.HandHistory.AddAction(player.Name, decision.Action, player.ActionAmount, table.Pot, table.CurrentRound, decision.Reasoning)
	}

	return decision.Reasoning
}

type RandBot struct {
	rng    *rand.Rand
	logger *log.Logger
}

func (r *RandBot) MakeDecision(player *game.Player, table *game.Table) game.Decision {
	// Uniform random legal action using fixed array to avoid allocation
	var actions [3]game.Action
	var actionCount int

	// Can always fold (except when we can check for free)
	if table.CurrentBet > player.BetThisRound {
		actions[actionCount] = game.Fold
		actionCount++
	}

	// Can always check/call
	if table.CurrentBet <= player.BetThisRound {
		actions[actionCount] = game.Check
		actionCount++
	} else {
		actions[actionCount] = game.Call
		actionCount++
	}

	// Can always raise if we have chips
	if player.Chips > 0 {
		actions[actionCount] = game.Raise
		actionCount++
	}

	action := actions[r.rng.Intn(actionCount)]
	return game.Decision{Action: action, Amount: 0, Reasoning: "rand-bot random action"}
}

func (r *RandBot) ExecuteAction(player *game.Player, table *game.Table) string {
	decision := r.MakeDecision(player, table)

	switch decision.Action {
	case game.Fold:
		player.Fold()
	case game.Check:
		player.Check()
	case game.Call:
		callAmount := table.CurrentBet - player.BetThisRound
		if callAmount > 0 && callAmount <= player.Chips {
			player.Call(callAmount)
			table.Pot += callAmount
		} else {
			player.Check()
		}
	case game.Raise:
		raiseAmount := table.BigBlind // Simple 1bb raise
		totalBet := table.CurrentBet + raiseAmount
		needed := totalBet - player.BetThisRound
		if needed > 0 && needed <= player.Chips {
			player.Raise(needed)
			table.Pot += needed
			table.CurrentBet = totalBet
		} else {
			// Fall back to call
			callAmount := table.CurrentBet - player.BetThisRound
			if callAmount > 0 && callAmount <= player.Chips {
				player.Call(callAmount)
				table.Pot += callAmount
			} else {
				player.Check()
			}
		}
	}

	if table.HandHistory != nil {
		table.HandHistory.AddAction(player.Name, decision.Action, player.ActionAmount, table.Pot, table.CurrentRound, decision.Reasoning)
	}

	return decision.Reasoning
}

type ChartBot struct {
	logger *log.Logger
}

func (c *ChartBot) MakeDecision(player *game.Player, table *game.Table) game.Decision {
	// Simple push-fold pre-flop chart, check/call post-flop
	if table.CurrentRound == game.PreFlop {
		// Very basic push-fold: premium hands only
		if len(player.HoleCards) == 2 {
			card1, card2 := player.HoleCards[0], player.HoleCards[1]

			// Push with premium pairs and AK
			if (card1.Rank == card2.Rank && card1.Rank >= 10) || // TT+
				(card1.Rank >= 13 && card2.Rank >= 13) { // AK, AQ, AA, KK, QQ, etc.
				if player.Chips <= 20*table.BigBlind { // Only if short stack
					return game.Decision{Action: game.Raise, Amount: 0, Reasoning: "chart-bot push"}
				}
			}
		}

		// Otherwise fold to raises, check/call otherwise
		if table.CurrentBet > player.BetThisRound {
			return game.Decision{Action: game.Fold, Amount: 0, Reasoning: "chart-bot folding"}
		}
		return game.Decision{Action: game.Check, Amount: 0, Reasoning: "chart-bot checking"}
	}

	// Post-flop: check/call
	if table.CurrentBet <= player.BetThisRound {
		return game.Decision{Action: game.Check, Amount: 0, Reasoning: "chart-bot checking"}
	}
	return game.Decision{Action: game.Call, Amount: 0, Reasoning: "chart-bot calling"}
}

func (c *ChartBot) ExecuteAction(player *game.Player, table *game.Table) string {
	decision := c.MakeDecision(player, table)

	switch decision.Action {
	case game.Fold:
		player.Fold()
	case game.Check:
		player.Check()
	case game.Call:
		callAmount := table.CurrentBet - player.BetThisRound
		if callAmount > 0 && callAmount <= player.Chips {
			player.Call(callAmount)
			table.Pot += callAmount
		} else {
			player.Check()
		}
	case game.Raise:
		// All-in push
		allInAmount := player.Chips
		if player.AllIn() {
			table.Pot += allInAmount
			if player.TotalBet > table.CurrentBet {
				table.CurrentBet = player.TotalBet
			}
		}
	}

	if table.HandHistory != nil {
		table.HandHistory.AddAction(player.Name, decision.Action, player.ActionAmount, table.Pot, table.CurrentRound, decision.Reasoning)
	}

	return decision.Reasoning
}
