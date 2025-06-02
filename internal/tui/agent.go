package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/lox/pokerforbots/internal/deck"
	"github.com/lox/pokerforbots/internal/game"
)

// TUIDisplayState holds local display state built from events
type TUIDisplayState struct {
	HandID         string
	SmallBlind     int
	BigBlind       int
	CurrentPot     int
	CurrentBet     int
	CommunityCards []deck.Card
	Players        []*game.Player
	ActivePlayers  []*game.Player
}

// TUIAgent handles human player interaction through a TUI
type TUIAgent struct {
	model                *TUIModel
	program              *tea.Program
	uiLogger, mainLogger *log.Logger

	// Local display state built from events (client-server ready)
	displayState *TUIDisplayState
}

// NewTUIAgent creates a new TUI-based agent
func NewTUIAgent(table *game.Table, logger *log.Logger) (*TUIAgent, error) {
	model := NewTUIModel(table, logger)
	program := tea.NewProgram(model, tea.WithAltScreen())

	return &TUIAgent{
		model:        model,
		program:      program,
		uiLogger:     logger.WithPrefix("ui"),
		mainLogger:   logger,
		displayState: &TUIDisplayState{},
	}, nil
}

// Start starts the TUI program
func (ti *TUIAgent) Start() error {
	go func() {
		if _, err := ti.program.Run(); err != nil {
			fmt.Printf("Error running TUI: %v\n", err)
		}
	}()
	return nil
}

// Close closes the TUI interface
func (ti *TUIAgent) Close() error {
	if ti.program != nil {
		ti.program.Quit()
		// Give the program a moment to clean up
		ti.program.Wait()

		// Only restore cursor, don't reset terminal completely
		fmt.Print("\033[?25h") // Show cursor
	}
	return nil
}

// MakeDecision implements the Agent interface for human players via TUI
func (ti *TUIAgent) MakeDecision(tableState game.TableState, validActions []game.ValidAction) game.Decision {
	// Update the TUI model with the valid actions from the engine
	ti.model.UpdateValidActions(validActions)

	// Set that it's the human's turn and provide the player info
	var humanPlayer *game.Player
	if tableState.ActingPlayerIdx >= 0 && tableState.ActingPlayerIdx < len(tableState.Players) {
		// Convert PlayerState back to Player for display (we need the full object)
		playerState := tableState.Players[tableState.ActingPlayerIdx]
		humanPlayer = &game.Player{
			Name:         playerState.Name,
			Chips:        playerState.Chips,
			Position:     playerState.Position,
			BetThisRound: playerState.BetThisRound,
			TotalBet:     playerState.TotalBet,
			HoleCards:    playerState.HoleCards,
			IsActive:     playerState.IsActive,
			IsFolded:     playerState.IsFolded,
			IsAllIn:      playerState.IsAllIn,
			LastAction:   playerState.LastAction,
		}
	}
	ti.model.SetHumanTurn(true, humanPlayer)

	for {
		ti.mainLogger.Info("Waiting for user action")
		action, args, shouldContinue, err := ti.model.WaitForAction()
		if err != nil {
			ti.mainLogger.Error("Error in WaitForAction", "error", err)
			ti.model.AddLogEntry(fmt.Sprintf("Error: %s", err.Error()))
			continue // Ask for input again
		}

		ti.mainLogger.Info("Received user action", "action", action, "args", args, "continue", shouldContinue)
		if !shouldContinue {
			ti.mainLogger.Info("User chose to quit")
			// Clear human turn before returning
			ti.model.SetHumanTurn(false, nil)
			return game.Decision{
				Action:    game.Fold,
				Amount:    0,
				Reasoning: "Player quit",
			}
		}

		// Process the action and return a decision
		decision := ti.processSimpleActionForDecision(action, args, tableState, validActions)

		// If we get an invalid action indicator, loop back for another try
		if decision.Reasoning == "Invalid input - try again" {
			continue
		}

		// Clear human turn before returning decision
		ti.model.SetHumanTurn(false, nil)
		return decision
	}
}

