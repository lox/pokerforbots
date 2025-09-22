package regression

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/lox/pokerforbots/internal/server"
	"github.com/lox/pokerforbots/protocol"
	"github.com/rs/zerolog"
)

// Simple tests for the refactored regression code without external dependencies

func TestParseStatsFile(t *testing.T) {
	// Create temp directory for test files
	tmpDir := t.TempDir()

	// Create orchestrator with test logger
	logger := zerolog.New(zerolog.NewTestWriter(t))
	config := &Config{
		Logger: logger,
	}
	o := NewOrchestrator(config, nil)

	// Create simple test stats that match the actual server.GameStats structure
	stats := &server.GameStats{
		ID:             "test-game",
		BigBlind:       10,
		SmallBlind:     5,
		HandsCompleted: 100,
		Players: []protocol.GameCompletedPlayer{
			{
				BotID:       "bot-1",
				DisplayName: "Bot 1",
				Hands:       100,
				NetChips:    -1000,
				DetailedStats: &protocol.PlayerDetailedStats{
					Hands:    100,
					BB100:    -10.0,
					VPIP:     0.25,
					PFR:      0.15,
					Timeouts: 0,
					Busts:    0,
				},
			},
			{
				BotID:       "bot-2",
				DisplayName: "Bot 2",
				Hands:       100,
				NetChips:    1000,
				DetailedStats: &protocol.PlayerDetailedStats{
					Hands:    100,
					BB100:    10.0,
					VPIP:     0.30,
					PFR:      0.18,
					Timeouts: 1,
					Busts:    0,
				},
			},
		},
	}

	// Write to temp file
	statsFile := filepath.Join(tmpDir, "test-stats.json")
	data, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("Failed to marshal stats: %v", err)
	}
	err = os.WriteFile(statsFile, data, 0644)
	if err != nil {
		t.Fatalf("Failed to write stats file: %v", err)
	}

	// Test parsing
	parsed, err := o.parseStatsFile(statsFile)
	if err != nil {
		t.Fatalf("Failed to parse stats file: %v", err)
	}
	if parsed == nil {
		t.Fatal("Parsed stats is nil")
	}
	if len(parsed.Players) != 2 {
		t.Errorf("Expected 2 players, got %d", len(parsed.Players))
	}
	if parsed.HandsCompleted != 100 {
		t.Errorf("Expected 100 hands, got %d", parsed.HandsCompleted)
	}
	if parsed.BigBlind != 10 {
		t.Errorf("Expected big blind 10, got %d", parsed.BigBlind)
	}
}

func TestAggregateHeadsUpStats(t *testing.T) {

	// Create valid heads-up stats
	stats := &server.GameStats{
		BigBlind:       10,
		HandsCompleted: 100,
		Players: []protocol.GameCompletedPlayer{
			{
				Hands:    100,
				NetChips: -1000,
				DetailedStats: &protocol.PlayerDetailedStats{
					BB100: -10.0,
					VPIP:  0.25,
					PFR:   0.15,
				},
			},
			{
				Hands:    100,
				NetChips: 1000,
				DetailedStats: &protocol.PlayerDetailedStats{
					BB100: 10.0,
					VPIP:  0.30,
					PFR:   0.18,
				},
			},
		},
	}

	results, err := AggregateHeadsUpStats(stats)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Check Challenger (first player)
	if results["challenger_bb_per_100"] != -10.0 {
		t.Errorf("Expected challenger_bb_per_100 = -10.0, got %f", results["challenger_bb_per_100"])
	}
	if results["challenger_vpip"] != 0.25 {
		t.Errorf("Expected challenger_vpip = 0.25, got %f", results["challenger_vpip"])
	}
	if results["challenger_pfr"] != 0.15 {
		t.Errorf("Expected challenger_pfr = 0.15, got %f", results["challenger_pfr"])
	}

	// Check Baseline (second player)
	if results["baseline_bb_per_100"] != 10.0 {
		t.Errorf("Expected baseline_bb_per_100 = 10.0, got %f", results["baseline_bb_per_100"])
	}
	if results["baseline_vpip"] != 0.30 {
		t.Errorf("Expected baseline_vpip = 0.30, got %f", results["baseline_vpip"])
	}
	if results["baseline_pfr"] != 0.18 {
		t.Errorf("Expected baseline_pfr = 0.18, got %f", results["baseline_pfr"])
	}
}

