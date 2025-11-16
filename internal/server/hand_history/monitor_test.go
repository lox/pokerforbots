package handhistory

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func newTestMonitor(t *testing.T, includeHoleCards bool) (*Monitor, string) {
	t.Helper()
	dir := t.TempDir()
	clock := stubClock{current: time.Date(2025, time.January, 2, 3, 4, 5, 0, time.UTC)}
	monitor, err := NewMonitor(MonitorConfig{
		GameID:           "test",
		OutputDir:        dir,
		FlushHands:       1,
		IncludeHoleCards: includeHoleCards,
		Variant:          "NT",
		Clock:            clock,
	}, zerolog.New(io.Discard))
	if err != nil {
		t.Fatalf("NewMonitor error: %v", err)
	}
	return monitor, filepath.Join(dir, "session.phhs")
}

func samplePlayers() []Player {
	return []Player{
		{Seat: 0, Name: "alice", Chips: 200, HoleCards: []string{"Ah", "Kh"}},
		{Seat: 1, Name: "bob", Chips: 200, HoleCards: []string{"7c", "2d"}},
	}
}

func TestMonitorMasksHoleCardsByDefault(t *testing.T) {
	monitor, path := newTestMonitor(t, false)

	monitor.OnHandStart("hand-1", samplePlayers(), 0, Blinds{Small: 1, Big: 2})
	monitor.OnHandComplete(Outcome{HandID: "hand-1"})

	if err := monitor.Flush(); err != nil {
		t.Fatalf("Flush error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Read file: %v", err)
	}
	if !strings.Contains(string(data), "????") {
		t.Fatalf("expected masked hole cards, got %s", data)
	}
}

func TestMonitorResetsSeatContributionsPerStreet(t *testing.T) {
	monitor, _ := newTestMonitor(t, true)

	monitor.OnHandStart("hand-1", samplePlayers(), 0, Blinds{Small: 1, Big: 2})
	monitor.OnPlayerAction("hand-1", 0, "raise", 10, 190) // total 10
	monitor.OnPlayerAction("hand-1", 0, "raise", 5, 185)  // total 15
	monitor.OnStreetChange("hand-1", "flop", []string{"Ah", "Kd", "Qs"})
	monitor.OnStreetChange("hand-1", "turn", []string{"Ah", "Kd", "Qs", "Tc"})
	monitor.OnPlayerAction("hand-1", 0, "raise", 20, 165) // new street total 20
	monitor.OnHandComplete(Outcome{HandID: "hand-1"})

	if len(monitor.buffer) != 1 {
		t.Fatalf("expected 1 buffered hand, got %d", len(monitor.buffer))
	}
	actions := monitor.buffer[0].Actions
	var hasPreflop bool
	var lastRaise string
	var sawFlop, sawTurn bool
	for _, action := range actions {
		if strings.HasPrefix(action, "p1 cbr") {
			if action == "p1 cbr 15" {
				hasPreflop = true
			}
			lastRaise = action
		}
		switch action {
		case "d db AhKdQs":
			sawFlop = true
		case "d db Tc":
			sawTurn = true
		}
	}
	if !hasPreflop {
		t.Fatalf("expected to capture cumulative preflop raise of 15, actions=%v", actions)
	}
	if lastRaise != "p1 cbr 20" {
		t.Fatalf("expected last raise after street reset to be 20, got %q", lastRaise)
	}
	if !sawFlop || !sawTurn {
		t.Fatalf("expected flop and turn board actions, got %v", actions)
	}
}

func TestMonitorRecordsShowdownActions(t *testing.T) {
	monitor, _ := newTestMonitor(t, false)
	players := samplePlayers()
	monitor.OnHandStart("hand-1", players, 0, Blinds{Small: 1, Big: 2})
	monitor.OnHandComplete(Outcome{
		HandID: "hand-1",
		Detail: &OutcomeDetail{
			BotOutcomes: []BotOutcome{
				{Seat: 0, HoleCards: []string{"Ah", "Kh"}, WentToShowdown: true},
				{Seat: 1, HoleCards: []string{"7c", "2d"}, WentToShowdown: true},
			},
		},
	})

	if len(monitor.buffer) != 1 {
		t.Fatalf("expected 1 buffered hand, got %d", len(monitor.buffer))
	}
	actions := monitor.buffer[0].Actions
	if !containsAction(actions, "p1 sm AhKh") {
		t.Fatalf("missing showdown action for seat 1: %v", actions)
	}
	if !containsAction(actions, "p2 sm 7c2d") {
		t.Fatalf("missing showdown action for seat 2: %v", actions)
	}
}

func containsAction(actions []string, target string) bool {
	for _, action := range actions {
		if action == target {
			return true
		}
	}
	return false
}

