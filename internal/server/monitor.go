package server

import "github.com/lox/pokerforbots/protocol"

// HandMonitor receives notifications about hand progress and outcomes.
type HandMonitor interface {
	// OnGameStart is called when the game starts.
	OnGameStart(handLimit uint64)

	// OnGameComplete is called when the game completes.
	OnGameComplete(handsCompleted uint64, reason string)

	// OnHandComplete is called after each hand completes.
	OnHandComplete(outcome HandOutcome)
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

func (NullHandMonitor) OnGameStart(uint64)            {}
func (NullHandMonitor) OnGameComplete(uint64, string) {}
func (NullHandMonitor) OnHandComplete(HandOutcome)    {}

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

func (m MultiHandMonitor) OnHandComplete(outcome HandOutcome) {
	for _, monitor := range m.monitors {
		monitor.OnHandComplete(outcome)
	}
}
