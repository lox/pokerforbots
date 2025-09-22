package server

import (
	"testing"
)

func TestNullStatsCollector(t *testing.T) {
	collector := &NullStatsCollector{}

	// Test that all methods are no-ops and return expected values
	if collector.IsEnabled() {
		t.Error("NullStatsCollector should not be enabled")
	}

	// RecordHandOutcome should be a no-op
	err := collector.RecordHandOutcome(HandOutcomeDetail{HandID: "test"})
	if err != nil {
		t.Errorf("RecordHandOutcome should not return error: %v", err)
	}

	// GetPlayerStats should return nil
	if stats := collector.GetPlayerStats(); stats != nil {
		t.Error("GetPlayerStats should return nil")
	}

	// GetDetailedStats should return nil
	if stats := collector.GetDetailedStats("bot1"); stats != nil {
		t.Error("GetDetailedStats should return nil")
	}

	// Reset should be a no-op
	collector.Reset()
}

func TestDetailedStatsCollector(t *testing.T) {
	collector := NewDetailedStatsCollector(100, 10)

	// Test that it's enabled
	if !collector.IsEnabled() {
		t.Error("DetailedStatsCollector should be enabled")
	}

	// Create test bots
	bot1 := &Bot{ID: "bot1", displayName: "TestBot1", role: "player"}
	bot2 := &Bot{ID: "bot2", displayName: "TestBot2", role: "npc"}

	// Record a hand outcome
	detail := HandOutcomeDetail{
		HandID:         "hand1",
		ButtonPosition: 0,
		StreetReached:  "river",
		Board:          []string{"As", "Kh", "Qd", "Jc", "Tc"},
		BotOutcomes: []BotHandOutcome{
			{
				Bot:            bot1,
				Position:       0,
				ButtonDistance: 0,
				HoleCards:      []string{"Ac", "Ad"},
				NetChips:       50,
				WentToShowdown: true,
				WonAtShowdown:  true,
				Actions: map[string]string{
					"preflop": "raise",
					"flop":    "bet",
					"turn":    "check",
					"river":   "call",
				},
			},
			{
				Bot:            bot2,
				Position:       1,
				ButtonDistance: 1,
				HoleCards:      []string{"7h", "2d"},
				NetChips:       -50,
				WentToShowdown: false,
				WonAtShowdown:  false,
				Actions: map[string]string{
					"preflop": "call",
					"flop":    "fold",
				},
			},
		},
	}

	err := collector.RecordHandOutcome(detail)
	if err != nil {
		t.Errorf("RecordHandOutcome failed: %v", err)
	}

	// GetPlayerStats now returns nil since basic stats are maintained by BotPool
	playerStats := collector.GetPlayerStats()
	if playerStats != nil {
		t.Error("Expected GetPlayerStats to return nil for DetailedStatsCollector")
	}

	// Check detailed stats for bot1
	detailedStats := collector.GetDetailedStats("bot1")
	if detailedStats == nil {
		t.Fatal("Expected detailed stats for bot1")
	}

	// BB/100 = (50/10) / 1 * 100 = 500
	expectedBB100 := 500.0
	if detailedStats.BB100 != expectedBB100 {
		t.Errorf("Expected BB100 %.2f, got %.2f", expectedBB100, detailedStats.BB100)
	}

	if detailedStats.WinRate != 100.0 {
		t.Errorf("Expected win rate 100%%, got %.2f%%", detailedStats.WinRate)
	}

	if detailedStats.ShowdownWinRate != 100.0 {
		t.Errorf("Expected showdown win rate 100%%, got %.2f%%", detailedStats.ShowdownWinRate)
	}

	// Check position stats
	if len(detailedStats.PositionStats) == 0 {
		t.Error("Expected position stats")
	}

	buttonStats, exists := detailedStats.PositionStats["Button"]
	if !exists {
		t.Error("Expected button position stats")
	}
	if buttonStats.Hands != 1 {
		t.Errorf("Expected 1 hand on button, got %d", buttonStats.Hands)
	}

	// Check street stats
	if len(detailedStats.StreetStats) == 0 {
		t.Error("Expected street stats")
	}

	// Check hand category stats are present in detailed mode
	if len(detailedStats.HandCategoryStats) == 0 {
		t.Error("Expected hand category stats")
	}
}

