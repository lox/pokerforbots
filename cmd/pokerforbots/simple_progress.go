package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/lox/pokerforbots/internal/server"
)

// SimpleProgressMonitor shows clean progress without overlapping animations
type SimpleProgressMonitor struct {
	mu             sync.Mutex
	totalHands     uint64
	handsCompleted uint64
	dotsPrinted    int
	startTime      time.Time
	batchStartTime time.Time
	currentBatch   int
	totalBatches   int
}

// NewSimpleProgressMonitor creates a simple progress monitor
func NewSimpleProgressMonitor(totalBatches int) *SimpleProgressMonitor {
	return &SimpleProgressMonitor{
		startTime:    time.Now(),
		totalBatches: totalBatches,
	}
}

// OnGameStart is called when the game starts
func (m *SimpleProgressMonitor) OnGameStart(handLimit uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalHands = handLimit
	m.batchStartTime = time.Now()

	// Don't print anything - batch info will show it
}

// OnHandComplete is called after each hand completes
func (m *SimpleProgressMonitor) OnHandComplete(outcome server.HandOutcome) {
	m.mu.Lock()
	defer m.mu.Unlock()

	handsCompleted := outcome.HandsCompleted
	handLimit := outcome.HandLimit
	if handLimit == 0 {
		handLimit = m.totalHands
	}
	if handLimit == 0 {
		handLimit = 1
	}

	m.handsCompleted = handsCompleted

	// Show 40 dots for a batch to fit in 80-char terminal with "Batch X/Y: " prefix
	// Each dot represents 2.5% progress
	pct := int(handsCompleted * 100 / handLimit)
	if pct > 100 {
		pct = 100
	}
	dotsTotal := 40

	// Calculate how many dots we should have shown by now
	targetDots := (pct * dotsTotal) / 100

	// Print new dots
	for i := m.dotsPrinted; i < targetDots; i++ {
		fmt.Print(".")
		m.dotsPrinted++
	}

	// Check if batch is complete
	if handsCompleted >= handLimit {
		// Fill in any remaining dots to reach exactly 40
		for i := m.dotsPrinted; i < dotsTotal; i++ {
			fmt.Print(".")
			m.dotsPrinted++
		}

		duration := time.Since(m.batchStartTime)
		handsPerSec := float64(handLimit) / duration.Seconds()
		fmt.Printf(" ✓ %d hands in %.1fs (%.0f/sec)\n", handLimit, duration.Seconds(), handsPerSec)
		m.dotsPrinted = 0
	}
}

// OnGameComplete is called when the game completes
func (m *SimpleProgressMonitor) OnGameComplete(handsCompleted uint64, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Don't print anything - handled by OnHandComplete
}

// OnHandStart is called when a new hand begins
func (m *SimpleProgressMonitor) OnHandStart(string, []server.HandPlayer, int, server.Blinds) {}

// OnPlayerAction is called when a player takes an action
func (m *SimpleProgressMonitor) OnPlayerAction(string, int, string, int, int) {}

// OnStreetChange is called when the game advances to a new street
func (m *SimpleProgressMonitor) OnStreetChange(string, string, []string) {}

// OnBatchStart is called when a new batch begins (ProgressReporter interface)
func (m *SimpleProgressMonitor) OnBatchStart(batch int, totalBatches int, totalHands int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.currentBatch = batch
	m.totalBatches = totalBatches
	m.batchStartTime = time.Now()

	// Print batch header
	fmt.Printf("Batch %d/%d: ", batch, totalBatches)
}

// OnBatchComplete is called when a batch completes (ProgressReporter interface)
func (m *SimpleProgressMonitor) OnBatchComplete(batch int, total int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Completion handled by OnHandComplete
}

// OnHandsProgress is called periodically during hand execution (ProgressReporter interface)
func (m *SimpleProgressMonitor) OnHandsProgress(handsCompleted int, handsTotal int) {
	// Progress is already handled by OnHandComplete
}

// PrintSummary prints the final summary
func (m *SimpleProgressMonitor) PrintSummary(totalHands int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	duration := time.Since(m.startTime)
	totalSec := duration.Seconds()
	handsPerSec := float64(totalHands) / totalSec

	fmt.Printf("\n")
	fmt.Printf("Completed %d hands in %.1f seconds (%.0f hands/sec)\n", totalHands, totalSec, handsPerSec)
}
