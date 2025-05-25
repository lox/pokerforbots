package game

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/chzyer/readline"
	"github.com/lox/holdem-cli/internal/deck"
)

// Command represents a game command
type Command struct {
	Name        string
	Aliases     []string
	Description string
	Handler     func(args []string) (bool, error) // bool indicates if game should continue
}

// HumanInterface handles human player interaction
type HumanInterface struct {
	rl       *readline.Instance
	table    *Table
	commands map[string]*Command
	styles   *InterfaceStyles
	testMode bool
}

// InterfaceStyles contains styling for the interface
type InterfaceStyles struct {
	Prompt       lipgloss.Style
	Info         lipgloss.Style
	Success      lipgloss.Style
	Error        lipgloss.Style
	Warning      lipgloss.Style
	Card         lipgloss.Style
	RedCard      lipgloss.Style
	BlackCard    lipgloss.Style
	Pot          lipgloss.Style
	Player       lipgloss.Style
	ActivePlayer lipgloss.Style
}

// NewHumanInterface creates a new human interface
func NewHumanInterface(table *Table, testMode bool) (*HumanInterface, error) {
	styles := &InterfaceStyles{
		Prompt:       lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Bold(true),
		Info:         lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")),
		Success:      lipgloss.NewStyle().Foreground(lipgloss.Color("#96CEB4")).Bold(true),
		Error:        lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Bold(true),
		Warning:      lipgloss.NewStyle().Foreground(lipgloss.Color("#FFEAA7")).Bold(true),
		Card:         lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),
		RedCard:      lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Bold(true),
		BlackCard:    lipgloss.NewStyle().Foreground(lipgloss.Color("#000000")).Bold(true),
		Pot:          lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700")).Bold(true),
		Player:       lipgloss.NewStyle().Foreground(lipgloss.Color("#74B9FF")),
		ActivePlayer: lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6B6B")).Bold(true),
	}

	hi := &HumanInterface{
		table:    table,
		styles:   styles,
		testMode: testMode,
	}

	// Initialize commands
	hi.initCommands()

	// Setup readline with tab completion
	completer := readline.NewPrefixCompleter()
	for name := range hi.commands {
		completer.Children = append(completer.Children, readline.PcItem(name))
	}

	var err error
	hi.rl, err = readline.NewEx(&readline.Config{
		Prompt:          hi.styles.Prompt.Render("Poker> "),
		HistoryFile:     "/tmp/holdem_history",
		AutoComplete:    completer,
		InterruptPrompt: "^C",
		EOFPrompt:       "quit",
	})
	if err != nil {
		return nil, err
	}

	return hi, nil
}

// Close closes the interface
func (hi *HumanInterface) Close() error {
	return hi.rl.Close()
}

// initCommands initializes the command system
func (hi *HumanInterface) initCommands() {
	hi.commands = map[string]*Command{
		"call": {
			Name:        "call",
			Aliases:     []string{"c"},
			Description: "Call the current bet",
			Handler:     hi.handleCall,
		},
		"raise": {
			Name:        "raise",
			Aliases:     []string{"r"},
			Description: "Raise to a specific amount (e.g., 'raise 50')",
			Handler:     hi.handleRaise,
		},
		"fold": {
			Name:        "fold",
			Aliases:     []string{"f"},
			Description: "Fold your hand",
			Handler:     hi.handleFold,
		},
		"check": {
			Name:        "check",
			Aliases:     []string{"ch"},
			Description: "Check (no bet when none required)",
			Handler:     hi.handleCheck,
		},
		"allin": {
			Name:        "allin",
			Aliases:     []string{"all", "a"},
			Description: "Go all-in with remaining chips",
			Handler:     hi.handleAllIn,
		},
		"hand": {
			Name:        "hand",
			Aliases:     []string{"h", "cards"},
			Description: "Show your hole cards",
			Handler:     hi.handleShowHand,
		},
		"pot": {
			Name:        "pot",
			Aliases:     []string{"p"},
			Description: "Show pot information",
			Handler:     hi.handleShowPot,
		},
		"players": {
			Name:        "players",
			Aliases:     []string{"pl"},
			Description: "Show all player information",
			Handler:     hi.handleShowPlayers,
		},
		"help": {
			Name:        "help",
			Aliases:     []string{"?"},
			Description: "Show available commands",
			Handler:     hi.handleHelp,
		},
		"quit": {
			Name:        "quit",
			Aliases:     []string{"q", "exit"},
			Description: "Quit the game",
			Handler:     hi.handleQuit,
		},
	}

	// Add aliases to commands map
	for _, cmd := range hi.commands {
		for _, alias := range cmd.Aliases {
			hi.commands[alias] = cmd
		}
	}
}

