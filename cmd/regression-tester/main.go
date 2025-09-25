package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/lox/pokerforbots/internal/regression"
	"github.com/rs/zerolog"
)

type CLI struct {
	// Test modes
	Mode string `kong:"default='heads-up',enum='heads-up,population,npc-benchmark,self-play,all',help='Test mode to run'"`

	// Bot binaries - unified approach
	Challenger string `kong:"help='Challenger bot binary path (all modes)'"`
	Baseline   string `kong:"help='Baseline bot binary path (all modes except self-play)'"`

	// Test configuration
	Hands         int    `kong:"default='10000',help='Total hands to play'"`
	BatchSize     int    `kong:"default='10000',help='Hands per batch'"`
	Seeds         string `kong:"default='42',help='Comma-separated list of seeds'"`
	StartingChips int    `kong:"default='1000',help='Starting chips in big blinds'"`

	// Table configuration
	ChallengerSeats int    `kong:"default='2',help='Number of challenger seats'"`
	BaselineSeats   int    `kong:"default='4',help='Number of baseline seats'"`
	NPCs            string `kong:"name='npcs',help='NPC configuration (e.g., aggressive:2,callbot:1,random:1)'"`

	// Statistical options
	InfiniteBankroll       bool    `kong:"help='Use infinite bankroll for pure performance measurement'"`
	SignificanceLevel      float64 `kong:"default='0.05',help='Statistical significance level'"`
	EffectSizeThreshold    float64 `kong:"default='0.2',help='Minimum effect size for practical significance'"`
	MultipleTestCorrection bool    `kong:"help='Apply Bonferroni correction for multiple comparisons'"`
	EarlyStopping          bool    `kong:"help='Enable early stopping when significance reached'"`
	MinHands               int     `kong:"default='10000',help='Minimum hands before early stopping'"`
	MaxHands               int     `kong:"default='100000',help='Maximum hands for early stopping'"`
	CheckInterval          int     `kong:"default='5000',help='Check interval for early stopping'"`
	StdDevClampMin         float64 `kong:"default='5',help='Minimum per-batch standard deviation (BB/100) before clamping'"`
	StdDevClampFallback    float64 `kong:"default='50',help='Fallback standard deviation (BB/100) applied after clamping'"`
	WarnStdDevClamp        bool    `kong:"help='Emit warnings when standard deviation clamping occurs'"`
	LatencyTracking        bool    `kong:"default='true',help='Collect per-action latency metrics'"`
	LatencyWarnMs          float64 `kong:"default='100',help='Warn when p95 response time exceeds this many milliseconds'"`

	// Server configuration
	ServerAddr string `kong:"default='localhost:8080',help='Poker server address'"`
	ServerCmd  string `kong:"default='go run ./cmd/server',help='Command to run the poker server'"`
	TimeoutMs  int    `kong:"default='100',help='Bot decision timeout in milliseconds'"`

	// Health monitoring
	MaxCrashes     int `kong:"default='3',help='Maximum crashes per bot before giving up'"`
	MaxTimeouts    int `kong:"default='10',help='Maximum timeouts per bot'"`
	RestartDelayMs int `kong:"default='100',help='Bot restart delay in milliseconds'"`

	// Output options
	Output     string `kong:"default='both',enum='json,summary,both',help='Output format'"`
	OutputFile string `kong:"help='Output file path (default: stdout)'"`
	Verbose    bool   `kong:"help='Enable verbose output'"`
	Debug      bool   `kong:"help='Enable debug logging'"`
	Quiet      bool   `kong:"help='Quiet mode - show progress dots instead of logs'"`

	// Special commands
	ValidateBinaries bool `kong:"help='Validate bot binaries and exit'"`
}

