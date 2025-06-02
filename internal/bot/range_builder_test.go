package bot

import (
	"math/rand"
	"testing"

	"github.com/lox/holdem-cli/internal/deck"
	"github.com/lox/holdem-cli/internal/evaluator"
	"github.com/lox/holdem-cli/internal/game"
)

func TestSimpleRangeBuilder_BuildOpponentRange(t *testing.T) {
	tests := []struct {
		name             string
		opponentPosition game.Position
		actions          []game.HandAction
		currentRound     game.BettingRound
		expectedRange    string
		description      string
	}{
		{
			name:             "UTG position baseline",
			opponentPosition: game.UnderTheGun,
			actions:          []game.HandAction{},
			currentRound:     game.PreFlop,
			expectedRange:    "tight",
			description:      "should use tight range for UTG",
		},
		{
			name:             "Button position baseline",
			opponentPosition: game.Button,
			actions:          []game.HandAction{},
			currentRound:     game.PreFlop,
			expectedRange:    "loose",
			description:      "should use loose range for Button",
		},
		{
			name:             "UTG preflop raise",
			opponentPosition: game.UnderTheGun,
			actions: []game.HandAction{
				{PlayerName: "Villain", Action: game.Raise, Round: game.PreFlop, Amount: 20},
			},
			currentRound:  game.PreFlop,
			expectedRange: "tight",
			description:   "UTG raise should keep tight range",
		},
		{
			name:             "Button preflop raise",
			opponentPosition: game.Button,
			actions: []game.HandAction{
				{PlayerName: "Villain", Action: game.Raise, Round: game.PreFlop, Amount: 20},
			},
			currentRound:  game.PreFlop,
			expectedRange: "tight",
			description:   "Button raise should tighten to tight range",
		},
		{
			name:             "Preflop raise + flop bet",
			opponentPosition: game.UnderTheGun,
			actions: []game.HandAction{
				{PlayerName: "Villain", Action: game.Raise, Round: game.PreFlop, Amount: 20},
				{PlayerName: "Villain", Action: game.Raise, Round: game.Flop, Amount: 40},
			},
			currentRound:  game.Flop,
			expectedRange: "tight",
			description:   "PF raise + flop bet should remain very tight",
		},
	}

	rangeBuilder := NewSimpleRangeBuilder()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock table state
			tableState := createMockTableStateForRange(tt.opponentPosition, tt.actions, tt.currentRound)

			// Build range
			range_, description := rangeBuilder.BuildOpponentRange("Villain", tableState)

			// Check that we got the expected range type
			actualRange := rangeBuilder.getRangeDescription(range_)
			if actualRange != tt.expectedRange {
				t.Errorf("Expected %s range, got %s range", tt.expectedRange, actualRange)
			}

			// Check that description is informative
			if len(description) < 10 {
				t.Errorf("Range description should be informative: %s", description)
			}

			t.Logf("Range: %s, Description: %s", actualRange, description)
		})
	}
}

func TestRangeEquityComparison(t *testing.T) {
	// Test the specific scenario from the log: J7 on A-T-7 flop vs aggressive opponent
	j7 := []deck.Card{
		{Rank: 11, Suit: 1}, // J♦
		{Rank: 7, Suit: 2},  // 7♥
	}

	board := []deck.Card{
		{Rank: 14, Suit: 1}, // A♥
		{Rank: 10, Suit: 2}, // T♦
		{Rank: 7, Suit: 3},  // 7♠
	}

	// Need RNG for equity calculation
	rng := rand.New(rand.NewSource(1))

	// Compare equity against different ranges
	randomEquity := evaluator.EstimateEquity(j7, board, evaluator.RandomRange{}, 1000, rng)
	tightEquity := evaluator.EstimateEquity(j7, board, evaluator.TightRange{}, 1000, rng)
	mediumEquity := evaluator.EstimateEquity(j7, board, evaluator.MediumRange{}, 1000, rng)

	t.Logf("J7 equity on A-T-7:")
	t.Logf("  vs Random range: %.1f%%", randomEquity*100)
	t.Logf("  vs Medium range: %.1f%%", mediumEquity*100)
	t.Logf("  vs Tight range:  %.1f%%", tightEquity*100)

	// Equity should decrease as opponent range gets tighter
	if tightEquity >= mediumEquity {
		t.Error("Equity vs tight range should be less than vs medium range")
	}

	if mediumEquity >= randomEquity {
		t.Error("Equity vs medium range should be less than vs random range")
	}

	// Against aggressive opponent (tight range), equity should be more realistic (~35%)
	if tightEquity > 0.50 {
		t.Errorf("J7 vs tight range should have reasonable equity, got %.1f%%", tightEquity*100)
	}
}

func TestRangeBuilderVsOldLogic(t *testing.T) {
	// Test that our range builder gives more realistic equity than the old random range approach

	// Scenario: Opponent raised preflop and bet flop (should be tight range)
	actions := []game.HandAction{
		{PlayerName: "Aggressor", Action: game.Raise, Round: game.PreFlop, Amount: 20},
		{PlayerName: "Aggressor", Action: game.Raise, Round: game.Flop, Amount: 40},
	}

	tableState := createMockTableStateForRange(game.UnderTheGun, actions, game.Flop)

	rangeBuilder := NewSimpleRangeBuilder()
	newRange, description := rangeBuilder.BuildOpponentRange("Aggressor", tableState)

	// Should use tight range for this aggressive sequence
	if rangeBuilder.getRangeDescription(newRange) != "tight" {
		t.Errorf("Expected tight range for PF raise + flop bet, got %s", rangeBuilder.getRangeDescription(newRange))
	}

	// Should mention the betting actions in description
	if !containsStr(description, "PF raise") && !containsStr(description, "bet") {
		t.Errorf("Description should mention betting actions: %s", description)
	}

	t.Logf("Range for aggressive opponent: %s", description)
}

// Helper functions
func createMockTableStateForRange(opponentPos game.Position, actions []game.HandAction, currentRound game.BettingRound) game.TableState {
	var handHistory *game.HandHistory
	if len(actions) > 0 {
		handHistory = &game.HandHistory{
			Actions: actions,
		}
	}

	players := []game.PlayerState{
		{Name: "Hero", Position: game.BigBlind, IsActive: true},
		{Name: "Villain", Position: opponentPos, IsActive: true},
		{Name: "Aggressor", Position: opponentPos, IsActive: true}, // For the other test
	}

	// Add community cards if postflop
	var communityCards []deck.Card
	if currentRound != game.PreFlop {
		communityCards = []deck.Card{
			{Rank: 14, Suit: 1}, // A♥
			{Rank: 10, Suit: 2}, // T♦
			{Rank: 7, Suit: 3},  // 7♠
		}
	}

	return game.TableState{
		Players:         players,
		CommunityCards:  communityCards,
		HandHistory:     handHistory,
		CurrentRound:    currentRound,
		ActingPlayerIdx: 0, // Hero acting
		Pot:             44,
		CurrentBet:      22,
		BigBlind:        2,
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
