package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/lox/holdem-cli/internal/evaluator"
	"github.com/lox/holdem-cli/internal/game"
)

// TUIAgent handles human player interaction through a TUI
type TUIAgent struct {
	model                *TUIModel
	program              *tea.Program
	table                *game.Table
	uiLogger, mainLogger *log.Logger
}

// NewTUIAgent creates a new TUI-based agent
func NewTUIAgent(table *game.Table, logger *log.Logger) (*TUIAgent, error) {
	model := NewTUIModel(table, logger)
	program := tea.NewProgram(model, tea.WithAltScreen())

	return &TUIAgent{
		model:      model,
		program:    program,
		table:      table,
		uiLogger:   logger.WithPrefix("ui"),
		mainLogger: logger,
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

		// Ensure terminal is restored
		fmt.Print("\033[?25h") // Show cursor
		fmt.Print("\033c")     // Reset terminal
	}
	return nil
}

// MakeDecision implements the Agent interface for human players via TUI
func (ti *TUIAgent) MakeDecision(player *game.Player, table *game.Table) game.Decision {
	ti.mainLogger.Info("Waiting for user action")
	action, args, shouldContinue, err := ti.model.WaitForAction()
	if err != nil {
		ti.mainLogger.Error("Error in WaitForAction", "error", err)
		ti.model.AddLogEntry(fmt.Sprintf("Error: %s", err.Error()))
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: fmt.Sprintf("Error occurred: %v", err),
		}
	}

	ti.mainLogger.Info("Received user action", "action", action, "args", args, "continue", shouldContinue)
	if !shouldContinue {
		ti.mainLogger.Info("User chose to quit")
		return game.Decision{
			Action:    game.Fold,
			Amount:    0,
			Reasoning: "Player quit",
		}
	}

	// Process the action and return a decision
	return ti.processActionForDecision(action, args, player, table)
}

// processActionForDecision processes a user action and returns a Decision
func (ti *TUIAgent) processActionForDecision(action string, args []string, player *game.Player, table *game.Table) game.Decision {
	ti.mainLogger.Info("Processing action for decision", "action", action, "args", args)

	// Handle quit commands first
	if action == "quit" || action == "q" || action == "exit" {
		ti.mainLogger.Info("Quit command received")
		return game.Decision{
			Action:    game.Fold,
			Amount:    0,
			Reasoning: "Player quit",
		}
	}

	// Handle empty action (Enter pressed) - this shouldn't happen in decision context
	if action == "" {
		ti.mainLogger.Info("Empty action received")
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: "No action specified",
		}
	}

	currentPlayer := ti.table.GetCurrentPlayer()
	if currentPlayer == nil || currentPlayer.Type != game.Human {
		ti.model.AddLogEntry("Error: Not your turn")
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: "Not player's turn",
		}
	}

	switch action {
	case "call", "c":
		return ti.handleCallForDecision(args)
	case "raise", "r":
		return ti.handleRaiseForDecision(args)
	case "fold", "f":
		return ti.handleFoldForDecision(args)
	case "check", "ch":
		return ti.handleCheckForDecision(args)
	case "allin", "all", "a":
		return ti.handleAllInForDecision(args)
	case "hand", "h", "cards":
		ti.handleShowHand(args)
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: "Showed hand information",
		}
	case "pot", "p":
		ti.handleShowPot(args)
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: "Showed pot information",
		}
	case "players", "pl":
		ti.handleShowPlayers(args)
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: "Showed player information",
		}
	case "help", "?":
		ti.handleHelp(args)
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: "Showed help",
		}
	default:
		ti.model.AddLogEntry(fmt.Sprintf("Unknown command: %s. Type 'help' for available commands.", action))
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: fmt.Sprintf("Unknown command: %s", action),
		}
	}
}

// ExecuteAction implements the Agent interface for human players via TUI
func (ti *TUIAgent) ExecuteAction(player *game.Player, table *game.Table) string {
	if !player.CanAct() {
		return "Player cannot act"
	}

	decision := ti.MakeDecision(player, table)
	
	// The TUI has already handled the action through processActionForDecision
	// Just return the reasoning
	return decision.Reasoning
}

