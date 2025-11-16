package main

import (
	"testing"

	"github.com/lox/pokerforbots/v2/internal/phh"
	"github.com/lox/pokerforbots/v2/internal/server"
)

func TestPlaybackOutcomeComputesNetResults(t *testing.T) {
	players := []server.HandPlayer{
		{Seat: 0, Name: "p1"},
		{Seat: 1, Name: "p2"},
	}
	hand := phh.HandHistory{
		Seats:           []int{1, 2},
		StartingStacks:  []int{100, 100},
		FinishingStacks: []int{95, 105},
	}
	detail := playbackOutcome(hand, players, []string{"5d", "Js", "Ah"}, []int{5, 10}, nil)
	if detail == nil {
		t.Fatalf("expected detail, got nil")
	}
	if got := detail.BotOutcomes[0].NetChips; got != -5 {
		t.Fatalf("seat0 net mismatch: want -5 got %d", got)
	}
	if got := detail.BotOutcomes[1].NetChips; got != 5 {
		t.Fatalf("seat1 net mismatch: want 5 got %d", got)
	}
	if detail.BotOutcomes[1].WentToShowdown {
		t.Fatalf("should not mark showdown without metadata")
	}
	if len(detail.BotOutcomes[1].HoleCards) != 0 {
		t.Fatalf("expected no showdown hole cards, got %v", detail.BotOutcomes[1].HoleCards)
	}
}

func TestPlaybackOutcomeMetadataFallback(t *testing.T) {
	players := []server.HandPlayer{
		{Seat: 0, Name: "p1"},
		{Seat: 1, Name: "p2"},
	}
	hand := phh.HandHistory{
		Players: []string{"p1", "p2"},
		Seats:   []int{1, 2},
		Metadata: map[string]any{
			"winners":   []any{"p2"},
			"total_pot": int64(20),
		},
	}
	detail := playbackOutcome(hand, players, nil, []int{0, 0}, nil)
	if detail == nil {
		t.Fatalf("expected detail for legacy metadata")
	}
	if detail.TotalPot != 20 {
		t.Fatalf("expected total pot from metadata, got %d", detail.TotalPot)
	}
}

func TestPlaybackOutcomeMapsSeatsFromPositionOrder(t *testing.T) {
	players := []server.HandPlayer{
		{Seat: 0, Name: "sb"},
		{Seat: 1, Name: "bb"},
		{Seat: 2, Name: "button"},
	}
	hand := phh.HandHistory{
		Seats:           []int{1, 2, 3},
		StartingStacks:  []int{100, 100, 100},
		FinishingStacks: []int{105, 95, 100},
	}
	detail := playbackOutcome(hand, players, nil, []int{5, 10, 0}, nil)
	if detail == nil {
		t.Fatalf("expected detail")
	}
	if detail.BotOutcomes[1].NetChips != -5 {
		t.Fatalf("expected seat 1 to lose 5, got %d", detail.BotOutcomes[1].NetChips)
	}
	if detail.BotOutcomes[0].NetChips != 5 {
		t.Fatalf("expected seat 0 net +5, got %d", detail.BotOutcomes[0].NetChips)
	}
	if detail.BotOutcomes[2].NetChips != 0 {
		t.Fatalf("expected button to break even, got %d", detail.BotOutcomes[2].NetChips)
	}
}

func TestPlaybackShowdownActionUpdatesHoleCards(t *testing.T) {
	playback := &phhPlayback{monitor: server.NullHandMonitor{}}
	players := []server.HandPlayer{{Seat: 0, Name: "p1"}}
	stacks := []int{100}
	contributions := []int{0}
	investments := []int{0}
	currentBet := 0
	positionToSeat := []int{0}
	revealed := []bool{false}
	if err := playback.playAction("hand-1", "p1 sm AhKh", positionToSeat, stacks, contributions, investments, players, revealed, &currentBet); err != nil {
		t.Fatalf("playAction error: %v", err)
	}
	got := players[0].HoleCards
	if len(got) != 2 || got[0] != "Ah" || got[1] != "Kh" {
		t.Fatalf("expected showdown cards to be recorded, got %v", got)
	}
}

func TestPlaybackOutcomeInfersShowdownFromCards(t *testing.T) {
	players := []server.HandPlayer{
		{Seat: 0, Name: "p1", HoleCards: []string{"Ah", "Ad"}},
	}
	hand := phh.HandHistory{
		Seats:           []int{1},
		StartingStacks:  []int{100},
		FinishingStacks: []int{120},
	}
	revealed := []bool{true}
	detail := playbackOutcome(hand, players, nil, []int{50}, revealed)
	if detail == nil {
		t.Fatalf("expected detail")
	}
	if !detail.BotOutcomes[0].WentToShowdown {
		t.Fatalf("expected showdown inference from hole cards")
	}
	if len(detail.BotOutcomes[0].HoleCards) != 2 {
		t.Fatalf("expected hole cards propagated")
	}
}
