package bot

import (
	"testing"

	"github.com/lox/pokerforbots/internal/deck"
	"github.com/lox/pokerforbots/internal/game"
)

func TestSituationRecognition(t *testing.T) {
	// Test the specific scenario from the hand history: 5♣6♣ from SB vs CO raise

	// Setup the situation
	player := game.PlayerState{
		Name:     "AI-4",
		Position: game.SmallBlind,
		HoleCards: []deck.Card{
			{Rank: 5, Suit: 0}, // 5♣
			{Rank: 6, Suit: 0}, // 6♣
		},
		Chips:        200,
		BetThisRound: 1,
		IsActive:     true,
	}

	// Create hand history showing CO raised
	handHistory := &game.HandHistory{
		Actions: []game.HandAction{
			{PlayerName: "CO", Action: game.Raise, Round: game.PreFlop, Amount: 8},
		},
	}

	tableState := game.TableState{
		CurrentRound: game.PreFlop,
		CurrentBet:   8, // CO raised to 8
		Pot:          13,
		BigBlind:     2,
		Players: []game.PlayerState{
			{Name: "CO", Position: game.Cutoff, BetThisRound: 8, IsActive: true},
			player,
		},
		ActingPlayerIdx: 1,
		HandHistory:     handHistory,
	}

	// Build situation context
	situationCtx := BuildSituationContext(player, tableState, Weak, 0.39, 1.6)

	t.Logf("Debug - ActionSequence: %v", situationCtx.ActionSequence)
	t.Logf("Debug - Position: %v", situationCtx.Player.Position)
	t.Logf("Debug - HandStrength: %v", situationCtx.HandStrength)
	t.Logf("Debug - CurrentBet: %v", situationCtx.TableState.CurrentBet)

	// Test situation recognition
	recognizer := NewSituationRecognizer()
	adjustment, reasoning := recognizer.EvaluateSituation(situationCtx)

	t.Logf("Situation: 5♣6♣ from SB vs CO raise")
	t.Logf("Reasoning: %s", reasoning)
	t.Logf("Adjustments: fold×%.2f, call×%.2f, raise×%.2f",
		adjustment.FoldMultiplier, adjustment.CallMultiplier, adjustment.RaiseMultiplier)

	// Should heavily discourage raising
	if adjustment.RaiseMultiplier > 0.3 {
		t.Errorf("Raise multiplier too high for 5♣6♣ from SB: %.2f", adjustment.RaiseMultiplier)
	}
}

func TestSituationRecognitionPostflop(t *testing.T) {
	// Test the postflop jam scenario: gut-shot draw OOP vs bet

	player := game.PlayerState{
		Name:     "AI-4",
		Position: game.SmallBlind,
		HoleCards: []deck.Card{
			{Rank: 5, Suit: 0}, // 5♣
			{Rank: 6, Suit: 0}, // 6♣
		},
		Chips:        150,
		BetThisRound: 0,
		IsActive:     true,
	}

	tableState := game.TableState{
		CurrentRound: game.Flop,
		CurrentBet:   76, // AI-2 bet 76
		Pot:          176,
		BigBlind:     2,
		CommunityCards: []deck.Card{
			{Rank: 7, Suit: 3},  // 7♠
			{Rank: 10, Suit: 0}, // T♣
			{Rank: 9, Suit: 3},  // 9♠
		},
		Players: []game.PlayerState{
			{Name: "AI-2", Position: game.LatePosition, BetThisRound: 76, IsActive: true},
			player,
		},
		ActingPlayerIdx: 1,
	}

	// Build situation context - gut-shot draw
	situationCtx := BuildSituationContext(player, tableState, Weak, 0.27, 2.3)

	// Test situation recognition
	recognizer := NewSituationRecognizer()
	adjustment, reasoning := recognizer.EvaluateSituation(situationCtx)

	t.Logf("Situation: 5♣6♣ gut-shot OOP vs pot-sized bet")
	t.Logf("Reasoning: %s", reasoning)
	t.Logf("Adjustments: fold×%.2f, call×%.2f, raise×%.2f",
		adjustment.FoldMultiplier, adjustment.CallMultiplier, adjustment.RaiseMultiplier)

	// Should heavily discourage raising (jamming)
	if adjustment.RaiseMultiplier > 0.4 {
		t.Errorf("Raise multiplier too high for gut-shot OOP vs bet: %.2f", adjustment.RaiseMultiplier)
	}
}

func TestPositionAdvantage(t *testing.T) {
	// Test that strong hands in position get encouraged to bet

	player := game.PlayerState{
		Name:     "Hero",
		Position: game.Button,
		HoleCards: []deck.Card{
			{Rank: 14, Suit: 0}, // A♣
			{Rank: 13, Suit: 1}, // K♥
		},
		Chips:        200,
		BetThisRound: 0,
		IsActive:     true,
	}

	tableState := game.TableState{
		CurrentRound: game.Flop,
		CurrentBet:   0, // Checked to us
		Pot:          40,
		BigBlind:     2,
		CommunityCards: []deck.Card{
			{Rank: 14, Suit: 1}, // A♥
			{Rank: 7, Suit: 2},  // 7♦
			{Rank: 2, Suit: 3},  // 2♠
		},
		Players: []game.PlayerState{
			{Name: "Opponent", Position: game.BigBlind, BetThisRound: 0, IsActive: true},
			player,
		},
		ActingPlayerIdx: 1,
	}

	// Build situation context - top pair in position
	situationCtx := BuildSituationContext(player, tableState, VeryStrong, 0.85, 0)

	// Test situation recognition
	recognizer := NewSituationRecognizer()
	adjustment, reasoning := recognizer.EvaluateSituation(situationCtx)

	t.Logf("Situation: TPTK in position vs check")
	t.Logf("Reasoning: %s", reasoning)
	t.Logf("Adjustments: fold×%.2f, call×%.2f, raise×%.2f",
		adjustment.FoldMultiplier, adjustment.CallMultiplier, adjustment.RaiseMultiplier)

	// Should encourage betting for value
	if adjustment.RaiseMultiplier < 1.2 {
		t.Errorf("Raise multiplier too low for strong hand in position: %.2f", adjustment.RaiseMultiplier)
	}
}