// PromptForAction prompts the human player for their action (legacy method)
func (ti *TUIAgent) PromptForAction() (bool, error) {
	ti.mainLogger.Info("Waiting for user action")
	action, args, shouldContinue, err := ti.model.WaitForAction()
	if err != nil {
		ti.mainLogger.Error("Error in WaitForAction", "error", err)
		ti.model.AddLogEntry(fmt.Sprintf("Error: %s", err.Error()))
		return true, nil // Continue on error
	}

	ti.mainLogger.Info("Received user action", "action", action, "args", args, "continue", shouldContinue)
	if !shouldContinue {
		ti.mainLogger.Info("User chose to quit")
		return false, nil
	}

	// Process the action
	result, err := ti.processAction(action, args)
	ti.mainLogger.Info("Action processed", "result", result, "error", err)
	return result, err
}

// processAction processes a user action and returns whether to continue
func (ti *TUIAgent) processAction(action string, args []string) (bool, error) {
	ti.mainLogger.Info("Processing action", "action", action, "args", args)

	// Handle quit commands first (before checking current player)
	if action == "quit" || action == "q" || action == "exit" {
		ti.mainLogger.Info("Quit command received, returning false to quit")
		return false, nil
	}

	// Handle empty action (Enter pressed) - continue the game
	if action == "" {
		ti.mainLogger.Info("Empty action (Enter pressed), returning true to continue")
		return true, nil
	}

	currentPlayer := ti.table.GetCurrentPlayer()
	if currentPlayer == nil || currentPlayer.Type != game.Human {
		ti.model.AddLogEntry("Error: Not your turn")
		return true, nil
	}

	switch action {
	case "call", "c":
		return ti.handleCall(args)
	case "raise", "r":
		return ti.handleRaise(args)
	case "fold", "f":
		return ti.handleFold(args)
	case "check", "ch":
		return ti.handleCheck(args)
	case "allin", "all", "a":
		return ti.handleAllIn(args)
	case "hand", "h", "cards":
		return ti.handleShowHand(args)
	case "pot", "p":
		return ti.handleShowPot(args)
	case "players", "pl":
		return ti.handleShowPlayers(args)
	case "help", "?":
		return ti.handleHelp(args)
	case "quit", "q", "exit":
		ti.mainLogger.Info("Quit command received, returning false to quit")
		return false, nil // Return false to indicate quit
	default:
		ti.model.AddLogEntry(fmt.Sprintf("Unknown command: %s. Type 'help' for available commands.", action))
		return true, nil
	}
}

// Command handlers (adapted from original interface.go)

func (ti *TUIAgent) handleCall(args []string) (bool, error) {
	currentPlayer := ti.table.GetCurrentPlayer()

	if ti.table.CurrentBet == 0 {
		ti.model.AddLogEntry("Error: No bet to call, use 'check' instead")
		return true, nil
	}

	callAmount := ti.table.CurrentBet - currentPlayer.BetThisRound
	if callAmount <= 0 {
		ti.model.AddLogEntry("Error: You have already called")
		return true, nil
	}

	if !currentPlayer.Call(callAmount) {
		ti.model.AddLogEntry("Error: Insufficient chips to call")
		return true, nil
	}

	ti.table.Pot += callAmount
	ti.model.AddLogEntry(fmt.Sprintf("Called $%d", callAmount))

	// Record action in hand history
	if ti.table.HandHistory != nil {
		ti.table.HandHistory.AddAction(currentPlayer.Name, game.Call, callAmount, ti.table.Pot, ti.table.CurrentRound, "")
	}

	return true, nil
}

