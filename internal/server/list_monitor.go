package server

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// ListMonitor implements HandMonitor for compact hand-by-hand output.
// Shows one line per hand with: hand-id, winner(s), BB won/lost.
type ListMonitor struct {
	writer     io.Writer
	bigBlind   int
	mu         sync.RWMutex
	handCount  int
	totalHands uint64
}

// NewListMonitor creates a new list monitor.
func NewListMonitor(writer io.Writer) *ListMonitor {
	if writer == nil {
		writer = os.Stdout
	}

	return &ListMonitor{
		writer: writer,
	}
}

// OnGameStart implements HandMonitor.
func (l *ListMonitor) OnGameStart(handLimit uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.totalHands = handLimit
}

// OnGameComplete implements HandMonitor.
func (l *ListMonitor) OnGameComplete(handsCompleted uint64, reason string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	fmt.Fprintf(l.writer, "\nCompleted %d hands (%s)\n", handsCompleted, reason)
}

// OnHandStart implements HandMonitor.
func (l *ListMonitor) OnHandStart(handID string, players []HandPlayer, button int, blinds Blinds) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.bigBlind = blinds.Big
}

// OnPlayerAction implements HandMonitor.
func (l *ListMonitor) OnPlayerAction(handID string, seat int, action string, amount int, stack int) {}

// OnStreetChange implements HandMonitor.
func (l *ListMonitor) OnStreetChange(handID string, street string, cards []string) {}

// OnHandComplete implements HandMonitor.
func (l *ListMonitor) OnHandComplete(outcome HandOutcome) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.handCount++

	if outcome.Detail == nil {
		fmt.Fprintf(l.writer, "%-12s <no detail>\n", outcome.HandID)
		return
	}

	detail := outcome.Detail

	// Find winners (positive chips)
	var winners []string
	var maxWin int
	for _, bot := range detail.BotOutcomes {
		if bot.NetChips > 0 {
			displayName := bot.Bot.DisplayName()
			if displayName == "" {
				displayName = bot.Bot.ID[:8]
			}

			// Color code custom bots
			if !l.isBuiltInBot(bot.Bot) {
				displayName = l.colorizeCustomBot(displayName, bot.NetChips)
			}

			winners = append(winners, displayName)
			if bot.NetChips > maxWin {
				maxWin = bot.NetChips
			}
		}
	}

	// Calculate BB won
	bbWon := float64(maxWin) / float64(l.bigBlind)

	// Format winner string
	winnersStr := "<none>"
	if len(winners) > 0 {
		winnersStr = strings.Join(winners, ", ")
	}

	// Format BB with sign and color
	bbStr := l.formatBB(bbWon, maxWin > 0)

	fmt.Fprintf(l.writer, "%-12s %-30s %s\n", outcome.HandID, winnersStr, bbStr)
}

// isBuiltInBot checks if a bot is a built-in bot based on its display name.
func (l *ListMonitor) isBuiltInBot(bot *Bot) bool {
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

// colorizeCustomBot adds color to custom bot names (green if won, red if lost).
func (l *ListMonitor) colorizeCustomBot(name string, netChips int) string {
	color := colorReset
	if netChips > 0 {
		color = colorGreen + colorBold
	} else if netChips < 0 {
		color = colorRed
	}
	return color + name + colorReset
}

// formatBB formats BB with color coding.
func (l *ListMonitor) formatBB(bb float64, isWin bool) string {
	sign := ""
	color := colorReset

	if bb > 0 {
		sign = "+"
		color = colorGreen
	} else if bb < 0 {
		color = colorRed
	}

	return fmt.Sprintf("%s%s%.1f BB%s", color, sign, bb, colorReset)
}
