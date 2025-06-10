package tui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/lox/pokerforbots/internal/client"
	"github.com/lox/pokerforbots/internal/deck"
	"github.com/lox/pokerforbots/internal/game"
	"github.com/lox/pokerforbots/internal/server"
)

// Bridge manages the connection between a client and TUI model
type Bridge struct {
	client       *client.Client
	tui          *TUIModel
	defaultBuyIn int
}

// NewBridge creates a new bridge between client and TUI
func NewBridge(client *client.Client, tui *TUIModel, defaultBuyIn int) *Bridge {
	bridge := &Bridge{
		client:       client,
		tui:          tui,
		defaultBuyIn: defaultBuyIn,
	}

	bridge.setupEventHandlers()
	return bridge
}

// Start begins the command handling loop (non-blocking)
func (b *Bridge) Start() {
	go b.commandLoop()
}

// setupEventHandlers configures all client event handlers
func (b *Bridge) setupEventHandlers() {
	b.client.AddEventHandler(server.MessageTypeHandStart, b.handleHandStart)
	b.client.AddEventHandler(server.MessageTypePlayerAction, b.handlePlayerAction)
	b.client.AddEventHandler(server.MessageTypeStreetChange, b.handleStreetChange)
	b.client.AddEventHandler(server.MessageTypeHandEnd, b.handleHandEnd)
	b.client.AddEventHandler(server.MessageTypeActionRequired, b.handleActionRequired)
	b.client.AddEventHandler(server.MessageTypeTableList, b.handleTableList)
	b.client.AddEventHandler(server.MessageTypeTableJoined, b.handleTableJoined)
	b.client.AddEventHandler(server.MessageTypeTableLeft, b.handleTableLeft)
	b.client.AddEventHandler(server.MessageTypeBotAdded, b.handleBotAdded)
	b.client.AddEventHandler(server.MessageTypeBotKicked, b.handleBotKicked)
	b.client.AddEventHandler(server.MessageTypeAuthResponse, b.handleAuthResponse)
	b.client.AddEventHandler(server.MessageTypeError, b.handleError)
	b.client.AddEventHandler(server.MessageTypePlayerTimeout, b.handlePlayerTimeout)
	b.client.AddEventHandler(server.MessageTypeGamePause, b.handleGamePause)
}

// commandLoop handles user actions from the TUI
func (b *Bridge) commandLoop() {
	for {
		action, args, shouldContinue, err := b.tui.WaitForAction()
		if err != nil {
			continue
		}

		if !shouldContinue {
			break
		}

		// Handle special commands
		if strings.HasPrefix(action, "/") || action == "quit" {
			b.handleCommand(action)
		} else {
			// Handle game actions (when it's the player's turn)
			b.handleGameAction(action, args)
		}
	}
}

// Event handlers
func (b *Bridge) handleHandStart(msg *server.Message) {
	var data server.HandStartData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}

	// Use EventFormatter for consistent hand start formatting
	event := game.HandStartEvent{
		HandID:     data.HandID,
		Players:    convertPlayersFromServer(data.Players),
		SmallBlind: data.SmallBlind,
		BigBlind:   data.BigBlind,
	}
	formatter := game.NewEventFormatter(game.FormattingOptions{})
	handStartText := formatter.FormatHandStart(event)
	b.tui.AddLogEntryAndScrollToShow(handStartText)
	b.tui.AddLogEntry("")
	b.tui.AddLogEntry("*** HOLE CARDS ***")

	// Show hole cards if we have them
	for _, player := range data.Players {
		if player.Name == b.client.GetPlayerName() && len(player.HoleCards) > 0 {
			cards := formatCards(player.HoleCards)
			b.tui.AddLogEntry(fmt.Sprintf("Dealt to You: %s", cards))
			break
		}
	}

	// Show blind posting
	for _, player := range data.Players {
		if player.BetThisRound > 0 {
			switch player.Position {
			case "Small Blind":
				b.tui.AddLogEntry(fmt.Sprintf("%s: posts small blind $%d", player.Name, player.BetThisRound))
			case "Big Blind":
				b.tui.AddLogEntry(fmt.Sprintf("%s: posts big blind $%d", player.Name, player.BetThisRound))
			}
		}
	}

	b.tui.AddLogEntry("")
	b.tui.AddLogEntry("*** PRE-FLOP ***")
	b.tui.UpdatePot(data.InitialPot)

	// Update sidebar with game state and players
	b.tui.UpdateGameState("Pre-flop", []deck.Card{}, "")

	// Convert players to PlayerInfo with positions
	var players []PlayerInfo
	for _, player := range data.Players {
		position := ""
		switch player.Position {
		case "Small Blind":
			position = "SB"
		case "Big Blind":
			position = "BB"
		case "Button":
			position = "D"
		}

		players = append(players, PlayerInfo{
			Name:       player.Name,
			Chips:      player.Chips,
			Position:   position,
			SeatNumber: player.SeatNumber,
			IsActive:   true,
		})
	}
	b.tui.SetTableInfo(b.tui.tableID, b.tui.seatNumber, players)

	// Notify test callback if in test mode
	b.tui.notifyMessageCallback(server.MessageTypeHandStart)
}