func (ti *TUIAgent) handleRaise(args []string) (bool, error) {
	ti.mainLogger.Info("handleRaise called", "args", args)
	currentPlayer := ti.table.GetCurrentPlayer()

	if len(args) == 0 {
		ti.mainLogger.Warn("No raise amount specified")
		ti.model.AddLogEntry("Error: Specify raise amount: 'raise <amount>'")
		return true, nil
	}

	ti.mainLogger.Info("Parsing raise amount", "input", args[0])
	amount, err := strconv.Atoi(args[0])
	if err != nil {
		ti.mainLogger.Error("Invalid amount format", "input", args[0], "error", err)
		ti.model.AddLogEntry(fmt.Sprintf("Error: Invalid amount: %s", args[0]))
		return true, nil
	}

	ti.mainLogger.Info("Validating raise", "amount", amount, "currentBet", ti.table.CurrentBet, "playerChips", currentPlayer.Chips)

	if amount <= ti.table.CurrentBet {
		ti.mainLogger.Warn("Raise amount too low", "amount", amount, "currentBet", ti.table.CurrentBet)
		ti.model.AddLogEntry(fmt.Sprintf("Error: Raise must be more than current bet of $%d", ti.table.CurrentBet))
		return true, nil
	}

	totalNeeded := amount - currentPlayer.BetThisRound
	if totalNeeded > currentPlayer.Chips {
		ti.mainLogger.Warn("Insufficient chips", "totalNeeded", totalNeeded, "playerChips", currentPlayer.Chips)
		ti.model.AddLogEntry(fmt.Sprintf("Error: Insufficient chips, you have $%d", currentPlayer.Chips))
		return true, nil
	}

	ti.mainLogger.Info("Executing raise", "totalNeeded", totalNeeded)
	if !currentPlayer.Raise(totalNeeded) {
		ti.mainLogger.Error("Raise failed")
		ti.model.AddLogEntry("Error: Failed to raise")
		return true, nil
	}

	ti.table.Pot += totalNeeded
	ti.table.CurrentBet = amount
	ti.mainLogger.Info("Raise successful", "amount", amount, "newPot", ti.table.Pot)
	ti.model.AddLogEntry(fmt.Sprintf("Raised to $%d", amount))

	// Record action in hand history
	if ti.table.HandHistory != nil {
		ti.table.HandHistory.AddAction(currentPlayer.Name, game.Raise, amount, ti.table.Pot, ti.table.CurrentRound, "")
	}

	return true, nil
}

func (ti *TUIAgent) handleFold(args []string) (bool, error) {
	currentPlayer := ti.table.GetCurrentPlayer()
	currentPlayer.Fold()
	ti.model.AddLogEntry("Folded")

	// Record action in hand history
	if ti.table.HandHistory != nil {
		ti.table.HandHistory.AddAction(currentPlayer.Name, game.Fold, 0, ti.table.Pot, ti.table.CurrentRound, "")
	}

	return true, nil
}

func (ti *TUIAgent) handleCheck(args []string) (bool, error) {
	currentPlayer := ti.table.GetCurrentPlayer()

	if ti.table.CurrentBet > currentPlayer.BetThisRound {
		ti.model.AddLogEntry(fmt.Sprintf("Error: Cannot check, current bet is $%d, use 'call' or 'fold'", ti.table.CurrentBet))
		return true, nil
	}

	currentPlayer.Check()
	ti.model.AddLogEntry("Checked")

	// Record action in hand history
	if ti.table.HandHistory != nil {
		ti.table.HandHistory.AddAction(currentPlayer.Name, game.Check, 0, ti.table.Pot, ti.table.CurrentRound, "")
	}

	return true, nil
}

func (ti *TUIAgent) handleAllIn(args []string) (bool, error) {
	currentPlayer := ti.table.GetCurrentPlayer()

	if currentPlayer.Chips == 0 {
		ti.model.AddLogEntry("Error: No chips to go all-in")
		return true, nil
	}

	allInAmount := currentPlayer.Chips
	if !currentPlayer.AllIn() {
		ti.model.AddLogEntry("Error: Failed to go all-in")
		return true, nil
	}

	ti.table.Pot += allInAmount
	if currentPlayer.TotalBet > ti.table.CurrentBet {
		ti.table.CurrentBet = currentPlayer.TotalBet
	}

	ti.model.AddLogEntry(fmt.Sprintf("ALL-IN for $%d!", allInAmount))

	// Record action in hand history
	if ti.table.HandHistory != nil {
		ti.table.HandHistory.AddAction(currentPlayer.Name, game.AllIn, allInAmount, ti.table.Pot, ti.table.CurrentRound, "")
	}

	return true, nil
}

func (ti *TUIAgent) handleShowHand(_ []string) (bool, error) {
	currentPlayer := ti.table.GetCurrentPlayer()
	cards := ti.model.formatCards(currentPlayer.HoleCards)
	ti.model.AddLogEntry(fmt.Sprintf("Your hole cards: %s", cards))
	return true, nil
}

func (ti *TUIAgent) handleShowPot(_ []string) (bool, error) {
	ti.model.AddLogEntry(fmt.Sprintf("Pot: $%d", ti.table.Pot))
	ti.model.AddLogEntry(fmt.Sprintf("Current bet: $%d", ti.table.CurrentBet))
	return true, nil
}

