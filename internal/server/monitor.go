package server

// HandMonitor receives notifications about hand progress
type HandMonitor interface {
	// OnHandComplete is called after each hand completes
	OnHandComplete(handsCompleted uint64, handLimit uint64)

	// OnGameStart is called when the game starts
	OnGameStart(handLimit uint64)

	// OnGameComplete is called when the game completes
	OnGameComplete(handsCompleted uint64, reason string)
}

// NullHandMonitor is a no-op implementation
type NullHandMonitor struct{}

func (NullHandMonitor) OnHandComplete(handsCompleted uint64, handLimit uint64) {}
func (NullHandMonitor) OnGameStart(handLimit uint64)                           {}
func (NullHandMonitor) OnGameComplete(handsCompleted uint64, reason string)    {}
