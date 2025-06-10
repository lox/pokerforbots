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

// SetupSimpleNetworkHandlers sets up direct event handlers between client and TUI
func SetupSimpleNetworkHandlers(client *client.Client, tui *TUIModel) {
	client.AddEventHandler(server.MessageTypeHandStart, func(msg *server.Message) {
		var data server.HandStartData
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return
		}

		tui.AddLogEntryAndScrollToShow(fmt.Sprintf("Hand %s • %d players • $%d/$%d",
			data.HandID, len(data.Players), data.SmallBlind, data.BigBlind))
		tui.AddLogEntry("")
		tui.AddLogEntry("*** HOLE CARDS ***")

		// Show hole cards if we have them
		for _, player := range data.Players {
			if player.Name == client.GetPlayerName() && len(player.HoleCards) > 0 {
				cards := formatCards(player.HoleCards)
				tui.AddLogEntry(fmt.Sprintf("Dealt to You: %s", cards))
				break
			}
		}

		// Show blind posting
		for _, player := range data.Players {
			if player.BetThisRound > 0 {
				switch player.Position {
				case "Small Blind":
					tui.AddLogEntry(fmt.Sprintf("%s: posts small blind $%d", player.Name, player.BetThisRound))
				case "Big Blind":
					tui.AddLogEntry(fmt.Sprintf("%s: posts big blind $%d", player.Name, player.BetThisRound))
				}
			}
		}

		tui.AddLogEntry("")
		tui.AddLogEntry("*** PRE-FLOP ***")
		tui.UpdatePot(data.InitialPot)

		// Update sidebar with game state and players
		tui.UpdateGameState("Pre-flop", []deck.Card{}, "")

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
		tui.SetTableInfo(tui.tableID, tui.seatNumber, players)

		// Notify test callback if in test mode
		tui.notifyMessageCallback(server.MessageTypeHandStart)
	})

	client.AddEventHandler(server.MessageTypePlayerAction, func(msg *server.Message) {
		var data server.PlayerActionData
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return
		}

		// Debug logging for player actions
		tui.logger.Info("EVENT player_action",
			"player", data.Player,
			"action", data.Action,
			"amount", data.Amount,
			"potAfter", data.PotAfter,
			"round", data.Round,
			"reasoning", data.Reasoning)

		tui.UpdatePot(data.PotAfter)

		var actionEntry string
		isTimeout := strings.Contains(data.Reasoning, "timeout") || strings.Contains(data.Reasoning, "Decision timeout")

		switch data.Action {
		case "fold":
			if isTimeout {
				actionEntry = fmt.Sprintf("%s: times out and folds", data.Player)
			} else {
				actionEntry = fmt.Sprintf("%s: folds", data.Player)
			}
		case "call":
			actionEntry = fmt.Sprintf("%s: calls $%d (pot now: $%d)", data.Player, data.Amount, data.PotAfter)
		case "check":
			if isTimeout {
				actionEntry = fmt.Sprintf("%s: times out and checks", data.Player)
			} else {
				actionEntry = fmt.Sprintf("%s: checks", data.Player)
			}
		case "raise":
			actionEntry = fmt.Sprintf("%s: raises by $%d (pot now: $%d)", data.Player, data.Amount, data.PotAfter)
		case "allin":
			actionEntry = fmt.Sprintf("%s: goes all-in for $%d", data.Player, data.Amount)
		default:
			actionEntry = fmt.Sprintf("%s: %s", data.Player, data.Action)
		}

		tui.AddLogEntry(actionEntry)

		// Notify test callback if in test mode
		tui.notifyMessageCallback(server.MessageTypePlayerAction)
	})

	client.AddEventHandler(server.MessageTypeStreetChange, func(msg *server.Message) {
		var data server.StreetChangeData
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return
		}

		// Debug logging for street change
		tui.logger.Info("EVENT street_change",
			"round", data.Round,
			"currentBet", data.CurrentBet,
			"communityCards", len(data.CommunityCards))

		tui.UpdateCurrentBet(data.CurrentBet)

		switch data.Round {
		case "Flop":
			tui.AddLogEntry("*** FLOP ***")
			if len(data.CommunityCards) >= 3 {
				flop := data.CommunityCards[:3]
				tui.AddLogEntry(fmt.Sprintf("Board: %s", formatCards(flop)))
			}
		case "Turn":
			tui.AddLogEntry("*** TURN ***")
			if len(data.CommunityCards) >= 4 {
				tui.AddLogEntry(fmt.Sprintf("Board: %s [%s]",
					formatCards(data.CommunityCards[:3]),
					data.CommunityCards[3].String()))
			}
		case "River":
			tui.AddLogEntry("*** RIVER ***")
			if len(data.CommunityCards) >= 5 {
				tui.AddLogEntry(fmt.Sprintf("Board: %s [%s]",
					formatCards(data.CommunityCards[:4]),
					data.CommunityCards[4].String()))
			}
		case "showdown":
			tui.AddLogEntry("*** SHOWDOWN ***")
			tui.AddLogEntry(fmt.Sprintf("Final Board: %s", formatCards(data.CommunityCards)))
		}

		// Update sidebar with new round and community cards
		tui.UpdateGameState(data.Round, data.CommunityCards, "")

		// Notify test callback if in test mode
		tui.notifyMessageCallback(server.MessageTypeStreetChange)
	})

	// Add handlers for other events
	client.AddEventHandler(server.MessageTypeHandEnd, func(msg *server.Message) {
		var data server.HandEndData
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return
		}

		tui.AddLogEntry("")
		tui.AddLogEntry(fmt.Sprintf("=== Hand %s Complete ===", data.HandID))
		tui.AddLogEntry(fmt.Sprintf("Pot: $%d", data.PotSize))
		for _, winner := range data.Winners {
			tui.AddLogEntry(fmt.Sprintf("Winner: %s ($%d) - %s", winner.PlayerName, winner.Amount, winner.HandRank))
		}
		tui.AddLogEntry("")

		// Notify test callback if in test mode
		tui.notifyMessageCallback(server.MessageTypeHandEnd)
	})

	client.AddEventHandler(server.MessageTypeActionRequired, func(msg *server.Message) {
		var data server.ActionRequiredData
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return
		}

		// Debug logging for action required
		tui.logger.Info("EVENT action_required",
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
		tui.UpdateValidActions(validActions)
		tui.UpdatePot(data.TableState.Pot)
		tui.UpdateCurrentBet(data.TableState.CurrentBet)
		tui.SetHumanTurn(true, humanPlayer)

		// Update current player in sidebar
		tui.UpdateGameState(tui.currentRound, tui.communityCards, data.PlayerName)

		// Debug log the updated state and valid actions
		tui.logger.Info("Updated TUI state",
			"pot", data.TableState.Pot,
			"currentBet", data.TableState.CurrentBet,
			"humanPlayer.BetThisRound", humanPlayer.BetThisRound)
		for i, action := range validActions {
			tui.logger.Info("ValidAction",
				"index", i,
				"action", action.Action.String(),
				"minAmount", action.MinAmount,
				"maxAmount", action.MaxAmount)
		}

		// Player's turn is now active, actions will be handled by the main command loop

		// Notify test callback if in test mode
		tui.notifyMessageCallback(server.MessageTypeActionRequired)
	})

	// Simple handlers for other events
	client.AddEventHandler(server.MessageTypeTableList, func(msg *server.Message) {
		var data server.TableListData
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return
		}

		tui.AddLogEntry("")
		tui.AddLogEntry("Available tables:")
		for _, table := range data.Tables {
			tui.AddLogEntry(fmt.Sprintf("  %s: %s (%d/%d players, stakes %s)",
				table.ID, table.Name, table.PlayerCount, table.MaxPlayers, table.Stakes))
		}
	})

	client.AddEventHandler(server.MessageTypeTableJoined, func(msg *server.Message) {
		var data server.TableJoinedData
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return
		}

		client.SetTableID(data.TableID)

		// Add bold table joined message at the top
		tui.AddBoldLogEntry(fmt.Sprintf("Joined table %s (seat %d)", data.TableID, data.SeatNumber))

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
		tui.SetTableInfo(data.TableID, data.SeatNumber, players)
	})

	client.AddEventHandler(server.MessageTypeTableLeft, func(msg *server.Message) {
		client.SetTableID("")
		tui.AddLogEntry("Left table")
	})

	client.AddEventHandler(server.MessageTypeBotAdded, func(msg *server.Message) {
		var data server.BotAddedData
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return
		}

		botList := strings.Join(data.BotNames, ", ")
		tui.AddLogEntry(fmt.Sprintf("Added bots: %s", botList))
	})

	client.AddEventHandler(server.MessageTypeBotKicked, func(msg *server.Message) {
		var data server.BotKickedData
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return
		}

		tui.AddLogEntry(fmt.Sprintf("Kicked bot: %s", data.BotName))
	})

	client.AddEventHandler(server.MessageTypeAuthResponse, func(msg *server.Message) {
		var data server.AuthResponseData
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return
		}

		if !data.Success {
			tui.AddLogEntry(fmt.Sprintf("Authentication failed: %s", data.Error))
		}
	})

	client.AddEventHandler(server.MessageTypeError, func(msg *server.Message) {
		var data server.ErrorData
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return
		}

		tui.AddLogEntry(fmt.Sprintf("Server error [%s]: %s", data.Code, data.Message))
	})

	client.AddEventHandler(server.MessageTypePlayerTimeout, func(msg *server.Message) {
		var data server.PlayerTimeoutData
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return
		}

		tui.AddLogEntry(fmt.Sprintf("⏰ %s timed out (%ds) and %ss",
			data.PlayerName, data.TimeoutSeconds, data.Action))

		// Notify test callback if in test mode
		tui.notifyMessageCallback(server.MessageTypePlayerTimeout)
	})

	client.AddEventHandler(server.MessageTypeGamePause, func(msg *server.Message) {
		var data server.GamePauseData
		if err := json.Unmarshal(msg.Data, &data); err != nil {
			return
		}

		tui.AddLogEntry(fmt.Sprintf("⏸️  Game paused: %s", data.Message))

		// Notify test callback if in test mode
		tui.notifyMessageCallback(server.MessageTypeGamePause)
	})
}