func (b *Bridge) handlePlayerAction(msg *server.Message) {
	var data server.PlayerActionData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}

	// Debug logging for player actions
	b.tui.logger.Info("EVENT player_action",
		"player", data.Player,
		"action", data.Action,
		"amount", data.Amount,
		"potAfter", data.PotAfter,
		"round", data.Round,
		"reasoning", data.Reasoning)

	b.tui.UpdatePot(data.PotAfter)

	// Create a game event from the server data
	player := &game.Player{
		Name:     data.Player,
		Position: game.UnknownPosition, // Position not available in server message
	}

	event := game.PlayerActionEvent{
		Player:    player,
		Action:    parseActionFromString(data.Action),
		Amount:    data.Amount,
		Round:     parseRoundFromString(data.Round),
		Reasoning: data.Reasoning,
		PotAfter:  data.PotAfter,
	}

	// Use EventFormatter to format the action
	formatter := game.NewEventFormatter(game.FormattingOptions{
		ShowTimeouts: true, // TUI shows timeout information
	})

	actionEntry := formatter.FormatPlayerAction(event)
	b.tui.AddLogEntry(actionEntry)

	// Notify test callback if in test mode
	b.tui.notifyMessageCallback(server.MessageTypePlayerAction)
}

func (b *Bridge) handleStreetChange(msg *server.Message) {
	var data server.StreetChangeData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}

	// Debug logging for street change
	b.tui.logger.Info("EVENT street_change",
		"round", data.Round,
		"currentBet", data.CurrentBet,
		"communityCards", len(data.CommunityCards))

	b.tui.UpdateCurrentBet(data.CurrentBet)

	// Create a game event from the server data
	event := game.StreetChangeEvent{
		Round:          parseRoundFromString(data.Round),
		CommunityCards: data.CommunityCards,
		CurrentBet:     data.CurrentBet,
	}

	// Use EventFormatter to format the street change
	formatter := game.NewEventFormatter(game.FormattingOptions{})
	streetText := formatter.FormatStreetChange(event)
	b.tui.AddLogEntry(streetText)

	// Update sidebar with new round and community cards
	b.tui.UpdateGameState(data.Round, data.CommunityCards, "")

	// Notify test callback if in test mode
	b.tui.notifyMessageCallback(server.MessageTypeStreetChange)
}

func (b *Bridge) handleHandEnd(msg *server.Message) {
	var data server.HandEndData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}

	b.tui.AddLogEntry("")
	b.tui.AddLogEntry(fmt.Sprintf("=== Hand %s Complete ===", data.HandID))
	b.tui.AddLogEntry(fmt.Sprintf("Pot: $%d", data.PotSize))
	for _, winner := range data.Winners {
		b.tui.AddLogEntry(fmt.Sprintf("Winner: %s ($%d) - %s", winner.PlayerName, winner.Amount, winner.HandRank))
	}
	b.tui.AddLogEntry("")

	// Notify test callback if in test mode
	b.tui.notifyMessageCallback(server.MessageTypeHandEnd)
}