func (ti *TUIAgent) handleShowPlayers(_ []string) (bool, error) {
	ti.model.AddLogEntry("Players:")
	for _, player := range ti.table.ActivePlayers {
		status := ""
		if player.IsFolded {
			status = " (folded)"
		} else if player.IsAllIn {
			status = " (all-in)"
		}

		marker := ""
		if player == ti.table.GetCurrentPlayer() {
			marker = " <-- current"
		}

		ti.model.AddLogEntry(fmt.Sprintf("  %s: $%d%s%s", player.Name, player.Chips, status, marker))
	}
	return true, nil
}

func (ti *TUIAgent) handleHelp(_ []string) (bool, error) {
	ti.model.AddLogEntry("Available commands:")
	ti.model.AddLogEntry("Game Actions:")
	ti.model.AddLogEntry("  call       - Call the current bet")
	ti.model.AddLogEntry("  raise <amt>- Raise to a specific amount")
	ti.model.AddLogEntry("  fold       - Fold your hand")
	ti.model.AddLogEntry("  check      - Check (no bet when none required)")
	ti.model.AddLogEntry("  allin      - Go all-in with remaining chips")
	ti.model.AddLogEntry("Information:")
	ti.model.AddLogEntry("  hand       - Show your hole cards")
	ti.model.AddLogEntry("  pot        - Show pot information")
	ti.model.AddLogEntry("  players    - Show all player information")
	ti.model.AddLogEntry("Utility:")
	ti.model.AddLogEntry("  help       - Show this help")
	ti.model.AddLogEntry("  quit       - Quit the game")
	return true, nil
}

// Decision-returning versions of action handlers for Agent interface

func (ti *TUIAgent) handleCallForDecision(args []string) game.Decision {
	currentPlayer := ti.table.GetCurrentPlayer()

	if ti.table.CurrentBet == 0 {
		ti.model.AddLogEntry("Error: No bet to call, use 'check' instead")
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: "No bet to call",
		}
	}

	callAmount := ti.table.CurrentBet - currentPlayer.BetThisRound
	if callAmount <= 0 {
		ti.model.AddLogEntry("Error: You have already called")
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: "Already called",
		}
	}

	if currentPlayer.Chips < callAmount {
		ti.model.AddLogEntry("Error: Insufficient chips to call")
		return game.Decision{
			Action:    game.Fold,
			Amount:    0,
			Reasoning: "Insufficient chips to call",
		}
	}

	if !currentPlayer.Call(callAmount) {
		ti.model.AddLogEntry("Error: Failed to call")
		return game.Decision{
			Action:    game.Fold,
			Amount:    0,
			Reasoning: "Call failed",
		}
	}

	ti.table.Pot += callAmount
	ti.model.AddLogEntry(fmt.Sprintf("Called $%d", callAmount))

	// Record action in hand history
	if ti.table.HandHistory != nil {
		ti.table.HandHistory.AddAction(currentPlayer.Name, game.Call, callAmount, ti.table.Pot, ti.table.CurrentRound, "")
	}

	return game.Decision{
		Action:    game.Call,
		Amount:    callAmount,
		Reasoning: fmt.Sprintf("Called $%d", callAmount),
	}
}

func (ti *TUIAgent) handleRaiseForDecision(args []string) game.Decision {
	currentPlayer := ti.table.GetCurrentPlayer()

	if len(args) == 0 {
		ti.model.AddLogEntry("Error: Specify raise amount: 'raise <amount>'")
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: "No raise amount specified",
		}
	}

	amount, err := strconv.Atoi(args[0])
	if err != nil {
		ti.model.AddLogEntry(fmt.Sprintf("Error: Invalid amount: %s", args[0]))
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: fmt.Sprintf("Invalid amount: %s", args[0]),
		}
	}

	if amount <= ti.table.CurrentBet {
		ti.model.AddLogEntry(fmt.Sprintf("Error: Raise must be more than current bet of $%d", ti.table.CurrentBet))
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: "Raise amount too low",
		}
	}

	totalNeeded := amount - currentPlayer.BetThisRound
	if totalNeeded > currentPlayer.Chips {
		ti.model.AddLogEntry(fmt.Sprintf("Error: Insufficient chips, you have $%d", currentPlayer.Chips))
		return game.Decision{
			Action:    game.Fold,
			Amount:    0,
			Reasoning: "Insufficient chips to raise",
		}
	}

	if !currentPlayer.Raise(totalNeeded) {
		ti.model.AddLogEntry("Error: Failed to raise")
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: "Raise failed",
		}
	}

	ti.table.Pot += totalNeeded
	ti.table.CurrentBet = amount
	ti.model.AddLogEntry(fmt.Sprintf("Raised to $%d", amount))

	// Record action in hand history
	if ti.table.HandHistory != nil {
		ti.table.HandHistory.AddAction(currentPlayer.Name, game.Raise, amount, ti.table.Pot, ti.table.CurrentRound, "")
	}

	return game.Decision{
		Action:    game.Raise,
		Amount:    amount,
		Reasoning: fmt.Sprintf("Raised to $%d", amount),
	}
}

