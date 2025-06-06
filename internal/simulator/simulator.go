package simulator

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/lox/pokerforbots/internal/bot"
	"github.com/lox/pokerforbots/internal/game"
	"github.com/lox/pokerforbots/internal/statistics"
)

// Config holds configuration for running simulations
type Config struct {
	Hands        int
	OpponentType string
	Seed         int64
	Timeout      time.Duration
	Logger       *log.Logger
}

// Simulator runs poker hand simulations
type Simulator struct {
	config Config
}

// New creates a new simulator with the given configuration
func New(config Config) *Simulator {
	return &Simulator{config: config}
}

// Run executes the simulation and returns results
func (s *Simulator) Run() (*statistics.Statistics, string, error) {
	stats := &statistics.Statistics{}

	// Determine opponent info string
	opponentInfo := s.config.OpponentType
	var opponentMix []string
	if s.config.OpponentType == "mixed" {
		opponentMix = createMixedOpponentTypes()
		opponentInfo = fmt.Sprintf("mixed(%s)", strings.Join(opponentMix, ","))
	}

	for hand := 0; hand < s.config.Hands; hand++ {
		// Generate independent seed for this hand
		handSeed := s.config.Seed + int64(hand)

		// Rotate OurBot's position (1-6) to eliminate positional bias
		ourPosition := (hand % 6) + 1

		// Play hand as usual
		result1, err1 := s.playHandWithTimeout(s.config.OpponentType, opponentMix, handSeed, ourPosition)
		if err1 != nil {
			return nil, "", fmt.Errorf("hang detected on hand %d: %w", hand+1, err1)
		}

		// Play duplicate hand with swapped seat (choose a different seat)
		// For simplicity, swap with seat 1 if not already, else seat 2
		var swappedPosition int
		if ourPosition != 1 {
			swappedPosition = 1
		} else {
			swappedPosition = 2
		}
		result2, err2 := s.playHandWithTimeout(s.config.OpponentType, opponentMix, handSeed, swappedPosition)
		if err2 != nil {
			return nil, "", fmt.Errorf("hang detected on duplicate hand %d: %w", hand+1, err2)
		}

		// Add both results to statistics
		stats.Add(result1)
		stats.Add(result2)
	}

	// Validate statistics before returning
	if err := stats.Validate(); err != nil {
		return nil, "", fmt.Errorf("statistics validation failed: %w", err)
	}

	return stats, opponentInfo, nil
}

// playHandWithTimeout runs a single hand with timeout protection
func (s *Simulator) playHandWithTimeout(opponentType string, opponentMix []string, handSeed int64, ourPosition int) (statistics.HandResult, error) {
	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), s.config.Timeout)
	defer cancel()

	// Channel to receive the result
	resultCh := make(chan statistics.HandResult, 1)
	errorCh := make(chan error, 1)

	// Run playHand in a goroutine
	go func() {
		result := s.playHand(opponentType, opponentMix, handSeed, ourPosition)
		resultCh <- result
	}()

	// Wait for either completion or timeout
	select {
	case result := <-resultCh:
		return result, nil
	case err := <-errorCh:
		return statistics.HandResult{}, err
	case <-ctx.Done():
		return statistics.HandResult{}, fmt.Errorf("hand timed out after %v (seed: %d, position: %d)", s.config.Timeout, handSeed, ourPosition)
	}
}

// playHand simulates a single poker hand
func (s *Simulator) playHand(opponentType string, opponentMix []string, handSeed int64, ourPosition int) statistics.HandResult {
	handRng := rand.New(rand.NewSource(handSeed))

	// Setup 6-max table with controlled RNG
	const STARTING_CHIPS = 200 // 100bb at $1/$2
	table := game.NewTable(handRng, game.TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
		Seed:       handSeed,
	})

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
	agents["OurBot"] = bot.NewBot(handRng, s.config.Logger)

	// Create opponent agents based on type
	typeIndex := 0
	for i := 1; i <= 6; i++ {
		if i != ourPosition {
			oppName := fmt.Sprintf("Opp%d", i)
			if opponentType == "mixed" {
				agents[oppName] = createOpponent(opponentMix[typeIndex], handRng, s.config.Logger)
				typeIndex++
			} else {
				agents[oppName] = createOpponent(opponentType, handRng, s.config.Logger)
			}
		}
	}

	// Create game engine and play hand
	engine := game.NewGameEngine(table, s.config.Logger)

	// Add agents to table
	for playerName, agent := range agents {
		engine.AddAgent(playerName, agent)
	}

	// Record initial chips
	initialChips := ourBot.Chips
	startChips := make(map[string]int)
	for _, player := range table.GetActivePlayers() {
		startChips[player.Name] = player.Chips
	}

	// Play the hand
	engine.StartNewHand()
	handResult, err := engine.PlayHand()
	if err != nil {
		s.config.Logger.Error("Failed to play hand", "error", err, "seed", handSeed)
		// Return a losing result to continue simulation
		return statistics.HandResult{
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

	return statistics.HandResult{
		NetBB:          netBB,
		Seed:           handSeed,
		Position:       ourPosition,
		WentToShowdown: wentToShowdown,
		FinalPotSize:   maxDelta, // Winner's chip gain = pot size
		StreetReached:  streetReached,
	}
}

// createMixedOpponentTypes returns a fixed mix of opponent types for consistent testing
func createMixedOpponentTypes() []string {
	// Fixed realistic opponent mix for consistent testing
	return []string{"tag", "rand", "tag", "maniac", "call"}
}

// createOpponent creates an AI opponent of the specified type
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

// RunSimulation is a convenience function for running a simulation with basic parameters
func RunSimulation(numHands int, opponentType string, seed int64, timeout time.Duration, logger *log.Logger) (*statistics.Statistics, string, error) {
	config := Config{
		Hands:        numHands,
		OpponentType: opponentType,
		Seed:         seed,
		Timeout:      timeout,
		Logger:       logger,
	}

	simulator := New(config)
	return simulator.Run()
}

// PrintSummary prints a comprehensive summary of simulation results
func PrintSummary(stats *statistics.Statistics, opponentType string) {
	mean := stats.Mean()
	median := stats.Median()
	stdDev := stats.StdDev()
	stdErr := stats.StdError()
	low, high := stats.ConfidenceInterval95()
	p25 := stats.Percentile(0.25)
	p75 := stats.Percentile(0.75)
	p95 := stats.Percentile(0.95)
	p05 := stats.Percentile(0.05)

	fmt.Printf("\n=== FINAL RESULTS vs %s-bot ===\n", opponentType)
	fmt.Printf("Hands played: %d\n", stats.Hands)

	fmt.Printf("\n=== STATISTICAL RESULTS ===\n")
	fmt.Printf("Mean: %.4f bb/hand\n", mean)
	fmt.Printf("Median: %.4f bb/hand\n", median)
	fmt.Printf("Std Dev: %.4f bb\n", stdDev)
	fmt.Printf("Std Error: %.4f bb\n", stdErr)
	fmt.Printf("95%% CI: [%.4f, %.4f] bb/hand\n", low, high)
	fmt.Printf("Percentiles: P5=%.3f, P25=%.3f, P75=%.3f, P95=%.3f\n", p05, p25, p75, p95)

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
			posMean := stats.PositionMean(pos)
			fmt.Printf("Position %d: %d hands, %.3f bb/hand\n", pos, ps.Hands, posMean)
		}
	}
}
