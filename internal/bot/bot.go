package bot

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/lox/holdem-cli/internal/deck"
	"github.com/lox/holdem-cli/internal/evaluator"
	"github.com/lox/holdem-cli/internal/game"
)

// Bot implements sophisticated poker AI that satisfies game.Bot interface
type Bot struct {
	rng    *rand.Rand
	logger *log.Logger
	config BotConfig
}

// NewBot creates a new bot with default configuration and time-based RNG
func NewBot(logger *log.Logger) *Bot {
	return NewBotWithConfig(logger, DefaultBotConfig())
}

// NewBotWithConfig creates a new bot with the specified configuration and time-based RNG
func NewBotWithConfig(logger *log.Logger, config BotConfig) *Bot {
	return &Bot{
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
		logger: logger.WithPrefix("bot").With("botType", config.Name),
		config: config,
	}
}

// NewBotWithRNG creates a new bot with controlled RNG for deterministic testing
func NewBotWithRNG(logger *log.Logger, config BotConfig, rng *rand.Rand) *Bot {
	return &Bot{
		rng:    rng,
		logger: logger.WithPrefix("bot").With("botType", config.Name),
		config: config,
	}
}

// MakeDecision analyzes the game state and returns a decision with reasoning
func (b *Bot) MakeDecision(tableState game.TableState, validActions []game.ValidAction) game.Decision {
	// Extract acting player from table state
	if tableState.ActingPlayerIdx < 0 || tableState.ActingPlayerIdx >= len(tableState.Players) {
		return game.Decision{
			Action:    game.Fold,
			Amount:    0,
			Reasoning: "Invalid acting player index",
		}
	}
	
	actingPlayer := tableState.Players[tableState.ActingPlayerIdx]
	
	// Create thinking context to accumulate thoughts
	thinking := &ThinkingContext{}

	// Evaluate hand strength with thinking
	equityCtx := b.evaluateHandStrengthWithThinking(actingPlayer, tableState, thinking)

	// Get position factor with thinking
	positionFactor := b.getPositionFactorWithThinking(actingPlayer.Position, thinking)

	// Calculate pot odds with thinking
	potOdds := b.calculatePotOddsWithThinking(actingPlayer, tableState, thinking)

	// Log decision factors
	holeCardsStr := ""
	if len(actingPlayer.HoleCards) >= 2 {
		holeCardsStr = actingPlayer.HoleCards[0].String() + " " + actingPlayer.HoleCards[1].String()
	}

	b.logger.Info("Bot decision analysis",
		"player", actingPlayer.Name,
		"round", tableState.CurrentRound.String(),
		"holeCards", holeCardsStr,
		"handStrength", equityCtx.Strength.String(),
		"equity", equityCtx.Equity,
		"position", actingPlayer.Position.String(),
		"positionFactor", positionFactor,
		"currentBet", tableState.CurrentBet,
		"playerBetThisRound", actingPlayer.BetThisRound,
		"playerChips", actingPlayer.Chips,
		"pot", tableState.Pot,
		"potOdds", potOdds)

	// Make decision based on hand strength, position, and pot odds with thinking
	action := b.makeDecisionBasedOnFactorsWithThinking(actingPlayer, tableState, equityCtx, positionFactor, potOdds, thinking, validActions)

	// Calculate bet amount if raising
	var amount int
	if action == game.Raise {
		amount = b.calculateRaiseAmount(actingPlayer, tableState, equityCtx.Strength)
	}

	// Validate decision against valid actions and apply fallback if needed
	decision := game.Decision{
		Action:    action,
		Amount:    amount,
		Reasoning: thinking.GetThoughts(),
	}
	
	decision = b.validateAndAdjustDecision(decision, validActions, thinking)

	b.logger.Info("Bot decision made",
		"player", actingPlayer.Name,
		"decision", decision.Action.String(),
		"amount", decision.Amount,
		"reasoning", decision.Reasoning)

	return decision
}



// HandStrength represents the relative strength of a hand
type HandStrength int

const (
	VeryWeak HandStrength = iota
	Weak
	Medium
	Strong
	VeryStrong
)

// EquityContext contains both hand strength category and raw equity value
type EquityContext struct {
	Strength HandStrength
	Equity   float64
}

