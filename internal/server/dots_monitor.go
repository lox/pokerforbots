package server

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

const (
	dotGreen = "\033[32m●\033[0m" // Green dot for wins
	dotRed   = "\033[31m●\033[0m" // Red dot for losses
	dotGray  = "\033[90m●\033[0m" // Gray dot for neutral
)

// Built-in bot name prefixes for detection
var builtInBotPrefixes = []string{
	"calling-bot-",
	"random-bot-",
	"aggressive-bot-",
	"complex-bot-",
}

// DotsMonitor implements HandMonitor for minimal progress output.
// Shows a colored dot for each hand: green for custom bot wins, red for losses,
// gray for neutral (no custom bot or neutral outcome).
type DotsMonitor struct {
	writer    io.Writer
	mu        sync.RWMutex
	dotCount  int
	lineWidth int // Wrap after this many dots
}

// NewDotsMonitor creates a new dots monitor.
func NewDotsMonitor(writer io.Writer) *DotsMonitor {
	if writer == nil {
		writer = os.Stdout
	}

	return &DotsMonitor{
		writer:    writer,
		lineWidth: 80,
	}
}

// OnGameStart implements HandMonitor.
func (d *DotsMonitor) OnGameStart(handLimit uint64) {}

// OnGameComplete implements HandMonitor.
func (d *DotsMonitor) OnGameComplete(handsCompleted uint64, reason string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Print newline if we have dots on the current line
	if d.dotCount > 0 {
		fmt.Fprintln(d.writer)
	}

	fmt.Fprintf(d.writer, "\nCompleted %d hands (%s)\n", handsCompleted, reason)
}

// OnHandStart implements HandMonitor.
func (d *DotsMonitor) OnHandStart(handID string, players []HandPlayer, button int, blinds Blinds) {}

// OnPlayerAction implements HandMonitor.
func (d *DotsMonitor) OnPlayerAction(handID string, seat int, action string, amount int, stack int) {}

// OnStreetChange implements HandMonitor.
func (d *DotsMonitor) OnStreetChange(handID string, street string, cards []string) {}

// OnHandComplete implements HandMonitor.
func (d *DotsMonitor) OnHandComplete(outcome HandOutcome) {
	d.mu.Lock()
	defer d.mu.Unlock()

	dot := d.selectDot(outcome)
	fmt.Fprint(d.writer, dot)

	d.dotCount++
	if d.dotCount >= d.lineWidth {
		fmt.Fprintln(d.writer)
		d.dotCount = 0
	}
}

// selectDot determines which color dot to show based on custom bot performance.
func (d *DotsMonitor) selectDot(outcome HandOutcome) string {
	if outcome.Detail == nil {
		return dotGray
	}

	// Find custom bot outcomes
	var customBotWon, customBotLost bool
	for _, botOutcome := range outcome.Detail.BotOutcomes {
		// Check if this is a custom bot (not a built-in bot)
		if d.isBuiltInBot(botOutcome.Bot) {
			continue
		}

		if botOutcome.NetChips > 0 {
			customBotWon = true
		} else if botOutcome.NetChips < 0 {
			customBotLost = true
		}
	}

	// Color based on custom bot performance
	switch {
	case customBotWon:
		return dotGreen
	case customBotLost:
		return dotRed
	default:
		return dotGray
	}
}

// isBuiltInBot checks if a bot is a built-in bot based on its display name.
func (d *DotsMonitor) isBuiltInBot(bot *Bot) bool {
	displayName := bot.DisplayName()
	if displayName == "" {
		return false
	}

	for _, prefix := range builtInBotPrefixes {
		if strings.HasPrefix(displayName, prefix) {
			return true
		}
	}

	return false
}
