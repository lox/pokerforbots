package game

import (
	"io"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/lox/holdem-cli/internal/deck"
)

func TestAIHandStrengthEvaluation(t *testing.T) {
	logger := log.NewWithOptions(io.Discard, log.Options{})
	ai := NewAIEngine(logger)

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
	if strength != VeryStrong {
		t.Errorf("Offsuit AK should be VeryStrong, got %s", strength)
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
	logger := log.NewWithOptions(io.Discard, log.Options{})
	ai := NewAIEngine(logger)

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
	logger := log.NewWithOptions(io.Discard, log.Options{})
	ai := NewAIEngine(logger)
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
	logger := log.NewWithOptions(io.Discard, log.Options{})
	ai := NewAIEngine(logger)
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
	logger := log.NewWithOptions(io.Discard, log.Options{})
	ai := NewAIEngine(logger)
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
	logger := log.NewWithOptions(io.Discard, log.Options{})
	ai := NewAIEngine(logger)
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

func TestAIDrawEvaluation(t *testing.T) {
	logger := log.NewWithOptions(io.Discard, log.Options{})
	ai := NewAIEngine(logger)

	// Test flush draw (K♥Q♥ on J♠T♠4♥)
	holeCards := []deck.Card{
		{Suit: deck.Hearts, Rank: deck.King},
		{Suit: deck.Hearts, Rank: deck.Queen},
	}
	communityCards := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Jack},
		{Suit: deck.Spades, Rank: deck.Ten},
		{Suit: deck.Hearts, Rank: deck.Four},
	}

	drawStrength := ai.evaluateDraws(holeCards, communityCards)
	if drawStrength < 1 {
		t.Errorf("K♥Q♥ on J♠T♠4♥ should have draw strength >= 1 (gutshot + overcards), got %d", drawStrength)
	}

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

	drawStrength = ai.evaluateDraws(flushDrawHole, flushDrawBoard)
	if drawStrength < 3 {
		t.Errorf("A♠K♠ on Q♠J♠2♥ should have high draw strength (flush + straight + overcards), got %d", drawStrength)
	}

	// Test open-ended straight draw (9♥8♥ on 7♠6♣2♦)
	straightDrawHole := []deck.Card{
		{Suit: deck.Hearts, Rank: deck.Nine},
		{Suit: deck.Hearts, Rank: deck.Eight},
	}
	straightDrawBoard := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Seven},
		{Suit: deck.Clubs, Rank: deck.Six},
		{Suit: deck.Diamonds, Rank: deck.Two},
	}

	drawStrength = ai.evaluateDraws(straightDrawHole, straightDrawBoard)
	if drawStrength < 2 {
		t.Errorf("9♥8♥ on 7♠6♣2♦ should have good draw strength (open-ended straight), got %d", drawStrength)
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

	drawStrength = ai.evaluateDraws(noDrawHole, noDrawBoard)
	if drawStrength > 0 {
		t.Errorf("2♥3♣ on A♠K♦Q♠ should have no draws, got %d", drawStrength)
	}
}

