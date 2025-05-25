package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"

	"github.com/lox/holdem-cli/internal/display"
	"github.com/lox/holdem-cli/internal/game"
)

var (
	titleStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(0, 1).
		Bold(true)
)

type CLI struct {
	Players int `short:"p" help:"Number of players at the table (6 or 9)" default:"6"`
}

func main() {
	var cli CLI
	ctx := kong.Parse(&cli)

	if cli.Players != 6 && cli.Players != 9 {
		log.Fatal("Invalid number of players. Must be 6 or 9.")
	}

	fmt.Print(titleStyle.Render(" ♠ ♥ Texas Hold'em CLI ♦ ♣ "))
	fmt.Println()
	fmt.Println()

	// Start interactive game
	err := startInteractiveGame(cli.Players)
	if err != nil {
		log.Fatal("Failed to start game", "error", err)
	}

	ctx.Exit(0)
}

func startInteractiveGame(seats int) error {
	// Set up debug logging
	debugFile, err := os.OpenFile("holdem-main.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if err != nil {
		return fmt.Errorf("failed to create main debug log: %w", err)
	}
	defer func() {
		if err := debugFile.Close(); err != nil {
			log.Error("Failed to close debug file", "error", err)
		}
	}()

	logger := log.NewWithOptions(debugFile, log.Options{
		ReportTimestamp: true,
		TimeFormat:      "15:04:05",
		Prefix:          "MAIN",
	})
	logger.Info("Starting interactive game", "seats", seats)
	// Create table
	table := game.NewTable(seats, 1, 2)

	// Add human player
	human := game.NewPlayer(1, "You", game.Human, 200)
	table.AddPlayer(human)

	// Add AI players
	for i := 2; i <= seats; i++ {
		aiPlayer := game.NewPlayer(i, fmt.Sprintf("AI-%d", i), game.AI, 200)
		table.AddPlayer(aiPlayer)
	}

	// Create human interface and AI engine
	hi, err := display.NewTUIInterface(table, logger, debugFile)
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

	ai := game.NewAIEngine(logger)

	// Start first hand
	table.StartNewHand()

	// Initialize TUI with game state
	hi.InitializeHand(seats)

	// Main game loop
	for {
		logger.Info("Starting new hand")

		// Hand loop
		for {
			currentPlayer := table.GetCurrentPlayer()
			if currentPlayer == nil {
				break
			}

			if currentPlayer.Type == game.Human {
				logger.Info("Prompting human player for action")
				shouldContinue, err := hi.PromptForAction()
				logger.Info("Received human action result", "continue", shouldContinue, "error", err)
				if err != nil {
					logger.Error("Error from PromptForAction", "error", err)
					return err
				}
				if !shouldContinue {
					logger.Info("Player quit, exiting game")
					return nil // Player quit
				}

				// Advance to next player after human acts
				logger.Info("Advancing to next player")
				table.AdvanceAction()
			} else {
				// AI player takes action
				ai.ExecuteAIAction(currentPlayer, table)
				hi.ShowPlayerAction(currentPlayer)
				table.AdvanceAction()
			}

			// Check if betting round is complete
			if table.IsBettingRoundComplete() {
				hi.ShowBettingRoundComplete()

				activePlayers := len(getActivePlayers(table))
				if activePlayers <= 1 {
					// Hand over, someone won by everyone else folding
					hi.ShowCompleteShowdown()
					hi.ShowHandSummary()
					break // Break out of hand loop
				}

				// Move to next betting round
				switch table.CurrentRound {
				case game.PreFlop:
					table.DealFlop()
					hi.ShowBettingRoundTransition()
				case game.Flop:
					table.DealTurn()
					hi.ShowBettingRoundTransition()
				case game.Turn:
					table.DealRiver()
					hi.ShowBettingRoundTransition()
				case game.River:
					// Go to showdown
					table.CurrentRound = game.Showdown
					hi.ShowCompleteShowdown()
					hi.ShowHandSummary()
				case game.Showdown:
					// Showdown is complete, end the hand
					break
				}
			}

			// If we're in showdown phase, don't continue the action loop
			if table.CurrentRound == game.Showdown {
				break // Break out of hand loop
			}
		}

		// Hand is complete, clear current player and ask if player wants to continue
		logger.Info("Hand complete, asking if player wants to continue")
		table.ActionOn = -1 // Clear current player so TUI shows continuation prompt

		shouldContinue, err := hi.PromptForAction()
		if err != nil {
			logger.Error("Error prompting for continuation", "error", err)
			return err
		}
		if shouldContinue {
			logger.Info("Player chose to continue to next hand")
		} else {
			logger.Info("Player chose to quit after hand completion")
			break
		}

		// Start new hand
		logger.Info("Starting new hand")
		hi.ClearLog() // Clear the display for fresh start
		table.StartNewHand()
		hi.InitializeHand(len(table.ActivePlayers))
	}

	return nil
}

func getActivePlayers(table *game.Table) []*game.Player {
	var active []*game.Player
	for _, player := range table.ActivePlayers {
		if player.IsInHand() {
			active = append(active, player)
		}
	}
	return active
}
