package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/lox/pokerforbots/internal/regression"
	"github.com/lox/pokerforbots/internal/server"
)

// DotProgressReporter shows dots for progress instead of verbose logging
type DotProgressReporter struct {
	mu             sync.Mutex
	currentBatch   int
	totalBatches   int
	handsCompleted int
	totalHands     int
	lastDotHands   int
	dotsPerLine    int
	spinner        *Spinner
	startTime      time.Time
	batchStartTime time.Time
}

// NewDotProgressReporter creates a progress reporter that shows dots
func NewDotProgressReporter(totalHands int) *DotProgressReporter {
	return &DotProgressReporter{
		totalHands:  totalHands,
		dotsPerLine: 50,
		spinner:     NewSpinner(),
		startTime:   time.Now(),
	}
}

// OnBatchStart is called when a batch starts
func (r *DotProgressReporter) OnBatchStart(batchNum int, totalBatches int, handsInBatch int) {
	r.spinner.Stop()

	r.mu.Lock()
	r.currentBatch = batchNum
	r.totalBatches = totalBatches
	r.batchStartTime = time.Now()
	r.mu.Unlock()

	batchInfo := fmt.Sprintf("Batch %d/%d (%d hands)", batchNum, totalBatches, handsInBatch)
	r.spinner.Start(batchInfo)
}

// OnBatchComplete is called when a batch completes
func (r *DotProgressReporter) OnBatchComplete(batchNum int, handsCompleted int) {
	r.spinner.Stop()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Show completion message for this batch
	batchDuration := time.Since(r.batchStartTime)
	fmt.Printf(" ✓ Batch %d complete (%.1fs)\n", batchNum, batchDuration.Seconds())
}

// OnHandsProgress is called periodically with hands completed
func (r *DotProgressReporter) OnHandsProgress(handsCompleted int, totalHands int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.handsCompleted = handsCompleted

	// Show dots for every 100 hands (or proportionally for smaller totals)
	dotInterval := 100
	if totalHands < 1000 {
		dotInterval = totalHands / 10
		if dotInterval < 10 {
			dotInterval = 10
		}
	}

	// Calculate how many dots we should have shown
	targetDots := handsCompleted / dotInterval
	currentDots := r.lastDotHands / dotInterval

	// Print new dots
	for i := currentDots; i < targetDots; i++ {
		fmt.Print(".")
		if (i+1)%r.dotsPerLine == 0 {
			pct := float64(handsCompleted) * 100 / float64(totalHands)
			fmt.Printf(" %d/%d (%.0f%%)\n", handsCompleted, totalHands, pct)
		}
	}

	r.lastDotHands = targetDots * dotInterval
}

// Finish shows the final summary
func (r *DotProgressReporter) Finish() {
	r.spinner.Stop()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Print final newline if we have dots on current line
	if r.lastDotHands%(r.dotsPerLine*100) != 0 {
		fmt.Println()
	}

	duration := time.Since(r.startTime)
	handsPerSec := float64(r.handsCompleted) / duration.Seconds()

	fmt.Printf("\n✅ Completed %d hands in %.1fs (%.0f hands/sec)\n\n",
		r.handsCompleted, duration.Seconds(), handsPerSec)
}

// Spinner shows an animated spinner during batch execution
type Spinner struct {
	mu       sync.Mutex
	active   bool
	stopChan chan struct{}
	frames   []string
	wg       sync.WaitGroup
}

// NewSpinner creates a new spinner
func NewSpinner() *Spinner {
	return &Spinner{
		frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	}
}

// Start begins spinning with the given message
func (s *Spinner) Start(message string) {
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		return
	}
	s.active = true
	stopChan := make(chan struct{})
	s.stopChan = stopChan
	s.wg.Add(1)
	s.mu.Unlock()

	go func(stop <-chan struct{}) {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		defer s.wg.Done()

		frame := 0
		for {
			select {
			case <-stop:
				// Clear the spinner line
				fmt.Printf("\r%s\r", strings.Repeat(" ", len(message)+4))
				return
			case <-ticker.C:
				fmt.Printf("\r%s %s", s.frames[frame], message)
				frame = (frame + 1) % len(s.frames)
			}
		}
	}(stopChan)
}