// processSimpleActionForDecision provides a simple decision based on user action and validActions
func (ti *TUIAgent) processSimpleActionForDecision(action string, args []string, tableState game.TableState, validActions []game.ValidAction) game.Decision {
	// Handle quit commands first
	if action == "quit" || action == "q" || action == "exit" {
		ti.mainLogger.Info("User typed quit command")
		// Clear human turn before returning
		ti.model.SetHumanTurn(false, nil)
		return game.Decision{
			Action:    game.Quit,
			Amount:    0,
			Reasoning: "Player quit",
		}
	}

	// Use the proper decision handlers
	// Extract current player info from tableState
	if tableState.ActingPlayerIdx < 0 || tableState.ActingPlayerIdx >= len(tableState.Players) {
		return game.Decision{Action: game.Fold, Amount: 0, Reasoning: "Invalid player index"}
	}
	currentPlayer := tableState.Players[tableState.ActingPlayerIdx]

	switch action {
	case "f", "fold":
		return ti.handleFoldForDecision(args, tableState, currentPlayer)
	case "c", "call":
		return ti.handleCallForDecision(args, tableState, currentPlayer)
	case "k", "check":
		return ti.handleCheckForDecision(args, tableState, currentPlayer)
	case "r", "raise":
		return ti.handleRaiseForDecision(args, tableState, currentPlayer)
	case "a", "allin", "all":
		return ti.handleAllInForDecision(args, tableState, currentPlayer)
	}

	// If action not recognized, ask again
	return game.Decision{Action: game.Fold, Amount: 0, Reasoning: "Invalid input - try again"}
}

// Decision-returning versions of action handlers for Agent interface

func (ti *TUIAgent) handleCallForDecision(args []string, tableState game.TableState, currentPlayerState game.PlayerState) game.Decision {
	if tableState.CurrentBet == 0 {
		ti.model.AddLogEntry("Error: No bet to call, use 'check' instead")
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: "No bet to call",
		}
	}

	callAmount := tableState.CurrentBet - currentPlayerState.BetThisRound
	if callAmount <= 0 {
		ti.model.AddLogEntry("Error: You have already called")
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: "Already called",
		}
	}

	if currentPlayerState.Chips < callAmount {
		ti.model.AddLogEntry("Error: Insufficient chips to call")
		return game.Decision{
			Action:    game.Fold,
			Amount:    0,
			Reasoning: "Insufficient chips to call",
		}
	}

	// Don't mutate state - just return the decision for the engine to apply
	return game.Decision{
		Action:    game.Call,
		Amount:    0, // Engine will calculate the actual amount
		Reasoning: "Player called",
	}
}

func (ti *TUIAgent) handleRaiseForDecision(args []string, tableState game.TableState, currentPlayerState game.PlayerState) game.Decision {
	if len(args) == 0 {
		ti.model.AddLogEntry("Error: Specify raise amount: 'raise <amount>'")
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: "Invalid input - try again",
		}
	}

	amount, err := strconv.Atoi(args[0])
	if err != nil {
		ti.model.AddLogEntry(fmt.Sprintf("Error: Invalid amount: %s", args[0]))
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: "Invalid input - try again",
		}
	}

	if amount <= tableState.CurrentBet {
		ti.model.AddLogEntry(fmt.Sprintf("Error: Raise must be more than current bet of $%d", tableState.CurrentBet))
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: "Invalid input - try again",
		}
	}

	totalNeeded := amount - currentPlayerState.BetThisRound
	if totalNeeded > currentPlayerState.Chips {
		ti.model.AddLogEntry(fmt.Sprintf("Error: Insufficient chips, you have $%d", currentPlayerState.Chips))
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: "Invalid input - try again",
		}
	}

	// Don't mutate state - just return the decision for the engine to apply
	return game.Decision{
		Action:    game.Raise,
		Amount:    amount, // Total bet amount (not the raise amount)
		Reasoning: "Player raised",
	}
}

func (ti *TUIAgent) handleFoldForDecision(args []string, tableState game.TableState, currentPlayerState game.PlayerState) game.Decision {
	// Don't mutate state - just return the decision for the engine to apply
	return game.Decision{
		Action:    game.Fold,
		Amount:    0,
		Reasoning: "Player folded",
	}
}

func (ti *TUIAgent) handleCheckForDecision(args []string, tableState game.TableState, currentPlayerState game.PlayerState) game.Decision {
	if tableState.CurrentBet > currentPlayerState.BetThisRound {
		ti.model.AddLogEntry(fmt.Sprintf("Error: Cannot check, current bet is $%d, use 'call' or 'fold'", tableState.CurrentBet))
		return game.Decision{
			Action:    game.Fold,
			Amount:    0,
			Reasoning: "Cannot check with outstanding bet",
		}
	}

	// Don't mutate state - just return the decision for the engine to apply
	return game.Decision{
		Action:    game.Check,
		Amount:    0,
		Reasoning: "Player checked",
	}
}

