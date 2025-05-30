package game

import (
	"errors"

	"github.com/charmbracelet/log"
)

// ActionObserver is notified when players take actions
type ActionObserver interface {
	OnPlayerAction(player *Player, reasoning string)
}

// GameEngine handles the core game loop logic that can be shared between
// interactive play and simulation
type GameEngine struct {
	table        *Table
	defaultAgent Agent
	logger       *log.Logger
	observers    []ActionObserver
}

// NewGameEngine creates a new game engine with a default agent
func NewGameEngine(table *Table, defaultAgent Agent, logger *log.Logger) *GameEngine {
	return &GameEngine{
		table:        table,
		defaultAgent: defaultAgent,
		logger:       logger,
		observers:    []ActionObserver{},
	}
}

// AddObserver adds an action observer
func (ge *GameEngine) AddObserver(observer ActionObserver) {
	ge.observers = append(ge.observers, observer)
}

// notifyObservers notifies all observers of a player action
func (ge *GameEngine) notifyObservers(player *Player, reasoning string) {
	for _, observer := range ge.observers {
		observer.OnPlayerAction(player, reasoning)
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
func (ge *GameEngine) PlayHand(agents map[string]Agent) (*HandResult, error) {
	result := &HandResult{
		HandID:  ge.table.HandID,
		Actions: make([]PlayerAction, 0),
	}

	ge.logger.Debug("Starting hand", "handID", ge.table.HandID)

	// Hand loop - continue until hand is complete
	for {
		currentPlayer := ge.table.GetCurrentPlayer()
		if currentPlayer != nil {
			var reasoning string
			var playerAction Action

			// Handle any player type using the unified Agent interface
			var selectedAgent Agent
			if agents != nil && agents[currentPlayer.Name] != nil {
				selectedAgent = agents[currentPlayer.Name]
			} else {
				selectedAgent = ge.defaultAgent
			}

			// Use new clean architecture: agents only make decisions, engine handles state
			tableState := ge.table.CreateTableState(currentPlayer)
			validActions := ge.table.GetValidActions()
			decision := selectedAgent.MakeDecision(tableState, validActions)

			// Engine applies the decision and handles state mutation
			reasoning, err := ge.table.ApplyDecision(decision)
			if err != nil {
				// Log error and use first valid action as fallback
				ge.logger.Error("Failed to apply agent decision", "error", err, "player", currentPlayer.Name)
				validActions := ge.table.GetValidActions()
				if len(validActions) > 0 {
					fallbackDecision := Decision{
						Action:    validActions[0].Action,
						Amount:    validActions[0].MinAmount,
						Reasoning: "fallback due to invalid decision",
					}
					reasoning, err = ge.table.ApplyDecision(fallbackDecision)
					if err != nil {
						ge.logger.Error("Fallback decision also failed", "error", err, "player", currentPlayer.Name)
						// This should not happen, but if it does, break to prevent infinite loop
						break
					}
				} else {
					ge.logger.Error("No valid actions available", "player", currentPlayer.Name)
					// This should not happen, but if it does, break to prevent infinite loop
					break
				}
			}

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

			// Notify observers of the action
			ge.notifyObservers(currentPlayer, reasoning)

			ge.table.AdvanceAction()
		}

		// Check if betting round is complete
		if ge.table.IsBettingRoundComplete() {
			ge.logger.Debug("Betting round complete", "round", ge.table.CurrentRound)

			activePlayers := len(ge.table.GetActivePlayers())
			if activePlayers <= 1 {
				// Hand over, someone won by everyone else folding
				potBeforeAwarding := ge.table.Pot
				winners := ge.table.FindWinners()
				if len(winners) > 0 {
					result.Winner = winners[0] // For compatibility, return first winner
					result.PotSize = potBeforeAwarding
					result.ShowdownType = "fold"
					ge.logger.Debug("Hand won by fold", "winner", winners[0].Name, "pot", potBeforeAwarding)
					ge.table.AwardPot()
				} else {
					ge.logger.Error("No winner found after fold", "activePlayers", activePlayers)
					result.Winner = nil
					result.PotSize = potBeforeAwarding
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
				winners := ge.table.FindWinners()
				if len(winners) > 0 {
					result.Winner = winners[0] // For compatibility, return first winner
					result.PotSize = potBeforeAwarding
					result.ShowdownType = "showdown"
					ge.logger.Debug("Hand went to showdown", "winner", winners[0].Name, "pot", potBeforeAwarding)
					ge.table.AwardPot()
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
		return nil, errors.New("hand completed but no winner found")
	}
	return result, nil
}

// StartNewHand initializes a new hand on the table
func (ge *GameEngine) StartNewHand() {
	ge.table.StartNewHand()
}

// GetTable returns the current table state
func (ge *GameEngine) GetTable() *Table {
	return ge.table
}