// PromptForAction prompts the human player for their action
func (hi *HumanInterface) PromptForAction() (bool, error) {
	// Show available actions once
	hi.showAvailableActions()

	// In test mode, make automatic decisions
	if hi.testMode {
		return hi.makeTestDecision()
	}

	for {
		// Update prompt with current player info
		hi.updatePrompt()

		line, err := hi.rl.Readline()
		if err == readline.ErrInterrupt {
			fmt.Println(hi.styles.Info.Render("Use 'quit' to exit"))
			continue
		} else if err == io.EOF {
			return false, nil
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse command
		parts := strings.Fields(strings.ToLower(line))
		if len(parts) == 0 {
			continue
		}

		cmdName := parts[0]
		args := parts[1:]

		// Find command
		cmd, exists := hi.commands[cmdName]
		if !exists {
			fmt.Println(hi.styles.Error.Render(fmt.Sprintf("Unknown command: %s. Type 'help' for available commands.", cmdName)))
			continue
		}

		// Execute command
		shouldContinue, err := cmd.Handler(args)
		if err != nil {
			fmt.Println(hi.styles.Error.Render(fmt.Sprintf("Error: %s", err.Error())))
			continue
		}

		if !shouldContinue {
			return false, nil // Game should end
		}

		// If it was a game action, break to continue game loop
		if cmdName == "call" || cmdName == "c" || cmdName == "raise" || cmdName == "r" ||
			cmdName == "fold" || cmdName == "f" || cmdName == "check" || cmdName == "ch" ||
			cmdName == "allin" || cmdName == "all" || cmdName == "a" {
			return true, nil
		}

		// For info commands, continue prompting
	}
}

// updatePrompt sets the prompt with current hand, position, and chips
func (hi *HumanInterface) updatePrompt() {
	currentPlayer := hi.table.GetCurrentPlayer()
	if currentPlayer == nil || currentPlayer.Type != Human {
		return
	}

	// Format: [K♥ T♥] BTN $200>
	hand := hi.formatCards(currentPlayer.HoleCards)
	position := currentPlayer.Position.String()
	chips := fmt.Sprintf("$%d", currentPlayer.Chips)

	prompt := fmt.Sprintf("%s %s %s> ", hand, position, chips)
	hi.rl.SetPrompt(hi.styles.Prompt.Render(prompt))
}

// makeTestDecision automatically makes a decision for test mode
func (hi *HumanInterface) makeTestDecision() (bool, error) {
	currentPlayer := hi.table.GetCurrentPlayer()
	if currentPlayer == nil || currentPlayer.Type != Human {
		return true, fmt.Errorf("not your turn")
	}

	// Simple test logic: call if cheap, fold if expensive, check when possible
	if hi.table.CurrentBet == 0 {
		// Check when possible
		fmt.Println("TEST MODE: check")
		return hi.handleCheck([]string{})
	} else {
		callAmount := hi.table.CurrentBet - currentPlayer.BetThisRound
		if callAmount <= 4 { // Call small bets
			fmt.Printf("TEST MODE: call $%d\n", callAmount)
			return hi.handleCall([]string{})
		} else {
			// Fold big bets
			fmt.Println("TEST MODE: fold")
			return hi.handleFold([]string{})
		}
	}
}

// formatCards formats a slice of cards with colors
func (hi *HumanInterface) formatCards(cards []deck.Card) string {
	if len(cards) == 0 {
		return ""
	}

	var formatted []string
	for _, card := range cards {
		if card.IsRed() {
			formatted = append(formatted, hi.styles.RedCard.Render(card.String()))
		} else {
			formatted = append(formatted, hi.styles.BlackCard.Render(card.String()))
		}
	}

	return "[" + strings.Join(formatted, " ") + "]"
}

// showAvailableActions shows what actions the player can take
func (hi *HumanInterface) showAvailableActions() {
	currentPlayer := hi.table.GetCurrentPlayer()
	if currentPlayer == nil || currentPlayer.Type != Human {
		return
	}

	var actions []string

	// Always available
	actions = append(actions, hi.styles.Error.Render("fold"))

	if hi.table.CurrentBet == 0 {
		actions = append(actions, hi.styles.Success.Render("check"))
	} else {
		callAmount := hi.table.CurrentBet - currentPlayer.BetThisRound
		if callAmount <= currentPlayer.Chips {
			actions = append(actions, hi.styles.Success.Render(fmt.Sprintf("call $%d", callAmount)))
		}
	}

	// Can always raise if have chips
	if currentPlayer.Chips > 0 {
		actions = append(actions, hi.styles.Warning.Render("raise <amount>"))
		actions = append(actions, hi.styles.Warning.Render(fmt.Sprintf("allin ($%d)", currentPlayer.Chips)))
	}

	fmt.Println()
	fmt.Printf("Actions: %s\n", strings.Join(actions, " | "))
	fmt.Printf("Info: %s\n", hi.styles.Info.Render("hand, pot, players, help, quit"))
	fmt.Println()
}

// Command handlers

func (hi *HumanInterface) handleCall(args []string) (bool, error) {
	currentPlayer := hi.table.GetCurrentPlayer()
	if currentPlayer == nil || currentPlayer.Type != Human {
		return true, fmt.Errorf("not your turn")
	}

	if hi.table.CurrentBet == 0 {
		return true, fmt.Errorf("no bet to call, use 'check' instead")
	}

	callAmount := hi.table.CurrentBet - currentPlayer.BetThisRound
	if callAmount <= 0 {
		return true, fmt.Errorf("you have already called")
	}

	if !currentPlayer.Call(callAmount) {
		return true, fmt.Errorf("insufficient chips to call")
	}

	hi.table.Pot += callAmount
	fmt.Println(hi.styles.Success.Render(fmt.Sprintf("Called $%d", callAmount)))
	return true, nil
}

func (hi *HumanInterface) handleRaise(args []string) (bool, error) {
	currentPlayer := hi.table.GetCurrentPlayer()
	if currentPlayer == nil || currentPlayer.Type != Human {
		return true, fmt.Errorf("not your turn")
	}

	if len(args) == 0 {
		return true, fmt.Errorf("specify raise amount: 'raise <amount>'")
	}

	amount, err := strconv.Atoi(args[0])
	if err != nil {
		return true, fmt.Errorf("invalid amount: %s", args[0])
	}

	if amount <= hi.table.CurrentBet {
		return true, fmt.Errorf("raise must be more than current bet of $%d", hi.table.CurrentBet)
	}

	totalNeeded := amount - currentPlayer.BetThisRound
	if totalNeeded > currentPlayer.Chips {
		return true, fmt.Errorf("insufficient chips, you have $%d", currentPlayer.Chips)
	}

	if !currentPlayer.Raise(totalNeeded) {
		return true, fmt.Errorf("failed to raise")
	}

	hi.table.Pot += totalNeeded
	hi.table.CurrentBet = amount
	fmt.Println(hi.styles.Success.Render(fmt.Sprintf("Raised to $%d", amount)))
	return true, nil
}

func (hi *HumanInterface) handleFold(args []string) (bool, error) {
	currentPlayer := hi.table.GetCurrentPlayer()
	if currentPlayer == nil || currentPlayer.Type != Human {
		return true, fmt.Errorf("not your turn")
	}

	currentPlayer.Fold()
	fmt.Println(hi.styles.Error.Render("Folded"))
	return true, nil
}

func (hi *HumanInterface) handleCheck(args []string) (bool, error) {
	currentPlayer := hi.table.GetCurrentPlayer()
	if currentPlayer == nil || currentPlayer.Type != Human {
		return true, fmt.Errorf("not your turn")
	}

	if hi.table.CurrentBet > currentPlayer.BetThisRound {
		return true, fmt.Errorf("cannot check, current bet is $%d, use 'call' or 'fold'", hi.table.CurrentBet)
	}

	currentPlayer.Check()
	fmt.Println(hi.styles.Success.Render("Checked"))
	return true, nil
}

func (hi *HumanInterface) handleAllIn(args []string) (bool, error) {
	currentPlayer := hi.table.GetCurrentPlayer()
	if currentPlayer == nil || currentPlayer.Type != Human {
		return true, fmt.Errorf("not your turn")
	}

	if currentPlayer.Chips == 0 {
		return true, fmt.Errorf("no chips to go all-in")
	}

	allInAmount := currentPlayer.Chips
	if !currentPlayer.AllIn() {
		return true, fmt.Errorf("failed to go all-in")
	}

	hi.table.Pot += allInAmount
	if currentPlayer.TotalBet > hi.table.CurrentBet {
		hi.table.CurrentBet = currentPlayer.TotalBet
	}

	fmt.Println(hi.styles.Warning.Render(fmt.Sprintf("ALL-IN for $%d!", allInAmount)))
	return true, nil
}

func (hi *HumanInterface) handleShowHand(args []string) (bool, error) {
	currentPlayer := hi.table.GetCurrentPlayer()
	if currentPlayer == nil || currentPlayer.Type != Human {
		return true, fmt.Errorf("not your turn")
	}

	fmt.Printf("Your hole cards: %s\n", hi.formatCards(currentPlayer.HoleCards))
	return true, nil
}

func (hi *HumanInterface) handleShowPot(args []string) (bool, error) {
	fmt.Printf("Pot: %s\n", hi.styles.Pot.Render(fmt.Sprintf("$%d", hi.table.Pot)))
	fmt.Printf("Current bet: %s\n", hi.styles.Warning.Render(fmt.Sprintf("$%d", hi.table.CurrentBet)))
	return true, nil
}

func (hi *HumanInterface) handleShowPlayers(args []string) (bool, error) {
	fmt.Println("Players:")
	for _, player := range hi.table.ActivePlayers {
		status := ""
		if player.IsFolded {
			status = " (folded)"
		} else if player.IsAllIn {
			status = " (all-in)"
		}

		style := hi.styles.Player
		if player == hi.table.GetCurrentPlayer() {
			style = hi.styles.ActivePlayer
		}

		fmt.Printf("  %s: $%d%s\n", style.Render(player.Name), player.Chips, status)
	}
	return true, nil
}

func (hi *HumanInterface) handleHelp(args []string) (bool, error) {
	fmt.Println("Available commands:")

	// Group commands by category
	gameActions := []string{"call", "raise", "fold", "check", "allin"}
	infoCommands := []string{"hand", "pot", "players"}
	utilityCommands := []string{"help", "quit"}

	fmt.Println(hi.styles.Success.Render("Game Actions:"))
	for _, cmdName := range gameActions {
		if cmd, ok := hi.commands[cmdName]; ok {
			fmt.Printf("  %-10s - %s\n", cmdName, cmd.Description)
		}
	}

	fmt.Println(hi.styles.Info.Render("Information:"))
	for _, cmdName := range infoCommands {
		if cmd, ok := hi.commands[cmdName]; ok {
			fmt.Printf("  %-10s - %s\n", cmdName, cmd.Description)
		}
	}

	fmt.Println(hi.styles.Warning.Render("Utility:"))
	for _, cmdName := range utilityCommands {
		if cmd, ok := hi.commands[cmdName]; ok {
			fmt.Printf("  %-10s - %s\n", cmdName, cmd.Description)
		}
	}

	return true, nil
}

func (hi *HumanInterface) handleQuit(args []string) (bool, error) {
	fmt.Println(hi.styles.Info.Render("Thanks for playing!"))
	return false, nil
}
