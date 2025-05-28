package game

import (
	"github.com/charmbracelet/log"
)

// GameEngine handles the core game loop logic that can be shared between
// interactive play and simulation
type GameEngine struct {
	table        *Table
	defaultAgent Agent
	logger       *log.Logger
}

// NewGameEngine creates a new game engine with a default agent
func NewGameEngine(table *Table, defaultAgent Agent, logger *log.Logger) *GameEngine {
	return &GameEngine{
		table:        table,
		defaultAgent: defaultAgent,
		logger:       logger,
	}
}

// HandResult contains the results of a completed hand
type HandResult struct {
	HandID       string
	Winner       *Player
	PotSize      int
	ShowdownType string // "fold" or "showdown"
	Actions      []PlayerAction
}

// PlayerAction represents an action taken by a player during the hand
type PlayerAction struct {
	PlayerName string
	Round      BettingRound
	Action     Action
	Amount     int
	Reasoning  string
}

// PlayHand runs a complete hand from start to finish and returns the result
func (ge *GameEngine) PlayHand(agents map[string]Agent) *HandResult {
	result := &HandResult{
		HandID:  ge.table.HandID,
		Actions: make([]PlayerAction, 0),
	}

	ge.logger.Debug("Starting hand", "handID", ge.table.HandID)

	// Hand loop - continue until hand is complete
	for {
		currentPlayer := ge.table.GetCurrentPlayer()
		if currentPlayer == nil {
			break
		}

		var reasoning string
		var playerAction Action

		// Handle any player type using the unified Agent interface
		var selectedAgent Agent
		if agents != nil && agents[currentPlayer.Name] != nil {
			selectedAgent = agents[currentPlayer.Name]
		} else {
			selectedAgent = ge.defaultAgent
		}

		reasoning = selectedAgent.ExecuteAction(currentPlayer, ge.table)
		playerAction = currentPlayer.LastAction

		// Record the action
		result.Actions = append(result.Actions, PlayerAction{
			PlayerName: currentPlayer.Name,
			Round:      ge.table.CurrentRound,
			Action:     playerAction,
			Amount:     currentPlayer.ActionAmount,
			Reasoning:  reasoning,
		})

		ge.logger.Debug("Player action", 
			"player", currentPlayer.Name,
			"action", playerAction,
			"amount", currentPlayer.ActionAmount,
			"reasoning", reasoning)

		ge.table.AdvanceAction()

		// Check if betting round is complete
		if ge.table.IsBettingRoundComplete() {
			ge.logger.Debug("Betting round complete", "round", ge.table.CurrentRound)

			activePlayers := len(ge.table.GetActivePlayers())
			if activePlayers <= 1 {
				// Hand over, someone won by everyone else folding
				winner := ge.table.FindWinner()
				if winner != nil {
					ge.table.AwardPot(winner)
					result.Winner = winner
					result.PotSize = 0 // Pot is now 0 after awarding
					result.ShowdownType = "fold"
					ge.logger.Debug("Hand won by fold", "winner", winner.Name)
				} else {
					ge.logger.Error("No winner found after fold", "activePlayers", activePlayers)
					result.Winner = nil
					result.PotSize = ge.table.Pot
					result.ShowdownType = "no_winner"
				}
				break
			}

			// Move to next betting round
			switch ge.table.CurrentRound {
			case PreFlop:
				ge.table.DealFlop()
				ge.logger.Debug("Dealt flop", "flop", ge.table.CommunityCards[:3])
			case Flop:
				ge.table.DealTurn()
				ge.logger.Debug("Dealt turn", "turn", ge.table.CommunityCards[3])
			case Turn:
				ge.table.DealRiver()
				ge.logger.Debug("Dealt river", "river", ge.table.CommunityCards[4])
			case River:
				// Go to showdown
				ge.table.CurrentRound = Showdown
				potBeforeAwarding := ge.table.Pot
				winner := ge.table.FindWinner()
				if winner != nil {
					ge.table.AwardPot(winner)
					result.Winner = winner
					result.PotSize = potBeforeAwarding
					result.ShowdownType = "showdown"
					ge.logger.Debug("Hand went to showdown", "winner", winner.Name, "pot", potBeforeAwarding)
				} else {
					ge.logger.Error("No winner found at showdown")
					result.Winner = nil
					result.PotSize = potBeforeAwarding
					result.ShowdownType = "no_winner"
				}
			case Showdown:
				// Showdown is complete, end the hand
				break
			}
		}

		// If we're in showdown phase, don't continue the action loop
		if ge.table.CurrentRound == Showdown {
			break
		}
	}

	if result.Winner != nil {
		ge.logger.Debug("Hand complete", "winner", result.Winner.Name, "pot", result.PotSize)
	} else {
		ge.logger.Error("Hand completed but no winner found")
	}
	return result
}

// StartNewHand initializes a new hand on the table
func (ge *GameEngine) StartNewHand() {
	ge.table.StartNewHand()
}

// GetTable returns the current table state
func (ge *GameEngine) GetTable() *Table {
	return ge.table
}