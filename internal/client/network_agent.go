package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/charmbracelet/log"
	"github.com/lox/holdem-cli/internal/game"
	"github.com/lox/holdem-cli/internal/server"
)

// NetworkAgent implements the Agent interface for remote game servers
type NetworkAgent struct {
	client       *Client
	tui          TUIInterface // Interface to communicate with TUI
	logger       *log.Logger
	decisionChan chan game.Decision

	// Current game state
	currentTableState   game.TableState
	currentValidActions []game.ValidAction
}

// TUIInterface defines the interface between NetworkAgent and TUI
type TUIInterface interface {
	// Event handling
	AddLogEntry(string)
	AddLogEntryAndScrollToShow(string)
	ClearLog()

	// Game state updates
	UpdatePot(int)
	UpdateCurrentBet(int)
	UpdateValidActions([]game.ValidAction)
	SetHumanTurn(bool, *game.Player)

	// User input
	WaitForAction() (string, []string, bool, error)

	// Card formatting
	FormatCards(cards interface{}) string
}

// NewNetworkAgent creates a new network agent
func NewNetworkAgent(client *Client, tui TUIInterface, logger *log.Logger) *NetworkAgent {
	na := &NetworkAgent{
		client:       client,
		tui:          tui,
		logger:       logger.WithPrefix("network-agent"),
		decisionChan: make(chan game.Decision, 1),
	}

	// Register event handlers
	na.setupEventHandlers()

	return na
}

// setupEventHandlers registers handlers for various game events
func (na *NetworkAgent) setupEventHandlers() {
	// Authentication response
	na.client.AddEventHandler("auth_response", na.handleAuthResponse)

	// Table events
	na.client.AddEventHandler("table_list", na.handleTableList)
	na.client.AddEventHandler("table_joined", na.handleTableJoined)
	na.client.AddEventHandler("table_left", na.handleTableLeft)

	// Game events
	na.client.AddEventHandler("hand_start", na.handleHandStart)
	na.client.AddEventHandler("player_action", na.handlePlayerAction)
	na.client.AddEventHandler("street_change", na.handleStreetChange)
	na.client.AddEventHandler("hand_end", na.handleHandEnd)
	na.client.AddEventHandler("action_required", na.handleActionRequired)

	// Error handling
	na.client.AddEventHandler("error", na.handleError)
}

// MakeDecision implements the Agent interface - this should not be called directly for NetworkAgent
// Instead, decisions are made through the action_required handler
func (na *NetworkAgent) MakeDecision(tableState game.TableState, validActions []game.ValidAction) game.Decision {
	na.logger.Warn("MakeDecision called directly on NetworkAgent - this should not happen")

	// Store state for potential use
	na.currentTableState = tableState
	na.currentValidActions = validActions

	// Wait for decision from server via action_required message
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	select {
	case decision := <-na.decisionChan:
		return decision
	case <-ctx.Done():
		na.logger.Warn("Decision timeout in MakeDecision")
		return game.Decision{
			Action:    game.Fold,
			Amount:    0,
			Reasoning: "Network timeout",
		}
	}
}

// Event Handlers

func (na *NetworkAgent) handleAuthResponse(msg *server.Message) {
	var data server.AuthResponseData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		na.logger.Error("Failed to parse auth response", "error", err)
		return
	}

	if data.Success {
		na.tui.AddLogEntry(fmt.Sprintf("Authenticated as %s", data.PlayerID))
	} else {
		na.tui.AddLogEntry(fmt.Sprintf("Authentication failed: %s", data.Error))
	}
}

func (na *NetworkAgent) handleTableList(msg *server.Message) {
	var data server.TableListData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		na.logger.Error("Failed to parse table list", "error", err)
		return
	}

	na.tui.AddLogEntry("Available tables:")
	for _, table := range data.Tables {
		na.tui.AddLogEntry(fmt.Sprintf("  %s: %s (%d/%d players, stakes %s)",
			table.ID, table.Name, table.PlayerCount, table.MaxPlayers, table.Stakes))
	}
}

func (na *NetworkAgent) handleTableJoined(msg *server.Message) {
	var data server.TableJoinedData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		na.logger.Error("Failed to parse table joined", "error", err)
		return
	}

	na.client.SetTableID(data.TableID)
	na.tui.AddLogEntry(fmt.Sprintf("Joined table %s (seat %d)", data.TableID, data.SeatNumber))

	// Show other players
	na.tui.AddLogEntry("Players at table:")
	for _, player := range data.Players {
		na.tui.AddLogEntry(fmt.Sprintf("  %s: $%d", player.Name, player.Chips))
	}
}

func (na *NetworkAgent) handleTableLeft(msg *server.Message) {
	na.client.SetTableID("")
	na.tui.AddLogEntry("Left table")
}

