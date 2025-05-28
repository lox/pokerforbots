package tui

import "github.com/charmbracelet/lipgloss"

// Static styles for content elements
var (
	HeaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Bold(true)

	GameLogStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA"))

	HandInfoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#96CEB4")).
			Bold(true)

	ActionsStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFD700")).
			Bold(true)

	RedCardStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6B6B")).
			Bold(true)

	BlackCardStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#000000")).
			Bold(true)

	PlayerInfoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA"))

	SuccessStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#96CEB4")).
			Bold(true)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6B6B")).
			Bold(true)

	WarningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFEAA7")).
			Bold(true)

	InfoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262"))
)