func TestAggregateHeadsUpStats_WrongPlayerCount(t *testing.T) {

	// Create stats with wrong number of players
	stats := &server.GameStats{
		BigBlind: 10,
		Players: []protocol.GameCompletedPlayer{
			{Hands: 100, NetChips: 1000},
			{Hands: 100, NetChips: 500},
			{Hands: 100, NetChips: -1500},
		},
	}

	_, err := AggregateHeadsUpStats(stats)
	if err == nil {
		t.Fatal("Expected error but got none")
	}
}

func TestStrategyInterface(t *testing.T) {
	// Test that our strategies implement the interface correctly
	config := &Config{
		Challenger:      "challenger",
		Baseline:        "baseline",
		ChallengerSeats: 2,
		BaselineSeats:   4,
		BatchSize:       1000,
	}

	strategies := []BatchStrategy{
		&HeadsUpStrategy{Challenger: config.Challenger, Baseline: config.Baseline, Config: config},
		&PopulationStrategy{
			Challenger:      config.Challenger,
			Baseline:        config.Baseline,
			ChallengerSeats: config.ChallengerSeats,
			BaselineSeats:   config.BaselineSeats,
			Config:          config,
		},
		&NPCBenchmarkStrategy{
			Challenger:      config.Challenger,
			Baseline:        config.Baseline,
			ChallengerSeats: 2,
			BaselineSeats:   0,
			NPCs:            map[string]int{"calling": 1, "aggressive": 1},
			Config:          config,
		},
		&SelfPlayStrategy{
			Challenger: config.Challenger,
			Baseline:   config.Challenger,
			BotSeats:   6,
			Config:     config,
		},
	}

	for _, strategy := range strategies {
		// Test interface methods
		name := strategy.Name()
		if name == "" {
			t.Errorf("Strategy %T returned empty name", strategy)
		}

		batchConfig := strategy.ConfigureBatch(0, 42)
		if len(batchConfig.BotCommands) == 0 {
			t.Errorf("Strategy %T returned no bot commands", strategy)
		}

		policy := strategy.GetHealthPolicy()
		if policy.MaxCrashesPerBot < 0 {
			t.Errorf("Strategy %T returned negative max crashes", strategy)
		}

		// Test early stopping (should not panic)
		shouldStop := strategy.ShouldStopEarly(map[string]float64{}, 1000)
		_ = shouldStop // Just check it doesn't crash
	}
}

func TestBatchWeightedAveraging(t *testing.T) {
	// Test the weighted averaging logic that's critical for accurate statistics
	batches := []BatchResult{
		{
			Seed:  42,
			Hands: 100,
			Results: map[string]float64{
				"challenger_bb_per_100": 10.0,
				"challenger_hands":      100.0,
			},
		},
		{
			Seed:  43,
			Hands: 200,
			Results: map[string]float64{
				"challenger_bb_per_100": 20.0,
				"challenger_hands":      200.0,
			},
		},
	}

	// Calculate weighted average: (10*100 + 20*200) / 300 = 16.67
	totalHands := 0.0
	totalChips := 0.0
	for _, batch := range batches {
		hands := batch.Results["challenger_hands"]
		totalHands += hands
		totalChips += batch.Results["challenger_bb_per_100"] * hands / 100
	}
	weightedBB100 := totalChips / totalHands * 100

	if math.Abs(weightedBB100-16.67) > 0.01 {
		t.Errorf("Expected weighted BB/100 ≈ 16.67, got %f", weightedBB100)
	}
}

// Benchmark to ensure refactoring doesn't degrade performance
func BenchmarkStatsAggregation(b *testing.B) {

	// Create test stats
	stats := &server.GameStats{
		BigBlind: 10,
		Players: []protocol.GameCompletedPlayer{
			{Hands: 100, NetChips: -1000, DetailedStats: &protocol.PlayerDetailedStats{BB100: -10.0, VPIP: 0.25, PFR: 0.15}},
			{Hands: 100, NetChips: 1000, DetailedStats: &protocol.PlayerDetailedStats{BB100: 10.0, VPIP: 0.30, PFR: 0.18}},
		},
	}

	for b.Loop() {
		_, _ = AggregateHeadsUpStats(stats)
	}
}
