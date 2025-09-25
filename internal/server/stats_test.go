package server

import (
	"math"
	"testing"
	"time"
)

func TestStatsMonitorBasicTracking(t *testing.T) {
	monitor := NewStatsMonitor(10, false, 0)

	bot := &Bot{ID: "bot1", done: make(chan struct{})}
	bot.botCommand = "./bot"
	bot.SetDisplayName("Test Bot")

	outcome := HandOutcome{
		HandID: "hand-1",
		Detail: &HandOutcomeDetail{
			HandID:         "hand-1",
			ButtonPosition: 0,
			StreetReached:  "river",
			BotOutcomes: []BotHandOutcome{
				{
					Bot:            bot,
					Position:       0,
					ButtonDistance: 0,
					HoleCards:      []string{"Ah", "Ad"},
					NetChips:       40,
					WentToShowdown: true,
					WonAtShowdown:  true,
					TimedOut:       true,
					InvalidActions: 2,
					Disconnected:   true,
					WentBroke:      true,
				},
			},
		},
	}

	monitor.OnHandComplete(outcome)

	players := monitor.GetPlayerStats()
	if len(players) != 1 {
		t.Fatalf("expected 1 player stat, got %d", len(players))
	}

	ps := players[0]
	if ps.BotID != "bot1" {
		t.Errorf("expected bot id bot1, got %s", ps.BotID)
	}
	if ps.Hands != 1 {
		t.Errorf("expected 1 hand, got %d", ps.Hands)
	}
	if ps.NetChips != 40 {
		t.Errorf("expected net chips 40, got %d", ps.NetChips)
	}
	if ps.Timeouts != 1 {
		t.Errorf("expected 1 timeout, got %d", ps.Timeouts)
	}
	if ps.InvalidActions != 2 {
		t.Errorf("expected 2 invalid actions, got %d", ps.InvalidActions)
	}
	if ps.Disconnects != 1 {
		t.Errorf("expected 1 disconnect, got %d", ps.Disconnects)
	}
	if ps.Busts != 1 {
		t.Errorf("expected 1 bust, got %d", ps.Busts)
	}
	if ps.DetailedStats != nil {
		t.Errorf("expected no detailed stats when detailed disabled")
	}
}

func TestStatsMonitorDetailedStats(t *testing.T) {
	monitor := NewStatsMonitor(10, true, 0)

	bot := &Bot{ID: "bot1", done: make(chan struct{})}
	bot.SetDisplayName("Aggro Bot")

	outcome := HandOutcome{
		HandID: "hand-2",
		Detail: &HandOutcomeDetail{
			HandID:         "hand-2",
			ButtonPosition: 1,
			StreetReached:  "turn",
			BotOutcomes: []BotHandOutcome{
				{
					Bot:            bot,
					Position:       0,
					ButtonDistance: 1,
					NetChips:       30,
					WentToShowdown: false,
					WonAtShowdown:  false,
					TimedOut:       false,
					InvalidActions: 0,
					WentBroke:      false,
				},
			},
		},
	}

	monitor.OnHandComplete(outcome)

	stats := monitor.GetDetailedStats("bot1")
	if stats == nil {
		t.Fatal("expected detailed stats for bot1")
	}

	expectedBB100 := 300.0
	if stats.BB100 != expectedBB100 {
		t.Errorf("expected BB/100 %.1f, got %.1f", expectedBB100, stats.BB100)
	}
	if stats.Timeouts != 0 {
		t.Errorf("expected 0 timeouts propagated, got %d", stats.Timeouts)
	}

	players := monitor.GetPlayerStats()
	if len(players) != 1 {
		t.Fatalf("expected 1 player stat, got %d", len(players))
	}
	if players[0].DetailedStats == nil {
		t.Fatalf("expected embedded detailed stats on player snapshot")
	}
}

func TestStatsMonitorResetsAtLimit(t *testing.T) {
	monitor := NewStatsMonitor(10, false, 2)

	bot := &Bot{ID: "bot1", done: make(chan struct{})}

	record := func(delta int) {
		outcome := HandOutcome{
			HandID: "hand",
			Detail: &HandOutcomeDetail{
				HandID:         "hand",
				ButtonPosition: 0,
				StreetReached:  "flop",
				BotOutcomes: []BotHandOutcome{
					{Bot: bot, NetChips: delta},
				},
			},
		}
		monitor.OnHandComplete(outcome)
	}

	record(10) // hand 1
	record(20) // hand 2
	record(-5) // triggers reset and records as first hand

	players := monitor.GetPlayerStats()
	if len(players) != 1 {
		t.Fatalf("expected 1 player, got %d", len(players))
	}

	ps := players[0]
	if ps.Hands != 1 {
		t.Errorf("expected hands reset to 1, got %d", ps.Hands)
	}
	if ps.NetChips != -5 {
		t.Errorf("expected net chips -5 after reset, got %d", ps.NetChips)
	}
}

func TestBotStatisticsRecordResponse(t *testing.T) {
	stats := NewBotStatistics(10)

	stats.RecordResponse(50*time.Millisecond, ResponseOutcomeSuccess)
	stats.RecordResponse(100*time.Millisecond, ResponseOutcomeSuccess)
	stats.RecordResponse(200*time.Millisecond, ResponseOutcomeSuccess)
	stats.RecordResponse(0, ResponseOutcomeTimeout)
	stats.RecordResponse(0, ResponseOutcomeDisconnect)
	stats.AddResult(0, false, false)

	proto := stats.ToProtocolStats()
	if proto == nil {
		t.Fatalf("expected protocol stats with latency data")
	}

	if proto.ResponsesTracked != 3 {
		t.Fatalf("expected 3 responses tracked, got %d", proto.ResponsesTracked)
	}
	if proto.ResponseTimeouts != 1 {
		t.Errorf("expected 1 response timeout, got %d", proto.ResponseTimeouts)
	}
	if proto.ResponseDisconnects != 1 {
		t.Errorf("expected 1 response disconnect, got %d", proto.ResponseDisconnects)
	}
	if proto.MaxResponseMs != 200 {
		t.Errorf("expected max response 200 ms, got %.1f", proto.MaxResponseMs)
	}
	if proto.MinResponseMs != 50 {
		t.Errorf("expected min response 50 ms, got %.1f", proto.MinResponseMs)
	}
	if math.Abs(proto.AvgResponseMs-116.6667) > 0.1 {
		t.Errorf("expected average latency ~116.67 ms, got %.2f", proto.AvgResponseMs)
	}
	if math.Abs(proto.P95ResponseMs-200) > 0.01 {
		t.Errorf("expected p95 latency 200 ms, got %.2f", proto.P95ResponseMs)
	}
	if math.Abs(proto.ResponseStdMs-62.360) > 0.5 {
		t.Errorf("expected stddev ~62.36 ms, got %.2f", proto.ResponseStdMs)
	}
}
