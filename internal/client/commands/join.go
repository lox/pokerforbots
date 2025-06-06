package commands

import (
	"fmt"

	"github.com/lox/pokerforbots/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

// JoinCommand joins a table and starts the TUI interface
type JoinTableCommand struct {
	Table string `arg:"" help:"Table ID to join"`
}

func (cmd *JoinTableCommand) Run(flags *GlobalFlags) error {
	// Create client with file logging (handles config loading and log file creation)
	wsClient, cfg, logger, cleanup, err := SetupClientWithFileLogging(flags)
	if err != nil {
		return err
	}
	defer cleanup()

	logger.Info("Starting Holdem Client TUI",
		"server", cfg.Server.URL,
		"player", cfg.Player.Name,
		"table", cmd.Table)

	// Create TUI model
	tuiModel := tui.NewTUIModel(logger)

	// Set up simple bridge between client and TUI
	tui.SetupSimpleNetworkHandlers(wsClient, tuiModel)

	// Join the specified table
	err = wsClient.JoinTable(cmd.Table, cfg.Player.DefaultBuyIn)
	if err != nil {
		return fmt.Errorf("failed to join table %s: %w", cmd.Table, err)
	}

	// Start TUI
	program := tea.NewProgram(tuiModel, tea.WithAltScreen())

	// Start command handler in TUI package
	tui.StartCommandHandler(wsClient, tuiModel, cfg.Player.DefaultBuyIn)

	// Run TUI
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("error running TUI: %w", err)
	}

	return nil
}
