package server

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/charmbracelet/log"
	"github.com/lox/holdem-cli/internal/game"
)

// NetworkAgent represents a server-side agent that proxies decisions to/from a remote client
type NetworkAgent struct {
	playerName     string
	tableID        string
	server         *Server
	logger         *log.Logger
	decisionChan   chan game.Decision
	timeoutSeconds int
}

// NewNetworkAgent creates a new network agent for a remote player
func NewNetworkAgent(playerName, tableID string, server *Server, logger *log.Logger) *NetworkAgent {
	return &NetworkAgent{
		playerName:     playerName,
		tableID:        tableID,
		server:         server,
		logger:         logger.WithPrefix("network-agent").With("player", playerName),
		decisionChan:   make(chan game.Decision, 1),
		timeoutSeconds: 30, // 30 second default timeout
	}
}

// MakeDecision implements the Agent interface by requesting a decision from the remote client
func (na *NetworkAgent) MakeDecision(tableState game.TableState, validActions []game.ValidAction) game.Decision {
	na.logger.Info("Requesting decision from remote player",
		"currentBet", tableState.CurrentBet,
		"pot", tableState.Pot,
		"validActions", len(validActions))

	// Convert valid actions to message format
	validActionInfos := make([]ValidActionInfo, len(validActions))
	for i, va := range validActions {
		validActionInfos[i] = ValidActionInfoFromGame(va)
	}

	// Create action required message
	actionData := ActionRequiredData{
		TableID:        na.tableID,
		PlayerName:     na.playerName,
		ValidActions:   validActionInfos,
		TableState:     TableStateFromGame(tableState),
		TimeoutSeconds: na.timeoutSeconds,
	}

	msg, err := NewMessage("action_required", actionData)
	if err != nil {
		na.logger.Error("Failed to create action required message", "error", err)
		return game.Decision{
			Action:    game.Fold,
			Amount:    0,
			Reasoning: "Failed to send action request",
		}
	}

	// Send message to the specific player
	if err := na.server.SendToPlayer(na.playerName, msg); err != nil {
		na.logger.Error("Failed to send action request to player", "error", err)
		return game.Decision{
			Action:    game.Fold,
			Amount:    0,
			Reasoning: "Player disconnected",
		}
	}

	// Wait for decision or timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(na.timeoutSeconds)*time.Second)
	defer cancel()

	select {
	case decision := <-na.decisionChan:
		na.logger.Info("Received decision from remote player",
			"action", decision.Action,
			"amount", decision.Amount,
			"reasoning", decision.Reasoning)
		return decision

	case <-ctx.Done():
		na.logger.Warn("Decision timeout, folding player")
		return game.Decision{
			Action:    game.Fold,
			Amount:    0,
			Reasoning: "Decision timeout",
		}
	}
}

// HandleDecision processes a decision received from the remote client
func (na *NetworkAgent) HandleDecision(data PlayerDecisionData) error {
	na.logger.Info("Processing decision from client",
		"action", data.Action,
		"amount", data.Amount,
		"reasoning", data.Reasoning)

	// Convert string action to game.Action
	var action game.Action
	switch data.Action {
	case "fold":
		action = game.Fold
	case "call":
		action = game.Call
	case "check":
		action = game.Check
	case "raise":
		action = game.Raise
	case "allin":
		action = game.AllIn
	default:
		return fmt.Errorf("invalid action: %s", data.Action)
	}

	decision := game.Decision{
		Action:    action,
		Amount:    data.Amount,
		Reasoning: data.Reasoning,
	}

	// Try to send decision without blocking
	select {
	case na.decisionChan <- decision:
		return nil
	default:
		return fmt.Errorf("decision channel full or no pending request")
	}
}

// SetTimeout sets the decision timeout for this agent
func (na *NetworkAgent) SetTimeout(seconds int) {
	na.timeoutSeconds = seconds
}

// NetworkAgentManager manages all network agents for a server
type NetworkAgentManager struct {
	agents map[string]*NetworkAgent // playerName -> agent
	server *Server
	logger *log.Logger
}

// NewNetworkAgentManager creates a new network agent manager
func NewNetworkAgentManager(server *Server, logger *log.Logger) *NetworkAgentManager {
	return &NetworkAgentManager{
		agents: make(map[string]*NetworkAgent),
		server: server,
		logger: logger.WithPrefix("agent-manager"),
	}
}

// CreateAgent creates a new network agent for a player
func (nam *NetworkAgentManager) CreateAgent(playerName, tableID string) *NetworkAgent {
	agent := NewNetworkAgent(playerName, tableID, nam.server, nam.logger)
	nam.agents[playerName] = agent
	nam.logger.Info("Created network agent", "player", playerName, "table", tableID)
	return agent
}

// GetAgent returns the network agent for a player
func (nam *NetworkAgentManager) GetAgent(playerName string) *NetworkAgent {
	return nam.agents[playerName]
}

// RemoveAgent removes a network agent for a player
func (nam *NetworkAgentManager) RemoveAgent(playerName string) {
	delete(nam.agents, playerName)
	nam.logger.Info("Removed network agent", "player", playerName)
}

// HandlePlayerDecision routes a player decision to the appropriate agent
func (nam *NetworkAgentManager) HandlePlayerDecision(playerName string, data PlayerDecisionData) error {
	agent := nam.GetAgent(playerName)
	if agent == nil {
		return fmt.Errorf("no agent found for player: %s", playerName)
	}

	return agent.HandleDecision(data)
}

// Helper function to parse action string and amount
func parseActionAmount(actionStr string) (game.Action, int, error) {
	switch actionStr {
	case "fold", "f":
		return game.Fold, 0, nil
	case "call", "c":
		return game.Call, 0, nil
	case "check", "k":
		return game.Check, 0, nil
	case "allin", "all", "a":
		return game.AllIn, 0, nil
	default:
		// Try to parse raise actions like "raise 100" or "r 100"
		if actionStr == "raise" || actionStr == "r" {
			// Amount should be specified separately
			return game.Raise, 0, nil
		}

		// Try to parse combined actions
		if len(actionStr) > 5 && actionStr[:5] == "raise" {
			amountStr := actionStr[5:]
			amount, err := strconv.Atoi(amountStr)
			if err != nil {
				return game.Fold, 0, fmt.Errorf("invalid raise amount: %s", amountStr)
			}
			return game.Raise, amount, nil
		}

		return game.Fold, 0, fmt.Errorf("unknown action: %s", actionStr)
	}
}