// BotConfig contains configuration flags for different bot strategies
type BotConfig struct {
	Name              string  // Bot identifier for logging/stats
	AggressionFactor  float64 // 0.5-2.0, multiplier for raise probabilities
	TightnessFactor   float64 // 0.5-2.0, multiplier for fold thresholds
	BluffFrequency    float64 // 0.0-1.0, how often to semi-bluff with draws
	CBetFrequency     float64 // 0.0-1.0, continuation bet frequency
	EquityThreshold   float64 // Minimum equity edge needed for aggressive plays
	OpponentModel     string  // "random", "tight", "loose" for equity calculations
	PositionAwareness float64 // 0.0-2.0, how much position affects decisions
	PotOddsWeight     float64 // 0.0-2.0, how heavily to weight pot odds
}

// DefaultBotConfig returns a balanced configuration similar to current bot behavior
func DefaultBotConfig() BotConfig {
	return BotConfig{
		Name:              "Default",
		AggressionFactor:  1.0,
		TightnessFactor:   1.0,
		BluffFrequency:    0.3,
		CBetFrequency:     0.6,
		EquityThreshold:   0.05,
		OpponentModel:     "random",
		PositionAwareness: 1.0,
		PotOddsWeight:     1.0,
	}
}

// Preset bot configurations for testing
var (
	TightBotConfig = BotConfig{
		Name:              "Tight",
		AggressionFactor:  0.7,
		TightnessFactor:   1.5,
		BluffFrequency:    0.1,
		CBetFrequency:     0.4,
		EquityThreshold:   0.10,
		OpponentModel:     "random",
		PositionAwareness: 0.8,
		PotOddsWeight:     1.2,
	}

	LooseBotConfig = BotConfig{
		Name:              "Loose",
		AggressionFactor:  1.3,
		TightnessFactor:   0.7,
		BluffFrequency:    0.5,
		CBetFrequency:     0.8,
		EquityThreshold:   0.0,
		OpponentModel:     "loose",
		PositionAwareness: 1.2,
		PotOddsWeight:     0.8,
	}

	AggressiveBotConfig = BotConfig{
		Name:              "Aggressive",
		AggressionFactor:  1.8,
		TightnessFactor:   0.8,
		BluffFrequency:    0.7,
		CBetFrequency:     0.9,
		EquityThreshold:   0.0,
		OpponentModel:     "tight",
		PositionAwareness: 1.5,
		PotOddsWeight:     0.6,
	}

	NitBotConfig = BotConfig{
		Name:              "Nit",
		AggressionFactor:  0.5,
		TightnessFactor:   2.0,
		BluffFrequency:    0.05,
		CBetFrequency:     0.3,
		EquityThreshold:   0.15,
		OpponentModel:     "random",
		PositionAwareness: 0.6,
		PotOddsWeight:     1.5,
	}

	// ExploitBotConfig designed to exploit extremely passive opponents (like fold-bots)
	ExploitBotConfig = BotConfig{
		Name:              "Exploit",
		AggressionFactor:  3.0,  // Maximum aggression
		TightnessFactor:   0.3,  // Very loose - bet with any hand
		BluffFrequency:    1.0,  // Always bluff when possible
		CBetFrequency:     1.0,  // Always continuation bet
		EquityThreshold:   0.0,  // No equity requirement for aggression
		OpponentModel:     "tight", // Assume opponents fold a lot
		PositionAwareness: 0.5,  // Position less important vs folders
		PotOddsWeight:     0.1,  // Ignore pot odds vs folders
	}
)

// BoardTexture represents how coordinated the board is
type BoardTexture int

const (
	DryBoard BoardTexture = iota
	SemiWetBoard
	WetBoard
	VeryWetBoard
)

// String returns the string representation of hand strength
func (hs HandStrength) String() string {
	switch hs {
	case VeryWeak:
		return "Very Weak"
	case Weak:
		return "Weak"
	case Medium:
		return "Medium"
	case Strong:
		return "Strong"
	case VeryStrong:
		return "Very Strong"
	default:
		return "Unknown"
	}
}

// ThinkingContext accumulates AI thoughts during decision making
type ThinkingContext struct {
	thoughts []string
}

// AddThought adds a thought to the thinking process
func (tc *ThinkingContext) AddThought(thought string) {
	tc.thoughts = append(tc.thoughts, thought)
}

// GetThoughts returns the complete stream of thoughts
func (tc *ThinkingContext) GetThoughts() string {
	if len(tc.thoughts) == 0 {
		return "No clear reasoning available"
	}
	return strings.Join(tc.thoughts, ". ")
}