func TestAIBoardTextureAnalysis(t *testing.T) {
	logger := log.NewWithOptions(io.Discard, log.Options{})
	ai := NewAIEngine(logger)

	// Test dry board (A♠7♥2♣)
	dryBoard := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Hearts, Rank: deck.Seven},
		{Suit: deck.Clubs, Rank: deck.Two},
	}
	texture := ai.analyzeBoardTexture(dryBoard)
	if texture != DryBoard {
		t.Errorf("A♠7♥2♣ should be dry board, got %v", texture)
	}

	// Test wet board (J♠T♠9♥)
	wetBoard := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Jack},
		{Suit: deck.Spades, Rank: deck.Ten},
		{Suit: deck.Hearts, Rank: deck.Nine},
	}
	texture = ai.analyzeBoardTexture(wetBoard)
	if texture == DryBoard {
		t.Errorf("J♠T♠9♥ should not be dry board, got %v", texture)
	}

	// Test very wet board (Q♠J♠T♠)
	veryWetBoard := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Queen},
		{Suit: deck.Spades, Rank: deck.Jack},
		{Suit: deck.Spades, Rank: deck.Ten},
	}
	texture = ai.analyzeBoardTexture(veryWetBoard)
	if texture < WetBoard {
		t.Errorf("Q♠J♠T♠ should be wet or very wet board, got %v", texture)
	}

	// Test paired board (K♠K♥7♣)
	pairedBoard := []deck.Card{
		{Suit: deck.Spades, Rank: deck.King},
		{Suit: deck.Hearts, Rank: deck.King},
		{Suit: deck.Clubs, Rank: deck.Seven},
	}
	texture = ai.analyzeBoardTexture(pairedBoard)
	if texture == DryBoard {
		t.Errorf("K♠K♥7♣ should not be dry board due to pair, got %v", texture)
	}
}

func TestAIPostFlopStrengthWithDraws(t *testing.T) {
	logger := log.NewWithOptions(io.Discard, log.Options{})
	ai := NewAIEngine(logger)

	// Test K♥Q♥ on J♠T♠4♥ (the problematic hand from the log)
	player := NewPlayer(1, "Test", AI, 100)
	player.HoleCards = []deck.Card{
		{Suit: deck.Hearts, Rank: deck.King},
		{Suit: deck.Hearts, Rank: deck.Queen},
	}

	communityCards := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Jack},
		{Suit: deck.Spades, Rank: deck.Ten},
		{Suit: deck.Hearts, Rank: deck.Four},
	}

	strength := ai.evaluatePostFlopStrength(player, communityCards)
	if strength == VeryWeak {
		t.Errorf("K♥Q♥ on J♠T♠4♥ should not be VeryWeak (has gutshot + overcards), got %s", strength.String())
	}

	// Test A♠K♠ with flush draw + straight draw
	player.HoleCards = []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Spades, Rank: deck.King},
	}

	flushDrawBoard := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Queen},
		{Suit: deck.Spades, Rank: deck.Jack},
		{Suit: deck.Hearts, Rank: deck.Two},
	}

	strength = ai.evaluatePostFlopStrength(player, flushDrawBoard)
	if strength < Medium {
		t.Errorf("A♠K♠ on Q♠J♠2♥ should be at least Medium (monster draw), got %s", strength.String())
	}

	// Test weak hand with no draws - adjust expectation since ace high might upgrade to Weak
	player.HoleCards = []deck.Card{
		{Suit: deck.Hearts, Rank: deck.Two},
		{Suit: deck.Clubs, Rank: deck.Three},
	}

	dryBoard := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Diamonds, Rank: deck.King},
		{Suit: deck.Hearts, Rank: deck.Queen},
	}

	strength = ai.evaluatePostFlopStrength(player, dryBoard)
	if strength > Weak {
		t.Errorf("2♥3♣ on A♠K♦Q♥ should not be stronger than Weak, got %s", strength.String())
	}
}