func (na *NetworkAgent) handleHandStart(msg *server.Message) {
	var data server.HandStartData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		na.logger.Error("Failed to parse hand start", "error", err)
		return
	}

	na.tui.AddLogEntryAndScrollToShow(fmt.Sprintf("Hand %s • %d players • $%d/$%d",
		data.HandID, len(data.Players), data.SmallBlind, data.BigBlind))
	na.tui.AddLogEntry("")
	na.tui.AddLogEntry("*** HOLE CARDS ***")

	// Show hole cards if we have them
	for _, player := range data.Players {
		if player.Name == na.client.GetPlayerName() && len(player.HoleCards) > 0 {
			cards := na.tui.FormatCards(player.HoleCards)
			na.tui.AddLogEntry(fmt.Sprintf("Dealt to You: %s", cards))
			break
		}
	}

	// Show blind posting
	for _, player := range data.Players {
		if player.BetThisRound > 0 {
			if player.Position == "small_blind" {
				na.tui.AddLogEntry(fmt.Sprintf("%s: posts small blind $%d", player.Name, player.BetThisRound))
			} else if player.Position == "big_blind" {
				na.tui.AddLogEntry(fmt.Sprintf("%s: posts big blind $%d", player.Name, player.BetThisRound))
			}
		}
	}

	na.tui.AddLogEntry("")
	na.tui.AddLogEntry("*** PRE-FLOP ***")
	na.tui.UpdatePot(data.InitialPot)
}

func (na *NetworkAgent) handlePlayerAction(msg *server.Message) {
	var data server.PlayerActionData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		na.logger.Error("Failed to parse player action", "error", err)
		return
	}

	na.tui.UpdatePot(data.PotAfter)

	var actionEntry string
	switch data.Action {
	case "fold":
		actionEntry = fmt.Sprintf("%s: folds", data.Player)
	case "call":
		actionEntry = fmt.Sprintf("%s: calls $%d (pot now: $%d)", data.Player, data.Amount, data.PotAfter)
	case "check":
		actionEntry = fmt.Sprintf("%s: checks", data.Player)
	case "raise":
		actionEntry = fmt.Sprintf("%s: raises to $%d (pot now: $%d)", data.Player, data.Amount, data.PotAfter)
	case "allin":
		actionEntry = fmt.Sprintf("%s: goes all-in for $%d", data.Player, data.Amount)
	default:
		actionEntry = fmt.Sprintf("%s: %s", data.Player, data.Action)
	}

	na.tui.AddLogEntry(actionEntry)
}

func (na *NetworkAgent) handleStreetChange(msg *server.Message) {
	var data server.StreetChangeData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		na.logger.Error("Failed to parse street change", "error", err)
		return
	}

	na.tui.UpdateCurrentBet(data.CurrentBet)
	na.tui.AddLogEntry("")

	switch data.Round {
	case "flop":
		na.tui.AddLogEntry("*** FLOP ***")
		if len(data.CommunityCards) >= 3 {
			flop := data.CommunityCards[:3]
			na.tui.AddLogEntry(fmt.Sprintf("Board: %s", na.tui.FormatCards(flop)))
		}
	case "turn":
		na.tui.AddLogEntry("*** TURN ***")
		if len(data.CommunityCards) >= 4 {
			na.tui.AddLogEntry(fmt.Sprintf("Board: %s [%s]",
				na.tui.FormatCards(data.CommunityCards[:3]),
				data.CommunityCards[3].String()))
		}
	case "river":
		na.tui.AddLogEntry("*** RIVER ***")
		if len(data.CommunityCards) >= 5 {
			na.tui.AddLogEntry(fmt.Sprintf("Board: %s [%s]",
				na.tui.FormatCards(data.CommunityCards[:4]),
				data.CommunityCards[4].String()))
		}
	case "showdown":
		na.tui.AddLogEntry("*** SHOWDOWN ***")
		na.tui.AddLogEntry(fmt.Sprintf("Final Board: %s", na.tui.FormatCards(data.CommunityCards)))
	}

	na.tui.AddLogEntry("")
}

func (na *NetworkAgent) handleHandEnd(msg *server.Message) {
	var data server.HandEndData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		na.logger.Error("Failed to parse hand end", "error", err)
		return
	}

	na.tui.AddLogEntry("")

	// Use the server's summary if available
	if data.Summary != "" {
		lines := make([]string, 0)
		for _, line := range lines {
			if line != "" {
				na.tui.AddLogEntry(line)
			}
		}
	} else {
		// Fallback summary
		na.tui.AddLogEntry(fmt.Sprintf("=== Hand %s Complete ===", data.HandID))
		na.tui.AddLogEntry(fmt.Sprintf("Pot: $%d", data.PotSize))
		for _, winner := range data.Winners {
			na.tui.AddLogEntry(fmt.Sprintf("Winner: %s ($%d) - %s", winner.PlayerName, winner.Amount, winner.HandRank))
		}
	}

	na.tui.AddLogEntry("")
}

