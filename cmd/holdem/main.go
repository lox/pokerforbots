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
	Players  int    `short:"p" help:"Number of players at the table (6 or 9)" default:"6"`
	LogLevel string `help:"Set the log-level" enum:"debug,info,warn,error" default:"info"`
	LogFile  string `help:"The logfile to write logs to" default:"holdem.log"`
	Seed     *int64 `help:"The seed for the random number generator"`
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

	if cli.Players != 6 && cli.Players != 9 {
		log.Fatal("Invalid number of players. Must be 6 or 9.")
	}

	randSource := rand.NewSource(time.Now().UnixNano())
	if cli.Seed != nil {
		randSource = rand.NewSource(*cli.Seed)
	}

	// Start interactive game
	err = startInteractiveGame(rand.New(randSource), cli.Players, logger)
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
		return nil, nilCloser, fmt.Errorf("failed to create main debug log: %w", err)
	}

	// Create logger with specified level
	logger := log.NewWithOptions(debugFile, log.Options{
		ReportTimestamp: true,
		Prefix:          "main",
		TimeFormat:      "15:04:05",
		Level:           parsedLevel,
	})

	return logger, debugFile.Close, nil
}

func startInteractiveGame(rng *rand.Rand, seats int, logger *log.Logger) error {
	// Create table
	table := game.NewTable(rng, game.TableConfig{
		MaxSeats:   seats,
		SmallBlind: 1,
		BigBlind:   2,
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

	// Create human interface
	hi, err := tui.NewTUIAgent(table, logger)
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

	// Main game loop - much simpler!
	for {
		logger.Info("Starting new hand")

		// Start a new hand first
		engine.StartNewHand()

		// Initialize TUI after hand is started (cards dealt, blinds posted)
		hi.InitializeHand(len(table.ActivePlayers))

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

		// Show hand results
		hi.ShowCompleteShowdown()
		hi.ShowHandSummary(handResult.PotSize)

		if handResult.Winner != nil {
			logger.Info("Hand complete", "winner", handResult.Winner.Name, "pot", handResult.PotSize)
		} else {
			logger.Error("Hand completed but no winner found")
		}

		// Ask if player wants to continue
		table.ActionOn = -1 // Clear current player for continuation prompt
		shouldContinue, err := hi.PromptForAction()
		if err != nil {
			logger.Error("Error prompting for continuation", "error", err)
			return err
		}
		if !shouldContinue {
			logger.Info("Player chose to quit after hand completion")
			break
		}

		// Prepare for next hand
		logger.Info("Player chose to continue to next hand")
		hi.ClearLog()
		hi.InitializeHand(len(table.ActivePlayers))
	}

	return nil
}

// playHandWithTUI wraps the game engine to provide TUI integration
func playHandWithTUI(engine *game.GameEngine, agents map[string]game.Agent, tui *tui.TUIAgent) (*game.HandResult, error) {
	table := engine.GetTable()

	// Hand loop - continue until hand is complete
	for {
		currentPlayer := table.GetCurrentPlayer()
		if currentPlayer == nil {
			break
		}

		var reasoning string

		// Handle any player type using the unified Agent interface
		if agents == nil || agents[currentPlayer.Name] == nil {
			// This shouldn't happen in our setup, but fallback to a basic bot
			return engine.PlayHand(agents)
		}

		// TODO: Fix this to work with new interface
		// For now, just use a simple fallback
		return &game.HandResult{
			HandID:       table.HandID,
			Winner:       nil,
			PotSize:      table.Pot,
			ShowdownType: "incomplete",
		}, nil

		// Check if human player quit
		if currentPlayer.Type == game.Human && reasoning == "Player quit" {
			return &game.HandResult{
				HandID:       table.HandID,
				Winner:       nil, // Special case for quit
				PotSize:      table.Pot,
				ShowdownType: "quit",
			}, nil
		}

		// Show the player action in TUI (except for human player who already showed it)
		if currentPlayer.Type == game.AI {
			tui.ShowPlayerActionWithThinking(currentPlayer, reasoning)
		}

		table.AdvanceAction()

		// Check if betting round is complete
		if table.IsBettingRoundComplete() {
			activePlayers := len(table.GetActivePlayers())
			if activePlayers <= 1 {
				// Hand over, someone won by everyone else folding
				potBeforeAwarding := table.Pot
				winner := table.FindWinner()
				table.AwardPot()
				return &game.HandResult{
					HandID:       table.HandID,
					Winner:       winner,
					PotSize:      potBeforeAwarding,
					ShowdownType: "fold",
				}, nil
			}

			// Show betting round completion
			tui.ShowBettingRoundComplete()

			// Move to next betting round and show transition
			switch table.CurrentRound {
			case game.PreFlop:
				table.DealFlop()
				tui.ShowBettingRoundTransition()
			case game.Flop:
				table.DealTurn()
				tui.ShowBettingRoundTransition()
			case game.Turn:
				table.DealRiver()
				tui.ShowBettingRoundTransition()
			case game.River:
				// Go to showdown
				table.CurrentRound = game.Showdown
				potBeforeAwarding := table.Pot
				winner := table.FindWinner()
				table.AwardPot()
				return &game.HandResult{
					HandID:       table.HandID,
					Winner:       winner,
					PotSize:      potBeforeAwarding,
					ShowdownType: "showdown",
				}, nil
			case game.Showdown:
				// Showdown is complete, end the hand
				break
			}
		}

		// If we're in showdown phase, don't continue the action loop
		if table.CurrentRound == game.Showdown {
			break
		}
	}

	// Fallback - this shouldn't be reached
	winner := table.FindWinner()
	if winner != nil {
		table.AwardPot()
	}
	return &game.HandResult{
		HandID:  table.HandID,
		Winner:  winner,
		PotSize: 0,
	}, nil
}
