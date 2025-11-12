package server

import "github.com/lox/pokerforbots/v2/protocol"

// HandMonitor receives notifications about hand progress and outcomes.
type HandMonitor interface {
	// OnGameStart is called when the game starts.
	OnGameStart(handLimit uint64)

	// OnGameComplete is called when the game completes.
	OnGameComplete(handsCompleted uint64, reason string)

	// OnHandStart is called when a new hand begins.
	OnHandStart(handID string, players []HandPlayer, button int, blinds Blinds)

	// OnPlayerAction is called when a player takes an action.
	OnPlayerAction(handID string, seat int, action string, amount int, stack int)

	// OnStreetChange is called when the street changes.
	OnStreetChange(handID string, street string, cards []string)

	// OnHandComplete is called after each hand completes.
	OnHandComplete(outcome HandOutcome)
}

// HandPlayer represents a player at the start of a hand.
type HandPlayer struct {
	Seat        int
	Name        string // Bot ID for stable stats tracking
	DisplayName string // Human-readable name for display
	Chips       int
	HoleCards   []string // Hole cards for debugging/testing (not sent to other players)
}

// Blinds represents the blind structure.
type Blinds struct {
	Small int
	Big   int
}

// StatsProvider exposes statistics collected by monitors.
type StatsProvider interface {
	GetPlayerStats() []PlayerStats
	GetDetailedStats(botID string) *protocol.PlayerDetailedStats
}

// HandOutcome captures the result of a single hand along with optional detail.
type HandOutcome struct {
	HandID         string
	HandsCompleted uint64
	HandLimit      uint64
	Detail         *HandOutcomeDetail
}

// HandOutcomeDetail contains the detailed outcome of a hand for statistics tracking.
type HandOutcomeDetail struct {
	HandID         string
	ButtonPosition int
	StreetReached  string
	Board          []string
	TotalPot       int
	BotOutcomes    []BotHandOutcome
}

// BotHandOutcome contains per-bot outcome details.
type BotHandOutcome struct {
	Bot            *Bot
	Position       int
	ButtonDistance int
	HoleCards      []string
	NetChips       int
	WentToShowdown bool
	WonAtShowdown  bool
	Actions        map[string]string
	TimedOut       bool
	InvalidActions int
	Disconnected   bool
	WentBroke      bool
}

// NullHandMonitor is a no-op implementation.
type NullHandMonitor struct{}

func (NullHandMonitor) OnGameStart(uint64)                            {}
func (NullHandMonitor) OnGameComplete(uint64, string)                 {}
func (NullHandMonitor) OnHandStart(string, []HandPlayer, int, Blinds) {}
func (NullHandMonitor) OnPlayerAction(string, int, string, int, int)  {}
func (NullHandMonitor) OnStreetChange(string, string, []string)       {}
func (NullHandMonitor) OnHandComplete(HandOutcome)                    {}

// MultiHandMonitor fan-outs events to multiple monitors.
type MultiHandMonitor struct {
	monitors []HandMonitor
}

// NewMultiHandMonitor builds a composite monitor, automatically pruning nil entries and returning
// a NullHandMonitor when no monitors are provided.
func NewMultiHandMonitor(monitors ...HandMonitor) HandMonitor {
	filtered := make([]HandMonitor, 0, len(monitors))
	for _, monitor := range monitors {
		if monitor != nil {
			filtered = append(filtered, monitor)
		}
	}

	switch len(filtered) {
	case 0:
		return NullHandMonitor{}
	case 1:
		return filtered[0]
	default:
		return MultiHandMonitor{monitors: filtered}
	}
}

func (m MultiHandMonitor) OnGameStart(handLimit uint64) {
	for _, monitor := range m.monitors {
		monitor.OnGameStart(handLimit)
	}
}

func (m MultiHandMonitor) OnGameComplete(handsCompleted uint64, reason string) {
	for _, monitor := range m.monitors {
		monitor.OnGameComplete(handsCompleted, reason)
	}
}

func (m MultiHandMonitor) OnHandStart(handID string, players []HandPlayer, button int, blinds Blinds) {
	for _, monitor := range m.monitors {
		monitor.OnHandStart(handID, players, button, blinds)
	}
}

func (m MultiHandMonitor) OnPlayerAction(handID string, seat int, action string, amount int, stack int) {
	for _, monitor := range m.monitors {
		monitor.OnPlayerAction(handID, seat, action, amount, stack)
	}
}

func (m MultiHandMonitor) OnStreetChange(handID string, street string, cards []string) {
	for _, monitor := range m.monitors {
		monitor.OnStreetChange(handID, street, cards)
	}
}

func (m MultiHandMonitor) OnHandComplete(outcome HandOutcome) {
	for _, monitor := range m.monitors {
		monitor.OnHandComplete(outcome)
	}
}