// Stop stops the spinner
func (s *Spinner) Stop() {
	s.mu.Lock()
	stopChan := s.stopChan
	if !s.active && stopChan == nil {
		s.mu.Unlock()
		return
	}
	s.active = false
	s.stopChan = nil
	s.mu.Unlock()

	if stopChan != nil {
		close(stopChan)
		s.wg.Wait()
	}
}

// Ensure interface compliance
var _ regression.ProgressReporter = (*DotProgressReporter)(nil)

// ServerProgressMonitor implements server.HandMonitor to track real-time progress
type ServerProgressMonitor struct {
	mu           sync.Mutex
	totalHands   uint64
	lastDotHands uint64
	dotsPerLine  int
	startTime    time.Time
}

// NewServerProgressMonitor creates a new server progress monitor
func NewServerProgressMonitor() *ServerProgressMonitor {
	return &ServerProgressMonitor{
		dotsPerLine: 50,
		startTime:   time.Now(),
	}
}

// OnGameStart is called when the game starts
func (m *ServerProgressMonitor) OnGameStart(handLimit uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.totalHands = handLimit
	fmt.Printf("Starting %d hands", handLimit)
}

// OnHandComplete is called after each hand completes
func (m *ServerProgressMonitor) OnHandComplete(outcome server.HandOutcome) {
	m.mu.Lock()
	defer m.mu.Unlock()

	handsCompleted := outcome.HandsCompleted
	handLimit := outcome.HandLimit
	if handLimit == 0 {
		handLimit = m.totalHands
	}

	// Show dot for every 100 hands (or proportionally for smaller totals)
	dotInterval := uint64(100)
	if handLimit > 0 && handLimit < 1000 {
		dotInterval = handLimit / 10
		if dotInterval < 10 {
			dotInterval = 10
		}
	}
	if dotInterval == 0 {
		dotInterval = 10
	}

	// Calculate dots to show
	targetDots := handsCompleted / dotInterval
	currentDots := m.lastDotHands / dotInterval

	// Print new dots
	for i := currentDots; i < targetDots; i++ {
		fmt.Print(".")
		if (i+1)%uint64(m.dotsPerLine) == 0 {
			if handLimit > 0 {
				pct := float64(handsCompleted) * 100 / float64(handLimit)
				fmt.Printf(" %d/%d (%.0f%%)\n", handsCompleted, handLimit, pct)
			} else {
				fmt.Printf(" %d hands\n", handsCompleted)
			}
		}
	}

	m.lastDotHands = targetDots * dotInterval
}

// OnGameComplete is called when the game completes
func (m *ServerProgressMonitor) OnGameComplete(handsCompleted uint64, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Print final newline if we have dots on current line
	if m.lastDotHands%(uint64(m.dotsPerLine)*100) != 0 {
		fmt.Println()
	}

	duration := time.Since(m.startTime)
	handsPerSec := float64(handsCompleted) / duration.Seconds()

	fmt.Printf(" ✅ %d hands in %.1fs (%.0f hands/sec)\n",
		handsCompleted, duration.Seconds(), handsPerSec)
}

// OnHandStart is called when a new hand begins
func (m *ServerProgressMonitor) OnHandStart(string, []server.HandPlayer, int, server.Blinds) {}

// OnPlayerAction is called when a player takes an action
func (m *ServerProgressMonitor) OnPlayerAction(string, int, string, int, int) {}

// OnStreetChange is called when the street changes
func (m *ServerProgressMonitor) OnStreetChange(string, string, []string) {}

// Ensure interface compliance
var _ server.HandMonitor = (*ServerProgressMonitor)(nil)
