package handhistory

import "time"

// Clock abstracts time for deterministic testing.
type Clock interface {
	Now() time.Time
}

// realClock implements Clock using time.Now.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Player describes the state of a player at hand start.
type Player struct {
	Seat        int
	Name        string
	DisplayName string
	Chips       int
	HoleCards   []string
}

// Blinds describes the blind structure for a hand.
type Blinds struct {
	Small int
	Big   int
}

// MonitorConfig configures a per-game monitor.
type MonitorConfig struct {
	GameID           string
	OutputDir        string
	Filename         string
	FlushHands       int
	IncludeHoleCards bool
	Variant          string
	Clock            Clock
}

// Outcome mirrors the data emitted when a hand completes.
type Outcome struct {
	HandID         string
	HandsCompleted uint64
	HandLimit      uint64
	Detail         *OutcomeDetail
}

// OutcomeDetail contains per-hand detail for stats.
type OutcomeDetail struct {
	Board       []string
	TotalPot    int
	BotOutcomes []BotOutcome
}

// BotOutcome captures the per-player result.
type BotOutcome struct {
	Name           string
	Seat           int
	NetChips       int
	HoleCards      []string
	Won            bool
	WentToShowdown bool
}

// ManagerConfig configures the server-wide manager.
type ManagerConfig struct {
	BaseDir          string
	FlushInterval    time.Duration
	FlushHands       int
	IncludeHoleCards bool
	Variant          string
	Clock            Clock
}
