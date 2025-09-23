package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/lox/pokerforbots/internal/regression"
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
func (m *SimpleProgressMonitor) OnHandComplete(handsCompleted uint64, handLimit uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()

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

// startBatch marks the beginning of a new batch
func (m *SimpleProgressMonitor) startBatch(batchNum int, totalBatches int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.currentBatch = batchNum
	m.totalBatches = totalBatches
	m.batchStartTime = time.Now()
	m.dotsPrinted = 0

	fmt.Printf("Batch %d/%d: ", batchNum, totalBatches)
}

// completeBatch marks the completion of a batch
func (m *SimpleProgressMonitor) completeBatch(batchNum int, totalHandsCompleted int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Don't print anything - OnHandComplete handles the completion message
}

// PrintSummary prints the final summary
func (m *SimpleProgressMonitor) PrintSummary(totalHands int) {
	duration := time.Since(m.startTime)
	handsPerSec := float64(totalHands) / duration.Seconds()

	fmt.Printf("\n✅ Completed %d hands in %.1fs (%.0f hands/sec)\n\n",
		totalHands, duration.Seconds(), handsPerSec)
}

// ProgressReporter interface implementation
func (m *SimpleProgressMonitor) OnBatchStart(batchNum int, totalBatches int, handsInBatch int) {
	m.startBatch(batchNum, totalBatches)
}

func (m *SimpleProgressMonitor) OnBatchComplete(batchNum int, handsCompleted int) {
	m.completeBatch(batchNum, handsCompleted)
}

func (m *SimpleProgressMonitor) OnHandsProgress(handsCompleted int, totalHands int) {
	// This is handled by OnHandComplete from the server
}

// Ensure interface compliance
var _ server.HandMonitor = (*SimpleProgressMonitor)(nil)
var _ regression.ProgressReporter = (*SimpleProgressMonitor)(nil)
