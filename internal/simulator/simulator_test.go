package simulator

import (
	"testing"
	"time"

	"github.com/charmbracelet/log"
)

func TestNew(t *testing.T) {
	logger := log.NewWithOptions(nil, log.Options{Level: log.WarnLevel})
	config := Config{
		Hands:        100,
		OpponentType: "fold",
		Seed:         12345,
		Timeout:      5 * time.Second,
		Logger:       logger,
	}

	simulator := New(config)
	if simulator == nil {
		t.Fatal("New() returned nil")
	}
	if simulator.config.Hands != 100 {
		t.Errorf("Expected 100 hands, got %d", simulator.config.Hands)
	}
	if simulator.config.OpponentType != "fold" {
		t.Errorf("Expected 'fold' opponent, got %s", simulator.config.OpponentType)
	}
	if simulator.config.Seed != 12345 {
		t.Errorf("Expected seed 12345, got %d", simulator.config.Seed)
	}
}

func TestRunSimulation_Convenience(t *testing.T) {
	logger := log.NewWithOptions(nil, log.Options{Level: log.WarnLevel})
	
	stats, opponentInfo, err := RunSimulation(2, "fold", 12345, 5*time.Second, logger)
	if err != nil {
		t.Fatalf("RunSimulation failed: %v", err)
	}
	if stats == nil {
		t.Fatal("Expected statistics, got nil")
	}
	if opponentInfo != "fold" {
		t.Errorf("Expected 'fold' opponent info, got %s", opponentInfo)
	}
	if stats.Hands != 4 { // 2 hands * 2 (duplicate mode)
		t.Errorf("Expected 4 total hands, got %d", stats.Hands)
	}
}

func TestSimulator_Run_FoldBot(t *testing.T) {
	logger := log.NewWithOptions(nil, log.Options{Level: log.WarnLevel})
	config := Config{
		Hands:        2, // Small number for fast test
		OpponentType: "fold",
		Seed:         12345,
		Timeout:      5 * time.Second,
		Logger:       logger,
	}

	simulator := New(config)
	stats, opponentInfo, err := simulator.Run()
	
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}
	if stats == nil {
		t.Fatal("Expected statistics, got nil")
	}
	if opponentInfo != "fold" {
		t.Errorf("Expected 'fold' opponent info, got %s", opponentInfo)
	}
	if stats.Hands != 4 { // 2 hands * 2 (duplicate mode)
		t.Errorf("Expected 4 total hands, got %d", stats.Hands)
	}
	
	// Against fold bots, we should always win
	if stats.Mean() <= 0 {
		t.Errorf("Expected positive mean against fold bots, got %f", stats.Mean())
	}
}

func TestSimulator_Run_CallBot(t *testing.T) {
	logger := log.NewWithOptions(nil, log.Options{Level: log.WarnLevel})
	config := Config{
		Hands:        2,
		OpponentType: "call",
		Seed:         12345,
		Timeout:      5 * time.Second,
		Logger:       logger,
	}

	simulator := New(config)
	stats, opponentInfo, err := simulator.Run()
	
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}
	if stats == nil {
		t.Fatal("Expected statistics, got nil")
	}
	if opponentInfo != "call" {
		t.Errorf("Expected 'call' opponent info, got %s", opponentInfo)
	}
	if stats.Hands != 4 {
		t.Errorf("Expected 4 total hands, got %d", stats.Hands)
	}
}

func TestSimulator_Run_MixedOpponents(t *testing.T) {
	logger := log.NewWithOptions(nil, log.Options{Level: log.WarnLevel})
	config := Config{
		Hands:        2,
		OpponentType: "mixed",
		Seed:         12345,
		Timeout:      5 * time.Second,
		Logger:       logger,
	}

	simulator := New(config)
	stats, opponentInfo, err := simulator.Run()
	
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}
	if stats == nil {
		t.Fatal("Expected statistics, got nil")
	}
	expectedInfo := "mixed(tag,rand,tag,maniac,call)"
	if opponentInfo != expectedInfo {
		t.Errorf("Expected '%s' opponent info, got %s", expectedInfo, opponentInfo)
	}
	if stats.Hands != 4 {
		t.Errorf("Expected 4 total hands, got %d", stats.Hands)
	}
}