func (ti *TUIAgent) handleAllInForDecision(args []string, tableState game.TableState, currentPlayerState game.PlayerState) game.Decision {
	if currentPlayerState.Chips == 0 {
		ti.model.AddLogEntry("Error: No chips to go all-in")
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: "No chips to go all-in",
		}
	}

	// Don't mutate state - just return the decision for the engine to apply
	// Engine will calculate the correct all-in amount
	return game.Decision{
		Action:    game.AllIn,
		Amount:    currentPlayerState.BetThisRound + currentPlayerState.Chips, // Total bet amount after all-in
		Reasoning: "Player went all-in",
	}
}

// stripANSI removes ANSI escape sequences from a string
func stripANSI(s string) string {
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return ansiRegex.ReplaceAllString(s, "")
}

// AddLogEntry adds an entry to the game log
func (ti *TUIAgent) AddLogEntry(entry string) {
	ti.model.AddLogEntry(entry)
	// Also log to file for complete history (strip ANSI codes)
	cleanEntry := stripANSI(entry)
	ti.uiLogger.Info(cleanEntry)
}

// AddLogEntryAndScrollToShow adds an entry and scrolls to show it at the top
func (ti *TUIAgent) AddLogEntryAndScrollToShow(entry string) {
	ti.model.AddLogEntryAndScrollToShow(entry)
	// Also log to file for complete history (strip ANSI codes)
	cleanEntry := stripANSI(entry)
	ti.uiLogger.Info(cleanEntry)
}

// ClearLog clears the game log display
func (ti *TUIAgent) ClearLog() {
	ti.model.ClearLog()
}

// InitializeHand initializes the display for a new hand
func (ti *TUIAgent) InitializeHand(event game.HandStartEvent) {
	// Log game state context to file
	ti.mainLogger.Info("New hand started",
		"handID", ti.displayState.HandID,
		"players", len(event.Players),
		"smallBlind", event.SmallBlind,
		"bigBlind", event.BigBlind,
		"pot", event.InitialPot)

	// Log all players' hole cards for complete game history
	for _, player := range event.Players {
		if len(player.HoleCards) > 0 {
			ti.mainLogger.Info("Player hole cards",
				"player", player.Name,
				"type", map[game.PlayerType]string{game.Human: "Human", game.AI: "AI"}[player.Type],
				"position", player.Position.String(),
				"chips", player.Chips,
				"cards", fmt.Sprintf("%s %s", player.HoleCards[0].String(), player.HoleCards[1].String()))
		}
	}

	// Add hand header (no separator for first hand)
	ti.AddLogEntryAndScrollToShow(fmt.Sprintf("Hand %s • %d players • $1/$2", ti.displayState.HandID, len(event.Players)))
	ti.AddLogEntry("")
	ti.AddLogEntry("*** HOLE CARDS ***")

	// Show hole cards for human player
	for _, player := range event.Players {
		if player.Type == game.Human && len(player.HoleCards) > 0 {
			cards := ti.model.formatCards(player.HoleCards)
			ti.AddLogEntry(fmt.Sprintf("Dealt to You: %s", cards))
			break
		}
	}

	// Show blind posting
	for _, player := range event.ActivePlayers {
		if player.Position == game.SmallBlind && player.BetThisRound > 0 {
			ti.AddLogEntry(fmt.Sprintf("%s: posts small blind $%d", player.Name, player.BetThisRound))
		} else if player.Position == game.BigBlind && player.BetThisRound > 0 {
			ti.AddLogEntry(fmt.Sprintf("%s: posts big blind $%d", player.Name, player.BetThisRound))
		}
	}

	// Show street
	ti.AddLogEntry("")
	ti.AddLogEntry("*** PRE-FLOP ***")
}

// ShowPlayerAction displays a player's action in the TUI
func (ti *TUIAgent) ShowPlayerAction(player *game.Player) {
	ti.ShowPlayerActionWithThinking(player, "")
}