// StartCommandHandler starts the command handling loop for the TUI
func StartCommandHandler(client *client.Client, tui *TUIModel, defaultBuyIn int) {
	go func() {
		for {
			action, args, shouldContinue, err := tui.WaitForAction()
			if err != nil {
				continue
			}

			if !shouldContinue {
				break
			}

			// Handle special commands
			if strings.HasPrefix(action, "/") || action == "quit" {
				handleCommand(client, tui, action)
			} else {
				// Handle game actions (when it's the player's turn)
				handleGameAction(client, tui, action, args)
			}
		}
	}()
}

// handleCommand processes simple TUI commands like /leave, /quit, /back
func handleCommand(client *client.Client, tui *TUIModel, action string) {
	switch action {
	case "/leave":
		tableID := client.GetTableID()
		if tableID == "" {
			tui.AddLogEntry("You're not at a table")
			return
		}

		err := client.LeaveTable(tableID)
		if err != nil {
			tui.AddLogEntry(fmt.Sprintf("Error leaving table: %v", err))
		}

	case "/back":
		// Send sit-in action to return from sitting out
		err := client.SendDecision("sit-in", 0, "Player returned from sitting out")
		if err != nil {
			tui.AddLogEntry(fmt.Sprintf("Error returning to play: %v", err))
		} else {
			tui.AddLogEntry("Returning to play - you'll be included in the next hand")
		}

	case "/quit", "quit":
		tui.SendQuitSignal()

	default:
		tui.AddLogEntry(fmt.Sprintf("Unknown command: %s", action))
		tui.AddLogEntry("Available commands: /leave, /quit, /back")
	}
}