func TestMonitorPopulatesStructuredFields(t *testing.T) {
	monitor, _ := newTestMonitor(t, true)
	players := samplePlayers()
	monitor.OnHandStart("hand-1", players, 1, Blinds{Small: 1, Big: 2})
	monitor.OnStreetChange("hand-1", "flop", []string{"Ah", "Kd", "Qs"})
	monitor.OnStreetChange("hand-1", "turn", []string{"Ah", "Kd", "Qs", "9c"})
	monitor.OnStreetChange("hand-1", "river", []string{"Ah", "Kd", "Qs", "9c", "2d"})
	monitor.OnHandComplete(Outcome{
		HandID:         "hand-1",
		HandsCompleted: 5,
		HandLimit:      10,
		Detail: &OutcomeDetail{
			TotalPot:    40,
			BotOutcomes: []BotOutcome{{Name: "alice", Seat: 0, NetChips: 20, Won: true}, {Name: "bob", Seat: 1, NetChips: -20}},
		},
	})

	hist := monitor.buffer[0]
	if hist.Table != "test" {
		t.Fatalf("expected table test, got %s", hist.Table)
	}
	if hist.SeatCount != 2 {
		t.Fatalf("expected seat count 2, got %d", hist.SeatCount)
	}
	if !slices.Equal(hist.Seats, []int{2, 1}) {
		t.Fatalf("expected seats [2 1], got %v", hist.Seats)
	}
	if !slices.Equal(hist.FinishingStacks, []int{180, 220}) {
		t.Fatalf("unexpected finishing stacks: %v", hist.FinishingStacks)
	}
	if !slices.Equal(hist.Winnings, []int{0, 20}) {
		t.Fatalf("unexpected winnings: %v", hist.Winnings)
	}
}

func TestMonitorOrdersPlayersFromSmallBlindToButton(t *testing.T) {
	monitor, _ := newTestMonitor(t, false)
	players := []Player{
		{Seat: 0, Name: "alice", Chips: 200},
		{Seat: 1, Name: "bob", Chips: 200},
		{Seat: 2, Name: "carol", Chips: 200},
		{Seat: 3, Name: "dave", Chips: 200},
		{Seat: 4, Name: "erin", Chips: 200},
	}
	monitor.OnHandStart("hand-rot", players, 1, Blinds{Small: 5, Big: 10})
	monitor.OnHandComplete(Outcome{HandID: "hand-rot"})

	if len(monitor.buffer) != 1 {
		t.Fatalf("expected one buffered hand, got %d", len(monitor.buffer))
	}
	hist := monitor.buffer[0]
	wantSeats := []int{3, 4, 5, 1, 2}
	if !slices.Equal(hist.Seats, wantSeats) {
		t.Fatalf("expected seats %v, got %v", wantSeats, hist.Seats)
	}
	wantPlayers := []string{"carol", "dave", "erin", "alice", "bob"}
	if !slices.Equal(hist.Players, wantPlayers) {
		t.Fatalf("expected players %v, got %v", wantPlayers, hist.Players)
	}
	wantStacks := []int{200, 200, 200, 200, 200}
	if !slices.Equal(hist.StartingStacks, wantStacks) {
		t.Fatalf("unexpected starting stacks: %v", hist.StartingStacks)
	}
	wantBlinds := []int{5, 10, 0, 0, 0}
	if !slices.Equal(hist.BlindsOrStraddles, wantBlinds) {
		t.Fatalf("unexpected blinds order: %v", hist.BlindsOrStraddles)
	}
	if hist.Seats[len(hist.Seats)-1] != 2 {
		t.Fatalf("expected button seat (2) to be last, got %d", hist.Seats[len(hist.Seats)-1])
	}
}

func TestMonitorFlushWritesSectionHeaders(t *testing.T) {
	monitor, path := newTestMonitor(t, false)
	monitor.OnHandStart("hand-1", samplePlayers(), 0, Blinds{Small: 1, Big: 2})
	monitor.OnHandComplete(Outcome{HandID: "hand-1"})
	monitor.OnHandStart("hand-2", samplePlayers(), 0, Blinds{Small: 1, Big: 2})
	monitor.OnHandComplete(Outcome{HandID: "hand-2"})
	if err := monitor.Flush(); err != nil {
		t.Fatalf("flush error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	contents := string(data)
	if !strings.Contains(contents, "[1]") || !strings.Contains(contents, "[2]") {
		t.Fatalf("expected numeric sections, got %s", contents)
	}
	if strings.Contains(contents, "[metadata]") {
		t.Fatalf("expected no metadata table, got %s", contents)
	}
}
