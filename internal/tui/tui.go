package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/lox/holdem-cli/internal/deck"
	"github.com/lox/holdem-cli/internal/game"
)

// TUIModel represents the Bubble Tea model for the poker game
type TUIModel struct {
	table  *game.Table
	logger *log.Logger

	// UI components
	logViewport viewport.Model
	actionInput textinput.Model

	// State
	gameLog      []string
	actionResult chan ActionResult
	quitting     bool
	focusedPane  int // 0 = log, 1 = input

	// Dimensions
	width  int
	height int
}

// ActionResult represents the result of a user action
type ActionResult struct {
	Action   string
	Args     []string
	Continue bool
	Error    error
}

// NewTUIModel creates a new TUI model
func NewTUIModel(table *game.Table, logger *log.Logger) *TUIModel {
	// Create viewport for game log with minimal initial size
	// Will be properly sized when WindowSizeMsg arrives
	vp := viewport.New(10, 5)
	vp.SetContent("")

	// Create textinput for action input
	ti := textinput.New()
	ti.Placeholder = "Enter your action (call, raise 10, raise to 15, fold, check, etc.)"
	ti.Focus()
	ti.CharLimit = 100
	ti.Width = 100
	ti.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FAFAFA"))
	ti.Prompt = "> "

	return &TUIModel{
		table:        table,
		logger:       logger.WithPrefix("tui"),
		logViewport:  vp,
		actionInput:  ti,
		gameLog:      []string{},
		actionResult: make(chan ActionResult, 1),
		focusedPane:  1, // Start with input focused
	}
}