// evaluateHandStrengthWithThinking evaluates hand strength while building thoughts
func (b *Bot) evaluateHandStrengthWithThinking(player game.PlayerState, tableState game.TableState, thinking *ThinkingContext) EquityContext {
	if len(player.HoleCards) != 2 {
		thinking.AddThought("Missing hole cards, assuming very weak")
		return EquityContext{Strength: VeryWeak, Equity: 0.1}
	}

	// Pre-flop hand strength evaluation using equity
	if tableState.CurrentRound == game.PreFlop {
		// Calculate equity against random opponent for pre-flop evaluation
		equity := evaluator.EstimateEquity(player.HoleCards, []deck.Card{}, evaluator.RandomRange{}, 1000, b.rng)
		thinking.AddThought(fmt.Sprintf("Pre-flop equity vs random: %.1f%%", equity*100))

		strength := b.equityToHandStrength(equity)
		thinking.AddThought(fmt.Sprintf("Preflop strength: %s", strength.String()))
		return EquityContext{Strength: strength, Equity: equity}
	}

	// Post-flop evaluation with community cards
	if len(tableState.CommunityCards) >= 3 {
		// Add board description
		boardStr := b.getBoardDescription(tableState.CommunityCards)
		thinking.AddThought(fmt.Sprintf("Board: %s", boardStr))

		equityCtx := b.evaluatePostFlopStrengthWithThinking(player, tableState.CommunityCards, thinking)
		return equityCtx
	}

	thinking.AddThought("Not enough information, assuming medium strength")
	return EquityContext{Strength: Medium, Equity: 0.5}
}

// evaluatePostFlopStrengthWithThinking evaluates post-flop hand strength with thinking
func (b *Bot) evaluatePostFlopStrengthWithThinking(player game.PlayerState, communityCards []deck.Card, thinking *ThinkingContext) EquityContext {
	// Determine if we're in position (simplified: Button or Cutoff = in position)
	inPosition := player.Position == game.Button || player.Position == game.Cutoff

	// Choose opponent range based on config and position
	var opponentRange evaluator.Range
	switch b.config.OpponentModel {
	case "tight":
		opponentRange = evaluator.TightRange{}
		thinking.AddThought("Using tight opponent model")
	case "loose":
		opponentRange = evaluator.LooseRange{}
		thinking.AddThought("Using loose opponent model")
	default: // "random"
		opponentRange = evaluator.RandomRange{}
		thinking.AddThought("Using random opponent model")
	}

	// Adjust based on position if position awareness is enabled
	if b.config.PositionAwareness > 0.5 {
		if inPosition {
			thinking.AddThought("In position - adjusting opponent model tighter")
			switch b.config.OpponentModel {
			case "loose":
				opponentRange = evaluator.RandomRange{}
			case "random":
				opponentRange = evaluator.TightRange{}
			}
		} else {
			thinking.AddThought("Out of position - opponents can be looser")
		}
	}

	// Calculate equity
	equity := evaluator.EstimateEquity(player.HoleCards, communityCards, opponentRange, 500, b.rng)
	thinking.AddThought(fmt.Sprintf("Equity: %.1f%%", equity*100))

	// Convert equity to hand strength
	strength := b.equityToHandStrength(equity)
	thinking.AddThought(fmt.Sprintf("Hand strength: %s", strength.String()))

	return EquityContext{Strength: strength, Equity: equity}
}

// equityToHandStrength maps equity percentage to hand strength categories
func (b *Bot) equityToHandStrength(equity float64) HandStrength {
	switch {
	case equity >= 0.80:
		return VeryStrong
	case equity >= 0.65:
		return Strong
	case equity >= 0.45:
		return Medium
	case equity >= 0.25:
		return Weak
	default:
		return VeryWeak
	}
}

// getBoardDescription returns a readable description of the board
func (b *Bot) getBoardDescription(communityCards []deck.Card) string {
	if len(communityCards) == 0 {
		return "no board"
	}

	cardStrs := make([]string, len(communityCards))
	for i, card := range communityCards {
		cardStrs[i] = card.String()
	}

	boardTexture := b.analyzeBoardTexture(communityCards)
	textureDesc := ""
	switch boardTexture {
	case DryBoard:
		textureDesc = " (dry)"
	case SemiWetBoard:
		textureDesc = " (semi-coordinated)"
	case WetBoard:
		textureDesc = " (coordinated)"
	case VeryWetBoard:
		textureDesc = " (very wet)"
	}

	return strings.Join(cardStrs, "-") + textureDesc
}

