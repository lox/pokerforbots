package bot

import (
	"io"
	"math/rand"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/lox/holdem-cli/internal/deck"
	"github.com/lox/holdem-cli/internal/game"
)

func TestBotHandStrengthEvaluation(t *testing.T) {
	logger := log.NewWithOptions(io.Discard, log.Options{})
	bot := NewBot(logger)

	// Create a pre-flop table for testing
	table := game.NewTable(rand.New(rand.NewSource(0)), game.TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})
	table.CurrentRound = game.PreFlop

	// Test pocket aces (should be very strong)
	thinking := &ThinkingContext{}
	player := game.NewPlayer(1, "Test", game.AI, 100)
	player.HoleCards = []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Hearts, Rank: deck.Ace},
	}
	equityCtx := bot.evaluateHandStrengthWithThinking(player, table, thinking)
	if equityCtx.Strength != VeryStrong {
		t.Errorf("Pocket aces should be VeryStrong, got %s", equityCtx.Strength)
	}
	if equityCtx.Equity < 0.8 {
		t.Errorf("Pocket aces should have high equity, got %.2f", equityCtx.Equity)
	}

	// Test 7-2 offsuit (should be weak)
	thinking = &ThinkingContext{}
	player.HoleCards = []deck.Card{
		{Suit: deck.Spades, Rank: deck.Seven},
		{Suit: deck.Hearts, Rank: deck.Two},
	}
	equityCtx = bot.evaluateHandStrengthWithThinking(player, table, thinking)
	if equityCtx.Strength == VeryStrong {
		t.Errorf("7-2 offsuit should not be VeryStrong, got %s", equityCtx.Strength)
	}
	if equityCtx.Equity > 0.4 {
		t.Errorf("7-2 offsuit should have low equity, got %.2f", equityCtx.Equity)
	}
}

func TestBotPositionFactor(t *testing.T) {
	logger := log.NewWithOptions(io.Discard, log.Options{})
	bot := NewBot(logger)

	// Early position should be tighter (lower factor)
	earlyFactor := bot.getPositionFactor(game.UnderTheGun)
	if earlyFactor >= 1.0 {
		t.Errorf("Early position should be tighter, got factor %f", earlyFactor)
	}

	// Button should be loosest (higher factor)
	buttonFactor := bot.getPositionFactor(game.Button)
	if buttonFactor <= 1.0 {
		t.Errorf("Button should be looser, got factor %f", buttonFactor)
	}

	// Button should be looser than early position
	if buttonFactor <= earlyFactor {
		t.Errorf("Button (%f) should be looser than early position (%f)", buttonFactor, earlyFactor)
	}
}

func TestBotDecisionMaking(t *testing.T) {
	logger := log.NewWithOptions(io.Discard, log.Options{})
	bot := NewBot(logger)
	table := game.NewTable(rand.New(rand.NewSource(0)), game.TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	// Create a player with very strong hand
	player := game.NewPlayer(1, "Test", game.AI, 100)
	player.Position = game.Button
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
		decision := bot.MakeDecision(player, table)
		if decision.Action == game.Fold {
			foldCount++
		}
	}

	// Should fold very rarely with pocket aces
	if float64(foldCount)/float64(totalTests) > 0.1 {
		t.Errorf("Bot folding too often (%d/%d) with pocket aces", foldCount, totalTests)
	}
}

func TestBotRaiseAmount(t *testing.T) {
	logger := log.NewWithOptions(io.Discard, log.Options{})
	bot := NewBot(logger)
	table := game.NewTable(rand.New(rand.NewSource(0)), game.TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})
	player := game.NewPlayer(1, "Test", game.AI, 100)

	table.Pot = 20
	table.CurrentBet = 5
	table.BigBlind = 2

	// Test raise with strong hand
	raiseAmount := bot.calculateRaiseAmount(player, table, Strong)

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