func (b *Bridge) handleActionRequired(msg *server.Message) {
	var data server.ActionRequiredData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}

	// Debug logging for action required
	b.tui.logger.Info("EVENT action_required",
		"player", data.PlayerName,
		"currentBet", data.TableState.CurrentBet,
		"pot", data.TableState.Pot,
		"validActions", len(data.ValidActions))

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
	b.tui.UpdateValidActions(validActions)
	b.tui.UpdatePot(data.TableState.Pot)
	b.tui.UpdateCurrentBet(data.TableState.CurrentBet)
	b.tui.SetHumanTurn(true, humanPlayer)

	// Update current player in sidebar
	b.tui.UpdateGameState(b.tui.currentRound, b.tui.communityCards, data.PlayerName)

	// Debug log the updated state and valid actions
	b.tui.logger.Info("Updated TUI state",
		"pot", data.TableState.Pot,
		"currentBet", data.TableState.CurrentBet,
		"humanPlayer.BetThisRound", humanPlayer.BetThisRound)
	for i, action := range validActions {
		b.tui.logger.Info("ValidAction",
			"index", i,
			"action", action.Action.String(),
			"minAmount", action.MinAmount,
			"maxAmount", action.MaxAmount)
	}

	// Notify test callback if in test mode
	b.tui.notifyMessageCallback(server.MessageTypeActionRequired)
}

func (b *Bridge) handleTableList(msg *server.Message) {
	var data server.TableListData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}

	b.tui.AddLogEntry("")
	b.tui.AddLogEntry("Available tables:")
	for _, table := range data.Tables {
		b.tui.AddLogEntry(fmt.Sprintf("  %s: %s (%d/%d players, stakes %s)",
			table.ID, table.Name, table.PlayerCount, table.MaxPlayers, table.Stakes))
	}
}

func (b *Bridge) handleTableJoined(msg *server.Message) {
	var data server.TableJoinedData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}

	b.client.SetTableID(data.TableID)

	// Add bold table joined message at the top
	b.tui.AddBoldLogEntry(fmt.Sprintf("Joined table %s (seat %d)", data.TableID, data.SeatNumber))

	// Convert players to PlayerInfo and set in sidebar
	var players []PlayerInfo
	for _, player := range data.Players {
		players = append(players, PlayerInfo{
			Name:       player.Name,
			Chips:      player.Chips,
			Position:   "", // Will be updated in hand_start
			SeatNumber: player.SeatNumber,
			IsActive:   true,
		})
	}
	b.tui.SetTableInfo(data.TableID, data.SeatNumber, players)
}

func (b *Bridge) handleTableLeft(msg *server.Message) {
	b.client.SetTableID("")
	b.tui.AddLogEntry("Left table")
}

func (b *Bridge) handleBotAdded(msg *server.Message) {
	var data server.BotAddedData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}

	botList := strings.Join(data.BotNames, ", ")
	b.tui.AddLogEntry(fmt.Sprintf("Added bots: %s", botList))
}

func (b *Bridge) handleBotKicked(msg *server.Message) {
	var data server.BotKickedData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}

	b.tui.AddLogEntry(fmt.Sprintf("Kicked bot: %s", data.BotName))
}

func (b *Bridge) handleAuthResponse(msg *server.Message) {
	var data server.AuthResponseData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}

	if !data.Success {
		b.tui.AddLogEntry(fmt.Sprintf("Authentication failed: %s", data.Error))
	}
}

func (b *Bridge) handleError(msg *server.Message) {
	var data server.ErrorData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}

	b.tui.AddLogEntry(fmt.Sprintf("Server error [%s]: %s", data.Code, data.Message))
}