// evaluateDraws evaluates drawing potential and returns draw strength (0-4)
func (b *Bot) evaluateDraws(holeCards []deck.Card, communityCards []deck.Card) int {
	if len(holeCards) != 2 || len(communityCards) < 3 {
		return 0
	}

	card1, card2 := holeCards[0], holeCards[1]
	drawStrength := 0

	// Check for flush draws
	if card1.Suit == card2.Suit {
		suitCount := 2 // Start with hole cards
		for _, comm := range communityCards {
			if comm.Suit == card1.Suit {
				suitCount++
			}
		}
		if suitCount == 4 {
			drawStrength += 2 // Flush draw (9 outs)
		}
	}

	// Check for straight draws
	allCards := make([]deck.Card, 0, len(holeCards)+len(communityCards))
	allCards = append(allCards, holeCards...)
	allCards = append(allCards, communityCards...)

	// Get unique ranks and sort them
	ranks := make(map[int]bool)
	for _, card := range allCards {
		ranks[card.Rank] = true
	}

	// Convert to sorted slice
	var sortedRanks []int
	for rank := deck.Two; rank <= deck.Ace; rank++ {
		if ranks[rank] {
			sortedRanks = append(sortedRanks, rank)
		}
	}

	// Check for straight draws
	straightDraws := b.countStraightDraws(sortedRanks)
	if straightDraws > 0 {
		// Open-ended straight draw (8 outs) or gutshot (4 outs)
		if straightDraws >= 8 {
			drawStrength += 2 // Open-ended
		} else if straightDraws >= 4 {
			drawStrength += 1 // Gutshot
		}
	}

	// Check for overcards
	if len(communityCards) >= 3 {
		overCards := 0
		boardHigh := deck.Two
		for _, comm := range communityCards {
			if comm.Rank > boardHigh {
				boardHigh = comm.Rank
			}
		}

		for _, hole := range holeCards {
			if hole.Rank > boardHigh {
				overCards++
			}
		}

		if overCards >= 2 {
			// Only count if both are decent overcards (Jack or higher)
			highOverCards := 0
			for _, hole := range holeCards {
				if hole.Rank > boardHigh && hole.Rank >= deck.Jack {
					highOverCards++
				}
			}
			if highOverCards >= 2 {
				drawStrength += 1 // Two high overcards (6 outs)
			}
		}
	}

	return drawStrength
}

// countStraightDraws estimates straight draw outs
func (b *Bot) countStraightDraws(sortedRanks []int) int {
	if len(sortedRanks) < 3 {
		return 0
	}

	maxOuts := 0

	// Check each possible 5-card straight
	for i := 0; i <= len(sortedRanks)-3; i++ {
		// Look for sequences that could make straights
		consecutive := 1
		gaps := 0

		for j := i + 1; j < len(sortedRanks) && j < i+5; j++ {
			diff := int(sortedRanks[j]) - int(sortedRanks[j-1])
			if diff == 1 {
				consecutive++
			} else if diff == 2 {
				gaps++
				consecutive++
			} else if diff > 2 {
				break
			}
		}

		// Estimate outs based on pattern
		if consecutive >= 4 && gaps <= 1 {
			if gaps == 0 {
				maxOuts = 8 // Open-ended straight draw
			} else {
				maxOuts = max(maxOuts, 4) // Gutshot
			}
		}
	}

	return maxOuts
}

// analyzeBoardTexture analyzes how coordinated the board is
func (b *Bot) analyzeBoardTexture(communityCards []deck.Card) BoardTexture {
	if len(communityCards) < 3 {
		return DryBoard
	}

	wetness := 0

	// Check for flush possibilities
	suitCounts := make(map[int]int)
	for _, card := range communityCards {
		suitCounts[card.Suit]++
	}

	maxSuitCount := 0
	for _, count := range suitCounts {
		if count > maxSuitCount {
			maxSuitCount = count
		}
	}

	if maxSuitCount >= 3 {
		wetness += 2 // Flush draw possible
	} else if maxSuitCount == 2 {
		wetness += 1 // Two-suited
	}

	// Check for straight possibilities
	ranks := make([]int, len(communityCards))
	for i, card := range communityCards {
		ranks[i] = card.Rank
	}

	// Sort ranks
	for i := 0; i < len(ranks); i++ {
		for j := i + 1; j < len(ranks); j++ {
			if ranks[i] > ranks[j] {
				ranks[i], ranks[j] = ranks[j], ranks[i]
			}
		}
	}

	// Check connectivity
	connectedCards := 1
	for i := 1; i < len(ranks); i++ {
		if int(ranks[i])-int(ranks[i-1]) <= 2 {
			connectedCards++
		}
	}

	if connectedCards >= 3 {
		wetness += 2 // Straight draws possible
	}

	// Check for pairs
	rankCounts := make(map[int]int)
	for _, card := range communityCards {
		rankCounts[card.Rank]++
	}

	pairs := 0
	for _, count := range rankCounts {
		if count >= 2 {
			pairs++
		}
	}

	if pairs >= 1 {
		wetness += 1 // Paired board
	}

	// Classify based on wetness
	switch {
	case wetness >= 5:
		return VeryWetBoard
	case wetness >= 3:
		return WetBoard
	case wetness >= 1:
		return SemiWetBoard
	default:
		return DryBoard
	}
}