func TestBotExecuteAction(t *testing.T) {
	logger := log.NewWithOptions(io.Discard, log.Options{})
	bot := NewBot(logger)
	table := game.NewTable(rand.New(rand.NewSource(0)), game.TableConfig{
		MaxSeats:   6,
		SmallBlind: 1,
		BigBlind:   2,
	})

	player := game.NewPlayer(1, "Test", game.AI, 100)
	player.Position = game.Button
	table.AddPlayer(player)

	// Test that bot can execute actions without errors
	initialChips := player.Chips
	table.CurrentBet = 5
	table.Pot = 10

	reasoning := bot.ExecuteAction(player, table)

	// Should return some reasoning
	if reasoning == "" {
		t.Error("Bot should return reasoning for its action")
	}

	// Player should have taken some action
	if player.LastAction == game.NoAction {
		t.Error("Bot should have taken an action")
	}

	// If player called or raised, chips should have changed
	if player.LastAction == game.Call || player.LastAction == game.Raise || player.LastAction == game.AllIn {
		if player.Chips >= initialChips {
			t.Error("Player chips should have decreased after betting action")
		}
	}
}

func TestBotDrawEvaluation(t *testing.T) {
	logger := log.NewWithOptions(io.Discard, log.Options{})
	bot := NewBot(logger)

	// Test flush draw (A♠K♠ on Q♠J♠2♥)
	flushDrawHole := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Spades, Rank: deck.King},
	}
	flushDrawBoard := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Queen},
		{Suit: deck.Spades, Rank: deck.Jack},
		{Suit: deck.Hearts, Rank: deck.Two},
	}

	drawStrength := bot.evaluateDraws(flushDrawHole, flushDrawBoard)
	if drawStrength < 3 {
		t.Errorf("A♠K♠ on Q♠J♠2♥ should have high draw strength (flush + straight + overcards), got %d", drawStrength)
	}

	// Test no draws (2♥3♣ on A♠K♦Q♠)
	noDrawHole := []deck.Card{
		{Suit: deck.Hearts, Rank: deck.Two},
		{Suit: deck.Clubs, Rank: deck.Three},
	}
	noDrawBoard := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Diamonds, Rank: deck.King},
		{Suit: deck.Spades, Rank: deck.Queen},
	}

	drawStrength = bot.evaluateDraws(noDrawHole, noDrawBoard)
	if drawStrength > 0 {
		t.Errorf("2♥3♣ on A♠K♦Q♠ should have no draws, got %d", drawStrength)
	}
}

func TestBotBoardTextureAnalysis(t *testing.T) {
	logger := log.NewWithOptions(io.Discard, log.Options{})
	bot := NewBot(logger)

	// Test dry board (A♠7♥2♣)
	dryBoard := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Hearts, Rank: deck.Seven},
		{Suit: deck.Clubs, Rank: deck.Two},
	}
	texture := bot.analyzeBoardTexture(dryBoard)
	if texture != DryBoard {
		t.Errorf("A♠7♥2♣ should be dry board, got %v", texture)
	}

	// Test wet board (J♠T♠9♥)
	wetBoard := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Jack},
		{Suit: deck.Spades, Rank: deck.Ten},
		{Suit: deck.Hearts, Rank: deck.Nine},
	}
	texture = bot.analyzeBoardTexture(wetBoard)
	if texture == DryBoard {
		t.Errorf("J♠T♠9♥ should not be dry board, got %v", texture)
	}
}

func TestBotInterface(t *testing.T) {
	logger := log.NewWithOptions(io.Discard, log.Options{})

	// Verify that Bot implements the Bot interface
	var _ game.Bot = NewBot(logger)
}

func TestBotConfiguration(t *testing.T) {
	logger := log.NewWithOptions(io.Discard, log.Options{})

	// Test default bot
	defaultBot := NewBot(logger)
	if defaultBot.config.Name != "Default" {
		t.Errorf("Expected default bot name 'Default', got %s", defaultBot.config.Name)
	}

	// Test configured bot
	tightBot := NewBotWithConfig(logger, TightBotConfig)
	if tightBot.config.Name != "Tight" {
		t.Errorf("Expected tight bot name 'Tight', got %s", tightBot.config.Name)
	}
	if tightBot.config.TightnessFactor != 1.5 {
		t.Errorf("Expected tight bot tightness factor 1.5, got %.1f", tightBot.config.TightnessFactor)
	}

	// Test aggressive bot
	aggroBot := NewBotWithConfig(logger, AggressiveBotConfig)
	if aggroBot.config.AggressionFactor != 1.8 {
		t.Errorf("Expected aggressive bot aggression factor 1.8, got %.1f", aggroBot.config.AggressionFactor)
	}
}