// Init initializes the TUI model
func (m *TUIModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages in the TUI
func (m *TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.logger.Debug("Updating dimensions", "width", m.width, "height", m.height)
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.quitting = true
			m.actionResult <- ActionResult{Action: "quit", Continue: false}
			return m, tea.Sequence(tea.ClearScreen, tea.Quit)
		case "tab":
			// Switch focus between log and input
			if m.focusedPane == 0 {
				m.focusedPane = 1
				m.actionInput.Focus()
			} else {
				m.focusedPane = 0
				m.actionInput.Blur()
			}
		case "enter":
			if m.focusedPane == 1 { // Only process enter in input pane
				action := strings.TrimSpace(m.actionInput.Value())
				// Process both empty and non-empty actions
				m.processAction(action)
				m.actionInput.SetValue("")
			}
		case "up", "k":
			if m.focusedPane == 0 { // Log pane focused
				m.logViewport.ScrollUp(1)
			}
		case "down", "j":
			if m.focusedPane == 0 { // Log pane focused
				m.logViewport.ScrollDown(1)
			}
		case "pgup", "b":
			if m.focusedPane == 0 { // Log pane focused
				m.logViewport.HalfPageUp()
			}
		case "pgdown", "f":
			if m.focusedPane == 0 { // Log pane focused
				m.logViewport.HalfPageDown()
			}
		case "home", "g":
			if m.focusedPane == 0 { // Log pane focused
				m.logViewport.GotoTop()
			}
		case "end", "G":
			if m.focusedPane == 0 { // Log pane focused
				m.logViewport.GotoBottom()
			}
		}
	}

	// Update components
	var cmd tea.Cmd

	// Only update input if it's focused
	if m.focusedPane == 1 {
		m.actionInput, cmd = m.actionInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Always update viewport (for scrolling)
	m.logViewport, cmd = m.logViewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View renders the TUI
func (m *TUIModel) View() string {
	if m.quitting {
		return ""
	}

	// Don't render until we have valid dimensions
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	// Action pane (bottom, full width)
	actionContent := m.renderActionPane()
	actionHeight := lipgloss.Height(actionContent)
	calculatedActionWidth := m.width - 2       // Full width minus border
	calculatedActionHeight := actionHeight - 2 // Content height minus border

	actionStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#04B575")).
		Width(calculatedActionWidth).
		Height(calculatedActionHeight)

	if m.focusedPane == 1 {
		actionStyle = actionStyle.BorderForeground(lipgloss.Color("#04B575"))
	}
	actionPane := actionStyle.Render(actionContent)

	// Sidebar pane (right side of log pane, same height as log pane)
	sidebarContent := m.renderSidebarPane()
	sidebarWidth := lipgloss.Width(sidebarContent)

	calculatedSidebarWidth := 25
	if sidebarWidth > calculatedSidebarWidth {
		calculatedSidebarWidth = sidebarWidth
	}

	calculatedSidebarHeight := m.height - actionHeight - 4 // Account for border x 2 and action pane

	sidebarStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#626262")).
		Width(calculatedSidebarWidth).
		Height(calculatedSidebarHeight)

	sidebarPane := sidebarStyle.Render(sidebarContent)

	// Log pane (top, fills height minus action pane)
	logContent := m.renderLogPane()
	m.logViewport.SetContent(logContent)

	calculatedLogWidth := m.width - calculatedSidebarWidth - 4 // Account for border x 2 and sidebar
	calculatedLogHeight := m.height - actionHeight - 4         // Account for border x 2 and action pane

	m.logViewport.Width = calculatedLogWidth
	m.logViewport.Height = calculatedLogHeight

	logStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#626262")).
		Width(calculatedLogWidth).
		Height(calculatedLogHeight)

	if m.focusedPane == 0 {
		logStyle = logStyle.BorderForeground(lipgloss.Color("#04B575"))
	}
	logPane := logStyle.Render(m.logViewport.View())

	// Top row (log pane + sidebar pane)
	topRow := lipgloss.JoinHorizontal(lipgloss.Top, logPane, sidebarPane)

	return lipgloss.JoinVertical(lipgloss.Top, topRow, actionPane)
}

// renderLogPane renders the game log pane content
func (m *TUIModel) renderLogPane() string {
	return strings.Join(m.gameLog, "\n")
}

// renderSidebarPane creates the sidebar content
func (m *TUIModel) renderSidebarPane() string {
	var content strings.Builder

	// Get players in seat order
	players := m.table.Players
	currentPlayer := m.table.GetCurrentPlayer()

	for _, player := range players {
		// Simple format: Name $chips [indicators]
		var indicators []string

		// Position indicators (only show the most important ones)
		if m.table.DealerPosition == player.SeatNumber {
			indicators = append(indicators, "D")
		}
		if player.Position == game.SmallBlind {
			indicators = append(indicators, "SB")
		}
		if player.Position == game.BigBlind {
			indicators = append(indicators, "BB")
		}

		// State indicators
		if player.IsFolded {
			indicators = append(indicators, "FOLD")
		} else if player.IsAllIn {
			indicators = append(indicators, "ALL-IN")
		}

		// Current player arrow
		prefix := "  "
		if currentPlayer != nil && currentPlayer.Name == player.Name {
			prefix = "▶ "
		}

		// Player name (shorter format)
		name := player.Name
		if player.Type == game.Human {
			name = "You"
		}

		// Format: ▶ Name $200 [D,SB]
		line := fmt.Sprintf("%s%s $%d", prefix, name, player.Chips)

		// Add indicators if any
		if len(indicators) > 0 {
			line += " [" + strings.Join(indicators, ",") + "]"
		}

		// Add bet if player has bet this round
		if player.BetThisRound > 0 {
			line += fmt.Sprintf(" ($%d)", player.BetThisRound)
		}

		// Color coding
		var style lipgloss.Style
		if player.IsFolded {
			style = InfoStyle // Dimmed for folded players
		} else if currentPlayer != nil && currentPlayer.Name == player.Name {
			style = SuccessStyle // Highlight current player
		} else {
			style = PlayerInfoStyle // Normal
		}

		content.WriteString(style.Render(line))
		content.WriteString("\n")
	}

	// Add pot and bet info at bottom (more compact)
	content.WriteString("\n")
	content.WriteString(WarningStyle.Render(fmt.Sprintf("Pot: $%d", m.table.Pot)))

	if m.table.CurrentBet > 0 {
		content.WriteString(" | ")
		content.WriteString(WarningStyle.Render(fmt.Sprintf("Bet: $%d", m.table.CurrentBet)))
	}

	return content.String()
}

// renderActionPane renders the action input pane
func (m *TUIModel) renderActionPane() string {
	var content strings.Builder

	// Show current hand info
	currentPlayer := m.table.GetCurrentPlayer()
	if currentPlayer != nil && currentPlayer.Type == game.Human {
		handInfo := m.renderHandInfo(currentPlayer)
		content.WriteString(handInfo)
		content.WriteString("\n")

		// Show available actions
		actions := m.renderAvailableActions(currentPlayer)
		content.WriteString(actions)
		content.WriteString("\n")
	} else if currentPlayer == nil {
		// Between hands - show continuation prompt
		content.WriteString(HandInfoStyle.Render("Hand Complete"))
		content.WriteString("\n")
	}

	// Update input placeholder based on game state and show input field
	if currentPlayer == nil {
		// Between hands
		m.actionInput.Placeholder = "Enter to continue, 'quit' to exit"
	} else {
		// During hand
		m.actionInput.Placeholder = "Enter your action (call, raise 10, raise to 15, fold, check, etc.)"
	}

	content.WriteString(m.actionInput.View())
	content.WriteString("\n")

	// Show help text
	if m.focusedPane == 0 {
		content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")).Render(
			"Log focused: ↑↓ scroll, PgUp/PgDn half page, Home/End, Tab to input"))
	} else {
		// Different help text based on game state
		currentPlayer := m.table.GetCurrentPlayer()
		if currentPlayer == nil {
			// Between hands - minimal help
			content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")).Render(
				"Tab to scroll log • Ctrl+C to quit"))
		} else {
			// During hand
			content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")).Render(
				"Tab to scroll log • Enter to submit • Ctrl+C to quit"))
		}
	}

	// Return content without styling - let the parent handle sizing and focus
	return content.String()
}