func TestDetailedStatsCollectorMemoryLimit(t *testing.T) {
	// Create collector with max 2 hands
	collector := NewDetailedStatsCollector(2, 10)

	bot := &Bot{ID: "bot1", displayName: "TestBot", role: "player"}

	// Record 3 hands
	for i := range 3 {
		detail := HandOutcomeDetail{
			HandID:         "hand" + string(rune(i)),
			ButtonPosition: 0,
			StreetReached:  "river",
			BotOutcomes: []BotHandOutcome{
				{
					Bot:            bot,
					Position:       0,
					ButtonDistance: 0,
					NetChips:       10,
					WentToShowdown: false,
					WonAtShowdown:  false,
				},
			},
		}

		err := collector.RecordHandOutcome(detail)
		if err != nil {
			t.Errorf("RecordHandOutcome failed: %v", err)
		}
	}

	// Check memory usage after circular buffer reset
	currentHands, maxHands := collector.GetMemoryUsage()
	if maxHands != 2 {
		t.Errorf("Expected max hands 2, got %d", maxHands)
	}
	if currentHands != 1 { // Should have reset and now have 1 hand
		t.Errorf("Expected current hands 1 after reset, got %d", currentHands)
	}

	// GetPlayerStats returns nil for DetailedStatsCollector
	playerStats := collector.GetPlayerStats()
	if playerStats != nil {
		t.Error("Expected GetPlayerStats to return nil")
	}
}

func TestDetailedStatsCollectorReset(t *testing.T) {
	collector := NewDetailedStatsCollector(100, 10)

	bot := &Bot{ID: "bot1", displayName: "TestBot", role: "player"}

	// Record a hand
	detail := HandOutcomeDetail{
		HandID:         "hand1",
		ButtonPosition: 0,
		StreetReached:  "flop",
		BotOutcomes: []BotHandOutcome{
			{
				Bot:            bot,
				Position:       0,
				ButtonDistance: 0,
				NetChips:       25,
				WentToShowdown: false,
				WonAtShowdown:  false,
			},
		},
	}

	err := collector.RecordHandOutcome(detail)
	if err != nil {
		t.Errorf("RecordHandOutcome failed: %v", err)
	}

	// Verify stats exist by checking detailed stats
	if collector.GetDetailedStats("bot1") == nil {
		t.Error("Expected detailed stats before reset")
	}

	// Reset
	collector.Reset()

	// Verify stats are cleared
	if collector.GetDetailedStats("bot1") != nil {
		t.Error("Expected no detailed stats after reset")
	}

	// Verify memory counters are reset
	currentHands, _ := collector.GetMemoryUsage()
	if currentHands != 0 {
		t.Errorf("Expected 0 hands after reset, got %d", currentHands)
	}
}

func TestHandCategorization(t *testing.T) {
	tests := []struct {
		cards    []string
		expected string
	}{
		{[]string{"As", "Ad"}, "Premium"}, // Pocket aces
		{[]string{"Kh", "Kc"}, "Premium"}, // Pocket kings
		{[]string{"Ac", "Kd"}, "Premium"}, // AK offsuit
		{[]string{"Ah", "Qh"}, "Strong"},  // AQ suited
		{[]string{"Tc", "Td"}, "Strong"},  // Pocket tens
		{[]string{"9s", "9d"}, "Medium"},  // Pocket nines
		{[]string{"7h", "7c"}, "Medium"},  // Pocket sevens
		{[]string{"5c", "5d"}, "Weak"},    // Small pocket pair
		{[]string{"7h", "2c"}, "Trash"},   // 72 offsuit
		{[]string{}, "Unknown"},           // No cards (capitalized now)
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := categorizeHoleCards(tt.cards)
			if result != tt.expected {
				t.Errorf("categorizeHoleCards(%v) = %s, want %s", tt.cards, result, tt.expected)
			}
		})
	}
}

func BenchmarkStatsCollection(b *testing.B) {
	collector := NewDetailedStatsCollector(10000, 10)
	bot := &Bot{ID: "bot1", displayName: "TestBot", role: "player"}

	detail := HandOutcomeDetail{
		HandID:         "hand1",
		ButtonPosition: 0,
		StreetReached:  "river",
		Board:          []string{"As", "Kh", "Qd", "Jc", "Tc"},
		BotOutcomes: []BotHandOutcome{
			{
				Bot:            bot,
				Position:       0,
				ButtonDistance: 0,
				HoleCards:      []string{"Ac", "Ad"},
				NetChips:       50,
				WentToShowdown: true,
				WonAtShowdown:  true,
				Actions: map[string]string{
					"preflop": "raise",
					"flop":    "bet",
					"turn":    "check",
					"river":   "call",
				},
			},
		},
	}

	for b.Loop() {
		_ = collector.RecordHandOutcome(detail)
	}
}

func BenchmarkNullStatsCollection(b *testing.B) {
	collector := &NullStatsCollector{}

	detail := HandOutcomeDetail{
		HandID: "hand1",
	}

	for b.Loop() {
		_ = collector.RecordHandOutcome(detail)
	}
}