func (ti *TUIAgent) handleFoldForDecision(args []string) game.Decision {
	currentPlayer := ti.table.GetCurrentPlayer()
	currentPlayer.Fold()
	ti.model.AddLogEntry("Folded")

	// Record action in hand history
	if ti.table.HandHistory != nil {
		ti.table.HandHistory.AddAction(currentPlayer.Name, game.Fold, 0, ti.table.Pot, ti.table.CurrentRound, "")
	}

	return game.Decision{
		Action:    game.Fold,
		Amount:    0,
		Reasoning: "Folded",
	}
}

func (ti *TUIAgent) handleCheckForDecision(args []string) game.Decision {
	currentPlayer := ti.table.GetCurrentPlayer()

	if ti.table.CurrentBet > currentPlayer.BetThisRound {
		ti.model.AddLogEntry(fmt.Sprintf("Error: Cannot check, current bet is $%d, use 'call' or 'fold'", ti.table.CurrentBet))
		return game.Decision{
			Action:    game.Fold,
			Amount:    0,
			Reasoning: "Cannot check with outstanding bet",
		}
	}

	currentPlayer.Check()
	ti.model.AddLogEntry("Checked")

	// Record action in hand history
	if ti.table.HandHistory != nil {
		ti.table.HandHistory.AddAction(currentPlayer.Name, game.Check, 0, ti.table.Pot, ti.table.CurrentRound, "")
	}

	return game.Decision{
		Action:    game.Check,
		Amount:    0,
		Reasoning: "Checked",
	}
}