func TestCreateMixedOpponentTypes(t *testing.T) {
	mix := createMixedOpponentTypes()
	expected := []string{"tag", "rand", "tag", "maniac", "call"}
	
	if len(mix) != len(expected) {
		t.Errorf("Expected %d opponent types, got %d", len(expected), len(mix))
	}
	
	for i, expectedType := range expected {
		if i >= len(mix) || mix[i] != expectedType {
			t.Errorf("Expected opponent type %d to be %s, got %s", i, expectedType, mix[i])
		}
	}
}

func TestCreateOpponent(t *testing.T) {
	logger := log.NewWithOptions(nil, log.Options{Level: log.WarnLevel})
	
	testCases := []string{"fold", "call", "rand", "chart", "maniac", "tag"}
	
	for _, opponentType := range testCases {
		t.Run(opponentType, func(t *testing.T) {
			agent := createOpponent(opponentType, nil, logger)
			if agent == nil {
				t.Errorf("createOpponent(%s) returned nil", opponentType)
			}
		})
	}
}

func TestSimulator_PlayHand_Deterministic(t *testing.T) {
	logger := log.NewWithOptions(nil, log.Options{Level: log.WarnLevel})
	config := Config{
		Hands:        1,
		OpponentType: "fold",
		Seed:         12345,
		Timeout:      5 * time.Second,
		Logger:       logger,
	}

	simulator := New(config)
	
	// Play the same hand twice with same parameters
	result1 := simulator.playHand("fold", nil, 12345, 3)
	result2 := simulator.playHand("fold", nil, 12345, 3)
	
	// Results should be identical for deterministic seed
	if result1.NetBB != result2.NetBB {
		t.Errorf("Expected identical NetBB, got %f vs %f", result1.NetBB, result2.NetBB)
	}
	if result1.Position != result2.Position {
		t.Errorf("Expected identical Position, got %d vs %d", result1.Position, result2.Position)
	}
	if result1.Seed != result2.Seed {
		t.Errorf("Expected identical Seed, got %d vs %d", result1.Seed, result2.Seed)
	}
}

func TestSimulator_PlayHand_PositionTracking(t *testing.T) {
	logger := log.NewWithOptions(nil, log.Options{Level: log.WarnLevel})
	config := Config{
		Hands:        1,
		OpponentType: "fold",
		Seed:         12345,
		Timeout:      5 * time.Second,
		Logger:       logger,
	}

	simulator := New(config)
	
	// Test different positions
	for position := 1; position <= 6; position++ {
		result := simulator.playHand("fold", nil, 12345, position)
		if result.Position != position {
			t.Errorf("Expected position %d, got %d", position, result.Position)
		}
	}
}

func TestSimulator_PlayHandWithTimeout_Success(t *testing.T) {
	logger := log.NewWithOptions(nil, log.Options{Level: log.WarnLevel})
	config := Config{
		Hands:        1,
		OpponentType: "fold",
		Seed:         12345,
		Timeout:      5 * time.Second, // Generous timeout
		Logger:       logger,
	}

	simulator := New(config)
	
	result, err := simulator.playHandWithTimeout("fold", nil, 12345, 3)
	if err != nil {
		t.Fatalf("playHandWithTimeout failed: %v", err)
	}
	if result.Position != 3 {
		t.Errorf("Expected position 3, got %d", result.Position)
	}
	if result.Seed != 12345 {
		t.Errorf("Expected seed 12345, got %d", result.Seed)
	}
}

func TestSimulator_PlayHandWithTimeout_VeryShortTimeout(t *testing.T) {
	logger := log.NewWithOptions(nil, log.Options{Level: log.WarnLevel})
	config := Config{
		Hands:        1,
		OpponentType: "fold",
		Seed:         12345,
		Timeout:      1 * time.Nanosecond, // Extremely short timeout
		Logger:       logger,
	}

	simulator := New(config)
	
	_, err := simulator.playHandWithTimeout("fold", nil, 12345, 3)
	if err == nil {
		t.Error("Expected timeout error with very short timeout, got nil")
	}
	if !isTimeoutError(err) {
		t.Errorf("Expected timeout error, got: %v", err)
	}
}

