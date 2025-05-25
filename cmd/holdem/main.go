package main

import (
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"

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
	Players  int  `short:"p" help:"Number of players at the table (6 or 9)" default:"6"`
	TestMode bool `short:"t" help:"Enable test mode with automatic decisions"`
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
	err := startInteractiveGame(cli.Players, cli.TestMode)
	if err != nil {
		log.Fatal("Failed to start game", "error", err)
	}

	ctx.Exit(0)
}

func startInteractiveGame(seats int, testMode bool) error {
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
	hi, err := game.NewHumanInterface(table, testMode)
	if err != nil {
		return fmt.Errorf("failed to create interface: %w", err)
	}
	defer hi.Close()

	ai := game.NewAIEngine()
	display := game.NewHandDisplay()

	// Start first hand
	table.StartNewHand()

	// Show compact hand header
	display.ShowHandHeader(seats, table)

	// Show single player list with positions and stacks
	display.ShowPlayerPositions(table)

	// Show hole cards phase
	display.ShowHoleCards(table)

	// Show blind posting
	display.ShowBlindPosting(table)

	// Show street indicator
	display.ShowStreet("PRE-FLOP")

	// Game loop - just demonstrate the interface for now
	for {
		currentPlayer := table.GetCurrentPlayer()
		if currentPlayer == nil {
			break
		}

		if currentPlayer.Type == game.Human {
			shouldContinue, err := hi.PromptForAction()
			if err != nil {
				return err
			}
			if !shouldContinue {
				break // Player quit
			}

			// For now, just advance to next player after human acts
			table.AdvanceAction()
		} else {
			// AI player takes action
			ai.ExecuteAIAction(currentPlayer, table)
			display.ShowPlayerAction(currentPlayer, table)
			table.AdvanceAction()
		}

		// Check if betting round is complete
		if table.IsBettingRoundComplete() {
			display.ShowBettingRoundComplete(table)

			activePlayers := len(getActivePlayers(table))
			if activePlayers <= 1 {
				// Hand over, someone won by everyone else folding
				display.ShowCompleteShowdown(table)
				display.ShowHandSummary(table)
				break
			}

			// Move to next betting round
			switch table.CurrentRound {
			case game.PreFlop:
				table.DealFlop()
				display.ShowBettingRoundTransition(table)
			case game.Flop:
				table.DealTurn()
				display.ShowBettingRoundTransition(table)
			case game.Turn:
				table.DealRiver()
				display.ShowBettingRoundTransition(table)
			case game.River:
				// Go to showdown
				table.CurrentRound = game.Showdown
				display.ShowCompleteShowdown(table)
				display.ShowHandSummary(table)
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