// getPositionFactorWithThinking returns position factor with thinking
func (b *Bot) getPositionFactorWithThinking(position game.Position, thinking *ThinkingContext) float64 {
	baseFactor := b.getPositionFactor(position)

	// Apply position awareness configuration
	adjustedFactor := 1.0 + (baseFactor-1.0)*b.config.PositionAwareness

	if b.config.PositionAwareness < 0.5 {
		thinking.AddThought("Low position awareness - similar play all positions")
	} else if b.config.PositionAwareness > 1.5 {
		thinking.AddThought("High position awareness - extreme positional adjustments")
	}

	switch position {
	case game.SmallBlind, game.BigBlind:
		thinking.AddThought("In the blinds, playing tighter")
	case game.UnderTheGun, game.EarlyPosition:
		thinking.AddThought("Early position, need strong hands")
	case game.MiddlePosition:
		thinking.AddThought("Middle position, standard play")
	case game.LatePosition, game.Cutoff:
		thinking.AddThought("Late position, can play looser")
	case game.Button:
		thinking.AddThought("On the button, maximum position advantage")
	default:
		thinking.AddThought("Standard position")
	}

	return adjustedFactor
}

// getPositionFactor returns a factor based on position (lower = tighter play)
func (b *Bot) getPositionFactor(position game.Position) float64 {
	switch position {
	case game.SmallBlind, game.BigBlind:
		return 0.8 // Play tighter in blinds
	case game.UnderTheGun, game.EarlyPosition:
		return 0.7 // Play very tight in early position
	case game.MiddlePosition:
		return 0.9 // Standard play
	case game.LatePosition, game.Cutoff:
		return 1.1 // Play looser in late position
	case game.Button:
		return 1.2 // Play loosest on the button
	default:
		return 1.0
	}
}

// calculatePotOddsWithThinking calculates pot odds with thinking
func (b *Bot) calculatePotOddsWithThinking(player game.PlayerState, tableState game.TableState, thinking *ThinkingContext) float64 {
	if tableState.CurrentBet <= player.BetThisRound {
		thinking.AddThought("No bet to call")
		return 0
	}

	callAmount := tableState.CurrentBet - player.BetThisRound
	if callAmount >= player.Chips {
		thinking.AddThought("Would be all-in to call")
		callAmount = player.Chips
	}

	if callAmount == 0 {
		thinking.AddThought("Free to check")
		return 100
	}

	potOdds := float64(tableState.Pot) / float64(callAmount)
	thinking.AddThought(fmt.Sprintf("Pot odds: %.1f:1 (risk $%d to win $%d)", potOdds, callAmount, tableState.Pot))

	return potOdds
}