func (b *Bridge) handlePlayerTimeout(msg *server.Message) {
	var data server.PlayerTimeoutData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}

	b.tui.AddLogEntry(fmt.Sprintf("⏰ %s timed out (%ds) and %ss",
		data.PlayerName, data.TimeoutSeconds, data.Action))

	// Notify test callback if in test mode
	b.tui.notifyMessageCallback(server.MessageTypePlayerTimeout)
}

func (b *Bridge) handleGamePause(msg *server.Message) {
	var data server.GamePauseData
	if err := json.Unmarshal(msg.Data, &data); err != nil {
		return
	}

	b.tui.AddLogEntry(fmt.Sprintf("⏸️  Game paused: %s", data.Message))

	// Notify test callback if in test mode
	b.tui.notifyMessageCallback(server.MessageTypeGamePause)
}

// Command handlers
func (b *Bridge) handleCommand(action string) {
	switch action {
	case "/leave":
		tableID := b.client.GetTableID()
		if tableID == "" {
			b.tui.AddLogEntry("You're not at a table")
			return
		}

		err := b.client.LeaveTable(tableID)
		if err != nil {
			b.tui.AddLogEntry(fmt.Sprintf("Error leaving table: %v", err))
		}

	case "/back":
		// Send sit-in action to return from sitting out
		err := b.client.SendDecision("sit-in", 0, "Player returned from sitting out")
		if err != nil {
			b.tui.AddLogEntry(fmt.Sprintf("Error returning to play: %v", err))
		} else {
			b.tui.AddLogEntry("Returning to play - you'll be included in the next hand")
		}

	case "/quit", "quit":
		b.tui.SendQuitSignal()

	default:
		b.tui.AddLogEntry(fmt.Sprintf("Unknown command: %s", action))
		b.tui.AddLogEntry("Available commands: /leave, /quit, /back")
	}
}

func (b *Bridge) handleGameAction(action string, args []string) {
	// Process the action
	decision := processUserAction(action, args, b.tui)
	if decision.Reasoning == "Invalid input - try again" {
		return
	}

	// Send decision to server
	err := b.client.SendDecision(actionToNetworkString(decision.Action), decision.Amount, decision.Reasoning)
	if err != nil {
		b.tui.AddLogEntry(fmt.Sprintf("Error sending action: %s", err.Error()))
		return
	}

	// Clear human turn
	b.tui.SetHumanTurn(false, nil)
}

// processUserAction converts user input to game decision with validation
func processUserAction(action string, args []string, tui *TUIModel) game.Decision {
	// First, parse the intended action
	var intendedAction game.Action
	var amount int
	var reasoning string

	switch action {
	case "f", "fold":
		intendedAction = game.Fold
		reasoning = "Player folded"
	case "c", "call":
		intendedAction = game.Call
		reasoning = "Player called"
	case "k", "check":
		intendedAction = game.Check
		reasoning = "Player checked"
	case "r", "raise":
		if len(args) == 0 {
			tui.AddLogEntry("Error: Specify raise amount: 'raise <amount>'")
			return game.Decision{Action: game.Check, Amount: 0, Reasoning: "Invalid input - try again"}
		}
		var err error
		amount, err = strconv.Atoi(args[0])
		if err != nil {
			tui.AddLogEntry(fmt.Sprintf("Error: Invalid amount: %s", args[0]))
			return game.Decision{Action: game.Check, Amount: 0, Reasoning: "Invalid input - try again"}
		}
		intendedAction = game.Raise
		reasoning = "Player raised"
	case "a", "allin", "all":
		intendedAction = game.AllIn
		reasoning = "Player went all-in"
	default:
		tui.AddLogEntry(fmt.Sprintf("Unknown action: %s", action))
		return game.Decision{Action: game.Check, Amount: 0, Reasoning: "Invalid input - try again"}
	}

	// Validate the intended action against valid actions
	validAction := findValidAction(intendedAction, tui.validActions)
	if validAction == nil {
		// Action is not valid - show helpful error message
		validActionsStr := formatValidActions(tui.validActions)
		tui.AddLogEntry(fmt.Sprintf("Invalid action '%s'. Valid actions: %s", action, validActionsStr))
		return game.Decision{Action: game.Check, Amount: 0, Reasoning: "Invalid input - try again"}
	}

	// For raises, validate the amount is within valid range
	if intendedAction == game.Raise {
		if amount < validAction.MinAmount || amount > validAction.MaxAmount {
			tui.AddLogEntry(fmt.Sprintf("Invalid raise amount $%d. Must be between $%d and $%d",
				amount, validAction.MinAmount, validAction.MaxAmount))
			return game.Decision{Action: game.Check, Amount: 0, Reasoning: "Invalid input - try again"}
		}
	} else {
		// For non-raise actions, use the valid amount from the server
		amount = validAction.MinAmount
	}

	return game.Decision{Action: intendedAction, Amount: amount, Reasoning: reasoning}
}

