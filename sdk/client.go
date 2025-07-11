package sdk

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/charmbracelet/log"
)

// BotClient provides a simplified interface for bots to connect to the poker server
type BotClient struct {
	client    *WSClient
	agent     Agent
	logger    *log.Logger
	tableID   string
	playerID  string
	authComplete chan bool
	validator *Validator
}

// NewBotClient creates a new bot client
func NewBotClient(serverURL, botName string, agent Agent, logger *log.Logger) *BotClient {
	wsClient := NewWSClient(serverURL, logger)
	
	validator, err := NewValidator()
	if err != nil {
		logger.Warn("Failed to initialize validator", "error", err)
	}

	return &BotClient{
		client:    wsClient,
		agent:     agent,
		logger:    logger,
		authComplete: make(chan bool, 1),
		validator: validator,
	}
}

// Connect establishes connection to the server and authenticates
func (bc *BotClient) Connect(ctx context.Context, botName string) error {
	if err := bc.client.Connect(); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}

	// Set up event handlers
	bc.setupEventHandlers()

	// Authenticate
	if err := bc.client.Auth(botName); err != nil {
		return fmt.Errorf("failed to send auth: %w", err)
	}

	// Wait for authentication to complete
	select {
	case success := <-bc.authComplete:
		if !success {
			return fmt.Errorf("authentication failed")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// JoinTable joins a poker table
func (bc *BotClient) JoinTable(tableID string, buyIn int) error {
	bc.tableID = tableID
	return bc.client.JoinTable(tableID, buyIn)
}

// setupEventHandlers configures event handling for bot decision making
func (bc *BotClient) setupEventHandlers() {
	// Handle decision requests
	bc.client.AddEventHandler(MessageTypeActionRequired, func(msg *Message) {
		if bc.validator != nil {
			if err := bc.validateMessage(msg); err != nil {
				bc.logger.Error("Message validation failed", "type", msg.Type, "error", err)
				return
			}
		}
		bc.handleActionRequired(msg)
	})

	// Handle authentication response
	bc.client.AddEventHandler(MessageTypeAuthResponse, func(msg *Message) {
		bc.handleAuthResponse(msg)
	})

	// Handle table join response
	bc.client.AddEventHandler(MessageTypeTableJoined, func(msg *Message) {
		bc.handleTableJoined(msg)
	})

	// Handle errors
	bc.client.AddEventHandler(MessageTypeError, func(msg *Message) {
		bc.handleError(msg)
	})
}

// handleActionRequired processes decision requests from the server
func (bc *BotClient) handleActionRequired(msg *Message) {
	var actionData struct {
		TableID      string            `json:"tableId"`
		PlayerName   string            `json:"playerName"`
		ValidActions []ValidActionInfo `json:"validActions"`
		TableState   TableState        `json:"tableState"`
		TimeoutSecs  int               `json:"timeoutSeconds"`
	}

	if err := json.Unmarshal(msg.Data, &actionData); err != nil {
		bc.logger.Error("Failed to parse action required", "error", err)
		return
	}

	// Convert ValidActionInfo to ValidAction
	validActions := make([]ValidAction, len(actionData.ValidActions))
	for i, actionInfo := range actionData.ValidActions {
		validActions[i] = ValidAction{
			Action:    ActionFromString(actionInfo.Action),
			MinAmount: actionInfo.MinAmount,
			MaxAmount: actionInfo.MaxAmount,
		}
	}

	// Let the bot make a decision
	decision := bc.agent.MakeDecision(actionData.TableState, validActions)

	// Send the decision back to the server
	if err := bc.client.SendDecision(actionData.TableID, decision.Action.String(), decision.Amount, decision.Reasoning); err != nil {
		bc.logger.Error("Failed to send decision", "error", err)
	}
}

func (bc *BotClient) handleAuthResponse(msg *Message) {
	var authData struct {
		Success  bool   `json:"success"`
		PlayerID string `json:"playerId"`
		Error    string `json:"error"`
	}

	if err := json.Unmarshal(msg.Data, &authData); err != nil {
		bc.logger.Error("Failed to parse auth response", "error", err)
		bc.authComplete <- false
		return
	}

	if authData.Success {
		bc.playerID = authData.PlayerID
		bc.logger.Info("Successfully authenticated", "playerId", bc.playerID)
		bc.authComplete <- true
	} else {
		bc.logger.Error("Authentication failed", "error", authData.Error)
		bc.authComplete <- false
	}
}

func (bc *BotClient) handleTableJoined(msg *Message) {
	bc.logger.Info("Successfully joined table", "tableId", bc.tableID)
}

func (bc *BotClient) handleError(msg *Message) {
	var errorData struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(msg.Data, &errorData); err != nil {
		bc.logger.Error("Failed to parse error message", "error", err)
		return
	}

	bc.logger.Error("Server error", "code", errorData.Code, "message", errorData.Message)
}

// Disconnect closes the connection to the server
func (bc *BotClient) Disconnect() error {
	return bc.client.Disconnect()
}

// validateMessage validates an incoming WebSocket message
func (bc *BotClient) validateMessage(msg *Message) error {
	if bc.validator == nil {
		return nil // Validation disabled
	}
	
	// Serialize message to JSON for validation
	msgData, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	
	return bc.validator.ValidateMessage(msgData)
}