// ShowPlayerActionWithThinking displays a player's action with optional AI thinking
func (ti *TUIAgent) ShowPlayerActionWithThinking(player *game.Player, thinking string) {
	action := player.LastAction.String()

	// Log detailed player action to file
	logArgs := []interface{}{
		"player", player.Name,
		"type", map[game.PlayerType]string{game.Human: "Human", game.AI: "AI"}[player.Type],
		"action", action,
		"position", player.Position.String(),
		"chipsBefore", player.Chips + player.ActionAmount, // Chips before this action
		"chipsAfter", player.Chips,
		"betThisRound", player.BetThisRound,
		"totalBet", player.TotalBet,
		"actionAmount", player.ActionAmount,
	}

	// Add hole cards for AI players in log (hidden from TUI)
	if player.Type == game.AI && len(player.HoleCards) > 0 {
		logArgs = append(logArgs, "holeCards", fmt.Sprintf("%s %s", player.HoleCards[0].String(), player.HoleCards[1].String()))
	}

	// Add AI thinking to log
	if thinking != "" {
		logArgs = append(logArgs, "thinking", thinking)
	}

	ti.mainLogger.Info("Player action", logArgs...)

	// Note: Actions are recorded in hand history by the game engine for all players

	var actionEntry string
	switch action {
	case "fold":
		actionEntry = fmt.Sprintf("%s: folds", player.Name)
	case "call":
		actionEntry = fmt.Sprintf("%s: calls $%d (pot now: $%d)", player.Name, player.ActionAmount, ti.displayState.CurrentPot)
	case "check":
		actionEntry = fmt.Sprintf("%s: checks", player.Name)
	case "raise":
		// For simplicity and accuracy, just show the total bet amount
		actionEntry = fmt.Sprintf("%s: raises to $%d (pot now: $%d)",
			player.Name, player.BetThisRound, ti.displayState.CurrentPot)
	case "allin":
		actionEntry = fmt.Sprintf("%s: goes all-in for $%d", player.Name, player.TotalBet)
	default:
		actionEntry = fmt.Sprintf("%s: %s", player.Name, action)
	}

	ti.AddLogEntry(actionEntry)
}

// OnEvent implements EventSubscriber interface
func (ti *TUIAgent) OnEvent(event game.GameEvent) {
	// Log all events for debugging
	ti.mainLogger.Info("Received event", "type", event.EventType(), "timestamp", event.Timestamp())

	switch e := event.(type) {
	case game.HandStartEvent:
		// Update local display state from event
		ti.displayState.HandID = e.HandID
		ti.displayState.Players = e.Players
		ti.displayState.ActivePlayers = e.ActivePlayers
		ti.displayState.SmallBlind = e.SmallBlind
		ti.displayState.BigBlind = e.BigBlind
		ti.displayState.CurrentPot = e.InitialPot
		ti.model.UpdatePot(e.InitialPot)
		ti.model.UpdateCurrentBet(0) // Start with no bet
		ti.InitializeHand(e)
	case game.HandEndEvent:
		// Show hand summary with detailed event data
		ti.mainLogger.Info("Processing HandEndEvent",
			"handID", e.HandID,
			"winners", len(e.Winners),
			"potSize", e.PotSize,
			"showdownType", e.ShowdownType,
			"finalBoard", len(e.FinalBoard),
			"summaryLength", len(e.Summary))

		// Log each winner in detail
		for i, winner := range e.Winners {
			ti.mainLogger.Info("HandEndEvent winner detail",
				"index", i,
				"name", winner.PlayerName,
				"amount", winner.Amount,
				"handRank", winner.HandRank,
				"holeCards", len(winner.HoleCards))
		}

		ti.ShowHandSummary(e)
	case game.PlayerActionEvent:
		// Update pot from event
		ti.displayState.CurrentPot = e.PotAfter
		ti.model.UpdatePot(e.PotAfter)

		// Show all player actions in the log for complete game history
		// For human players, don't show AI thinking reasoning
		if e.Player.Type == game.Human {
			ti.ShowPlayerActionWithThinking(e.Player, "")
		} else {
			// Show AI player actions with thinking
			ti.ShowPlayerActionWithThinking(e.Player, e.Reasoning)
		}
	case game.StreetChangeEvent:
		// Update display state from event
		ti.displayState.CommunityCards = e.CommunityCards
		ti.displayState.CurrentBet = e.CurrentBet
		ti.model.UpdateCurrentBet(e.CurrentBet)

		// Show transition using event data
		ti.ShowBettingRoundTransition(e)
	}
}

// ShowBettingRoundComplete shows when a betting round completes
func (ti *TUIAgent) ShowBettingRoundComplete() {
	activePlayers := 0
	for _, player := range ti.displayState.ActivePlayers {
		if player.IsInHand() {
			activePlayers++
		}
	}

	if activePlayers <= 1 {
		// Don't show "All players folded" - we'll show it in the summary
		return
	}

	// Note: Round name would come from events in a true client-server setup
}

