package display

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/log"
	"github.com/lox/holdem-cli/internal/game"
)

// TUIInterface handles human player interaction through a TUI
type TUIInterface struct {
	model                *TUIModel
	program              *tea.Program
	table                *game.Table
	uiLogger, mainLogger *log.Logger
	handActions          []string // Store action log for current hand
}

// NewTUIInterface creates a new TUI-based interface
func NewTUIInterface(table *game.Table, logger *log.Logger) (*TUIInterface, error) {
	model := NewTUIModel(table, logger)
	program := tea.NewProgram(model, tea.WithAltScreen())

	return &TUIInterface{
		model:      model,
		program:    program,
		table:      table,
		uiLogger:   logger.WithPrefix("ui"),
		mainLogger: logger,
	}, nil
}

// Start starts the TUI program
func (ti *TUIInterface) Start() error {
	go func() {
		if _, err := ti.program.Run(); err != nil {
			fmt.Printf("Error running TUI: %v\n", err)
		}
	}()
	return nil
}

// Close closes the TUI interface
func (ti *TUIInterface) Close() error {
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

// PromptForAction prompts the human player for their action
func (ti *TUIInterface) PromptForAction() (bool, error) {
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
func (ti *TUIInterface) processAction(action string, args []string) (bool, error) {
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

	// Handle save command (can be done anytime, doesn't require current player check)
	if action == "save" || action == "s" {
		return ti.handleSave(args)
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

func (ti *TUIInterface) handleCall(args []string) (bool, error) {
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
	return true, nil
}

func (ti *TUIInterface) handleRaise(args []string) (bool, error) {
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
	return true, nil
}

func (ti *TUIInterface) handleFold(args []string) (bool, error) {
	currentPlayer := ti.table.GetCurrentPlayer()
	currentPlayer.Fold()
	ti.model.AddLogEntry("Folded")
	return true, nil
}

func (ti *TUIInterface) handleCheck(args []string) (bool, error) {
	currentPlayer := ti.table.GetCurrentPlayer()

	if ti.table.CurrentBet > currentPlayer.BetThisRound {
		ti.model.AddLogEntry(fmt.Sprintf("Error: Cannot check, current bet is $%d, use 'call' or 'fold'", ti.table.CurrentBet))
		return true, nil
	}

	currentPlayer.Check()
	ti.model.AddLogEntry("Checked")
	return true, nil
}

func (ti *TUIInterface) handleAllIn(args []string) (bool, error) {
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
	return true, nil
}

func (ti *TUIInterface) handleShowHand(_ []string) (bool, error) {
	currentPlayer := ti.table.GetCurrentPlayer()
	cards := ti.model.formatCards(currentPlayer.HoleCards)
	ti.model.AddLogEntry(fmt.Sprintf("Your hole cards: %s", cards))
	return true, nil
}

func (ti *TUIInterface) handleShowPot(_ []string) (bool, error) {
	ti.model.AddLogEntry(fmt.Sprintf("Pot: $%d", ti.table.Pot))
	ti.model.AddLogEntry(fmt.Sprintf("Current bet: $%d", ti.table.CurrentBet))
	return true, nil
}

func (ti *TUIInterface) handleShowPlayers(_ []string) (bool, error) {
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

func (ti *TUIInterface) handleHelp(_ []string) (bool, error) {
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
	ti.model.AddLogEntry("  save       - Save hand history to file")
	ti.model.AddLogEntry("  help       - Show this help")
	ti.model.AddLogEntry("  quit       - Quit the game")
	return true, nil
}

// stripANSI removes ANSI escape sequences from a string
func stripANSI(s string) string {
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return ansiRegex.ReplaceAllString(s, "")
}

// AddLogEntry adds an entry to the game log
func (ti *TUIInterface) AddLogEntry(entry string) {
	ti.model.AddLogEntry(entry)
	// Also log to file for complete history (strip ANSI codes)
	cleanEntry := stripANSI(entry)
	ti.uiLogger.Info(cleanEntry)
	
	// Store action for hand history (strip ANSI codes)
	ti.handActions = append(ti.handActions, cleanEntry)
}

// ClearLog clears the game log display
func (ti *TUIInterface) ClearLog() {
	ti.model.ClearLog()
}

// ResetHandActions clears the hand action log for a new hand
func (ti *TUIInterface) ResetHandActions() {
	ti.handActions = nil
}

// InitializeHand initializes the display for a new hand
func (ti *TUIInterface) InitializeHand(seats int) {
	// Reset hand actions for new hand
	ti.ResetHandActions()
	
	// Log game state context to file
	ti.mainLogger.Info("New hand started",
		"handNumber", ti.table.HandNumber,
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
	ti.AddLogEntry(fmt.Sprintf("Hand #%d • %d players • $1/$2", ti.table.HandNumber, seats))
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
func (ti *TUIInterface) ShowPlayerAction(player *game.Player) {
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

	ti.mainLogger.Info("Player action", logArgs...)

	switch action {
	case "fold":
		ti.AddLogEntry(fmt.Sprintf("%s: folds", player.Name))
	case "call":
		ti.AddLogEntry(fmt.Sprintf("%s: calls $%d", player.Name, player.BetThisRound))
	case "check":
		ti.AddLogEntry(fmt.Sprintf("%s: checks", player.Name))
	case "raise":
		ti.AddLogEntry(fmt.Sprintf("%s: raises $%d to $%d (pot now: $%d)",
			player.Name, player.BetThisRound, ti.table.CurrentBet, ti.table.Pot))
	case "allin":
		ti.AddLogEntry(fmt.Sprintf("%s: goes all-in for $%d", player.Name, player.TotalBet))
	}
}

// ShowBettingRoundComplete shows when a betting round completes
func (ti *TUIInterface) ShowBettingRoundComplete() {
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
func (ti *TUIInterface) ShowBettingRoundTransition() {
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
func (ti *TUIInterface) ShowCompleteShowdown() {
	// Don't show anything here - we'll handle it all in ShowHandSummary
}

// ShowHandSummary shows the hand summary
func (ti *TUIInterface) ShowHandSummary() {
	// Store pot amount (will be reset by AwardPot after this method)
	finalPot := ti.table.Pot
	
	// Log final hand state to file
	ti.mainLogger.Info("=== HAND COMPLETE ===",
		"handNumber", ti.table.HandNumber,
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

	// Clean poker-style summary
	ti.AddLogEntry("")
	ti.AddLogEntry("*** SUMMARY ***")
	ti.AddLogEntry(fmt.Sprintf("Total pot $%d", finalPot))

	if len(ti.table.CommunityCards) > 0 {
		ti.AddLogEntry(fmt.Sprintf("Board %s", ti.model.formatCards(ti.table.CommunityCards)))
	}

	// Show each player's result
	winner := ti.table.FindWinner()

	for _, player := range ti.table.Players {
		seatInfo := fmt.Sprintf("Seat %d: %s", player.ID, player.Name)

		// Add position info
		switch player.Position {
		case game.Button:
			seatInfo += " (button)"
		case game.SmallBlind:
			seatInfo += " (small blind)"
		case game.BigBlind:
			seatInfo += " (big blind)"
		}

		if player.IsFolded {
			seatInfo += " folded"
			if player.TotalBet > 0 {
				seatInfo += fmt.Sprintf(" and lost $%d", player.TotalBet)
			}
		} else if player == winner {
			cards := ti.model.formatCards(player.HoleCards)
			seatInfo += fmt.Sprintf(" showed %s and won ($%d)", cards, finalPot)
		} else {
			cards := ti.model.formatCards(player.HoleCards)
			seatInfo += fmt.Sprintf(" mucked %s", cards)
			if player.TotalBet > 0 {
				seatInfo += fmt.Sprintf(" and lost $%d", player.TotalBet)
			}
		}

		ti.AddLogEntry(seatInfo)
	}
}

// Helper functions for hand summary
func getInHandPlayers(table *game.Table) []*game.Player {
	var inHand []*game.Player
	for _, player := range table.ActivePlayers {
		if !player.IsFolded {
			inHand = append(inHand, player)
		}
	}
	return inHand
}

// handleSave saves the current hand history to a file
func (ti *TUIInterface) handleSave(args []string) (bool, error) {
	handNumber := ti.table.HandNumber
	
	// Create handhistory directory if it doesn't exist
	if err := os.MkdirAll("handhistory", 0755); err != nil {
		ti.model.AddLogEntry(fmt.Sprintf("Error creating handhistory directory: %v", err))
		return true, nil
	}
	
	// Generate filename
	filename := filepath.Join("handhistory", fmt.Sprintf("hand_%d.txt", handNumber))
	
	// Generate hand history content
	content := ti.generateHandHistory()
	
	// Write to file
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		ti.model.AddLogEntry(fmt.Sprintf("Error saving hand history: %v", err))
		return true, nil
	}
	
	ti.model.AddLogEntry(fmt.Sprintf("Hand history saved to %s", filename))
	return true, nil
}

// generateHandHistory creates a detailed hand history string
func (ti *TUIInterface) generateHandHistory() string {
	t := ti.table
	var history string
	
	// Header
	history += fmt.Sprintf("=== HAND #%d ===\n", t.HandNumber)
	history += fmt.Sprintf("Date: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	history += fmt.Sprintf("Blinds: %d/%d\n", t.SmallBlind, t.BigBlind)
	history += fmt.Sprintf("Players: %d\n", len(t.Players))
	history += fmt.Sprintf("Dealer Position: %d\n\n", t.DealerPosition)
	
	// Starting positions and chip counts
	history += "STARTING POSITIONS:\n"
	for i, player := range t.Players {
		history += fmt.Sprintf("Seat %d: %s (%d chips)\n", i+1, player.Name, player.Chips)
	}
	history += "\n"
	
	// Hole cards (only show if showdown reached or player folded)
	history += "HOLE CARDS:\n"
	for _, player := range t.Players {
		if len(player.HoleCards) > 0 {
			history += fmt.Sprintf("%s: %s %s\n", player.Name, 
				player.HoleCards[0].String(), player.HoleCards[1].String())
		}
	}
	history += "\n"
	
	// Hand action sequence  
	if len(ti.handActions) > 0 {
		history += "HAND ACTION:\n"
		for _, action := range ti.handActions {
			// Filter out non-game actions and UI messages
			if action != "" && !isUIMessage(action) {
				history += fmt.Sprintf("%s\n", action)
			}
		}
		history += "\n"
	}
	
	// Current pot and player positions
	history += fmt.Sprintf("CURRENT POT: %d\n", t.Pot)
	
	// Show current player positions
	history += "\nCURRENT POSITIONS:\n"
	for _, player := range t.Players {
		status := "active"
		if player.IsFolded {
			status = "folded"
		} else if player.IsAllIn {
			status = "all-in"
		}
		history += fmt.Sprintf("%s: %d chips (%s)\n", player.Name, player.Chips, status)
	}
	
	history += "\n=== END HAND ===\n"
	
	return history
}

// isUIMessage checks if a log entry is a UI message that shouldn't be in hand history
func isUIMessage(entry string) bool {
	// Filter out UI-specific messages that aren't part of game action
	uiKeywords := []string{
		"Error:",
		"Unknown command:",
		"Not your turn",
		"Hand history saved",
		"Available commands:",
		"Game Actions:",
		"Information:",
		"Utility:",
		"Hand #", // Remove duplicate hand header
		"*** HOLE CARDS ***", // Remove section headers (already have hole cards above)
	}
	
	for _, keyword := range uiKeywords {
		if len(entry) >= len(keyword) && entry[:len(keyword)] == keyword {
			return true
		}
	}
	return false
}
