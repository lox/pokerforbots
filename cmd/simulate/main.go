package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/log"
	"github.com/lox/pokerforbots/internal/simulator"
	"github.com/lox/pokerforbots/internal/statistics"
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
			_ = f.Close()
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

	fmt.Printf("Starting simulation: %d hands (%d total hands) vs %s-bot (seed: %d)\n",
		cli.Hands, cli.Hands*6, cli.Opponent, cli.Seed)

	startTime := time.Now()
	stats, opponentInfo, err := simulator.RunSimulation(cli.Hands, cli.Opponent, cli.Seed, cli.Timeout, logger)
	if err != nil {
		fmt.Printf("\n❌ SIMULATION FAILED!\n")
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	duration := time.Since(startTime)

	// Print simulation summary
	simulator.PrintSummary(stats, opponentInfo)

	// Print performance metrics and pass/fail gates
	printPerformanceAndGates(stats, opponentInfo, duration)

	// Stop CPU profiling before exit
	if cpuFile != nil {
		pprof.StopCPUProfile()
		_ = cpuFile.Close()
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
			_ = f.Close()
			os.Exit(1)
		}
		_ = f.Close()
		fmt.Printf("Memory profile written to %s\n", cli.MemProfile)
	}

	ctx.Exit(0)
}

func printPerformanceAndGates(stats *statistics.Statistics, opponentType string, duration time.Duration) {
	mean := stats.Mean()
	stdErr := stats.StdError()

	// Performance metrics
	handsPerSec := float64(stats.Hands) / duration.Seconds()
	msPerHand := duration.Seconds() * 1000 / float64(stats.Hands)

	fmt.Printf("Total time: %v\n", duration.Round(time.Millisecond))
	fmt.Printf("Performance: %.1f hands/sec, %.2fms/hand\n", handsPerSec, msPerHand)

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