// handleGameAction processes game actions when it's the player's turn
func handleGameAction(client *client.Client, tui *TUIModel, action string, args []string) {
	// Process the action
	decision := processUserAction(action, args, tui)
	if decision.Reasoning == "Invalid input - try again" {
		return
	}

	// Send decision to server
	err := client.SendDecision(actionToNetworkString(decision.Action), decision.Amount, decision.Reasoning)
	if err != nil {
		tui.AddLogEntry(fmt.Sprintf("Error sending action: %s", err.Error()))
		return
	}

	// Clear human turn
	tui.SetHumanTurn(false, nil)
}

// processUserAction converts user input to game decision
func processUserAction(action string, args []string, tui *TUIModel) game.Decision {
	switch action {
	case "f", "fold":
		return game.Decision{Action: game.Fold, Amount: 0, Reasoning: "Player folded"}
	case "c", "call":
		return game.Decision{Action: game.Call, Amount: 0, Reasoning: "Player called"}
	case "k", "check":
		return game.Decision{Action: game.Check, Amount: 0, Reasoning: "Player checked"}
	case "r", "raise":
		if len(args) == 0 {
			tui.AddLogEntry("Error: Specify raise amount: 'raise <amount>'")
			return game.Decision{Action: game.Check, Amount: 0, Reasoning: "Invalid input - try again"}
		}
		amount, err := strconv.Atoi(args[0])
		if err != nil {
			tui.AddLogEntry(fmt.Sprintf("Error: Invalid amount: %s", args[0]))
			return game.Decision{Action: game.Check, Amount: 0, Reasoning: "Invalid input - try again"}
		}
		return game.Decision{Action: game.Raise, Amount: amount, Reasoning: "Player raised"}
	case "a", "allin", "all":
		return game.Decision{Action: game.AllIn, Amount: 0, Reasoning: "Player went all-in"}
	default:
		tui.AddLogEntry(fmt.Sprintf("Unknown action: %s", action))
		return game.Decision{Action: game.Check, Amount: 0, Reasoning: "Invalid input - try again"}
	}
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
