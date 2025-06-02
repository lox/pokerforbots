package evaluator

import (
	"math/rand"
	"testing"

	"github.com/lox/holdem-cli/internal/deck"
)

// TestTightRangeImprovements validates our tight range improvements
func TestTightRangeImprovements(t *testing.T) {
	// Test that tight range gives realistic equity for weak hands like 5♣6♣
	fiveClubs := deck.Card{Rank: 5, Suit: 0}   // 5♣
	sixClubs := deck.Card{Rank: 6, Suit: 0}    // 6♣
	hand := []deck.Card{fiveClubs, sixClubs}

	// Verify this hand is not considered "tight"
	isTight := isTightHand(hand)
	if isTight {
		t.Error("5♣6♣ should not be considered a tight hand with our improved definition")
	}

	// Test equity calculation vs tight range
	rng := rand.New(rand.NewSource(1))
	equity := EstimateEquity(hand, []deck.Card{}, TightRange{}, 1000, rng)
	t.Logf("5♣6♣ equity vs TightRange: %.1f%%", equity*100)

	// Should be around 30-40% vs a realistic tight range (includes AK, AT, etc.)
	if equity > 0.45 || equity < 0.25 {
		t.Errorf("5♣6♣ equity vs tight range outside expected range: %.1f%% (should be ~30-40%%)", equity*100)
	}
}

func TestTightRangeSamplingQuality(t *testing.T) {
	// Verify TightRange samples mostly tight hands
	rng := rand.New(rand.NewSource(1))
	
	var availableCards []deck.Card
	for suit := 0; suit < 4; suit++ {
		for rank := 2; rank <= 14; rank++ {
			availableCards = append(availableCards, deck.Card{Rank: rank, Suit: suit})
		}
	}
	
	tightRange := TightRange{}
	tightCount := 0
	samples := 100
	
	for i := 0; i < samples; i++ {
		hand, ok := tightRange.SampleHand(availableCards, rng)
		if !ok {
			t.Error("Failed to sample hand")
			continue
		}
		
		if isTightHand(hand) {
			tightCount++
		}
		
		if i < 5 { // Log first 5 samples for debugging
			t.Logf("Sample %d: %s %s (tight: %v)", 
				i+1, hand[0].String(), hand[1].String(), isTightHand(hand))
		}
	}
	
	tightPercentage := float64(tightCount) * 100 / float64(samples)
	t.Logf("TightRange sampling: %d/%d tight hands (%.1f%%)", tightCount, samples, tightPercentage)
	
	// Should sample mostly tight hands (allowing for some fallback to MediumRange)
	if tightPercentage < 70 {
		t.Errorf("TightRange should sample mostly tight hands, got %.1f%%", tightPercentage)
	}
}

func TestEquityProgressionAcrossRanges(t *testing.T) {
	// Test the key insight: equity should decrease as opponent range gets tighter
	j7 := []deck.Card{
		{Rank: 11, Suit: 1}, // J♦
		{Rank: 7, Suit: 2},  // 7♥
	}
	
	board := []deck.Card{
		{Rank: 14, Suit: 1}, // A♥ - Ace on board
		{Rank: 10, Suit: 2}, // T♦ 
		{Rank: 7, Suit: 3},  // 7♠ - Bottom pair for J7
	}

	rng := rand.New(rand.NewSource(1))

	// Calculate equity against different range tightness levels
	randomEquity := EstimateEquity(j7, board, RandomRange{}, 1000, rng)
	mediumEquity := EstimateEquity(j7, board, MediumRange{}, 1000, rng)
	tightEquity := EstimateEquity(j7, board, TightRange{}, 1000, rng)
	
	t.Logf("J7 with bottom pair on A-T-7 board:")
	t.Logf("  vs Random range: %.1f%%", randomEquity*100)
	t.Logf("  vs Medium range: %.1f%%", mediumEquity*100) 
	t.Logf("  vs Tight range:  %.1f%%", tightEquity*100)
	
	// Equity should decrease as opponent range gets tighter (fundamental poker principle)
	if tightEquity >= mediumEquity {
		t.Error("Equity vs tight range should be less than vs medium range")
	}
	
	if mediumEquity >= randomEquity {
		t.Error("Equity vs medium range should be less than vs random range")
	}
	
	// The original bot bug: showing 64% equity vs aggressive opponents
	// Our fix should show more realistic numbers
	if tightEquity > 0.50 {
		t.Errorf("J7 vs tight range too high: %.1f%% (should be <50%% against aggressive opponents)", tightEquity*100)
	}
}

func TestTightHandDefinitionUpdates(t *testing.T) {
	// Test our updated tight hand definition
	testCases := []struct {
		rank1, suit1, rank2, suit2 int
		shouldBeTight               bool
		description                 string
	}{
		// Should be tight
		{14, 0, 13, 0, true, "A♣K♣ - premium suited"},
		{13, 0, 13, 1, true, "K♣K♦ - pocket kings"},
		{10, 0, 10, 1, true, "T♣T♦ - pocket tens (TT+ rule)"},
		{12, 0, 11, 1, true, "Q♣J♦ - two high cards (JJ+ rule)"},
		{14, 0, 10, 1, true, "A♣T♦ - ace with good kicker (AT+ rule)"},
		{10, 0, 9, 0, true, "T♣9♣ - premium suited connector"},

		// Should NOT be tight (fixed from original loose definition)
		{5, 0, 6, 0, false, "5♣6♣ - small suited connector"},
		{7, 0, 8, 0, false, "7♣8♣ - suited connector but not premium"},
		{14, 0, 9, 1, false, "A♣9♦ - ace with weak kicker (now requires T+)"},
		{9, 0, 9, 1, false, "9♣9♦ - pocket nines (only TT+ tight)"},
		{8, 0, 8, 1, false, "8♣8♦ - pocket eights (only TT+ tight)"},
		{9, 0, 8, 1, false, "9♣8♦ - medium cards, not tight"},
	}

	for _, tc := range testCases {
		hand := []deck.Card{
			{Rank: tc.rank1, Suit: tc.suit1},
			{Rank: tc.rank2, Suit: tc.suit2},
		}
		
		result := isTightHand(hand)
		if result != tc.shouldBeTight {
			t.Errorf("%s: expected tight=%v, got tight=%v", tc.description, tc.shouldBeTight, result)
		}
	}
}

func TestMediumRangeExists(t *testing.T) {
	// Verify MediumRange works as expected
	rng := rand.New(rand.NewSource(1))
	
	var availableCards []deck.Card
	for suit := 0; suit < 4; suit++ {
		for rank := 2; rank <= 14; rank++ {
			availableCards = append(availableCards, deck.Card{Rank: rank, Suit: suit})
		}
	}
	
	mediumRange := MediumRange{}
	samples := 50
	
	for i := 0; i < samples; i++ {
		hand, ok := mediumRange.SampleHand(availableCards, rng)
		if !ok {
			t.Error("Failed to sample from MediumRange")
			continue
		}
		
		if i < 3 { // Log a few samples
			t.Logf("MediumRange sample %d: %s %s", i+1, hand[0].String(), hand[1].String())
		}
	}
}
