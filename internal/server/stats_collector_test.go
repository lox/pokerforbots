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
	collector := NewDetailedStatsCollector(StatsDepthFull, 100, 10)

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

	// Check hand category stats (only for full depth)
	if collector.depth == StatsDepthFull {
		if len(detailedStats.HandCategoryStats) == 0 {
			t.Error("Expected hand category stats for full depth")
		}
	}
}

func TestDetailedStatsCollectorMemoryLimit(t *testing.T) {
	// Create collector with max 2 hands
	collector := NewDetailedStatsCollector(StatsDepthBasic, 2, 10)

	bot := &Bot{ID: "bot1", displayName: "TestBot", role: "player"}

	// Record 3 hands
	for i := 0; i < 3; i++ {
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
	collector := NewDetailedStatsCollector(StatsDepthDetailed, 100, 10)

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

func TestStatsDepthLevels(t *testing.T) {
	tests := []struct {
		depth               StatisticsDepth
		expectPositionStats bool
		expectStreetStats   bool
		expectCategoryStats bool
	}{
		{StatsDepthBasic, false, false, false},
		{StatsDepthDetailed, true, true, false},
		{StatsDepthFull, true, true, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.depth), func(t *testing.T) {
			collector := NewDetailedStatsCollector(tt.depth, 100, 10)

			bot := &Bot{ID: "bot1", displayName: "TestBot", role: "player"}

			detail := HandOutcomeDetail{
				HandID:         "hand1",
				ButtonPosition: 0,
				StreetReached:  "river",
				BotOutcomes: []BotHandOutcome{
					{
						Bot:            bot,
						Position:       0,
						ButtonDistance: 2, // Middle position
						HoleCards:      []string{"As", "Ks"},
						NetChips:       30,
						WentToShowdown: true,
						WonAtShowdown:  true,
						Actions: map[string]string{
							"preflop": "raise",
							"flop":    "bet",
							"turn":    "bet",
							"river":   "check",
						},
					},
				},
			}

			err := collector.RecordHandOutcome(detail)
			if err != nil {
				t.Errorf("RecordHandOutcome failed: %v", err)
			}

			detailedStats := collector.GetDetailedStats("bot1")
			if tt.depth == StatsDepthBasic {
				if detailedStats != nil {
					t.Error("Basic depth should not return detailed stats")
				}
			} else {
				if detailedStats == nil {
					t.Fatal("Expected detailed stats")
				}

				hasPositionStats := len(detailedStats.PositionStats) > 0
				if hasPositionStats != tt.expectPositionStats {
					t.Errorf("Position stats: expected %v, got %v", tt.expectPositionStats, hasPositionStats)
				}

				hasStreetStats := len(detailedStats.StreetStats) > 0
				if hasStreetStats != tt.expectStreetStats {
					t.Errorf("Street stats: expected %v, got %v", tt.expectStreetStats, hasStreetStats)
				}

				hasCategoryStats := len(detailedStats.HandCategoryStats) > 0
				if hasCategoryStats != tt.expectCategoryStats {
					t.Errorf("Category stats: expected %v, got %v", tt.expectCategoryStats, hasCategoryStats)
				}
			}
		})
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
		{[]string{"Ah", "Qh"}, "Premium"}, // AQ suited (implementation treats AQ as Premium)
		{[]string{"Tc", "Td"}, "Strong"},  // Pocket tens
		{[]string{"9s", "9d"}, "Medium"},  // Pocket nines
		{[]string{"7h", "7c"}, "Medium"},  // Pocket sevens
		{[]string{"5c", "5d"}, "Weak"},    // Small pocket pair
		{[]string{"7h", "2c"}, "Weak"},    // 72 offsuit (implementation has simpler categorization)
		{[]string{}, "unknown"},           // No cards
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
	collector := NewDetailedStatsCollector(StatsDepthFull, 10000, 10)
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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = collector.RecordHandOutcome(detail)
	}
}

func BenchmarkNullStatsCollection(b *testing.B) {
	collector := &NullStatsCollector{}

	detail := HandOutcomeDetail{
		HandID: "hand1",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = collector.RecordHandOutcome(detail)
	}
}