func TestAIContinuationBetting(t *testing.T) {
	logger := log.NewWithOptions(io.Discard, log.Options{})
	ai := NewAIEngine(logger)

	table := NewTable(6, 1, 2)
	table.CurrentRound = Flop
	table.CurrentBet = 0 // No bet to call

	player := NewPlayer(1, "Test", AI, 100)
	player.Position = Button // In position

	// Test on dry board - should c-bet more frequently
	dryBoard := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Hearts, Rank: deck.Seven},
		{Suit: deck.Clubs, Rank: deck.Two},
	}
	table.CommunityCards = dryBoard

	cBetCount := 0
	totalTests := 100
	for i := 0; i < totalTests; i++ {
		if ai.shouldContinuationBet(player, table, Weak, 1.2) {
			cBetCount++
		}
	}

	// Should c-bet frequently on dry boards in position
	cBetFreq := float64(cBetCount) / float64(totalTests)
	if cBetFreq < 0.5 {
		t.Errorf("Should c-bet more frequently on dry board in position, got %.2f", cBetFreq)
	}

	// Test on wet board - should c-bet less frequently
	wetBoard := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Jack},
		{Suit: deck.Spades, Rank: deck.Ten},
		{Suit: deck.Hearts, Rank: deck.Nine},
	}
	table.CommunityCards = wetBoard

	cBetCount = 0
	for i := 0; i < totalTests; i++ {
		if ai.shouldContinuationBet(player, table, Weak, 1.2) {
			cBetCount++
		}
	}

	wetCBetFreq := float64(cBetCount) / float64(totalTests)
	if wetCBetFreq >= cBetFreq {
		t.Errorf("Should c-bet less on wet board (%.2f) than dry board (%.2f)", wetCBetFreq, cBetFreq)
	}

	// Test out of position - should c-bet less
	player.Position = UnderTheGun // Out of position
	table.CommunityCards = dryBoard

	cBetCount = 0
	for i := 0; i < totalTests; i++ {
		if ai.shouldContinuationBet(player, table, Weak, 0.7) {
			cBetCount++
		}
	}

	oopCBetFreq := float64(cBetCount) / float64(totalTests)
	if oopCBetFreq >= cBetFreq {
		t.Errorf("Should c-bet less out of position (%.2f) than in position (%.2f)", oopCBetFreq, cBetFreq)
	}

	// Test pre-flop - should not c-bet
	table.CurrentRound = PreFlop
	if ai.shouldContinuationBet(player, table, Strong, 1.2) {
		t.Error("Should not c-bet pre-flop")
	}

	// Test when there's a bet to call - should not c-bet
	table.CurrentRound = Flop
	table.CurrentBet = 10
	if ai.shouldContinuationBet(player, table, Strong, 1.2) {
		t.Error("Should not c-bet when there's already a bet")
	}
}

func TestAIEnhancedPositionPlay(t *testing.T) {
	logger := log.NewWithOptions(io.Discard, log.Options{})
	ai := NewAIEngine(logger)

	table := NewTable(6, 1, 2)
	table.CurrentRound = Flop
	table.CurrentBet = 0
	table.Pot = 20

	// Player in position with draw
	player := NewPlayer(1, "Test", AI, 100)
	player.Position = Button
	player.HoleCards = []deck.Card{
		{Suit: deck.Hearts, Rank: deck.King},
		{Suit: deck.Hearts, Rank: deck.Queen},
	}

	table.CommunityCards = []deck.Card{
		{Suit: deck.Spades, Rank: deck.Jack},
		{Suit: deck.Spades, Rank: deck.Ten},
		{Suit: deck.Hearts, Rank: deck.Four},
	}
	table.ActivePlayers = []*Player{player}

	// Count aggressive actions (raise) vs passive (check/call)
	aggressiveCount := 0
	totalTests := 100

	for i := 0; i < totalTests; i++ {
		action := ai.MakeDecision(player, table)
		if action == Raise {
			aggressiveCount++
		}
	}

	aggressiveFreq := float64(aggressiveCount) / float64(totalTests)

	// Now test same hand out of position
	player.Position = UnderTheGun
	oopAggressiveCount := 0

	for i := 0; i < totalTests; i++ {
		action := ai.MakeDecision(player, table)
		if action == Raise {
			oopAggressiveCount++
		}
	}

	oopAggressiveFreq := float64(oopAggressiveCount) / float64(totalTests)

	// Should be more aggressive in position with draws
	if aggressiveFreq <= oopAggressiveFreq {
		t.Errorf("Should be more aggressive in position (%.2f) than out of position (%.2f) with draws",
			aggressiveFreq, oopAggressiveFreq)
	}
}
