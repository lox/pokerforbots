package server

import (
	"fmt"
	"time"

	"github.com/charmbracelet/log"
	"github.com/coder/quartz"
	"github.com/lox/pokerforbots/internal/game"
)

// NetworkAgent represents a server-side agent that proxies decisions to/from a remote client
type NetworkAgent struct {
	playerName     string
	tableID        string
	server         *Server
	logger         *log.Logger
	decisionChan   chan game.Decision
	timeoutSeconds int
	clock          quartz.Clock
	validActions   []game.ValidAction // Current valid actions for validation
}

// NewNetworkAgent creates a new network agent for a remote player
func NewNetworkAgent(playerName, tableID string, server *Server, logger *log.Logger, timeoutSeconds int, clock quartz.Clock) *NetworkAgent {
	return &NetworkAgent{
		playerName:     playerName,
		tableID:        tableID,
		server:         server,
		logger:         logger.WithPrefix("network-agent").With("player", playerName),
		decisionChan:   make(chan game.Decision, 1),
		timeoutSeconds: timeoutSeconds,
		clock:          clock,
	}
}

// MakeDecision implements the Agent interface by requesting a decision from the remote client
func (na *NetworkAgent) MakeDecision(tableState game.TableState, validActions []game.ValidAction) game.Decision {
	// Store valid actions for validation in HandleDecision
	na.validActions = validActions

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

	msg, err := NewMessage(MessageTypeActionRequired, actionData)
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

	// Wait for decision or timeout using quartz clock
	timeoutDuration := time.Duration(na.timeoutSeconds) * time.Second
	timeoutFired := make(chan struct{})

	timer := na.clock.AfterFunc(timeoutDuration, func() {
		close(timeoutFired)
	})
	defer timer.Stop()

	select {
	case decision := <-na.decisionChan:
		na.logger.Info("Received decision from remote player",
			"action", decision.Action,
			"amount", decision.Amount,
			"reasoning", decision.Reasoning)
		return decision

	case <-timeoutFired:
		na.logger.Warn("Decision timeout, putting player in sitting out state")

		// Send timeout event to all players at the table
		timeoutData := PlayerTimeoutData{
			TableID:        na.tableID,
			PlayerName:     na.playerName,
			TimeoutSeconds: na.timeoutSeconds,
			Action:         "sit-out",
		}

		timeoutMsg, err := NewMessage(MessageTypePlayerTimeout, timeoutData)
		if err == nil {
			na.server.BroadcastToTable(na.tableID, timeoutMsg)
		}

		return game.Decision{
			Action:    game.SitOut,
			Amount:    0,
			Reasoning: "Decision timeout - sitting out",
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
	case "sit-out":
		action = game.SitOut
	case "sit-in":
		action = game.SitIn
	default:
		return fmt.Errorf("invalid action: %s", data.Action)
	}

	// Validate action against current valid actions (defense in depth)
	if !na.isActionValid(action, data.Amount) {
		return fmt.Errorf("action '%s' is not valid in current game state", data.Action)
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

// isActionValid checks if the action is valid against stored validActions
func (na *NetworkAgent) isActionValid(action game.Action, amount int) bool {
	// Find matching valid action
	for _, validAction := range na.validActions {
		if validAction.Action == action {
			// For non-raise actions, amount constraints are automatically satisfied
			if action != game.Raise {
				return true
			}
			// For raises, check amount is within valid range
			return amount >= validAction.MinAmount && amount <= validAction.MaxAmount
		}
	}

	return false
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
	clock  quartz.Clock
}

// NewNetworkAgentManager creates a new network agent manager
func NewNetworkAgentManager(server *Server, logger *log.Logger, clock quartz.Clock) *NetworkAgentManager {
	return &NetworkAgentManager{
		agents: make(map[string]*NetworkAgent),
		server: server,
		logger: logger.WithPrefix("agent-manager"),
		clock:  clock,
	}
}

// CreateAgent creates a new network agent for a player
func (nam *NetworkAgentManager) CreateAgent(playerName, tableID string, timeoutSeconds int) *NetworkAgent {
	agent := NewNetworkAgent(playerName, tableID, nam.server, nam.logger, timeoutSeconds, nam.clock)
	nam.agents[playerName] = agent
	nam.logger.Info("Created network agent", "player", playerName, "table", tableID, "timeout", timeoutSeconds)
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
