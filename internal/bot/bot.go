package bot

import (
	"fmt"
	"math"
	"math/rand"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/lox/pokerforbots/internal/deck"
	"github.com/lox/pokerforbots/internal/evaluator"
	"github.com/lox/pokerforbots/internal/game"
)

// Bot is a sophisticated poker AI that combines advanced decision logic with opponent modeling
type Bot struct {
	rng    *rand.Rand
	logger *log.Logger
	config BotConfig

	// Opponent modeling from new bot
	opponentStats map[string]*OpponentProfile
	handHistory   []HandHistory

	// Learning parameters
	exploitationLevel float64 // 0.0 = pure theory, 1.0 = pure exploitation
}

// OpponentProfile tracks key stats for opponent modeling
type OpponentProfile struct {
	HandsPlayed     int
	HandsVoluntary  int     // Hands where they put money in voluntarily
	HandsRaised     int     // Hands where they raised pre-flop
	VPIP            float64 // Voluntarily Put money In Pot
	PFR             float64 // Pre-Flop Raise frequency
	Aggression      float64 // Bet/Raise frequency post-flop
	FoldToBet       float64 // How often they fold to bets
	BetsFaced       int     // Total bets they faced
	FoldsToBet      int     // Times they folded to bets
	BetsAndRaises   int     // Aggressive actions post-flop
	PostflopActions int     // Total post-flop actions
	ShowdownWins    int     // Hands won at showdown
	ShowdownHands   int     // Total hands that went to showdown
}