// findValidAction searches for a valid action matching the requested action
func findValidAction(action game.Action, validActions []game.ValidAction) *game.ValidAction {
	for _, validAction := range validActions {
		if validAction.Action == action {
			return &validAction
		}
	}
	return nil
}

// formatValidActions creates a human-readable string of valid actions
func formatValidActions(validActions []game.ValidAction) string {
	if len(validActions) == 0 {
		return "none"
	}

	var actions []string
	for _, va := range validActions {
		switch va.Action {
		case game.Fold:
			actions = append(actions, "fold")
		case game.Call:
			actions = append(actions, fmt.Sprintf("call $%d", va.MinAmount))
		case game.Check:
			actions = append(actions, "check")
		case game.Raise:
			if va.MinAmount == va.MaxAmount {
				actions = append(actions, fmt.Sprintf("raise $%d", va.MinAmount))
			} else {
				actions = append(actions, fmt.Sprintf("raise $%d-$%d", va.MinAmount, va.MaxAmount))
			}
		case game.AllIn:
			actions = append(actions, fmt.Sprintf("allin $%d", va.MinAmount))
		}
	}
	return strings.Join(actions, ", ")
}

// actionToNetworkString converts game.Action to the string format expected by the network agent
func actionToNetworkString(action game.Action) string {
	switch action {
	case game.Fold:
		return "fold"
	case game.Call:
		return "call"
	case game.Check:
		return "check"
	case game.Raise:
		return "raise"
	case game.AllIn:
		return "allin" // Network expects "allin" not "all-in"
	default:
		return "check" // Default to check for invalid actions
	}
}

// Helper functions
func formatCards(cards []deck.Card) string {
	if len(cards) == 0 {
		return ""
	}

	var formatted []string
	for _, card := range cards {
		if card.IsRed() {
			formatted = append(formatted, fmt.Sprintf("\033[31m%s\033[0m", card.String()))
		} else {
			formatted = append(formatted, fmt.Sprintf("\033[30m%s\033[0m", card.String()))
		}
	}

	return "[" + strings.Join(formatted, " ") + "]"
}

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

func parseRoundFromString(roundStr string) game.BettingRound {
	switch roundStr {
	case "Pre-flop":
		return game.PreFlop
	case "Flop":
		return game.Flop
	case "Turn":
		return game.Turn
	case "River":
		return game.River
	case "Showdown":
		return game.Showdown
	default:
		return game.PreFlop
	}
}

func convertPlayersFromServer(serverPlayers []server.PlayerState) []*game.Player {
	players := make([]*game.Player, len(serverPlayers))
	for i, sp := range serverPlayers {
		players[i] = &game.Player{
			Name:         sp.Name,
			Chips:        sp.Chips,
			Position:     parsePositionFromString(sp.Position),
			BetThisRound: sp.BetThisRound,
			TotalBet:     sp.TotalBet,
			HoleCards:    sp.HoleCards,
			IsActive:     sp.IsActive,
			IsFolded:     sp.IsFolded,
			IsAllIn:      sp.IsAllIn,
			SeatNumber:   sp.SeatNumber,
		}
	}
	return players
}