// renderHandInfo renders current hand information
func (m *TUIModel) renderHandInfo(player *game.Player) string {
	hand := m.formatCards(player.HoleCards)
	pot := fmt.Sprintf("$%d", m.table.Pot)

	return HandInfoStyle.Render(
		fmt.Sprintf("Hand: %s  Pot: %s", hand, pot))
}

// renderAvailableActions renders available action buttons
func (m *TUIModel) renderAvailableActions(player *game.Player) string {
	var actions []string

	// Always can fold
	actions = append(actions, ErrorStyle.Render("[fold]"))

	if m.table.CurrentBet == 0 {
		actions = append(actions, SuccessStyle.Render("[check]"))
	} else {
		callAmount := m.table.CurrentBet - player.BetThisRound
		if callAmount <= player.Chips {
			actions = append(actions, SuccessStyle.Render(fmt.Sprintf("[call $%d]", callAmount)))
		}
	}

	// Can always raise if have chips
	if player.Chips > 0 {
		actions = append(actions, WarningStyle.Render("[raise]"))
		actions = append(actions, WarningStyle.Render(fmt.Sprintf("[allin $%d]", player.Chips)))
	}

	return ActionsStyle.Render("Actions: " + strings.Join(actions, " "))
}

// formatCards formats cards with colors
func (m *TUIModel) formatCards(cards []deck.Card) string {
	if len(cards) == 0 {
		return ""
	}

	var formatted []string
	for _, card := range cards {
		if card.IsRed() {
			formatted = append(formatted, RedCardStyle.Render(card.String()))
		} else {
			formatted = append(formatted, BlackCardStyle.Render(card.String()))
		}
	}

	return "[" + strings.Join(formatted, " ") + "]"
}

// AddLogEntry adds an entry to the game log
func (m *TUIModel) AddLogEntry(entry string) {
	m.gameLog = append(m.gameLog, entry)
	// Update content and auto-scroll to bottom
	content := strings.Join(m.gameLog, "\n")
	m.logViewport.SetContent(content)

	// Only call GotoBottom if viewport has valid dimensions
	if m.logViewport.Height > 0 && m.logViewport.Width > 0 {
		m.logViewport.GotoBottom()
	}
}

// ClearLog clears the game log
func (m *TUIModel) ClearLog() {
	m.gameLog = []string{}
	m.logViewport.SetContent("")
}

// processAction processes a user action
func (m *TUIModel) processAction(input string) {
	parts := strings.Fields(strings.ToLower(input))

	var action string
	var args []string

	if len(parts) == 0 {
		// Empty input (Enter pressed with no text)
		action = ""
		args = []string{}
	} else {
		action = parts[0]
		args = parts[1:]
	}

	// Send action result through channel
	m.actionResult <- ActionResult{
		Action:   action,
		Args:     args,
		Continue: action != "quit",
	}
}

// WaitForAction waits for user input (for use by main game loop)
func (m *TUIModel) WaitForAction() (string, []string, bool, error) {
	result := <-m.actionResult
	return result.Action, result.Args, result.Continue, result.Error
}