// makeDecisionBasedOnFactorsWithThinking makes the final decision with thinking
func (b *Bot) makeDecisionBasedOnFactorsWithThinking(player game.PlayerState, tableState game.TableState, equityCtx EquityContext, positionFactor, potOdds float64, thinking *ThinkingContext, validActions []game.ValidAction) game.Action {
	// Check for continuation betting opportunity
	shouldCBet := b.shouldContinuationBet(player, tableState, equityCtx.Strength, positionFactor)
	if shouldCBet && tableState.CurrentBet == 0 {
		thinking.AddThought("Good spot to continuation bet")
	}

	// Base probabilities for each action based on hand strength
	var foldProb, callProb, raiseProb float64

	switch equityCtx.Strength {
	case VeryWeak:
		foldProb, callProb, raiseProb = 0.85, 0.15, 0.0
		thinking.AddThought("Very weak hand, likely folding")
	case Weak:
		foldProb, callProb, raiseProb = 0.60, 0.35, 0.05
		thinking.AddThought("Weak hand, cautious approach")
	case Medium:
		foldProb, callProb, raiseProb = 0.15, 0.70, 0.15
		thinking.AddThought("Playable hand, looking to see more cards")
	case Strong:
		foldProb, callProb, raiseProb = 0.05, 0.40, 0.55
		thinking.AddThought("Strong hand, want to build pot")
	case VeryStrong:
		foldProb, callProb, raiseProb = 0.0, 0.20, 0.80
		thinking.AddThought("Premium hand, aggressive value betting")
	}

	// Apply aggression factor - increases raise probability, decreases call probability
	if b.config.AggressionFactor != 1.0 {
		thinking.AddThought(fmt.Sprintf("Applying aggression factor %.1f", b.config.AggressionFactor))
		aggressionBoost := (b.config.AggressionFactor - 1.0) * 0.3
		raiseProb += aggressionBoost
		callProb -= aggressionBoost * 0.6
		foldProb -= aggressionBoost * 0.4
	}

	// Apply tightness factor - increases fold probability
	if b.config.TightnessFactor != 1.0 {
		thinking.AddThought(fmt.Sprintf("Applying tightness factor %.1f", b.config.TightnessFactor))
		tightnessBoost := (b.config.TightnessFactor - 1.0) * 0.25
		foldProb += tightnessBoost
		callProb -= tightnessBoost * 0.7
		raiseProb -= tightnessBoost * 0.3
	}

	// Adjust for position
	if positionFactor < 1.0 {
		thinking.AddThought("Tightening up due to position")
		foldProb = foldProb * (2.0 - positionFactor)
		callProb = callProb * positionFactor
		raiseProb = raiseProb * positionFactor
	} else {
		thinking.AddThought("Playing looser with position advantage")
		foldProb = foldProb / positionFactor
		callProb = callProb * positionFactor
		raiseProb = raiseProb * positionFactor
	}

	// Enhanced position-based adjustments for draws and aggression
	if tableState.CurrentRound > game.PreFlop && positionFactor > 1.0 {
		drawStrength := b.evaluateDraws(player.HoleCards, tableState.CommunityCards)
		if drawStrength >= 2 && equityCtx.Strength == VeryWeak {
			// Apply bluff frequency configuration
			bluffChance := b.config.BluffFrequency
			if b.rng.Float64() < bluffChance {
				thinking.AddThought(fmt.Sprintf("Strong draws make semi-bluffing viable (%.0f%% frequency)", bluffChance*100))
				raiseProb += 0.2 * bluffChance
				callProb += 0.1
				foldProb -= 0.3 * bluffChance
			} else {
				thinking.AddThought("Strong draws but low bluff frequency - more passive")
			}
		}
	}

	// Use raw equity for precise pot odds comparison
	if potOdds > 0 && tableState.CurrentBet > player.BetThisRound {
		// Calculate required equity for profitable call
		requiredEquity := 1.0 / (1.0 + potOdds)
		equityDiff := equityCtx.Equity - requiredEquity

		// Apply pot odds weight configuration
		potOddsInfluence := b.config.PotOddsWeight * 0.25

		if equityDiff > b.config.EquityThreshold { // Equity significantly exceeds pot odds
			thinking.AddThought(fmt.Sprintf("Equity (%.1f%%) exceeds pot odds (%.1f%%) - profitable call (weight %.1f)",
				equityCtx.Equity*100, requiredEquity*100, b.config.PotOddsWeight))
			callProb += potOddsInfluence
			foldProb -= potOddsInfluence * 0.8
			raiseProb += potOddsInfluence * 0.2
		} else if equityDiff > 0 { // Marginal equity advantage
			thinking.AddThought(fmt.Sprintf("Marginal equity edge (%.1f%% vs %.1f%% required)",
				equityCtx.Equity*100, requiredEquity*100))
			callProb += potOddsInfluence * 0.6
			foldProb -= potOddsInfluence * 0.6
		} else if equityDiff < -0.10 { // Significant equity disadvantage
			thinking.AddThought(fmt.Sprintf("Poor equity (%.1f%% vs %.1f%% required) - favoring fold",
				equityCtx.Equity*100, requiredEquity*100))
			foldProb += potOddsInfluence * 0.8
			callProb -= potOddsInfluence * 0.6
			raiseProb -= potOddsInfluence * 0.2
		}
	}

	// Continuation betting logic
	if shouldCBet && tableState.CurrentBet == 0 {
		cBetBoost := b.config.CBetFrequency * 0.4
		thinking.AddThought(fmt.Sprintf("C-bet opportunity (%.0f%% frequency)", b.config.CBetFrequency*100))
		raiseProb += cBetBoost
		callProb -= cBetBoost * 0.5
		foldProb -= cBetBoost * 0.5
	}

	// Legacy pot odds adjustment (now supplemented by precise equity calculation above)
	if potOdds > 2.0 && equityCtx.Strength >= Weak {
		thinking.AddThought("Good pot odds make calling more attractive")
		potOddsBonus := (potOdds - 2.0) * 0.1 // Reduced since we have precise equity calc
		callProb += potOddsBonus
		foldProb -= potOddsBonus * 0.8
		raiseProb += potOddsBonus * 0.2
	} else if potOdds > 0 && potOdds < 2.0 {
		thinking.AddThought("Poor pot odds, less inclined to call")
	}

	// Normalize probabilities
	total := foldProb + callProb + raiseProb
	if total > 0 {
		foldProb /= total
		callProb /= total
		raiseProb /= total
	}

	// Special cases
	if tableState.CurrentBet == 0 {
		if foldProb > 0.5 {
			thinking.AddThought("Checking instead of folding when possible")
			return game.Check
		}
		if raiseProb > callProb {
			thinking.AddThought("Taking the initiative with a bet")
			return game.Raise
		}
		thinking.AddThought("Checking to see next card")
		return game.Check
	}

	// If can't afford to call, must fold or go all-in
	callAmount := tableState.CurrentBet - player.BetThisRound
	if callAmount >= player.Chips {
		if equityCtx.Strength >= Strong && b.rng.Float64() < 0.3 {
			thinking.AddThought("Strong hand, going all-in")
			return game.AllIn
		}
		thinking.AddThought("Can't afford to call, must fold")
		return game.Fold
	}

	// Make random decision based on probabilities
	r := b.rng.Float64()
	if r < foldProb {
		thinking.AddThought("Deciding to fold")
		return game.Fold
	} else if r < foldProb+callProb {
		thinking.AddThought("Deciding to call")
		return game.Call
	} else {
		thinking.AddThought("Deciding to raise")
		return game.Raise
	}
}

