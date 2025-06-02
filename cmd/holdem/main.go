package main

import (
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/log"

	"github.com/lox/holdem-cli/internal/bot"
	"github.com/lox/holdem-cli/internal/game"
	"github.com/lox/holdem-cli/internal/tui"
)

type CLI struct {
	Players        int    `short:"p" help:"Number of players at the table (6 or 9)" default:"6"`
	LogLevel       string `help:"Set the log-level" enum:"debug,info,warn,error" default:"info"`
	LogFile        string `help:"The logfile to write logs to" default:"holdem.log"`
	HandHistoryDir string `help:"Directory to save hand history files" default:"handhistory"`
	Seed           *int64 `help:"The seed for the random number generator"`
}

func main() {
	var cli CLI
	ctx := kong.Parse(&cli)

	// Set up logging
	logger, closer, err := createLogger(cli.LogFile, cli.LogLevel)
	if err != nil {
		log.Error("Error creating logger: %v", err)
		ctx.Exit(1)
	}
	defer func() {
		if err := closer; err != nil {
			log.Error("Failed to close debug file", "error", err)
		}
	}()

	// Create separate TUI logger
	tuiLogger, tuiCloser, err := createLogger("holdem-tui.log", cli.LogLevel)
	if err != nil {
		log.Error("Error creating TUI logger: %v", err)
		ctx.Exit(1)
	}
	defer func() {
		if err := tuiCloser; err != nil {
			log.Error("Failed to close TUI debug file", "error", err)
		}
	}()

	if cli.Players != 6 && cli.Players != 9 {
		log.Fatal("Invalid number of players. Must be 6 or 9.")
	}

	var seed int64
	seed = time.Now().UnixNano()
	if cli.Seed != nil {
		seed = *cli.Seed
	}
	randSource := rand.NewSource(seed)

	// Start interactive game
	err = startInteractiveGame(rand.New(randSource), seed, cli.Players, cli.HandHistoryDir, logger, tuiLogger)
	if err != nil {
		log.Fatal("Failed to start game", "error", err)
	}

	ctx.Exit(0)
}

func createLogger(logFile string, level string) (*log.Logger, func() error, error) {
	nilCloser := func() error { return nil }

	parsedLevel, err := log.ParseLevel(level)
	if err != nil {
		return nil, nilCloser, fmt.Errorf("error parsing level %s: %w", level, err)
	}

	// Set up debug logging
	debugFile, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return nil, nilCloser, fmt.Errorf("failed to create debug log: %w", err)
	}

	// Determine prefix based on log file
	prefix := "main"
	if logFile == "holdem-tui.log" {
		prefix = "tui"
	}

	// Create logger with specified level
	logger := log.NewWithOptions(debugFile, log.Options{
		ReportTimestamp: true,
		Prefix:          prefix,
		TimeFormat:      "15:04:05",
		Level:           parsedLevel,
	})

	return logger, debugFile.Close, nil
}

func startInteractiveGame(rng *rand.Rand, seed int64, seats int, handHistoryDir string, logger *log.Logger, tuiLogger *log.Logger) error {
	// Create hand history writer
	handHistoryWriter := game.NewFileHandHistoryWriter(handHistoryDir)

	// Create table
	table := game.NewTable(rng, game.TableConfig{
		MaxSeats:          seats,
		SmallBlind:        1,
		BigBlind:          2,
		Seed:              seed,
		HandHistoryWriter: handHistoryWriter,
	})

	agents := make(map[string]game.Agent)

	// Add human player
	human := game.NewPlayer(1, "You", game.Human, 200)
	table.AddPlayer(human)

	// Add AI players
	for i := 2; i <= seats; i++ {
		aiPlayer := game.NewPlayer(i, fmt.Sprintf("AI-%d", i), game.AI, 200)
		table.AddPlayer(aiPlayer)
		// Create AI agents
		agents[aiPlayer.Name] = bot.NewBotWithRNG(logger, bot.DefaultBotConfig(), rng)
	}

	// Create human interface with separate TUI logger
	hi, err := tui.NewTUIAgent(table, tuiLogger)
	if err != nil {
		return fmt.Errorf("failed to create interface: %w", err)
	}
	err = hi.Start()
	if err != nil {
		return fmt.Errorf("failed to start TUI: %w", err)
	}

	// Set up signal handling for graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		if err := hi.Close(); err != nil {
			log.Error("Failed to close interface", "error", err)
		}
		os.Exit(0)
	}()

	defer func() {
		if err := hi.Close(); err != nil {
			log.Error("Failed to close interface", "error", err)
		}
	}()

	// Use TUI interface directly as the human agent
	agents["You"] = hi

	// Create game engine
	defaultAgent := bot.NewBotWithRNG(logger, bot.DefaultBotConfig(), rng)
	engine := game.NewGameEngine(table, defaultAgent, logger)

	// Subscribe TUI to game events
	eventBus := engine.GetEventBus()
	eventBus.Subscribe(hi) // TUI displays events

	// Main game loop - much simpler!
	for {
		logger.Info("Starting new hand")

		// Start a new hand (this will fire HandStartEvent for TUI initialization)
		engine.StartNewHand()

		// Use the unified game engine with TUI integration
		handResult, err := playHandWithTUI(engine, agents, hi)
		if err != nil {
			log.Error("Error playing hand", "error", err)
			return err
		}

		// Check if player quit during hand
		if handResult.ShowdownType == "quit" {
			logger.Info("Player quit during hand")
			break
		}

		// Hand results and summary are now handled via HandEndEvent

		if handResult.Winner != nil {
			logger.Info("Hand complete", "winner", handResult.Winner.Name, "pot", handResult.PotSize)
		} else {
			logger.Error("Hand completed but no winner found")
		}

		// Auto-continue to next hand (user can quit with Ctrl+C if needed)
		logger.Info("Auto-continuing to next hand")
	}

	return nil
}

// playHandWithTUI wraps the game engine to provide TUI integration
func playHandWithTUI(engine *game.GameEngine, agents map[string]game.Agent, tui *tui.TUIAgent) (*game.HandResult, error) {
	// Simply delegate to the engine - it handles all agent types including TUI
	return engine.PlayHand(agents)
}
