package game

import (
	"errors"
	"fmt"

	"github.com/charmbracelet/log"
	"github.com/lox/holdem-cli/internal/evaluator"
)

// GameEngine handles the core game loop logic that can be shared between
// interactive play and simulation
type GameEngine struct {
	table        *Table
	defaultAgent Agent
	logger       *log.Logger
	eventBus     EventBus
	startingChipTotal int // Track total chips when engine was created
}

// NewGameEngine creates a new game engine with a default agent
func NewGameEngine(table *Table, defaultAgent Agent, logger *log.Logger) *GameEngine {
	eventBus := NewEventBus()
	table.SetEventBus(eventBus)

	// Calculate starting chip total for conservation validation
	startingTotal := table.GetTotalChips()

	return &GameEngine{
		table:             table,
		defaultAgent:      defaultAgent,
		logger:            logger,
		eventBus:          eventBus,
		startingChipTotal: startingTotal,
	}
}

// GetEventBus returns the event bus for subscribing to game events
func (ge *GameEngine) GetEventBus() EventBus {
	return ge.eventBus
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
		HandID:  ge.table.handID,
		Actions: make([]PlayerAction, 0),
	}

	ge.logger.Debug("Starting hand", "handID", ge.table.handID)

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

			// Check if player quit
			if playerAction == Quit {
				ge.logger.Info("Player quit during hand", "player", currentPlayer.Name)
				result.ShowdownType = "quit"
				result.Winner = nil
				result.PotSize = ge.table.pot
				return result, nil
			}

			// Record the action
			result.Actions = append(result.Actions, PlayerAction{
				PlayerName: currentPlayer.Name,
				Round:      ge.table.currentRound,
				Action:     playerAction,
				Amount:     currentPlayer.ActionAmount,
				Reasoning:  reasoning,
			})

			ge.logger.Debug("Player action",
				"player", currentPlayer.Name,
				"action", playerAction,
				"amount", currentPlayer.ActionAmount,
				"reasoning", reasoning)

			// Publish player action event
			event := NewPlayerActionEvent(currentPlayer, playerAction, currentPlayer.ActionAmount, ge.table.currentRound, reasoning, ge.table.pot)
			ge.eventBus.Publish(event)

			ge.table.AdvanceAction()
		}

		// Check if betting round is complete
		if ge.table.IsBettingRoundComplete() {
			ge.logger.Debug("Betting round complete", "round", ge.table.currentRound)

			activePlayers := len(ge.table.GetActivePlayers())
			if activePlayers <= 1 {
				// Hand over, someone won by everyone else folding
				potBeforeAwarding := ge.table.pot
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
			switch ge.table.currentRound {
			case PreFlop:
				ge.table.DealFlop()
				ge.logger.Debug("Dealt flop", "flop", ge.table.communityCards[:3])
				// Publish street change event
				streetEvent := NewStreetChangeEvent(ge.table.currentRound, ge.table.communityCards, ge.table.currentBet)
				ge.eventBus.Publish(streetEvent)
			case Flop:
				ge.table.DealTurn()
				ge.logger.Debug("Dealt turn", "turn", ge.table.communityCards[3])
				// Publish street change event
				streetEvent := NewStreetChangeEvent(ge.table.currentRound, ge.table.communityCards, ge.table.currentBet)
				ge.eventBus.Publish(streetEvent)
			case Turn:
				ge.table.DealRiver()
				ge.logger.Debug("Dealt river", "river", ge.table.communityCards[4])
				// Publish street change event
				streetEvent := NewStreetChangeEvent(ge.table.currentRound, ge.table.communityCards, ge.table.currentBet)
				ge.eventBus.Publish(streetEvent)
			case River:
				// Go to showdown
				ge.table.currentRound = Showdown
				potBeforeAwarding := ge.table.pot
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
		if ge.table.currentRound == Showdown {
			break
		}
	}

	if result.Winner != nil {
		ge.logger.Debug("Hand complete", "winner", result.Winner.Name, "pot", result.PotSize)
		
		// Validate chip conservation after pot awarding
		if err := ge.validateChipConservation(); err != nil {
			ge.logger.Error("Chip conservation violation detected!", "error", err)
			return nil, fmt.Errorf("chip conservation violation: %w", err)
		}

		// Create detailed winner info for the event
		winnerInfo := ge.createWinnerInfo(result.Winner, result.PotSize, result.ShowdownType)
		winners := []WinnerInfo{winnerInfo}

		// Finalize hand history with results BEFORE generating summary
		if ge.table.handHistory != nil {
			// Set final results in hand history
			ge.table.handHistory.SetFinalResults(result.PotSize, winners)
			ge.table.handHistory.SetCommunityCards(ge.table.communityCards)

			// Add hole cards for showdown or human perspective
			for _, player := range ge.table.players {
				if len(player.HoleCards) > 0 {
					// Show hole cards for human player or if hand went to showdown
					if player.Type == Human || result.ShowdownType == "showdown" {
						ge.table.handHistory.AddPlayerHoleCards(player.Name, player.HoleCards)
					}
				}
			}

			// Save hand history to file (server responsibility)
			if err := ge.table.handHistory.SaveToFile(); err != nil {
				ge.logger.Error("Failed to save hand history", "error", err, "handID", result.HandID)
			} else {
				ge.logger.Info("Hand history saved", "handID", result.HandID)
			}
		}

		// Generate rich summary from COMPLETED HandHistory
		var summary string
		if ge.table.handHistory != nil {
			summary = ge.table.handHistory.GenerateSummary(SummaryOpts{
				PlayerPerspective: "You", // TUI perspective
			})
		} else {
			// Fallback basic summary
			summary = fmt.Sprintf("*** SUMMARY ***\nTotal pot $%d\nWinner: %s\n", result.PotSize, result.Winner.Name)
		}

		// Publish hand end event with detailed winner information and rich summary
		handEndEvent := NewHandEndEvent(result.HandID, winners, result.PotSize, result.ShowdownType, ge.table.communityCards, summary)
		ge.logger.Debug("Publishing HandEndEvent", "winners", len(winners), "showdownType", result.ShowdownType)
		ge.eventBus.Publish(handEndEvent)
	} else {
		return nil, errors.New("hand completed but no winner found")
	}
	return result, nil
}

// StartNewHand initializes a new hand on the table
func (ge *GameEngine) StartNewHand() {
	ge.table.StartNewHand() // This now publishes HandStartEvent internally

	// Subscribe the hand history to events for this hand
	if ge.table.handHistory != nil {
		ge.eventBus.Subscribe(ge.table.handHistory)
	}
}

// GetTable returns the current table state
func (ge *GameEngine) GetTable() *Table {
	return ge.table
}

// validateChipConservation checks that total chips haven't changed unexpectedly
func (ge *GameEngine) validateChipConservation() error {
	return ge.table.ValidateChipConservation(ge.startingChipTotal)
}

// createWinnerInfo creates detailed winner information for events
func (ge *GameEngine) createWinnerInfo(winner *Player, amount int, showdownType string) WinnerInfo {
	var handRank string

	// Determine hand rank using evaluator
	if len(winner.HoleCards) == 2 && len(ge.table.communityCards) == 5 {
		// Only evaluate when we have exactly 7 cards (2 hole + 5 community)
		allCards := append(winner.HoleCards, ge.table.communityCards...)
		handScore := evaluator.Evaluate7(allCards)
		handRank = handScore.String()
	} else if showdownType == "showdown" {
		// Hand went to showdown but not all cards dealt
		handRank = "Win at showdown"
	} else {
		// Hand ended by folds
		handRank = "Win by fold"
	}

	return WinnerInfo{
		PlayerName: winner.Name,
		Amount:     amount,
		HoleCards:  winner.HoleCards,
		HandRank:   handRank,
	}
}