// shouldContinuationBet determines if player should continuation bet
func (b *Bot) shouldContinuationBet(_ game.PlayerState, tableState game.TableState, strength HandStrength, positionFactor float64) bool {
	// Only apply on post-flop streets
	if tableState.CurrentRound == game.PreFlop {
		return false
	}

	// Only if there's no bet to call (we can bet)
	if tableState.CurrentBet > 0 {
		return false
	}

	// Simple heuristic: more likely to c-bet in position with any playable hand
	// This should be enhanced with proper pre-flop aggressor tracking
	if positionFactor > 1.0 && strength >= VeryWeak {
		// Analyze board texture
		boardTexture := b.analyzeBoardTexture(tableState.CommunityCards)

		// More likely to c-bet on dry boards, scaled by configuration
		baseCBetFreq := b.config.CBetFrequency
		switch boardTexture {
		case DryBoard:
			return b.rng.Float64() < baseCBetFreq*1.2 // High c-bet frequency on dry boards
		case SemiWetBoard:
			return b.rng.Float64() < baseCBetFreq // Base frequency
		case WetBoard:
			return b.rng.Float64() < baseCBetFreq*0.6 // Lower frequency on wet boards
		case VeryWetBoard:
			return b.rng.Float64() < baseCBetFreq*0.3 // Very low frequency
		}
	}

	return false
}