// ShowBettingRoundTransition shows transition to new betting round
func (ti *TUIAgent) ShowBettingRoundTransition(event game.StreetChangeEvent) {
	// Log betting round transition to file
	ti.mainLogger.Info("Betting round transition",
		"round", event.Round.String(),
		"pot", ti.displayState.CurrentPot,
		"currentBet", event.CurrentBet)

	// Log community cards from event
	if len(event.CommunityCards) > 0 {
		cardStrings := make([]string, len(event.CommunityCards))
		for i, card := range event.CommunityCards {
			cardStrings[i] = card.String()
		}
		ti.mainLogger.Info("Community cards", "cards", cardStrings)
	}

	ti.AddLogEntry("")

	switch event.Round {
	case game.Flop:
		ti.AddLogEntry("*** FLOP ***")
		if len(event.CommunityCards) >= 3 {
			flop := event.CommunityCards[:3]
			ti.AddLogEntry(fmt.Sprintf("Board: %s", ti.model.formatCards(flop)))
		}
	case game.Turn:
		ti.AddLogEntry("*** TURN ***")
		if len(event.CommunityCards) >= 4 {
			ti.AddLogEntry(fmt.Sprintf("Board: %s [%s]",
				ti.model.formatCards(event.CommunityCards[:3]),
				event.CommunityCards[3].String()))
		}
	case game.River:
		ti.AddLogEntry("*** RIVER ***")
		if len(event.CommunityCards) >= 5 {
			ti.AddLogEntry(fmt.Sprintf("Board: %s [%s]",
				ti.model.formatCards(event.CommunityCards[:4]),
				event.CommunityCards[4].String()))
		}
	case game.Showdown:
		ti.AddLogEntry("*** SHOWDOWN ***")
		ti.AddLogEntry(fmt.Sprintf("Final Board: %s", ti.model.formatCards(event.CommunityCards)))
	}

	ti.AddLogEntry(fmt.Sprintf("Pot: $%d", ti.displayState.CurrentPot))
	ti.AddLogEntry("")
}

// ShowHandSummary shows the hand summary using the rich summary from HandHistory
func (ti *TUIAgent) ShowHandSummary(handEndEvent game.HandEndEvent) {
	// Log final hand state to file
	ti.logFinalHandState(handEndEvent.PotSize)

	// Use the beautiful summary from HandHistory
	ti.AddLogEntry("")

	// Split the summary into lines and add each one
	if handEndEvent.Summary != "" {
		lines := strings.Split(handEndEvent.Summary, "\n")
		for _, line := range lines {
			if line != "" {
				ti.AddLogEntry(line)
			}
		}
	} else {
		// Fallback if no summary provided
		ti.AddLogEntry(fmt.Sprintf("=== Hand %s Complete ===", ti.displayState.HandID))
		ti.AddLogEntry(fmt.Sprintf("Pot: $%d", handEndEvent.PotSize))
		for _, winner := range handEndEvent.Winners {
			ti.AddLogEntry(fmt.Sprintf("Winner: %s ($%d)", winner.PlayerName, winner.Amount))
		}
	}

	ti.AddLogEntry("")
}

// logFinalHandState logs the final hand state to the log file
func (ti *TUIAgent) logFinalHandState(finalPot int) {
	ti.mainLogger.Info("=== HAND COMPLETE ===",
		"handID", ti.displayState.HandID,
		"finalPot", finalPot,
		"finalBet", ti.displayState.CurrentBet)

	// Log final board
	if len(ti.displayState.CommunityCards) > 0 {
		cardStrings := make([]string, len(ti.displayState.CommunityCards))
		for i, card := range ti.displayState.CommunityCards {
			cardStrings[i] = card.String()
		}
		ti.mainLogger.Info("Final board", "cards", cardStrings)
	}

	// Log all players' final state
	for _, player := range ti.displayState.Players {
		logArgs := []interface{}{
			"player", player.Name,
			"type", map[game.PlayerType]string{game.Human: "Human", game.AI: "AI"}[player.Type],
			"finalChips", player.Chips,
			"folded", player.IsFolded,
			"allIn", player.IsAllIn,
			"totalBet", player.TotalBet,
		}
		// Include hole cards for complete game record
		if len(player.HoleCards) > 0 {
			logArgs = append(logArgs, "holeCards", fmt.Sprintf("%s %s", player.HoleCards[0].String(), player.HoleCards[1].String()))
		}
		ti.mainLogger.Info("Final player state", logArgs...)
	}
}