func (ti *TUIAgent) handleAllInForDecision(args []string) game.Decision {
	currentPlayer := ti.table.GetCurrentPlayer()

	if currentPlayer.Chips == 0 {
		ti.model.AddLogEntry("Error: No chips to go all-in")
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: "No chips to go all-in",
		}
	}

	allInAmount := currentPlayer.Chips
	if !currentPlayer.AllIn() {
		ti.model.AddLogEntry("Error: Failed to go all-in")
		return game.Decision{
			Action:    game.Check,
			Amount:    0,
			Reasoning: "All-in failed",
		}
	}

	ti.table.Pot += allInAmount
	if currentPlayer.TotalBet > ti.table.CurrentBet {
		ti.table.CurrentBet = currentPlayer.TotalBet
	}

	ti.model.AddLogEntry(fmt.Sprintf("ALL-IN for $%d!", allInAmount))

	// Record action in hand history
	if ti.table.HandHistory != nil {
		ti.table.HandHistory.AddAction(currentPlayer.Name, game.AllIn, allInAmount, ti.table.Pot, ti.table.CurrentRound, "")
	}

	return game.Decision{
		Action:    game.AllIn,
		Amount:    allInAmount,
		Reasoning: fmt.Sprintf("All-in for $%d", allInAmount),
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

// ClearLog clears the game log display
func (ti *TUIAgent) ClearLog() {
	ti.model.ClearLog()
}

// InitializeHand initializes the display for a new hand
func (ti *TUIAgent) InitializeHand(seats int) {
	// Log game state context to file
	ti.mainLogger.Info("New hand started",
		"handID", ti.table.HandID,
		"players", seats,
		"smallBlind", ti.table.SmallBlind,
		"bigBlind", ti.table.BigBlind,
		"pot", ti.table.Pot)

	// Log all players' hole cards for complete game history
	for _, player := range ti.table.Players {
		if len(player.HoleCards) > 0 {
			ti.mainLogger.Info("Player hole cards",
				"player", player.Name,
				"type", map[game.PlayerType]string{game.Human: "Human", game.AI: "AI"}[player.Type],
				"position", player.Position.String(),
				"chips", player.Chips,
				"cards", fmt.Sprintf("%s %s", player.HoleCards[0].String(), player.HoleCards[1].String()))
		}
	}

	// Show hand header
	ti.AddLogEntry(fmt.Sprintf("Hand %s • %d players • $1/$2", ti.table.HandID, seats))
	ti.AddLogEntry("")
	ti.AddLogEntry("*** HOLE CARDS ***")

	// Show hole cards for human player
	humanPlayer := ti.table.Players[0] // Human is first player
	if len(humanPlayer.HoleCards) > 0 {
		cards := ti.model.formatCards(humanPlayer.HoleCards)
		ti.AddLogEntry(fmt.Sprintf("Dealt to You: %s", cards))
	}

	// Show blind posting
	for _, player := range ti.table.ActivePlayers {
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

	// Record action in hand history for human players (AI actions already recorded)
	if player.Type == game.Human && ti.table.HandHistory != nil {
		ti.table.HandHistory.AddAction(player.Name, player.LastAction, player.ActionAmount, ti.table.Pot, ti.table.CurrentRound, "")
	}

	var actionEntry string
	switch action {
	case "fold":
		actionEntry = fmt.Sprintf("%s: folds", player.Name)
	case "call":
		actionEntry = fmt.Sprintf("%s: calls $%d", player.Name, player.BetThisRound)
	case "check":
		actionEntry = fmt.Sprintf("%s: checks", player.Name)
	case "raise":
		actionEntry = fmt.Sprintf("%s: raises to $%d (pot now: $%d)",
			player.Name, player.BetThisRound, ti.table.Pot)
	case "allin":
		actionEntry = fmt.Sprintf("%s: goes all-in for $%d", player.Name, player.TotalBet)
	default:
		actionEntry = fmt.Sprintf("%s: %s", player.Name, action)
	}

	ti.AddLogEntry(actionEntry)

	// Note: AI thinking is recorded in HandHistory but NOT shown in TUI
	// to preserve the poker experience
}

// ShowBettingRoundComplete shows when a betting round completes
func (ti *TUIAgent) ShowBettingRoundComplete() {
	activePlayers := 0
	for _, player := range ti.table.ActivePlayers {
		if player.IsInHand() {
			activePlayers++
		}
	}

	if activePlayers <= 1 {
		// Don't show "All players folded" - we'll show it in the summary
		return
	}

	// roundName := ti.table.CurrentRound.String()
	// ti.AddLogEntry(fmt.Sprintf("--- %s betting complete ---", roundName))
}

// ShowBettingRoundTransition shows transition to new betting round
func (ti *TUIAgent) ShowBettingRoundTransition() {
	// Log betting round transition to file
	ti.mainLogger.Info("Betting round transition",
		"round", ti.table.CurrentRound.String(),
		"pot", ti.table.Pot,
		"currentBet", ti.table.CurrentBet)

	// Log community cards
	if len(ti.table.CommunityCards) > 0 {
		cardStrings := make([]string, len(ti.table.CommunityCards))
		for i, card := range ti.table.CommunityCards {
			cardStrings[i] = card.String()
		}
		ti.mainLogger.Info("Community cards", "cards", cardStrings)
	}

	ti.AddLogEntry("")

	switch ti.table.CurrentRound {
	case game.Flop:
		ti.AddLogEntry("*** FLOP ***")
		if len(ti.table.CommunityCards) >= 3 {
			flop := ti.table.CommunityCards[:3]
			ti.AddLogEntry(fmt.Sprintf("Board: %s", ti.model.formatCards(flop)))
		}
	case game.Turn:
		ti.AddLogEntry("*** TURN ***")
		if len(ti.table.CommunityCards) >= 4 {
			ti.AddLogEntry(fmt.Sprintf("Board: %s [%s]",
				ti.model.formatCards(ti.table.CommunityCards[:3]),
				ti.table.CommunityCards[3].String()))
		}
	case game.River:
		ti.AddLogEntry("*** RIVER ***")
		if len(ti.table.CommunityCards) >= 5 {
			ti.AddLogEntry(fmt.Sprintf("Board: %s [%s]",
				ti.model.formatCards(ti.table.CommunityCards[:4]),
				ti.table.CommunityCards[4].String()))
		}
	case game.Showdown:
		ti.AddLogEntry("*** SHOWDOWN ***")
		ti.AddLogEntry(fmt.Sprintf("Final Board: %s", ti.model.formatCards(ti.table.CommunityCards)))
	}

	ti.AddLogEntry(fmt.Sprintf("Pot: $%d", ti.table.Pot))
	ti.AddLogEntry("")
}

// ShowCompleteShowdown handles the showdown display
func (ti *TUIAgent) ShowCompleteShowdown() {
	// Don't show anything here - we'll handle it all in ShowHandSummary
}

// ShowHandSummary shows the hand summary
func (ti *TUIAgent) ShowHandSummary() {
	// Store pot amount (will be reset by AwardPot after this method)
	finalPot := ti.table.Pot

	// Populate HandHistory with final results
	if ti.table.HandHistory != nil {
		// Set community cards
		ti.table.HandHistory.SetCommunityCards(ti.table.CommunityCards)

		// Set final pot and winner info
		winner := ti.table.FindWinner()
		var winners []game.WinnerInfo
		if winner != nil {
			// Determine hand rank using evaluator
			var handRank string
			if len(winner.HoleCards) == 2 && len(ti.table.CommunityCards) == 5 {
				// Only evaluate when we have exactly 7 cards (2 hole + 5 community)
				allCards := append(winner.HoleCards, ti.table.CommunityCards...)
				handScore := evaluator.Evaluate7(allCards)
				handRank = handScore.String()
			} else if len(ti.table.CommunityCards) == 0 {
				// Hand ended pre-flop, winner by fold
				handRank = "Win by fold"
			} else {
				// Hand ended before river, winner by fold
				handRank = "Win by fold"
			}

			winners = []game.WinnerInfo{
				{
					PlayerName: winner.Name,
					Amount:     finalPot,
					HoleCards:  winner.HoleCards,
					HandRank:   handRank,
				},
			}
		}
		ti.table.HandHistory.SetFinalResults(finalPot, winners)
	}

	// Log final hand state to file
	ti.mainLogger.Info("=== HAND COMPLETE ===",
		"handID", ti.table.HandID,
		"finalPot", finalPot,
		"finalBet", ti.table.CurrentBet)

	// Log final board
	if len(ti.table.CommunityCards) > 0 {
		cardStrings := make([]string, len(ti.table.CommunityCards))
		for i, card := range ti.table.CommunityCards {
			cardStrings[i] = card.String()
		}
		ti.mainLogger.Info("Final board", "cards", cardStrings)
	}

	// Log all players' final state
	for _, player := range ti.table.Players {
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

	// Clean poker-style summary using unified implementation
	ti.AddLogEntry("")
	if ti.table.HandHistory != nil {
		// Use unified summary but without showing hole cards in TUI
		summaryText := ti.table.HandHistory.GenerateSummary(game.SummaryOpts{
			PlayerPerspective: "You",
		})
		lines := strings.Split(strings.TrimSpace(summaryText), "\n")
		for _, line := range lines {
			if line != "" {
				ti.AddLogEntry(line)
			}
		}
	}

	// Automatically save hand history
	ti.autoSaveHandHistory()
}

// autoSaveHandHistory automatically saves the current hand history to a file
func (ti *TUIAgent) autoSaveHandHistory() {
	handID := ti.table.HandID

	// Create handhistory directory if it doesn't exist
	if err := os.MkdirAll("handhistory", 0755); err != nil {
		ti.mainLogger.Error("Error creating handhistory directory", "error", err)
		return
	}

	// Generate filename
	filename := filepath.Join("handhistory", fmt.Sprintf("hand_%s.txt", handID))

	// Generate hand history content from HandHistory service
	var content string
	if ti.table.HandHistory != nil {
		content = ti.table.HandHistory.GenerateHistoryText()
	} else {
		content = fmt.Sprintf("=== HAND %s ===\nNo hand history available\n=== END HAND ===\n", handID)
	}

	// Write to file
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		ti.mainLogger.Error("Error saving hand history", "error", err, "filename", filename)
		return
	}

	ti.mainLogger.Info("Hand history saved", "filename", filename)
}