func (na *NetworkAgent) handleActionRequired(msg *server.Message) {
	var data server.ActionRequiredData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		na.logger.Error("Failed to parse action required", "error", err)
		return
	}

	// Convert server data to game types
	validActions := make([]game.ValidAction, len(data.ValidActions))
	for i, va := range data.ValidActions {
		action := parseActionFromString(va.Action)
		validActions[i] = game.ValidAction{
			Action:    action,
			MinAmount: va.MinAmount,
			MaxAmount: va.MaxAmount,
		}
	}

	// Create player object for TUI
	var humanPlayer *game.Player
	if data.TableState.ActingPlayerIdx >= 0 && data.TableState.ActingPlayerIdx < len(data.TableState.Players) {
		playerState := data.TableState.Players[data.TableState.ActingPlayerIdx]
		humanPlayer = &game.Player{
			Name:         playerState.Name,
			Chips:        playerState.Chips,
			Position:     parsePositionFromString(playerState.Position),
			BetThisRound: playerState.BetThisRound,
			TotalBet:     playerState.TotalBet,
			HoleCards:    playerState.HoleCards,
			IsActive:     playerState.IsActive,
			IsFolded:     playerState.IsFolded,
			IsAllIn:      playerState.IsAllIn,
			LastAction:   parseActionFromString(playerState.LastAction),
		}
	}

	// Update TUI state
	na.tui.UpdateValidActions(validActions)
	na.tui.UpdatePot(data.TableState.Pot)
	na.tui.UpdateCurrentBet(data.TableState.CurrentBet)
	na.tui.SetHumanTurn(true, humanPlayer)

	// Wait for user input
	go na.waitForUserDecision(data.TimeoutSeconds)
}

func (na *NetworkAgent) waitForUserDecision(timeoutSeconds int) {
	for {
		action, args, shouldContinue, err := na.tui.WaitForAction()
		if err != nil {
			na.logger.Error("Error waiting for action", "error", err)
			continue
		}

		if !shouldContinue {
			// User wants to quit
			na.client.SendDecision("fold", 0, "Player quit")
			return
		}

		// Process the action
		decision := na.processUserAction(action, args)
		if decision.Reasoning == "Invalid input - try again" {
			continue
		}

		// Send decision to server
		err = na.client.SendDecision(decision.Action.String(), decision.Amount, decision.Reasoning)
		if err != nil {
			na.logger.Error("Failed to send decision", "error", err)
			na.tui.AddLogEntry(fmt.Sprintf("Error sending action: %s", err.Error()))
			continue
		}

		// Clear human turn
		na.tui.SetHumanTurn(false, nil)
		break
	}
}

func (na *NetworkAgent) processUserAction(action string, args []string) game.Decision {
	switch action {
	case "f", "fold":
		return game.Decision{Action: game.Fold, Amount: 0, Reasoning: "Player folded"}
	case "c", "call":
		return game.Decision{Action: game.Call, Amount: 0, Reasoning: "Player called"}
	case "k", "check":
		return game.Decision{Action: game.Check, Amount: 0, Reasoning: "Player checked"}
	case "r", "raise":
		if len(args) == 0 {
			na.tui.AddLogEntry("Error: Specify raise amount: 'raise <amount>'")
			return game.Decision{Action: game.Check, Amount: 0, Reasoning: "Invalid input - try again"}
		}
		amount, err := strconv.Atoi(args[0])
		if err != nil {
			na.tui.AddLogEntry(fmt.Sprintf("Error: Invalid amount: %s", args[0]))
			return game.Decision{Action: game.Check, Amount: 0, Reasoning: "Invalid input - try again"}
		}
		return game.Decision{Action: game.Raise, Amount: amount, Reasoning: "Player raised"}
	case "a", "allin", "all":
		return game.Decision{Action: game.AllIn, Amount: 0, Reasoning: "Player went all-in"}
	default:
		na.tui.AddLogEntry(fmt.Sprintf("Unknown action: %s", action))
		return game.Decision{Action: game.Check, Amount: 0, Reasoning: "Invalid input - try again"}
	}
}

func (na *NetworkAgent) handleError(msg *server.Message) {
	var data server.ErrorData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		na.logger.Error("Failed to parse error message", "error", err)
		return
	}

	na.tui.AddLogEntry(fmt.Sprintf("Server error [%s]: %s", data.Code, data.Message))
}

// Helper functions to convert between string and enum types

func parseActionFromString(actionStr string) game.Action {
	switch actionStr {
	case "fold":
		return game.Fold
	case "call":
		return game.Call
	case "check":
		return game.Check
	case "raise":
		return game.Raise
	case "allin":
		return game.AllIn
	default:
		return game.Fold
	}
}

func parsePositionFromString(posStr string) game.Position {
	switch posStr {
	case "small_blind":
		return game.SmallBlind
	case "big_blind":
		return game.BigBlind
	case "under_the_gun":
		return game.UnderTheGun
	case "middle_position":
		return game.MiddlePosition
	case "cutoff":
		return game.Cutoff
	case "button":
		return game.Button
	default:
		return game.MiddlePosition
	}
}