// calculateRaiseAmount determines how much to raise based on position and stack depth
func (b *Bot) calculateRaiseAmount(player game.PlayerState, tableState game.TableState, strength HandStrength) int {
	potSize := tableState.Pot
	currentBet := tableState.CurrentBet
	bigBlind := tableState.BigBlind

	// Calculate effective stack depth
	maxOpponentStack := 0
	for _, p := range tableState.Players {
		if p.Name != player.Name && p.Chips > maxOpponentStack {
			maxOpponentStack = p.Chips
		}
	}
	effectiveStack := player.Chips
	if maxOpponentStack < effectiveStack {
		effectiveStack = maxOpponentStack
	}
	stackDepth := float64(effectiveStack) / float64(bigBlind)

	var baseRaise int

	// Street-specific sizing (position and stack-based only)
	if tableState.CurrentRound == game.PreFlop {
		// Preflop sizing based on position
		positionFactor := b.getPositionFactor(player.Position)
		var pfSizing float64
		if positionFactor <= 0.8 { // Early position
			pfSizing = 2.5 + b.rng.Float64()*0.3 // 2.5-2.8x BB
		} else { // Late position
			pfSizing = 2.0 + b.rng.Float64()*0.4 // 2.0-2.4x BB
		}

		// Adjust for stack depth
		if stackDepth < 20 {
			pfSizing *= 0.8 // Smaller sizing with short stacks
		}

		baseRaise = int(float64(bigBlind) * pfSizing)
	} else {
		// Post-flop sizing based on street
		var potFactor float64
		switch tableState.CurrentRound {
		case game.Flop:
			potFactor = 0.6 + b.rng.Float64()*0.2 // 0.6-0.8x pot
		case game.Turn, game.River:
			potFactor = 0.5 + b.rng.Float64()*0.2 // 0.5-0.7x pot
		default:
			potFactor = 0.65
		}

		// Adjust for stack depth
		if stackDepth < 15 {
			potFactor *= 0.8 // Smaller sizing with short stacks
		}

		baseRaise = int(float64(potSize) * potFactor)
	}

	// Handle minimum raise requirements
	var minRaise int
	if currentBet > bigBlind {
		// Re-raise: current bet + the previous raise amount (proper min-raise)
		previousRaise := currentBet - bigBlind
		minRaise = currentBet + previousRaise

		// Cap re-raise at reasonable level to prevent escalation
		maxReRaise := currentBet + (potSize / 2)
		if minRaise > maxReRaise {
			minRaise = maxReRaise
		}
	} else {
		// Initial raise: big blind + minimum raise increment
		minRaise = currentBet + bigBlind
	}

	if baseRaise < minRaise {
		baseRaise = minRaise
	}

	// Cap maximum bet at 2.5x pot to prevent oversized bets
	maxBet := int(float64(potSize)*2.5) + currentBet
	if baseRaise > maxBet {
		baseRaise = maxBet
	}

	// Can't raise more than we have
	maxRaise := player.Chips + player.BetThisRound
	if baseRaise > maxRaise {
		baseRaise = maxRaise
	}

	return baseRaise
}

// validateAndAdjustDecision ensures the decision is valid and falls back if needed
func (b *Bot) validateAndAdjustDecision(decision game.Decision, validActions []game.ValidAction, thinking *ThinkingContext) game.Decision {
	// Check if the action is valid
	for _, validAction := range validActions {
		if validAction.Action == decision.Action {
			// For raises, check if amount is in valid range
			if decision.Action == game.Raise {
				if decision.Amount >= validAction.MinAmount && decision.Amount <= validAction.MaxAmount {
					return decision // Valid raise
				}
				// Adjust raise amount to be within valid range
				if decision.Amount < validAction.MinAmount {
					decision.Amount = validAction.MinAmount
					thinking.AddThought(fmt.Sprintf("Adjusted raise to minimum: %d", decision.Amount))
				} else if decision.Amount > validAction.MaxAmount {
					decision.Amount = validAction.MaxAmount
					thinking.AddThought(fmt.Sprintf("Adjusted raise to maximum (all-in): %d", decision.Amount))
				}
				decision.Reasoning = thinking.GetThoughts()
				return decision
			}
			return decision // Valid non-raise action
		}
	}
	
	// Action not valid, need fallback
	thinking.AddThought(fmt.Sprintf("Invalid action %s, falling back", decision.Action.String()))
	
	// Fallback priority: Call > Check > Fold
	for _, validAction := range validActions {
		if validAction.Action == game.Call {
			decision.Action = game.Call
			decision.Amount = validAction.MinAmount
			decision.Reasoning = thinking.GetThoughts()
			return decision
		}
	}
	
	for _, validAction := range validActions {
		if validAction.Action == game.Check {
			decision.Action = game.Check
			decision.Amount = 0
			decision.Reasoning = thinking.GetThoughts()
			return decision
		}
	}
	
	for _, validAction := range validActions {
		if validAction.Action == game.Fold {
			decision.Action = game.Fold
			decision.Amount = 0
			decision.Reasoning = thinking.GetThoughts()
			return decision
		}
	}
	
	// Should never reach here, but if no valid actions, fold
	decision.Action = game.Fold
	decision.Amount = 0
	thinking.AddThought("No valid actions found, defaulting to fold")
	decision.Reasoning = thinking.GetThoughts()
	return decision
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