func TestSimulator_Run_PositionRotation(t *testing.T) {
	logger := log.NewWithOptions(nil, log.Options{Level: log.WarnLevel})
	config := Config{
		Hands:        6, // Test full rotation
		OpponentType: "fold",
		Seed:         12345,
		Timeout:      5 * time.Second,
		Logger:       logger,
	}

	simulator := New(config)
	stats, _, err := simulator.Run()
	
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}
	
	// With 6 hands in duplicate mode, we should have 12 total hands
	// Each position should be represented
	if stats.Hands != 12 {
		t.Errorf("Expected 12 total hands, got %d", stats.Hands)
	}
	
	// Check that positions are being tracked
	totalPositionHands := 0
	for pos := 1; pos <= 6; pos++ {
		totalPositionHands += stats.PositionResults[pos].Hands
	}
	if totalPositionHands != 12 {
		t.Errorf("Expected 12 total position hands, got %d", totalPositionHands)
	}
}

func TestSimulator_Run_StatisticsIntegration(t *testing.T) {
	logger := log.NewWithOptions(nil, log.Options{Level: log.WarnLevel})
	config := Config{
		Hands:        3,
		OpponentType: "fold",
		Seed:         12345,
		Timeout:      5 * time.Second,
		Logger:       logger,
	}

	simulator := New(config)
	stats, _, err := simulator.Run()
	
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}
	
	// Verify statistics are being collected properly
	if stats.Hands != 6 { // 3 hands * 2 (duplicate mode)
		t.Errorf("Expected 6 total hands, got %d", stats.Hands)
	}
	
	// Against fold bots, should be profitable
	if stats.Mean() <= 0 {
		t.Errorf("Expected positive mean against fold bots, got %f", stats.Mean())
	}
	
	// Should have some winning hands
	totalWins := stats.ShowdownWins + stats.NonShowdownWins
	if totalWins == 0 {
		t.Error("Expected some winning hands against fold bots")
	}
	
	// Ledger should balance
	if !stats.IsLedgerBalanced() {
		t.Error("Expected balanced ledger")
	}
}

// Helper function to check if an error is a timeout error
func isTimeoutError(err error) bool {
	return err != nil && (err.Error() == "context deadline exceeded" || 
		err.Error() == "hand timed out after 1ns" ||
		err.Error()[:14] == "hand timed out")
}

func BenchmarkSimulator_PlayHand(b *testing.B) {
	logger := log.NewWithOptions(nil, log.Options{Level: log.WarnLevel})
	config := Config{
		Hands:        1,
		OpponentType: "fold",
		Seed:         12345,
		Timeout:      5 * time.Second,
		Logger:       logger,
	}

	simulator := New(config)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = simulator.playHand("fold", nil, int64(i), 3)
	}
}

func TestSimulator_Run_ValidationSuccess(t *testing.T) {
	logger := log.NewWithOptions(nil, log.Options{Level: log.WarnLevel})
	config := Config{
		Hands:        2,
		OpponentType: "fold",
		Seed:         12345,
		Timeout:      5 * time.Second,
		Logger:       logger,
	}

	simulator := New(config)
	stats, opponentInfo, err := simulator.Run()
	if err != nil {
		t.Fatalf("Run() should succeed with valid simulation, got error: %v", err)
	}
	
	if stats == nil {
		t.Fatal("Expected valid statistics, got nil")
	}
	if opponentInfo != "fold" {
		t.Errorf("Expected 'fold' opponent info, got %s", opponentInfo)
	}
	
	// Statistics should be valid after Run() completes
	if validationErr := stats.Validate(); validationErr != nil {
		t.Errorf("Statistics should be valid after successful Run(), got: %v", validationErr)
	}
}

func TestPrintSummary(t *testing.T) {
	logger := log.NewWithOptions(nil, log.Options{Level: log.WarnLevel})
	config := Config{
		Hands:        2,
		OpponentType: "fold",
		Seed:         12345,
		Timeout:      5 * time.Second,
		Logger:       logger,
	}

	simulator := New(config)
	stats, opponentInfo, err := simulator.Run()
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}

	// PrintSummary should not panic and should work with valid stats
	PrintSummary(stats, opponentInfo)
	
	// Test with mixed opponent type
	PrintSummary(stats, "mixed(tag,rand,tag,maniac,call)")
}

func BenchmarkSimulator_Run_SmallSim(b *testing.B) {
	logger := log.NewWithOptions(nil, log.Options{Level: log.WarnLevel})
	config := Config{
		Hands:        10,
		OpponentType: "fold",
		Seed:         12345,
		Timeout:      5 * time.Second,
		Logger:       logger,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		simulator := New(config)
		simulator.config.Seed = int64(i) // Vary seed
		_, _, err := simulator.Run()
		if err != nil {
			b.Fatalf("Run() failed: %v", err)
		}
	}
}