// HandHistory tracks decisions for learning
type HandHistory struct {
	Street game.BettingRound
	Action game.Action
	Amount int
	Result float64 // BB won/lost
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

// DefaultBotConfig returns a balanced configuration
func DefaultBotConfig() BotConfig {
	return BotConfig{
		Name:              "Default",
		AggressionFactor:  1.15, // Moderately aggressive
		TightnessFactor:   0.95, // Slightly tighter
		BluffFrequency:    0.3,  // Controlled bluffing
		CBetFrequency:     0.65, // Selective continuation betting
		EquityThreshold:   0.04, // Higher threshold for aggression
		OpponentModel:     "random",
		PositionAwareness: 1.3, // Strong position awareness
		PotOddsWeight:     1.2, // Moderate pot odds weight
	}
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

// EquityContext contains both hand strength category and raw equity value
type EquityContext struct {
	Strength HandStrength
	Equity   float64
}

// BoardTexture represents how coordinated the board is
type BoardTexture int

const (
	DryBoard BoardTexture = iota
	SemiWetBoard
	WetBoard
	VeryWetBoard
)

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

// NewBot creates a new bot with default configuration
func NewBot(rng *rand.Rand, logger *log.Logger) *Bot {
	return NewBotWithConfig(rng, logger, DefaultBotConfig())
}

// NewBotWithConfig creates a new bot with the specified configuration
func NewBotWithConfig(rng *rand.Rand, logger *log.Logger, config BotConfig) *Bot {
	return &Bot{
		rng:               rng,
		logger:            logger.WithPrefix("bot").With("botType", config.Name),
		config:            config,
		opponentStats:     make(map[string]*OpponentProfile),
		handHistory:       make([]HandHistory, 0),
		exploitationLevel: 0.5, // Higher exploitation against weak opponents
	}
}

// NewBotWithRNG creates a new bot with controlled RNG for backward compatibility
func NewBotWithRNG(logger *log.Logger, config BotConfig, rng *rand.Rand) *Bot {
	return NewBotWithConfig(rng, logger, config)
}

// MakeDecision analyzes the game state and returns a decision with reasoning
func (b *Bot) MakeDecision(tableState game.TableState, validActions []game.ValidAction) game.Decision {
	// Update opponent modeling
	b.updateOpponentStats(tableState)

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

// findRelevantOpponent identifies the most relevant opponent for equity calculation
func (b *Bot) findRelevantOpponent(player game.PlayerState, tableState game.TableState) string {
	// Priority 1: Player who just bet/raised in current round
	if tableState.HandHistory != nil {
		for i := len(tableState.HandHistory.Actions) - 1; i >= 0; i-- {
			action := tableState.HandHistory.Actions[i]
			if action.Round != tableState.CurrentRound {
				break
			}
			if action.PlayerName != player.Name && action.Action == game.Raise {
				return action.PlayerName
			}
		}

		// Priority 2: Preflop raiser if no current round betting
		if tableState.CurrentRound != game.PreFlop {
			for _, action := range tableState.HandHistory.Actions {
				if action.Round != game.PreFlop {
					continue
				}
				if action.PlayerName != player.Name && action.Action == game.Raise {
					return action.PlayerName
				}
			}
		}
	}

	// Priority 3: Next active player
	actingIdx := tableState.ActingPlayerIdx
	for i := 1; i < len(tableState.Players); i++ {
		nextIdx := (actingIdx + i) % len(tableState.Players)
		nextPlayer := tableState.Players[nextIdx]
		if nextPlayer.IsActive && !nextPlayer.IsFolded && nextPlayer.Name != player.Name {
			return nextPlayer.Name
		}
	}

	return ""
}

// updateOpponentStats initializes and updates opponent profiles
func (b *Bot) updateOpponentStats(tableState game.TableState) {
	// Initialize profiles for new players with neutral assumptions
	for _, player := range tableState.Players {
		if _, exists := b.opponentStats[player.Name]; !exists {
			b.opponentStats[player.Name] = &OpponentProfile{
				HandsPlayed:     0,
				HandsVoluntary:  0,
				HandsRaised:     0,
				VPIP:            0.22, // Start assuming TAG-like behavior
				PFR:             0.17,
				Aggression:      0.45,
				FoldToBet:       0.6,
				BetsFaced:       0,
				FoldsToBet:      0,
				BetsAndRaises:   0,
				PostflopActions: 0,
			}
		}
	}

	// Update stats based on recent actions if hand history is available
	if tableState.HandHistory != nil {
		b.updateFromHandHistory(tableState.HandHistory)
	}
}

// updateFromHandHistory processes hand history to update opponent stats
func (b *Bot) updateFromHandHistory(handHistory *game.HandHistory) {
	// Process actions from hand history to update opponent profiles
	// This is a simplified version - could be much more sophisticated
	for _, action := range handHistory.Actions {
		if _, exists := b.opponentStats[action.PlayerName]; exists {
			b.trackOpponentAction(action.PlayerName, action.Action, handHistory)
		}
	}
}

// trackOpponentAction updates opponent profile based on an action
func (b *Bot) trackOpponentAction(playerName string, action game.Action, handHistory *game.HandHistory) {
	profile, exists := b.opponentStats[playerName]
	if !exists {
		return
	}

	// Determine current round from context
	currentRound := game.PreFlop // Default
	if handHistory != nil && len(handHistory.Actions) > 0 {
		// Use the round from the most recent action context
		currentRound = handHistory.Actions[len(handHistory.Actions)-1].Round
	}

	// Update hand count for pre-flop actions
	if currentRound == game.PreFlop {
		profile.HandsPlayed++

		// Track VPIP (voluntary put money in pot)
		if action == game.Call || action == game.Raise {
			profile.HandsVoluntary++
			profile.VPIP = float64(profile.HandsVoluntary) / float64(profile.HandsPlayed)
		}

		// Track PFR (pre-flop raise)
		if action == game.Raise {
			profile.HandsRaised++
			profile.PFR = float64(profile.HandsRaised) / float64(profile.HandsPlayed)
		}
	}

	// Track post-flop aggression
	if currentRound != game.PreFlop {
		profile.PostflopActions++

		if action == game.Raise {
			profile.BetsAndRaises++
		}

		if profile.PostflopActions > 0 {
			profile.Aggression = float64(profile.BetsAndRaises) / float64(profile.PostflopActions)
		}
	}

	// Track fold to bet tendencies only when player actually faced a bet
	// This requires checking if there was betting action before this fold
	if action == game.Fold && handHistory != nil {
		// Check if there was aggressive action (bet/raise) before this fold in the current round
		facedBet := false
		for i := len(handHistory.Actions) - 1; i >= 0; i-- {
			prevAction := handHistory.Actions[i]
			if prevAction.Round != currentRound {
				break // Different round
			}
			if prevAction.PlayerName == playerName {
				break // This player's previous action in same round
			}
			if prevAction.Action == game.Raise {
				facedBet = true
				break
			}
		}

		if facedBet {
			profile.BetsFaced++
			profile.FoldsToBet++
			if profile.BetsFaced > 0 {
				// Apply recency bias - recent folds matter more
				// Use exponential moving average for faster adaptation
				if profile.BetsFaced == 1 {
					profile.FoldToBet = 1.0 // First fold to bet = 100% fold rate
				} else {
					// Weighted average: 70% new data, 30% old data for fast adaptation
					newFoldRate := float64(profile.FoldsToBet) / float64(profile.BetsFaced)
					profile.FoldToBet = 0.7*newFoldRate + 0.3*profile.FoldToBet
				}
			}
		}
	}
}

// getBettingContext analyzes the current betting action to determine opponent ranges
func (b *Bot) getBettingContext(tableState game.TableState) string {
	if tableState.CurrentRound != game.PreFlop {
		return "postflop"
	}

	// Count raises in current hand
	raises := 0
	for _, player := range tableState.Players {
		if player.BetThisRound > tableState.BigBlind {
			raises++
		}
	}

	switch {
	case raises == 0:
		return "unopened"
	case raises == 1:
		return "single-raised"
	case raises >= 2:
		return "multi-raised"
	default:
		return "unopened"
	}
}

// getDynamicExploitationLevel adjusts exploitation based on number of opponents
func (b *Bot) getDynamicExploitationLevel(tableState game.TableState) float64 {
	activeOpponents := 0
	for _, player := range tableState.Players {
		if !player.IsFolded && player.IsActive &&
			player.Name != tableState.Players[tableState.ActingPlayerIdx].Name {
			activeOpponents++
		}
	}

	// Reduce exploitation in multi-way pots
	switch {
	case activeOpponents <= 1:
		return b.exploitationLevel // Full exploitation heads-up
	case activeOpponents == 2:
		return b.exploitationLevel * 0.7 // Reduce in 3-way
	case activeOpponents >= 3:
		return b.exploitationLevel * 0.4 // Much more conservative in 4+ way
	default:
		return b.exploitationLevel * 0.5
	}
}

// evaluateHandStrengthWithThinking evaluates hand strength while building thoughts
func (b *Bot) evaluateHandStrengthWithThinking(player game.PlayerState, tableState game.TableState, thinking *ThinkingContext) EquityContext {
	if len(player.HoleCards) != 2 {
		thinking.AddThought("Missing hole cards, assuming very weak")
		return EquityContext{Strength: VeryWeak, Equity: 0.1}
	}

	// Find the most relevant opponent for range construction
	opponentName := b.findRelevantOpponent(player, tableState)

	var opponentRange evaluator.Range
	var rangeDescription string

	if opponentName != "" {
		// Use improved range builder
		rangeBuilder := NewSimpleRangeBuilder()
		opponentRange, rangeDescription = rangeBuilder.BuildOpponentRange(opponentName, tableState)
		thinking.AddThought(fmt.Sprintf("Range for %s: %s", opponentName, rangeDescription))
	} else {
		// Fallback to old logic for backward compatibility
		if tableState.CurrentRound == game.PreFlop {
			context := b.getBettingContext(tableState)
			switch context {
			case "unopened":
				opponentRange = evaluator.LooseRange{} // Limpers have wide ranges
				thinking.AddThought("Unopened pot - using loose opponent range")
			case "single-raised":
				opponentRange = evaluator.TightRange{} // Raisers have tighter ranges
				thinking.AddThought("Single raised pot - using tight opponent range")
			case "multi-raised":
				opponentRange = evaluator.TightRange{}
				thinking.AddThought("Multi-raised pot - using very tight opponent range")
			default:
				opponentRange = evaluator.MediumRange{}
			}
		} else {
			// Post-flop: use position-aware range instead of random
			opponentRange = evaluator.MediumRange{}
			thinking.AddThought("Using medium opponent model postflop")
		}
	}

	// Calculate equity with the determined range
	equity := evaluator.EstimateEquity(player.HoleCards, tableState.CommunityCards, opponentRange, 1000, b.rng)

	if tableState.CurrentRound == game.PreFlop {
		thinking.AddThought(fmt.Sprintf("Pre-flop equity: %.1f%%", equity*100))
	} else {
		// Add board description for postflop
		boardStr := b.getBoardDescription(tableState.CommunityCards)
		thinking.AddThought(fmt.Sprintf("Board: %s", boardStr))
		thinking.AddThought(fmt.Sprintf("Equity: %.1f%%", equity*100))
	}

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
		return 0.75 // Play tighter in blinds due to position disadvantage
	case game.UnderTheGun, game.EarlyPosition:
		return 0.65 // Play very tight in early position
	case game.MiddlePosition:
		return 0.85 // Slightly tighter than standard
	case game.LatePosition, game.Cutoff:
		return 1.25 // Play significantly looser in late position
	case game.Button:
		return 1.4 // Maximum aggression on button
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
	// Use situation recognition to evaluate this spot
	situationCtx := BuildSituationContext(player, tableState, equityCtx.Strength, equityCtx.Equity, potOdds)
	recognizer := NewSituationRecognizer()
	adjustment, reasoning := recognizer.EvaluateSituation(situationCtx)

	thinking.AddThought(fmt.Sprintf("Situation analysis: %s", reasoning))
	// Check for continuation betting opportunity
	shouldCBet := b.shouldContinuationBet(player, tableState, equityCtx.Strength, positionFactor)
	if shouldCBet && tableState.CurrentBet == 0 {
		thinking.AddThought("Good spot to continuation bet")
	}

	// Adapt to opponents using opponent modeling
	adaptedEquity := b.adaptEquityToOpponents(equityCtx.Equity, tableState, thinking)

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

	// Apply adapted equity for opponent modeling
	if adaptedEquity != equityCtx.Equity {
		thinking.AddThought(fmt.Sprintf("Opponent modeling adjusted equity from %.1f%% to %.1f%%", equityCtx.Equity*100, adaptedEquity*100))

		// Adjust probabilities based on adapted equity
		equityDiff := adaptedEquity - equityCtx.Equity
		if equityDiff > 0 {
			// Opponents are weak, be more aggressive
			raiseProb += equityDiff * 0.5
			foldProb -= equityDiff * 0.4
			callProb -= equityDiff * 0.1
		} else {
			// Opponents are strong, be more conservative
			foldProb += (-equityDiff) * 0.4
			raiseProb -= (-equityDiff) * 0.3
			callProb -= (-equityDiff) * 0.1
		}
	}

	// Apply situation-based adjustments instead of hardcoded factors
	thinking.AddThought(fmt.Sprintf("Applying situation adjustments: fold×%.2f, call×%.2f, raise×%.2f",
		adjustment.FoldMultiplier, adjustment.CallMultiplier, adjustment.RaiseMultiplier))

	foldProb *= adjustment.FoldMultiplier
	callProb *= adjustment.CallMultiplier
	raiseProb *= adjustment.RaiseMultiplier

	// Still apply config factors for overall bot personality
	if b.config.AggressionFactor != 1.0 {
		thinking.AddThought(fmt.Sprintf("Applying aggression factor %.1f", b.config.AggressionFactor))
		aggressionBoost := (b.config.AggressionFactor - 1.0) * 0.15 // Reduced impact
		raiseProb += aggressionBoost
		callProb -= aggressionBoost * 0.6
		foldProb -= aggressionBoost * 0.4
	}

	if b.config.TightnessFactor != 1.0 {
		thinking.AddThought(fmt.Sprintf("Applying tightness factor %.1f", b.config.TightnessFactor))
		tightnessBoost := (b.config.TightnessFactor - 1.0) * 0.15 // Reduced impact
		foldProb += tightnessBoost
		callProb -= tightnessBoost * 0.7
		raiseProb -= tightnessBoost * 0.3
	}

	// Use raw equity for precise pot odds comparison
	if potOdds > 0 && tableState.CurrentBet > player.BetThisRound {
		// Calculate required equity for profitable call
		requiredEquity := 1.0 / (1.0 + potOdds)
		equityDiff := adaptedEquity - requiredEquity

		// Apply pot odds weight configuration
		potOddsInfluence := b.config.PotOddsWeight * 0.25

		if equityDiff > b.config.EquityThreshold {
			thinking.AddThought(fmt.Sprintf("Equity (%.1f%%) exceeds pot odds (%.1f%%) - profitable call",
				adaptedEquity*100, requiredEquity*100))
			callProb += potOddsInfluence
			foldProb -= potOddsInfluence * 0.8
			raiseProb += potOddsInfluence * 0.2
		} else if equityDiff < -0.10 {
			thinking.AddThought(fmt.Sprintf("Poor equity (%.1f%% vs %.1f%% required) - favoring fold",
				adaptedEquity*100, requiredEquity*100))
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

	// Normalize probabilities
	total := foldProb + callProb + raiseProb
	if total > 0 {
		foldProb /= total
		callProb /= total
		raiseProb /= total
	}

	// Situation recognition already handles position and draw penalties

	// Special cases
	if tableState.CurrentBet == 0 {
		// Check if we're against very passive opponents and increase aggression
		totalFoldToBet := 0.0
		activeOpponents := 0
		for _, p := range tableState.Players {
			if !p.IsFolded && p.IsActive && p.Name != tableState.Players[tableState.ActingPlayerIdx].Name {
				if stats, exists := b.opponentStats[p.Name]; exists && stats.BetsFaced > 0 {
					totalFoldToBet += stats.FoldToBet
					activeOpponents++
				} else {
					// Default assumption for unknown opponents: moderately passive
					totalFoldToBet += 0.65
					activeOpponents++
				}
			}
		}

		// Against very passive opponents, be more aggressive but with caps
		if activeOpponents > 0 {
			avgFoldToBet := totalFoldToBet / float64(activeOpponents)
			// Cap bluff frequency even against tight opponents
			maxBluffFreq := 0.4 // Never bluff more than 40% of the time

			if avgFoldToBet > 0.8 && equityCtx.Strength >= Weak && b.rng.Float64() < maxBluffFreq {
				// Against extreme folders, but only with some equity
				thinking.AddThought("Tight opponents - selective aggression with weak+ hands")
				return game.Raise
			} else if avgFoldToBet > 0.7 && equityCtx.Strength >= Medium && raiseProb > 0.05 {
				thinking.AddThought("Moderately tight opponents - value betting focus")
				return game.Raise
			} else if avgFoldToBet > 0.6 && raiseProb > 0.1 {
				thinking.AddThought("Moderately passive opponents - selective aggression")
				return game.Raise
			}
		}

		// Increase bluffing frequency in late position but be selective
		if positionFactor > 1.2 && equityCtx.Strength <= Weak && raiseProb > 0.15 && b.rng.Float64() < 0.4 {
			thinking.AddThought("Late position selective bluff opportunity")
			return game.Raise
		}

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

// adaptEquityToOpponents adjusts equity based on opponent modeling
func (b *Bot) adaptEquityToOpponents(baseEquity float64, tableState game.TableState, thinking *ThinkingContext) float64 {
	// Calculate average opponent stats
	totalFoldToBet := 0.0
	totalVPIP := 0.0
	totalAggression := 0.0
	activeOpponents := 0

	for _, player := range tableState.Players {
		if !player.IsFolded && player.IsActive && player.Name != tableState.Players[tableState.ActingPlayerIdx].Name {
			if stats, exists := b.opponentStats[player.Name]; exists {
				totalFoldToBet += stats.FoldToBet
				totalVPIP += stats.VPIP
				totalAggression += stats.Aggression
				activeOpponents++
			} else {
				// Default stats for unknown players (assume random-like)
				totalFoldToBet += 0.5
				totalVPIP += 0.5
				totalAggression += 0.5
				activeOpponents++
			}
		}
	}

	if activeOpponents == 0 {
		return baseEquity
	}

	avgFoldToBet := totalFoldToBet / float64(activeOpponents)
	avgVPIP := totalVPIP / float64(activeOpponents)
	avgAggression := totalAggression / float64(activeOpponents)

	// Classify opponent pool
	playerType := b.classifyOpponentPool(avgVPIP, avgFoldToBet, avgAggression)

	// Adjust equity based on opponent type
	switch playerType {
	case "tight-passive":
		if avgFoldToBet > 0.9 {
			// Against extreme fold bots - any hand becomes profitable to bet
			thinking.AddThought("Extreme fold bots detected - maximum bluff equity")
			return math.Max(baseEquity, 0.9)
		} else if avgFoldToBet > 0.75 {
			// Against very tight players - significant bluff boost
			thinking.AddThought("Very tight players - large bluff equity boost")
			return math.Max(baseEquity, 0.75)
		}
		// Against moderately tight players: increase bluff equity with faster learning
		bluffBonus := (avgFoldToBet - 0.5) * 0.6 // Increased from 0.5 to 0.6
		dynamicExploit := b.getDynamicExploitationLevel(tableState)
		return baseEquity + bluffBonus*dynamicExploit

	case "tight-aggressive":
		// Against TAG: play fundamentally sound, no exploitation
		thinking.AddThought("TAG opponents - playing fundamentally sound")
		return baseEquity * 0.95 // Slightly more conservative

	case "loose-passive":
		// Against calling stations: need stronger hands for value but can extract more
		thinking.AddThought("Calling stations - value betting aggressively")
		if baseEquity > 0.55 {
			return baseEquity * 1.15 // Increase value betting equity
		}
		return baseEquity * 0.85 // Reduce bluff equity

	case "loose-aggressive":
		// Against maniacs: tighten up significantly
		thinking.AddThought("Maniacs detected - tightening ranges")
		return baseEquity * 0.8

	default:
		// Unknown/balanced opponents: assume some exploitability
		thinking.AddThought("Unknown opponents - assuming moderate passivity")
		if baseEquity > 0.6 {
			return baseEquity * 1.15 // Moderate value betting against unknowns
		} else if baseEquity < 0.35 {
			return baseEquity + 0.1 // Conservative bluff equity boost
		}
		return baseEquity * 1.05
	}
}

// classifyOpponentPool categorizes opponents for exploitation
func (b *Bot) classifyOpponentPool(vpip, foldToBet, aggression float64) string {
	if vpip < 0.15 && foldToBet > 0.8 {
		return "tight-passive" // Nits/fold bots
	} else if vpip >= 0.15 && vpip <= 0.3 && foldToBet >= 0.5 && aggression > 0.4 {
		return "tight-aggressive" // TAGs
	} else if vpip > 0.4 && foldToBet < 0.5 {
		return "loose-passive" // Fish
	} else if vpip > 0.35 && aggression > 0.6 {
		return "loose-aggressive" // Maniacs
	} else {
		return "balanced"
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
	if positionFactor > 1.0 && strength >= VeryWeak {
		// Analyze board texture
		boardTexture := b.analyzeBoardTexture(tableState.CommunityCards)

		// More likely to c-bet on dry boards
		baseCBetFreq := b.config.CBetFrequency
		switch boardTexture {
		case DryBoard:
			return b.rng.Float64() < baseCBetFreq*1.3
		case SemiWetBoard:
			return b.rng.Float64() < baseCBetFreq*1.1
		case WetBoard:
			return b.rng.Float64() < baseCBetFreq*0.7
		case VeryWetBoard:
			return b.rng.Float64() < baseCBetFreq*0.4
		}
	}

	return false
}

// calculateRaiseAmount determines how much to raise
func (b *Bot) calculateRaiseAmount(player game.PlayerState, tableState game.TableState, strength HandStrength) int {
	potSize := tableState.Pot
	currentBet := tableState.CurrentBet
	bigBlind := tableState.BigBlind

	var baseRaise int

	// Street-specific sizing
	if tableState.CurrentRound == game.PreFlop {
		// Preflop sizing based on position
		positionFactor := b.getPositionFactor(player.Position)
		var pfSizing float64
		if positionFactor <= 0.8 {
			pfSizing = 2.5 + b.rng.Float64()*0.3 // 2.5-2.8x BB
		} else {
			pfSizing = 2.0 + b.rng.Float64()*0.4 // 2.0-2.4x BB
		}
		baseRaise = int(float64(bigBlind) * pfSizing)
	} else {
		// Post-flop sizing
		var potFactor float64
		switch tableState.CurrentRound {
		case game.Flop:
			potFactor = 0.6 + b.rng.Float64()*0.2 // 0.6-0.8x pot
		case game.Turn, game.River:
			potFactor = 0.5 + b.rng.Float64()*0.2 // 0.5-0.7x pot
		default:
			potFactor = 0.65
		}
		baseRaise = int(float64(potSize) * potFactor)
	}

	// Handle minimum raise requirements
	var minRaise int
	if currentBet > bigBlind {
		// Re-raise: current bet + the previous raise amount
		previousRaise := currentBet - bigBlind
		minRaise = currentBet + previousRaise
	} else {
		// Initial raise: big blind + minimum raise increment
		minRaise = currentBet + bigBlind
	}

	if baseRaise < minRaise {
		baseRaise = minRaise
	}

	// Cap maximum bet at 2.5x pot
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