func main() {
	var cli CLI
	ctx := kong.Parse(&cli,
		kong.Name("regression-tester"),
		kong.Description("Regression testing framework for poker bot snapshots"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
		}),
	)

	// Configure logging
	var logger zerolog.Logger
	if cli.Quiet {
		// In quiet mode, disable most logging
		logger = zerolog.New(zerolog.ConsoleWriter{Out: io.Discard}).
			Level(zerolog.ErrorLevel).
			With().
			Timestamp().
			Logger()
	} else {
		level := zerolog.InfoLevel
		if cli.Debug {
			level = zerolog.DebugLevel
		}
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
			Level(level).
			With().
			Timestamp().
			Logger()
	}

	// Parse seeds
	var seeds []int64
	seedParts := strings.SplitSeq(cli.Seeds, ",")
	for s := range seedParts {
		s = strings.TrimSpace(s)
		if s != "" {
			seed, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				ctx.Fatalf("Invalid seed value '%s': %v", s, err)
			}
			seeds = append(seeds, seed)
		}
	}

	// Parse NPCs configuration
	npcs := make(map[string]int)
	if cli.NPCs != "" {
		npcParts := strings.SplitSeq(cli.NPCs, ",")
		for npc := range npcParts {
			parts := strings.Split(npc, ":")
			if len(parts) != 2 {
				ctx.Fatalf("Invalid NPC format '%s': expected 'type:count'", npc)
			}

			count, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				ctx.Fatalf("Invalid NPC count in '%s': %v", npc, err)
			}
			if count <= 0 {
				ctx.Fatalf("NPC count must be positive in '%s'", npc)
			}

			npcType := strings.TrimSpace(parts[0])
			if npcType == "" {
				ctx.Fatalf("Empty NPC type in '%s'", npc)
			}
			npcs[npcType] = count
		}
	}

	// Build config from CLI
	config := &regression.Config{
		Mode: regression.TestMode(cli.Mode),

		// Bot binaries - unified
		Challenger: cli.Challenger,
		Baseline:   cli.Baseline,

		// Test configuration
		HandsTotal:    cli.Hands,
		BatchSize:     cli.BatchSize,
		Seeds:         seeds,
		StartingChips: cli.StartingChips,

		// Table configuration
		ChallengerSeats: cli.ChallengerSeats,
		BaselineSeats:   cli.BaselineSeats,
		NPCs:            npcs,

		// Statistical
		InfiniteBankroll:          cli.InfiniteBankroll,
		SignificanceLevel:         cli.SignificanceLevel,
		EffectSizeThreshold:       cli.EffectSizeThreshold,
		MultipleTestCorrection:    cli.MultipleTestCorrection,
		EarlyStopping:             cli.EarlyStopping,
		MinHands:                  cli.MinHands,
		MaxHands:                  cli.MaxHands,
		CheckInterval:             cli.CheckInterval,
		StdDevClampMin:            cli.StdDevClampMin,
		StdDevClampFallback:       cli.StdDevClampFallback,
		WarnOnStdDevClamp:         cli.WarnStdDevClamp,
		EnableLatencyTracking:     cli.LatencyTracking,
		LatencyWarningThresholdMs: cli.LatencyWarnMs,

		// Server
		ServerAddr: cli.ServerAddr,
		ServerCmd:  cli.ServerCmd,
		TimeoutMs:  cli.TimeoutMs,

		// Health
		MaxCrashesPerBot:  cli.MaxCrashes,
		MaxTimeoutsPerBot: cli.MaxTimeouts,
		RestartDelayMs:    cli.RestartDelayMs,

		// Output
		OutputFormat: cli.Output,
		OutputFile:   cli.OutputFile,
		Verbose:      cli.Verbose,

		// Special commands
		ValidateOnly: cli.ValidateBinaries,

		Logger: logger,
	}

	// Apply statistics clamp configuration before running tests
	regression.ConfigureStatisticsClamp(regression.StatisticsClampConfig{
		MinStdDevBB100:      config.StdDevClampMin,
		FallbackStdDevBB100: config.StdDevClampFallback,
		WarnOnClamp:         config.WarnOnStdDevClamp,
		Logger:              &config.Logger,
	})

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create runner with options
	var opts []regression.RunnerOption
	var simpleMonitor *SimpleProgressMonitor

	if cli.Quiet {
		// Calculate total batches
		totalBatches := (cli.Hands + cli.BatchSize - 1) / cli.BatchSize

		// Use simple unified progress monitor
		simpleMonitor = NewSimpleProgressMonitor(totalBatches)
		opts = append(opts, regression.WithHandMonitor(simpleMonitor))
		opts = append(opts, regression.WithProgressReporter(simpleMonitor))
	}

	runner := regression.NewRunner(config, opts...)

	// Handle special commands
	if config.ValidateOnly {
		if err := runner.ValidateBinaries(); err != nil {
			ctx.Fatalf("Binary validation failed: %v", err)
		}
		fmt.Println("All binaries validated successfully")
		return
	}

	// Run tests with signal handling
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-sigChan
		logger.Info().Msg("Received interrupt signal, shutting down...")
		cancel()
	}()

	// Run the regression tests
	if err := runner.Run(runCtx); err != nil {
		ctx.Fatalf("Test failed: %v", err)
	}

	// Print final summary if in quiet mode
	if simpleMonitor != nil {
		simpleMonitor.PrintSummary(cli.Hands)
	}
}
