package game

import (
	"testing"

	"github.com/lox/holdem-cli/internal/deck"
)

func TestAIHandStrengthEvaluation(t *testing.T) {
	ai := NewAIEngine()
	
	// Test pocket aces (very strong)
	pocketAces := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Hearts, Rank: deck.Ace},
	}
	strength := ai.evaluatePreFlopStrength(pocketAces)
	if strength != VeryStrong {
		t.Errorf("Pocket aces should be VeryStrong, got %s", strength)
	}
	
	// Test suited ace-king (very strong)
	suitedAK := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Spades, Rank: deck.King},
	}
	strength = ai.evaluatePreFlopStrength(suitedAK)
	if strength != VeryStrong {
		t.Errorf("Suited AK should be VeryStrong, got %s", strength)
	}
	
	// Test off-suit ace-king (strong)
	offsuitAK := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Hearts, Rank: deck.King},
	}
	strength = ai.evaluatePreFlopStrength(offsuitAK)
	if strength != Strong {
		t.Errorf("Offsuit AK should be Strong, got %s", strength)
	}
	
	// Test 7-2 offsuit (very weak)
	trash := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Seven},
		{Suit: deck.Hearts, Rank: deck.Two},
	}
	strength = ai.evaluatePreFlopStrength(trash)
	if strength != VeryWeak {
		t.Errorf("7-2 offsuit should be VeryWeak, got %s", strength)
	}
}

func TestAIPositionFactor(t *testing.T) {
	ai := NewAIEngine()
	
	// Early position should be tighter (lower factor)
	earlyFactor := ai.getPositionFactor(UnderTheGun)
	if earlyFactor >= 1.0 {
		t.Errorf("Early position should be tighter, got factor %f", earlyFactor)
	}
	
	// Button should be loosest (higher factor)
	buttonFactor := ai.getPositionFactor(Button)
	if buttonFactor <= 1.0 {
		t.Errorf("Button should be looser, got factor %f", buttonFactor)
	}
	
	// Button should be looser than early position
	if buttonFactor <= earlyFactor {
		t.Errorf("Button (%f) should be looser than early position (%f)", buttonFactor, earlyFactor)
	}
}

func TestAIPotOdds(t *testing.T) {
	ai := NewAIEngine()
	table := NewTable(6, 1, 2)
	player := NewPlayer(1, "Test", AI, 100)
	
	// Test pot odds calculation
	table.Pot = 20
	table.CurrentBet = 10
	player.BetThisRound = 5
	
	odds := ai.calculatePotOdds(player, table)
	expected := 20.0 / 5.0 // pot / call amount
	if odds != expected {
		t.Errorf("Expected pot odds %f, got %f", expected, odds)
	}
	
	// Test when already called
	player.BetThisRound = 10
	odds = ai.calculatePotOdds(player, table)
	if odds != 0 {
		t.Errorf("Expected 0 pot odds when already called, got %f", odds)
	}
}

func TestAIDecisionMaking(t *testing.T) {
	ai := NewAIEngine()
	table := NewTable(6, 1, 2)
	
	// Create a player with very strong hand
	player := NewPlayer(1, "Test", AI, 100)
	player.Position = Button
	player.HoleCards = []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Hearts, Rank: deck.Ace},
	}
	
	table.AddPlayer(player)
	table.CurrentBet = 5
	table.Pot = 10
	
	// With pocket aces, should rarely fold
	foldCount := 0
	totalTests := 100
	
	for i := 0; i < totalTests; i++ {
		action := ai.MakeDecision(player, table)
		if action == Fold {
			foldCount++
		}
	}
	
	// Should fold very rarely with pocket aces
	if float64(foldCount)/float64(totalTests) > 0.1 {
		t.Errorf("AI folding too often (%d/%d) with pocket aces", foldCount, totalTests)
	}
}

func TestAIRaiseAmount(t *testing.T) {
	ai := NewAIEngine()
	table := NewTable(6, 1, 2)
	player := NewPlayer(1, "Test", AI, 100)
	
	table.Pot = 20
	table.CurrentBet = 5
	table.BigBlind = 2
	
	// Test raise with strong hand
	raiseAmount := ai.GetRaiseAmount(player, table, Strong)
	
	// Should be at least minimum raise
	minRaise := table.CurrentBet + table.BigBlind
	if raiseAmount < minRaise {
		t.Errorf("Raise amount %d should be at least min raise %d", raiseAmount, minRaise)
	}
	
	// Should not exceed player's chips
	maxRaise := player.Chips + player.BetThisRound
	if raiseAmount > maxRaise {
		t.Errorf("Raise amount %d should not exceed max %d", raiseAmount, maxRaise)
	}
}

func TestAIExecuteAction(t *testing.T) {
	ai := NewAIEngine()
	table := NewTable(6, 1, 2)
	
	player := NewPlayer(1, "Test", AI, 100)
	player.Position = Button
	table.AddPlayer(player)
	
	// Test that AI can execute actions without errors
	initialChips := player.Chips
	table.CurrentBet = 5
	table.Pot = 10
	
	ai.ExecuteAIAction(player, table)
	
	// Player should have taken some action
	if player.LastAction == NoAction {
		t.Error("AI should have taken an action")
	}
	
	// If player called or raised, chips should have changed
	if player.LastAction == Call || player.LastAction == Raise || player.LastAction == AllIn {
		if player.Chips >= initialChips {
			t.Error("Player chips should have decreased after betting action")
		}
	}
}