package game

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/lox/holdem-cli/internal/deck"
	"github.com/lox/holdem-cli/internal/evaluator"
)

// AIEngine handles AI decision making
type AIEngine struct {
	rng    *rand.Rand
	logger *log.Logger
}

// NewAIEngine creates a new AI engine
func NewAIEngine(logger *log.Logger) *AIEngine {
	return &AIEngine{
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
		logger: logger.WithPrefix("ai"),
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

// AIDecision represents an AI decision with reasoning
type AIDecision struct {
	Action    Action
	Reasoning string
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

// MakeDecision makes an AI decision for the given player
func (ai *AIEngine) MakeDecision(player *Player, table *Table) Action {
	decision := ai.MakeDecisionWithReasoning(player, table)
	return decision.Action
}

// MakeDecisionWithReasoning makes an AI decision and returns both action and reasoning
func (ai *AIEngine) MakeDecisionWithReasoning(player *Player, table *Table) AIDecision {
	// Create thinking context to accumulate thoughts
	thinking := &ThinkingContext{}

	// Evaluate hand strength with thinking
	strength := ai.evaluateHandStrengthWithThinking(player, table, thinking)

	// Get position factor with thinking
	positionFactor := ai.getPositionFactorWithThinking(player.Position, thinking)

	// Calculate pot odds with thinking
	potOdds := ai.calculatePotOddsWithThinking(player, table, thinking)

	// Log decision factors
	holeCardsStr := ""
	if len(player.HoleCards) >= 2 {
		holeCardsStr = player.HoleCards[0].String() + " " + player.HoleCards[1].String()
	}

	ai.logger.Info("AI decision analysis",
		"player", player.Name,
		"round", table.CurrentRound.String(),
		"holeCards", holeCardsStr,
		"handStrength", strength.String(),
		"position", player.Position.String(),
		"positionFactor", positionFactor,
		"currentBet", table.CurrentBet,
		"playerBetThisRound", player.BetThisRound,
		"playerChips", player.Chips,
		"pot", table.Pot,
		"potOdds", potOdds)

	// Make decision based on hand strength, position, and pot odds with thinking
	action := ai.makeDecisionBasedOnFactorsWithThinking(player, table, strength, positionFactor, potOdds, thinking)

	reasoning := thinking.GetThoughts()

	ai.logger.Info("AI decision made",
		"player", player.Name,
		"decision", action.String(),
		"reasoning", reasoning)

	return AIDecision{
		Action:    action,
		Reasoning: reasoning,
	}
}

// evaluateHandStrength evaluates the current hand strength
func (ai *AIEngine) evaluateHandStrength(player *Player, table *Table) HandStrength {
	if len(player.HoleCards) != 2 {
		return VeryWeak
	}

	// Pre-flop hand strength evaluation
	if table.CurrentRound == PreFlop {
		return ai.evaluatePreFlopStrength(player.HoleCards)
	}

	// Post-flop evaluation with community cards
	if len(table.CommunityCards) >= 3 {
		return ai.evaluatePostFlopStrength(player, table.CommunityCards)
	}

	return Medium
}

// evaluateHandStrengthWithThinking evaluates hand strength while building thoughts
func (ai *AIEngine) evaluateHandStrengthWithThinking(player *Player, table *Table, thinking *ThinkingContext) HandStrength {
	if len(player.HoleCards) != 2 {
		thinking.AddThought("Missing hole cards, assuming very weak")
		return VeryWeak
	}

	// Add initial hand description
	handPercentile := deck.GetHandPercentile(player.HoleCards)
	handKey := ai.getHandKey(player.HoleCards)
	thinking.AddThought(fmt.Sprintf("I have %s (top %.0f%% hand)", handKey, handPercentile*100))

	// Pre-flop hand strength evaluation
	if table.CurrentRound == PreFlop {
		strength := ai.evaluatePreFlopStrength(player.HoleCards)
		thinking.AddThought(fmt.Sprintf("Preflop strength: %s", strength.String()))
		return strength
	}

	// Post-flop evaluation with community cards
	if len(table.CommunityCards) >= 3 {
		// Add board description
		boardStr := ai.getBoardDescription(table.CommunityCards)
		thinking.AddThought(fmt.Sprintf("Board: %s", boardStr))

		strength := ai.evaluatePostFlopStrengthWithThinking(player, table.CommunityCards, thinking)
		return strength
	}

	thinking.AddThought("Not enough information, assuming medium strength")
	return Medium
}

// EvaluatePreFlopStrength evaluates pre-flop hand strength using percentile rankings (exported for testing)
func (ai *AIEngine) EvaluatePreFlopStrength(holeCards []deck.Card) HandStrength {
	return ai.evaluatePreFlopStrength(holeCards)
}

// evaluatePreFlopStrength evaluates pre-flop hand strength using percentile rankings
func (ai *AIEngine) evaluatePreFlopStrength(holeCards []deck.Card) HandStrength {
	percentile := deck.GetHandPercentile(holeCards)

	// Convert percentile rank to hand strength categories
	switch {
	case percentile >= 0.85: // Top 15% (premium hands)
		return VeryStrong
	case percentile >= 0.65: // Top 35% (strong hands)
		return Strong
	case percentile >= 0.40: // Top 60% (playable hands)
		return Medium
	case percentile >= 0.20: // Top 80% (marginal hands)
		return Weak
	default: // Bottom 20% (trash hands)
		return VeryWeak
	}
}

// evaluatePostFlopStrength evaluates post-flop hand strength using equity
func (ai *AIEngine) evaluatePostFlopStrength(player *Player, communityCards []deck.Card) HandStrength {
	return ai.evaluatePostFlopStrengthEquity(player, communityCards, false)
}

// evaluatePostFlopStrengthEquity evaluates hand strength using equity against position-aware ranges
func (ai *AIEngine) evaluatePostFlopStrengthEquity(player *Player, communityCards []deck.Card, inPosition bool) HandStrength {
	// Choose opponent range based on position
	var opponentRange evaluator.Range
	if inPosition {
		// Opponents act before us, likely tighter
		opponentRange = evaluator.TightRange{}
	} else {
		// We act first, opponents can be looser
		opponentRange = evaluator.LooseRange{}
	}

	// Calculate equity
	equity := evaluator.EstimateEquity(player.HoleCards, communityCards, opponentRange, 500)

	// Convert equity to hand strength
	return ai.equityToHandStrength(equity)
}

// equityToHandStrength maps equity percentage to hand strength categories
func (ai *AIEngine) equityToHandStrength(equity float64) HandStrength {
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

// evaluatePostFlopStrengthWithThinking evaluates post-flop hand strength with thinking
func (ai *AIEngine) evaluatePostFlopStrengthWithThinking(player *Player, communityCards []deck.Card, thinking *ThinkingContext) HandStrength {
	// Determine if we're in position (simplified: Button or Cutoff = in position)
	inPosition := player.Position == Button || player.Position == Cutoff

	// Choose opponent range based on position
	var opponentRange evaluator.Range
	if inPosition {
		opponentRange = evaluator.TightRange{}
		thinking.AddThought("In position - opponents likely tighter")
	} else {
		opponentRange = evaluator.LooseRange{}
		thinking.AddThought("Out of position - opponents can be looser")
	}

	// Calculate equity
	equity := evaluator.EstimateEquity(player.HoleCards, communityCards, opponentRange, 500)
	thinking.AddThought(fmt.Sprintf("Equity: %.1f%%", equity*100))

	// Convert equity to hand strength
	strength := ai.equityToHandStrength(equity)
	thinking.AddThought(fmt.Sprintf("Hand strength: %s", strength.String()))

	return strength
}

// getHandKey returns a readable description of hole cards
func (ai *AIEngine) getHandKey(holeCards []deck.Card) string {
	if len(holeCards) != 2 {
		return "unknown"
	}

	card1, card2 := holeCards[0], holeCards[1]
	rank1, rank2 := card1.Rank, card2.Rank

	// Ensure higher rank comes first
	if rank2 > rank1 {
		rank1, rank2 = rank2, rank1
	}

	rankStr1 := ai.rankToString(rank1)
	rankStr2 := ai.rankToString(rank2)

	// Handle pairs
	if rank1 == rank2 {
		return rankStr1 + rankStr2
	}

	// Determine if suited
	suitChar := "o"
	if card1.Suit == card2.Suit {
		suitChar = "s"
	}

	return rankStr1 + rankStr2 + suitChar
}

// getBoardDescription returns a readable description of the board
func (ai *AIEngine) getBoardDescription(communityCards []deck.Card) string {
	if len(communityCards) == 0 {
		return "no board"
	}

	cardStrs := make([]string, len(communityCards))
	for i, card := range communityCards {
		cardStrs[i] = card.String()
	}

	boardTexture := ai.analyzeBoardTexture(communityCards)
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

// rankToString converts rank to string (helper for thinking)
func (ai *AIEngine) rankToString(rank int) string {
	return deck.RankString(rank)
}

// evaluateDraws evaluates drawing potential and returns draw strength (0-4)
func (ai *AIEngine) evaluateDraws(holeCards []deck.Card, communityCards []deck.Card) int {
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
	straightDraws := ai.countStraightDraws(sortedRanks)
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
func (ai *AIEngine) countStraightDraws(sortedRanks []int) int {
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
func (ai *AIEngine) analyzeBoardTexture(communityCards []deck.Card) BoardTexture {
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

// getPositionFactor returns a factor based on position (lower = tighter play)
func (ai *AIEngine) getPositionFactor(position Position) float64 {
	switch position {
	case SmallBlind, BigBlind:
		return 0.8 // Play tighter in blinds
	case UnderTheGun, EarlyPosition:
		return 0.7 // Play very tight in early position
	case MiddlePosition:
		return 0.9 // Standard play
	case LatePosition, Cutoff:
		return 1.1 // Play looser in late position
	case Button:
		return 1.2 // Play loosest on the button
	default:
		return 1.0
	}
}

// getPositionFactorWithThinking returns position factor with thinking
func (ai *AIEngine) getPositionFactorWithThinking(position Position, thinking *ThinkingContext) float64 {
	factor := ai.getPositionFactor(position)

	switch position {
	case SmallBlind, BigBlind:
		thinking.AddThought("In the blinds, playing tighter")
	case UnderTheGun, EarlyPosition:
		thinking.AddThought("Early position, need strong hands")
	case MiddlePosition:
		thinking.AddThought("Middle position, standard play")
	case LatePosition, Cutoff:
		thinking.AddThought("Late position, can play looser")
	case Button:
		thinking.AddThought("On the button, maximum position advantage")
	default:
		thinking.AddThought("Standard position")
	}

	return factor
}

// calculatePotOdds calculates the pot odds for calling
func (ai *AIEngine) calculatePotOdds(player *Player, table *Table) float64 {
	if table.CurrentBet <= player.BetThisRound {
		return 0 // No bet to call
	}

	callAmount := table.CurrentBet - player.BetThisRound
	if callAmount >= player.Chips {
		callAmount = player.Chips // All-in scenario
	}

	if callAmount == 0 {
		return 100 // Free to call (check)
	}

	return float64(table.Pot) / float64(callAmount)
}

// calculatePotOddsWithThinking calculates pot odds with thinking
func (ai *AIEngine) calculatePotOddsWithThinking(player *Player, table *Table, thinking *ThinkingContext) float64 {
	if table.CurrentBet <= player.BetThisRound {
		thinking.AddThought("No bet to call")
		return 0
	}

	callAmount := table.CurrentBet - player.BetThisRound
	if callAmount >= player.Chips {
		thinking.AddThought("Would be all-in to call")
		callAmount = player.Chips
	}

	if callAmount == 0 {
		thinking.AddThought("Free to check")
		return 100
	}

	potOdds := float64(table.Pot) / float64(callAmount)
	thinking.AddThought(fmt.Sprintf("Pot odds: %.1f:1 (risk $%d to win $%d)", potOdds, callAmount, table.Pot))

	return potOdds
}

// makeDecisionBasedOnFactorsWithThinking makes the final decision with thinking
func (ai *AIEngine) makeDecisionBasedOnFactorsWithThinking(player *Player, table *Table, strength HandStrength, positionFactor, potOdds float64, thinking *ThinkingContext) Action {
	// Check for continuation betting opportunity
	shouldCBet := ai.shouldContinuationBet(player, table, strength, positionFactor)
	if shouldCBet && table.CurrentBet == 0 {
		thinking.AddThought("Good spot to continuation bet")
	}

	// Base probabilities for each action based on hand strength
	var foldProb, callProb, raiseProb float64

	switch strength {
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
	if table.CurrentRound > PreFlop && positionFactor > 1.0 {
		drawStrength := ai.evaluateDraws(player.HoleCards, table.CommunityCards)
		if drawStrength >= 2 && strength == VeryWeak {
			thinking.AddThought("Strong draws make semi-bluffing viable in position")
			raiseProb += 0.2
			callProb += 0.1
			foldProb -= 0.3
		}
	}

	// Continuation betting logic
	if shouldCBet && table.CurrentBet == 0 {
		thinking.AddThought("Increasing aggression for c-bet")
		raiseProb += 0.3
		callProb -= 0.15
		foldProb -= 0.15
	}

	// Adjust for pot odds
	if potOdds > 2.0 && strength >= Weak {
		thinking.AddThought("Good pot odds make calling more attractive")
		potOddsBonus := (potOdds - 2.0) * 0.15
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
	if table.CurrentBet == 0 {
		if foldProb > 0.5 {
			thinking.AddThought("Checking instead of folding when possible")
			return Check
		}
		if raiseProb > callProb {
			thinking.AddThought("Taking the initiative with a bet")
			return Raise
		}
		thinking.AddThought("Checking to see next card")
		return Check
	}

	// If can't afford to call, must fold or go all-in
	callAmount := table.CurrentBet - player.BetThisRound
	if callAmount >= player.Chips {
		if strength >= Strong && ai.rng.Float64() < 0.3 {
			thinking.AddThought("Strong hand, going all-in")
			return AllIn
		}
		thinking.AddThought("Can't afford to call, must fold")
		return Fold
	}

	// Make random decision based on probabilities
	r := ai.rng.Float64()
	if r < foldProb {
		thinking.AddThought("Deciding to fold")
		return Fold
	} else if r < foldProb+callProb {
		thinking.AddThought("Deciding to call")
		return Call
	} else {
		thinking.AddThought("Deciding to raise")
		return Raise
	}
}

// shouldContinuationBet determines if player should continuation bet
func (ai *AIEngine) shouldContinuationBet(_ *Player, table *Table, strength HandStrength, positionFactor float64) bool {
	// Only apply on post-flop streets
	if table.CurrentRound == PreFlop {
		return false
	}

	// Only if there's no bet to call (we can bet)
	if table.CurrentBet > 0 {
		return false
	}

	// Simple heuristic: more likely to c-bet in position with any playable hand
	// This should be enhanced with proper pre-flop aggressor tracking
	if positionFactor > 1.0 && strength >= VeryWeak {
		// Analyze board texture
		boardTexture := ai.analyzeBoardTexture(table.CommunityCards)

		// More likely to c-bet on dry boards
		switch boardTexture {
		case DryBoard:
			return ai.rng.Float64() < 0.7 // High c-bet frequency on dry boards
		case SemiWetBoard:
			return ai.rng.Float64() < 0.5 // Medium frequency
		case WetBoard:
			return ai.rng.Float64() < 0.3 // Lower frequency on wet boards
		case VeryWetBoard:
			return ai.rng.Float64() < 0.2 // Very low frequency
		}
	}

	return false
}

// GetRaiseAmount determines how much to raise based on position and stack depth
func (ai *AIEngine) GetRaiseAmount(player *Player, table *Table, strength HandStrength) int {
	potSize := table.Pot
	currentBet := table.CurrentBet
	bigBlind := table.BigBlind

	// Calculate effective stack depth
	maxOpponentStack := 0
	for _, p := range table.Players {
		if p != player && p.Chips > maxOpponentStack {
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
	if table.CurrentRound == PreFlop {
		// Preflop sizing based on position
		positionFactor := ai.getPositionFactor(player.Position)
		var pfSizing float64
		if positionFactor <= 0.8 { // Early position
			pfSizing = 2.5 + ai.rng.Float64()*0.3 // 2.5-2.8x BB
		} else { // Late position
			pfSizing = 2.0 + ai.rng.Float64()*0.4 // 2.0-2.4x BB
		}

		// Adjust for stack depth
		if stackDepth < 20 {
			pfSizing *= 0.8 // Smaller sizing with short stacks
		}

		baseRaise = int(float64(bigBlind) * pfSizing)
	} else {
		// Post-flop sizing based on street
		var potFactor float64
		switch table.CurrentRound {
		case Flop:
			potFactor = 0.6 + ai.rng.Float64()*0.2 // 0.6-0.8x pot
		case Turn, River:
			potFactor = 0.5 + ai.rng.Float64()*0.2 // 0.5-0.7x pot
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

// ExecuteAIAction executes an AI player's action
func (ai *AIEngine) ExecuteAIAction(player *Player, table *Table) {
	if player.Type != AI || !player.CanAct() {
		return
	}

	decision := ai.MakeDecisionWithReasoning(player, table)
	action := decision.Action

	switch action {
	case Fold:
		player.Fold()
	case Call:
		callAmount := table.CurrentBet - player.BetThisRound
		if callAmount > 0 && callAmount <= player.Chips {
			player.Call(callAmount)
			table.Pot += callAmount
		} else {
			player.Check() // Fall back to check if can't call
		}
	case Check:
		player.Check()
	case Raise:
		strength := ai.evaluateHandStrength(player, table)
		raiseAmount := ai.GetRaiseAmount(player, table, strength)
		totalNeeded := raiseAmount - player.BetThisRound

		if totalNeeded > 0 && totalNeeded <= player.Chips {
			player.Raise(totalNeeded)
			table.Pot += totalNeeded
			table.CurrentBet = raiseAmount
		} else {
			// Fall back to call or check
			callAmount := table.CurrentBet - player.BetThisRound
			if callAmount > 0 && callAmount <= player.Chips {
				player.Call(callAmount)
				table.Pot += callAmount
			} else {
				player.Check()
			}
		}
	case AllIn:
		allInAmount := player.Chips
		if player.AllIn() {
			table.Pot += allInAmount
			if player.TotalBet > table.CurrentBet {
				table.CurrentBet = player.TotalBet
			}
		}
	}
}

// ExecuteAIActionWithReasoning executes an AI player's action and returns the reasoning
func (ai *AIEngine) ExecuteAIActionWithReasoning(player *Player, table *Table) string {
	if player.Type != AI || !player.CanAct() {
		return ""
	}

	decision := ai.MakeDecisionWithReasoning(player, table)
	action := decision.Action

	switch action {
	case Fold:
		player.Fold()
	case Call:
		callAmount := table.CurrentBet - player.BetThisRound
		if callAmount > 0 && callAmount <= player.Chips {
			player.Call(callAmount)
			table.Pot += callAmount
		} else {
			player.Check() // Fall back to check if can't call
		}
	case Check:
		player.Check()
	case Raise:
		strength := ai.evaluateHandStrength(player, table)
		raiseAmount := ai.GetRaiseAmount(player, table, strength)
		totalNeeded := raiseAmount - player.BetThisRound

		if totalNeeded > 0 && totalNeeded <= player.Chips {
			player.Raise(totalNeeded)
			table.Pot += totalNeeded
			table.CurrentBet = raiseAmount
		} else {
			// Fall back to call or check
			callAmount := table.CurrentBet - player.BetThisRound
			if callAmount > 0 && callAmount <= player.Chips {
				player.Call(callAmount)
				table.Pot += callAmount
			} else {
				player.Check()
			}
		}
	case AllIn:
		allInAmount := player.Chips
		if player.AllIn() {
			table.Pot += allInAmount
			if player.TotalBet > table.CurrentBet {
				table.CurrentBet = player.TotalBet
			}
		}
	}

	// Record the action in hand history
	if table.HandHistory != nil {
		table.HandHistory.AddAction(player.Name, action, player.ActionAmount, table.Pot, table.CurrentRound, decision.Reasoning)
	}

	return decision.Reasoning
}
