package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/lox/pokerforbots/internal/regression"
	"github.com/rs/zerolog"
)

type CLI struct {
	// Test modes
	Mode string `kong:"default='heads-up',enum='heads-up,population,npc-benchmark,self-play,all',help='Test mode to run'"`

	// Bot binaries
	BotA       string `kong:"help='Bot A binary path (heads-up mode)'"`
	BotB       string `kong:"help='Bot B binary path (heads-up mode)'"`
	Challenger string `kong:"help='Challenger bot binary path (population mode)'"`
	Baseline   string `kong:"help='Baseline bot binary path (population mode)'"`
	Bot        string `kong:"help='Bot binary path (self-play/npc modes)'"`

	// Test configuration
	Hands         int    `kong:"default='10000',help='Total hands to play'"`
	BatchSize     int    `kong:"default='10000',help='Hands per batch'"`
	Seeds         string `kong:"default='42',help='Comma-separated list of seeds'"`
	StartingChips int    `kong:"default='1000',help='Starting chips in big blinds'"`

	// Table configuration
	ChallengerSeats int    `kong:"default='2',help='Number of challenger seats (population mode)'"`
	BaselineSeats   int    `kong:"default='4',help='Number of baseline seats (population mode)'"`
	BotSeats        int    `kong:"default='2',help='Number of bot seats (npc mode)'"`
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

	// Special commands
	ValidateBinaries bool    `kong:"help='Validate bot binaries and exit'"`
	PowerAnalysis    bool    `kong:"help='Run power analysis and exit'"`
	EffectSize       float64 `kong:"default='0.2',help='Effect size for power analysis'"`
	Power            float64 `kong:"default='0.8',help='Desired statistical power'"`
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
	level := zerolog.InfoLevel
	if cli.Debug {
		level = zerolog.DebugLevel
	}
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
		Level(level).
		With().
		Timestamp().
		Logger()

	// Parse seeds
	var seeds []int64
	for s := range strings.SplitSeq(cli.Seeds, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			var seed int64
			fmt.Sscanf(s, "%d", &seed)
			seeds = append(seeds, seed)
		}
	}

	// Parse NPCs configuration
	npcs := make(map[string]int)
	if cli.NPCs != "" {
		for npc := range strings.SplitSeq(cli.NPCs, ",") {
			parts := strings.Split(npc, ":")
			if len(parts) == 2 {
				var count int
				fmt.Sscanf(parts[1], "%d", &count)
				npcs[strings.TrimSpace(parts[0])] = count
			}
		}
	}

	// Build config from CLI
	config := &regression.Config{
		Mode: regression.TestMode(cli.Mode),

		// Bot binaries
		BotA:       cli.BotA,
		BotB:       cli.BotB,
		Challenger: cli.Challenger,
		Baseline:   cli.Baseline,
		Bot:        cli.Bot,

		// Test configuration
		HandsTotal:    cli.Hands,
		BatchSize:     cli.BatchSize,
		Seeds:         seeds,
		StartingChips: cli.StartingChips,

		// Table configuration
		ChallengerSeats: cli.ChallengerSeats,
		BaselineSeats:   cli.BaselineSeats,
		BotSeats:        cli.BotSeats,
		NPCs:            npcs,

		// Statistical
		InfiniteBankroll:       cli.InfiniteBankroll,
		SignificanceLevel:      cli.SignificanceLevel,
		EffectSizeThreshold:    cli.EffectSizeThreshold,
		MultipleTestCorrection: cli.MultipleTestCorrection,
		EarlyStopping:          cli.EarlyStopping,
		MinHands:               cli.MinHands,
		MaxHands:               cli.MaxHands,
		CheckInterval:          cli.CheckInterval,

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

		// Power analysis
		PowerAnalysis: cli.PowerAnalysis,
		EffectSize:    cli.EffectSize,
		Power:         cli.Power,

		// Special commands
		ValidateOnly: cli.ValidateBinaries,

		Logger: logger,
	}

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create runner
	runner := regression.NewRunner(config)

	// Handle special commands
	if config.ValidateOnly {
		if err := runner.ValidateBinaries(); err != nil {
			ctx.Fatalf("Binary validation failed: %v", err)
		}
		fmt.Println("All binaries validated successfully")
		return
	}

	if config.PowerAnalysis {
		runner.RunPowerAnalysis(func(format string, args ...any) {
			fmt.Printf(format, args...)
		})
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
}
