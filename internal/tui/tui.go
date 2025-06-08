package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/lox/pokerforbots/internal/deck"
	"github.com/lox/pokerforbots/internal/game"
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
	quitSignal   chan bool
	quitting     bool
	focusedPane  int // 0 = log, 1 = input

	// Display state (event-driven, client-server ready)
	currentPot   int
	currentBet   int
	validActions []game.ValidAction
	isHumansTurn bool
	humanPlayer  *game.Player // Current human player info when it's their turn

	// Table info for sidebar
	tableID    string
	seatNumber int
	players    []PlayerInfo

	// Dimensions
	width       int
	height      int
	initialized bool // Track if viewport has been properly sized

	// Test mode
	testMode      bool
	capturedLog   []string               // For test assertions
	eventCallback func(eventType string) // Callback for test event synchronization
}

// ActionResult represents the result of a user action
type ActionResult struct {
	Action   string
	Args     []string
	Continue bool
	Error    error
}

// QuitMsg is a custom message to signal quit
type QuitMsg struct{}

// PlayerInfo holds basic player information for the sidebar
type PlayerInfo struct {
	Name  string
	Chips int
}

// NewTUIModel creates a new TUI model for network mode
func NewTUIModel(logger *log.Logger) *TUIModel {
	return NewTUIModelWithOptions(logger, false)
}

// NewTUIModelWithOptions creates a new TUI model with test mode option
func NewTUIModelWithOptions(logger *log.Logger, testMode bool) *TUIModel {
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
		table:        nil, // No local table - all state comes from network
		logger:       logger.WithPrefix("tui"),
		logViewport:  vp,
		actionInput:  ti,
		gameLog:      []string{},
		actionResult: make(chan ActionResult, 1),
		quitSignal:   make(chan bool, 1),
		focusedPane:  1, // Start with input focused
		testMode:     testMode,
		capturedLog:  []string{},
	}
}

// Init initializes the TUI model
func (m *TUIModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.listenForQuit())
}

// listenForQuit returns a command that listens for quit signals
func (m *TUIModel) listenForQuit() tea.Cmd {
	return func() tea.Msg {
		<-m.quitSignal
		return QuitMsg{}
	}
}

// Update handles messages in the TUI
func (m *TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case QuitMsg:
		m.quitting = true
		return m, tea.Sequence(tea.ClearScreen, tea.Quit)

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

	// Ensure action pane dimensions are valid (minimum 1x1)
	if calculatedActionWidth < 1 {
		calculatedActionWidth = 1
	}
	if calculatedActionHeight < 1 {
		calculatedActionHeight = 1
	}

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

	// Ensure sidebar dimensions are valid (minimum 1x1)
	if calculatedSidebarWidth < 1 {
		calculatedSidebarWidth = 1
	}
	if calculatedSidebarHeight < 1 {
		calculatedSidebarHeight = 1
	}

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

	// Ensure viewport dimensions are valid (minimum 1x1)
	if calculatedLogWidth < 1 {
		calculatedLogWidth = 1
	}
	if calculatedLogHeight < 1 {
		calculatedLogHeight = 1
	}

	m.logViewport.Width = calculatedLogWidth
	m.logViewport.Height = calculatedLogHeight

	// On first proper sizing, reset to top to avoid starting scrolled down
	if !m.initialized && calculatedLogWidth > 1 && calculatedLogHeight > 1 {
		m.logViewport.GotoTop()
		m.initialized = true
	}

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

	// Show pot and bet info at top
	content.WriteString(WarningStyle.Render(fmt.Sprintf("Pot: $%d", m.currentPot)))
	if m.currentBet > 0 {
		content.WriteString(" | ")
		content.WriteString(WarningStyle.Render(fmt.Sprintf("Bet: $%d", m.currentBet)))
	}
	content.WriteString("\n\n")

	// Show players list if we have table info
	if m.tableID != "" && len(m.players) > 0 {
		content.WriteString(InfoStyle.Render("Players at table:"))
		content.WriteString("\n")
		for _, player := range m.players {
			content.WriteString(fmt.Sprintf("  %s: $%d", player.Name, player.Chips))
			content.WriteString("\n")
		}
	}

	return content.String()
}

// renderActionPane renders the action input pane
func (m *TUIModel) renderActionPane() string {
	var content strings.Builder

	// Show current hand info
	if m.isHumansTurn && m.humanPlayer != nil {
		handInfo := m.renderHandInfo(m.humanPlayer)
		content.WriteString(handInfo)
		content.WriteString("\n")

		// Show available actions
		actions := m.renderAvailableActions()
		content.WriteString(actions)
		content.WriteString("\n")
	} else if !m.isHumansTurn {
		// Between hands or waiting for other players
		content.WriteString(HandInfoStyle.Render("Waiting..."))
		content.WriteString("\n")
	}

	// Update input placeholder based on game state and show input field
	if !m.isHumansTurn {
		// Between hands or waiting for others
		m.actionInput.Placeholder = "Enter to continue, 'quit' to exit"
	} else {
		// During hand - human's turn
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
		if !m.isHumansTurn {
			// Between hands or waiting - minimal help
			content.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")).Render(
				"Tab to scroll log • Ctrl+C to quit"))
		} else {
			// During hand - human's turn
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
	pot := fmt.Sprintf("$%d", m.currentPot)

	return HandInfoStyle.Render(
		fmt.Sprintf("Hand: %s  Pot: %s", hand, pot))
}

