package server

import "testing"

type testMonitor struct {
	startCalls      int
	completeCalls   int
	handCallCount   int
	lastHandOutcome HandOutcome
}

func (tm *testMonitor) OnGameStart(handLimit uint64) {
	tm.startCalls++
}

func (tm *testMonitor) OnGameComplete(handsCompleted uint64, reason string) {
	tm.completeCalls++
}

func (tm *testMonitor) OnHandStart(string, []HandPlayer, int, Blinds) {}

func (tm *testMonitor) OnPlayerAction(string, int, string, int, int) {}

func (tm *testMonitor) OnStreetChange(string, string, []string) {}

func (tm *testMonitor) OnHandComplete(outcome HandOutcome) {
	tm.handCallCount++
	tm.lastHandOutcome = outcome
}

func TestNewMultiHandMonitor(t *testing.T) {
	m1 := &testMonitor{}
	m2 := &testMonitor{}

	monitor := NewMultiHandMonitor(nil, m1, m2)

	monitor.OnGameStart(42)
	monitor.OnHandComplete(HandOutcome{HandID: "hand-1", HandsCompleted: 1})
	monitor.OnGameComplete(10, "reason")

	if m1.startCalls != 1 || m2.startCalls != 1 {
		t.Fatalf("expected both monitors to receive start event, got m1=%d m2=%d", m1.startCalls, m2.startCalls)
	}
	if m1.handCallCount != 1 || m2.handCallCount != 1 {
		t.Fatalf("expected both monitors to receive hand event, got m1=%d m2=%d", m1.handCallCount, m2.handCallCount)
	}
	if m1.lastHandOutcome.HandID != "hand-1" || m2.lastHandOutcome.HandID != "hand-1" {
		t.Fatalf("expected hand outcome propagation")
	}
	if m1.completeCalls != 1 || m2.completeCalls != 1 {
		t.Fatalf("expected both monitors to receive completion event, got m1=%d m2=%d", m1.completeCalls, m2.completeCalls)
	}
}

func TestNewMultiHandMonitorReturnsNullWhenEmpty(t *testing.T) {
	monitor := NewMultiHandMonitor()

	if _, ok := monitor.(NullHandMonitor); !ok {
		t.Fatalf("expected null hand monitor when no monitors provided")
	}
}

func TestNewMultiHandMonitorReturnsMonitorWhenSingle(t *testing.T) {
	m := &testMonitor{}
	monitor := NewMultiHandMonitor(m)

	if monitor != m {
		t.Fatalf("expected single monitor to be returned directly")
	}
}
