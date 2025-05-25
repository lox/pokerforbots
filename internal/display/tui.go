package display

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lox/holdem-cli/internal/deck"
	"github.com/lox/holdem-cli/internal/game"
)

// TUIModel represents the Bubble Tea model for the poker game
type TUIModel struct {
	table *game.Table

	// UI components
	logViewport viewport.Model
	actionInput textinput.Model

	// State
	gameLog      []string
	actionResult chan ActionResult
	quitting     bool
	focusedPane  int // 0 = log, 1 = input

	// Styles
	styles *TUIStyles

	// Dimensions
	width  int
	height int
}

// TUIStyles contains all styling for the TUI
type TUIStyles struct {
	// Pane styles
	LogPane    lipgloss.Style
	ActionPane lipgloss.Style
	Border     lipgloss.Style

	// Content styles
	Header    lipgloss.Style
	GameLog   lipgloss.Style
	HandInfo  lipgloss.Style
	Actions   lipgloss.Style
	RedCard   lipgloss.Style
	BlackCard lipgloss.Style

	// Status styles
	Success lipgloss.Style
	Error   lipgloss.Style
	Warning lipgloss.Style
	Info    lipgloss.Style
}

// ActionResult represents the result of a user action
type ActionResult struct {
	Action   string
	Args     []string
	Continue bool
	Error    error
}

// NewTUIModel creates a new TUI model
func NewTUIModel(table *game.Table) *TUIModel {
	styles := &TUIStyles{
		LogPane: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#626262")).
			Padding(1),
		ActionPane: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#04B575")).
			Padding(1),
		Border: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#626262")),
		Header: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1).
			Bold(true),
		GameLog: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")),
		HandInfo: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#96CEB4")).
			Bold(true),
		Actions: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFD700")).
			Bold(true),
		RedCard: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6B6B")).
			Bold(true),
		BlackCard: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Bold(true),
		Success: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#96CEB4")).
			Bold(true),
		Error: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6B6B")).
			Bold(true),
		Warning: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFEAA7")).
			Bold(true),
		Info: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262")),
	}

	// Create viewport for game log
	vp := viewport.New(100, 25)
	vp.SetContent("")

	// Create textinput for action input
	ti := textinput.New()
	ti.Placeholder = "Enter your action (call, raise 50, fold, check, etc.)"
	ti.Focus()
	ti.CharLimit = 100
	ti.Width = 100
	ti.PromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FAFAFA"))
	ti.Prompt = "> "

	return &TUIModel{
		table:        table,
		logViewport:  vp,
		actionInput:  ti,
		gameLog:      []string{},
		actionResult: make(chan ActionResult, 1),
		styles:       styles,
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
		m.width = msg.Width
		m.height = msg.Height
		m.updateDimensions()

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

	// Build the layout
	logPane := m.renderLogPane()
	actionPane := m.renderActionPane()

	// Stack panes vertically
	return lipgloss.JoinVertical(
		lipgloss.Left,
		logPane,
		actionPane,
	)
}

// renderLogPane renders the game log pane
func (m *TUIModel) renderLogPane() string {
	content := strings.Join(m.gameLog, "\n")
	m.logViewport.SetContent(content)

	// Style based on focus
	style := m.styles.LogPane.Width(m.width - 4) // Account for border/padding
	if m.focusedPane == 0 {
		style = style.BorderForeground(lipgloss.Color("#04B575")) // Green when focused
	}

	return style.Render(m.logViewport.View())
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
		content.WriteString(m.styles.HandInfo.Render("Hand Complete"))
		content.WriteString("\n")
	}

	// Update input placeholder based on game state and show input field
	if currentPlayer == nil {
		// Between hands
		m.actionInput.Placeholder = "Enter to continue, 'quit' to exit"
	} else {
		// During hand
		m.actionInput.Placeholder = "Enter your action (call, raise 50, fold, check, etc.)"
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

	// Style based on focus
	style := m.styles.ActionPane.Width(m.width - 4) // Account for border/padding
	if m.focusedPane == 1 {
		style = style.BorderForeground(lipgloss.Color("#04B575")) // Green when focused
	}

	return style.Render(content.String())
}

// renderHandInfo renders current hand information
func (m *TUIModel) renderHandInfo(player *game.Player) string {
	hand := m.formatCards(player.HoleCards)
	position := player.Position.String()
	chips := fmt.Sprintf("$%d", player.Chips)
	pot := fmt.Sprintf("$%d", m.table.Pot)

	return m.styles.HandInfo.Render(
		fmt.Sprintf("Hand: %s  Position: %s  Chips: %s  Pot: %s",
			hand, position, chips, pot))
}

// renderAvailableActions renders available action buttons
func (m *TUIModel) renderAvailableActions(player *game.Player) string {
	var actions []string

	// Always can fold
	actions = append(actions, m.styles.Error.Render("[fold]"))

	if m.table.CurrentBet == 0 {
		actions = append(actions, m.styles.Success.Render("[check]"))
	} else {
		callAmount := m.table.CurrentBet - player.BetThisRound
		if callAmount <= player.Chips {
			actions = append(actions, m.styles.Success.Render(fmt.Sprintf("[call $%d]", callAmount)))
		}
	}

	// Can always raise if have chips
	if player.Chips > 0 {
		actions = append(actions, m.styles.Warning.Render("[raise]"))
		actions = append(actions, m.styles.Warning.Render(fmt.Sprintf("[allin $%d]", player.Chips)))
	}

	return m.styles.Actions.Render("Actions: " + strings.Join(actions, " "))
}

// formatCards formats cards with colors
func (m *TUIModel) formatCards(cards []deck.Card) string {
	if len(cards) == 0 {
		return ""
	}

	var formatted []string
	for _, card := range cards {
		if card.IsRed() {
			formatted = append(formatted, m.styles.RedCard.Render(card.String()))
		} else {
			formatted = append(formatted, m.styles.BlackCard.Render(card.String()))
		}
	}

	return "[" + strings.Join(formatted, " ") + "]"
}

// updateDimensions updates component dimensions based on terminal size
func (m *TUIModel) updateDimensions() {
	if m.height <= 0 || m.width <= 0 {
		return // Don't update if we don't have valid dimensions yet
	}

	// Calculate action pane needs:
	// - Hand info line (1)
	// - Actions line (1)
	// - Input field (1)
	// - Help text (1)
	// - Border top/bottom (2)
	// - Padding (2)
	// Total: 8 lines, but reduce to 7 for safety margin
	actionPaneHeight := 7

	// Reserve space for action pane, give rest to log
	// Leave 1 line margin at top to prevent border clipping
	logHeight := m.height - actionPaneHeight - 1
	if logHeight < 3 {
		logHeight = 3                // Minimum log height
		_ = m.height - logHeight - 1 // Recalculate actionPaneHeight but don't store
	}

	// Account for borders and padding (2 for border, 2 for padding)
	m.logViewport.Width = m.width - 4
	m.logViewport.Height = logHeight - 4

	// Input width should fit within action pane
	m.actionInput.Width = m.width - 8 // Extra margin within the pane
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