// renderAvailableActions renders available action buttons based on engine's valid actions
func (m *TUIModel) renderAvailableActions() string {
	var actions []string

	// Use the valid actions provided by the game engine
	for _, validAction := range m.validActions {
		switch validAction.Action {
		case game.Fold:
			actions = append(actions, ErrorStyle.Render("[fold]"))
		case game.Check:
			actions = append(actions, SuccessStyle.Render("[check]"))
		case game.Call:
			// Just show the call amount - simpler and more reliable
			actions = append(actions, SuccessStyle.Render(fmt.Sprintf("[call $%d]", validAction.MinAmount)))
		case game.Raise:
			actions = append(actions, WarningStyle.Render("[raise]"))
		case game.AllIn:
			actions = append(actions, WarningStyle.Render(fmt.Sprintf("[allin $%d]", validAction.MinAmount)))
		}
	}

	// Fallback if no valid actions (shouldn't happen)
	if len(actions) == 0 {
		actions = append(actions, ErrorStyle.Render("[no actions available]"))
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

	// In test mode, also capture the log entry
	if m.testMode {
		m.capturedLog = append(m.capturedLog, entry)
		return // Skip UI updates in test mode
	}

	// Update content and auto-scroll to bottom
	content := strings.Join(m.gameLog, "\n")
	m.logViewport.SetContent(content)

	// Only call GotoBottom if viewport has valid dimensions
	if m.logViewport.Height > 0 && m.logViewport.Width > 0 {
		m.logViewport.GotoBottom()
	}
}

// AddLogEntryAndScrollToShow adds an entry and scrolls to show it at the top
func (m *TUIModel) AddLogEntryAndScrollToShow(entry string) {
	m.gameLog = append(m.gameLog, entry)

	// In test mode, also capture the log entry
	if m.testMode {
		m.capturedLog = append(m.capturedLog, entry)
		return // Skip UI updates in test mode
	}

	content := strings.Join(m.gameLog, "\n")
	m.logViewport.SetContent(content)

	// Scroll to show the new entry at the top of the viewport
	if m.logViewport.Height > 0 && m.logViewport.Width > 0 {
		// Calculate line position (number of previous lines)
		targetLine := len(m.gameLog) - 1
		// Set the viewport position to show this line at the top
		m.logViewport.SetYOffset(targetLine)
	}
}

// SetTableInfo sets the table information for the sidebar
func (m *TUIModel) SetTableInfo(tableID string, seatNumber int, players []PlayerInfo) {
	m.tableID = tableID
	m.seatNumber = seatNumber
	m.players = players
}

// AddBoldLogEntry adds a bold entry to the top of the game log
func (m *TUIModel) AddBoldLogEntry(entry string) {
	// Use ANSI bold codes for the entry
	boldEntry := fmt.Sprintf("\033[1m%s\033[0m", entry)

	// In test mode, capture without ANSI codes
	if m.testMode {
		m.capturedLog = append([]string{entry}, m.capturedLog...)
		m.gameLog = append([]string{boldEntry}, m.gameLog...)
		return // Skip UI updates in test mode
	}

	// Insert at the beginning of the log
	m.gameLog = append([]string{boldEntry}, m.gameLog...)
	content := strings.Join(m.gameLog, "\n")
	m.logViewport.SetContent(content)
	m.logViewport.GotoTop()
}

// ClearLog clears the game log
func (m *TUIModel) ClearLog() {
	m.gameLog = []string{}
	m.logViewport.SetContent("")
}

// UpdatePot updates the current pot display value
func (m *TUIModel) UpdatePot(pot int) {
	m.currentPot = pot
}

// UpdateCurrentBet updates the current bet display value
func (m *TUIModel) UpdateCurrentBet(bet int) {
	m.currentBet = bet
}

// UpdateValidActions updates the valid actions display
func (m *TUIModel) UpdateValidActions(actions []game.ValidAction) {
	m.validActions = actions
}

// SetHumanTurn sets whether it's currently the human's turn to act
func (m *TUIModel) SetHumanTurn(isHumansTurn bool, player *game.Player) {
	m.isHumansTurn = isHumansTurn
	m.humanPlayer = player
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
		Continue: true, // Let the command handler decide whether to continue
	}
}

// WaitForAction waits for user input (for use by main game loop)
func (m *TUIModel) WaitForAction() (string, []string, bool, error) {
	result := <-m.actionResult
	return result.Action, result.Args, result.Continue, result.Error
}

// SendQuitSignal signals the TUI to quit gracefully
func (m *TUIModel) SendQuitSignal() {
	select {
	case m.quitSignal <- true:
	default:
		// Channel is full, quit signal already sent
	}
}

// GetCapturedLog returns the captured log entries (test mode only)
func (m *TUIModel) GetCapturedLog() []string {
	if !m.testMode {
		return nil
	}
	// Return a copy to prevent modification
	result := make([]string, len(m.capturedLog))
	copy(result, m.capturedLog)
	return result
}

// InjectAction programmatically injects an action (test mode only)
func (m *TUIModel) InjectAction(action string, args []string) error {
	if !m.testMode {
		return fmt.Errorf("action injection only available in test mode")
	}

	select {
	case m.actionResult <- ActionResult{
		Action:   action,
		Args:     args,
		Continue: true,
	}:
		return nil
	default:
		return fmt.Errorf("action channel full")
	}
}

// IsTestMode returns whether the TUI is in test mode
func (m *TUIModel) IsTestMode() bool {
	return m.testMode
}

// SetEventCallback sets a callback function for test event synchronization
func (m *TUIModel) SetEventCallback(callback func(eventType string)) {
	if m.testMode {
		m.eventCallback = callback
	}
}

// notifyEventCallback calls the event callback if in test mode
func (m *TUIModel) notifyEventCallback(eventType string) {
	if m.testMode && m.eventCallback != nil {
		m.eventCallback(eventType)
	}
}
