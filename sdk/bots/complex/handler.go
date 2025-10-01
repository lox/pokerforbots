package complex

import (
	"fmt"
	"io"
	"math"
	rand "math/rand/v2"
	"os"
	"slices"
	"time"

	"github.com/lox/pokerforbots/v2/poker"
	"github.com/lox/pokerforbots/v2/protocol"
	"github.com/lox/pokerforbots/v2/sdk/analysis"
	"github.com/lox/pokerforbots/v2/sdk/classification"
	"github.com/lox/pokerforbots/v2/sdk/client"
	"github.com/lox/pokerforbots/v2/sdk/config"
	"github.com/rs/zerolog"
)

// tableState holds the latest state the bot knows about.
type tableState struct {
	HandID       string
	Seat         int
	Pot          int
	Chips        int
	Players      []protocol.Player
	LastAction   protocol.PlayerAction
	HoleCards    poker.Hand // Changed from []string
	HoleCardsStr []string   // Keep for stats/logging
	Board        poker.Hand // Changed from []string
	BoardStr     []string   // Keep for stats/logging
	Street       string
	Button       int
	ActiveCount  int
	BetsThisHand int

	// For simple tracking
	StartingChips int
	HandNum       int

	// Opponent tracking
	NumLimpers int  // Count of limpers before any raise
	raiseSeen  bool // Whether a preflop raise has occurred this hand

	// Cached equity snapshot to avoid repeated calculations within a street
	equity equitySnapshot
}

type equitySnapshot struct {
	valid       bool
	street      string
	boardCards  int
	activeCount int
	class       string
	equity      float64
}

// complexBot implements advanced poker strategy with SDK components.
type complexBot struct {
	id       string
	logger   zerolog.Logger
	state    tableState
	rng      *rand.Rand
	handNum  int
	bigBlind int // Track the big blind amount
	strategy *StrategyConfig
}

func newComplexBot(logger zerolog.Logger) *complexBot {
	// Parse configuration from environment
	cfg, err := config.FromEnv()

	// Read seed from environment, fallback to timestamp if not provided
	seed := time.Now().UnixNano()
	if err == nil && cfg.Seed != 0 {
		seed = cfg.Seed
	}

	// Create deterministic RNG for decision-making using the provided seed
	rng := rand.New(rand.NewPCG(uint64(seed), 0))

	// Check if bot ID is provided by server, otherwise generate one
	var id string
	if err == nil && cfg.BotID != "" {
		// Use the server-provided ID with a complex bot prefix
		id = fmt.Sprintf("complex-%s", cfg.BotID)
	} else {
		// Generate our own ID if not provided (e.g., when run standalone)
		id = fmt.Sprintf("complex-improved-%04d", rng.IntN(10000))
	}

	logger.Debug().Int64("seed", seed).Str("bot_id", id).Msg("Bot initialized with seed")

	return &complexBot{
		id:       id,
		logger:   logger.With().Str("bot_id", id).Logger(),
		rng:      rng,
		handNum:  0,
		bigBlind: 10, // Default big blind
		strategy: defaultStrategy,
	}
}

// SDK Handler interface implementation
func (b *complexBot) OnHandStart(state *client.GameState, start protocol.HandStart) error {
	b.handNum++
	// Update big blind if provided (it should be in every hand)
	if start.BigBlind > 0 {
		b.bigBlind = start.BigBlind
	}
	b.state.HandID = start.HandID
	b.state.Seat = start.YourSeat
	b.state.Players = start.Players
	b.state.Chips = start.Players[start.YourSeat].Chips
	b.state.StartingChips = start.Players[start.YourSeat].Chips

	// Parse hole cards once and store both formats
	b.state.HoleCardsStr = start.HoleCards
	if holeHand, err := poker.ParseHand(start.HoleCards...); err == nil {
		b.state.HoleCards = holeHand
	} else {
		b.state.HoleCards = 0 // Empty hand on parse error
		b.logger.Warn().Err(err).Strs("cards", start.HoleCards).Msg("failed to parse hole cards")
	}

	b.state.Board = 0 // Empty board at start
	b.state.BoardStr = nil
	b.state.Street = "preflop"
	b.state.Button = start.Button
	b.state.BetsThisHand = 0
	b.state.HandNum = b.handNum

	// Count active players
	active := 0
	for _, p := range start.Players {
		if !p.Folded && p.Chips > 0 {
			active++
		}
	}
	b.state.ActiveCount = active

	b.state.NumLimpers = 0
	b.state.raiseSeen = false
	b.state.equity = equitySnapshot{}

	b.logger.Debug().
		Strs("holes", b.state.HoleCardsStr).
		Int("position", b.getPosition()).
		Int("active_players", b.state.ActiveCount).
		Msg("hand start")
	return nil
}

func (b *complexBot) OnActionRequest(state *client.GameState, req protocol.ActionRequest) (string, int, error) {
	// Calculate hand equity once per action decision
	class, equity := b.computeEquity()
	position := b.getPosition()
	potOdds := b.calculatePotOdds(req)

	action, amount := b.makeStrategicDecision(req, class, equity, position, potOdds)

	b.logger.Debug().
		Float64("equity", equity).
		Int("position", position).
		Float64("pot_odds", potOdds).
		Str("action", action).
		Int("amount", amount).
		Msg("decision")

	return action, amount, nil
}

func (b *complexBot) OnGameUpdate(state *client.GameState, update protocol.GameUpdate) error {
	b.state.Pot = update.Pot
	b.state.Players = update.Players
	if b.state.Seat >= 0 && b.state.Seat < len(update.Players) {
		b.state.Chips = update.Players[b.state.Seat].Chips
	}

	// Update active count
	active := 0
	for _, p := range update.Players {
		if !p.Folded {
			active++
		}
	}
	b.state.ActiveCount = active
	return nil
}

func (b *complexBot) OnPlayerAction(state *client.GameState, action protocol.PlayerAction) error {
	b.state.LastAction = action

	// Track our own bets
	if action.Seat == b.state.Seat && (action.Action == "raise" || action.Action == "allin") {
		b.state.BetsThisHand++
	}

	// Maintain simple preflop counters
	if b.state.Street == "preflop" {
		b.updatePreflopCounters(action)
	}

	return nil
}

func (b *complexBot) OnStreetChange(state *client.GameState, street protocol.StreetChange) error {
	b.state.Street = street.Street

	// Parse board once and store both formats
	b.state.BoardStr = street.Board
	if boardHand, err := poker.ParseHand(street.Board...); err == nil {
		b.state.Board = boardHand
	} else {
		b.state.Board = 0 // Empty board on parse error
		b.logger.Warn().Err(err).Strs("cards", street.Board).Msg("failed to parse board")
	}
	return nil
}

func (b *complexBot) OnHandResult(state *client.GameState, result protocol.HandResult) error {
	// Simple logging of result
	netChips := state.Chips - state.StartingChips
	netBB := float64(netChips) / float64(b.bigBlind)

	// Check if we won
	won := false
	myWinnerName := b.ownWinnerName()
	for _, winner := range result.Winners {
		if winner.Name == myWinnerName {
			won = true
			break
		}
	}

	b.logger.Debug().
		Float64("net_bb", netBB).
		Bool("won", won).
		Bool("showdown", len(result.Showdown) > 0).
		Msg("hand completed")

	return nil
}

func (b *complexBot) OnGameCompleted(state *client.GameState, completed protocol.GameCompleted) error {
	// Stop the bot on game completion; server handles stats aggregation/printing.
	return io.EOF
}

func (b *complexBot) computeEquity() (string, float64) {
	street := b.state.Street
	boardCards := b.state.Board.CountCards()
	active := b.state.ActiveCount

	cache := b.state.equity
	if cache.valid && cache.street == street && cache.boardCards == boardCards && cache.activeCount == active {
		return cache.class, cache.equity
	}

	var (
		class  string
		equity float64
	)

	if street == "preflop" {
		class = "preflop"
		equity = b.preflopEquity()
	} else {
		class, equity = b.classifyPostflopSDK()
	}

	if equity <= 0 {
		equity = 0.3
	}

	b.state.equity = equitySnapshot{
		valid:       true,
		street:      street,
		boardCards:  boardCards,
		activeCount: active,
		class:       class,
		equity:      equity,
	}

	return class, equity
}

func (b *complexBot) preflopEquity() float64 {
	if b.state.HoleCards.CountCards() != 2 {
		return 0.5
	}
	if len(b.state.HoleCardsStr) != 2 {
		return 0.5
	}

	category := analysis.GetHandCategory(b.state.HoleCardsStr[0], b.state.HoleCardsStr[1])
	if category == "" {
		return 0.5
	}

	opponents := b.state.ActiveCount - 1
	if opponents < 1 {
		opponents = 1
	}

	equity := analysis.GetPreflopEquity(category, opponents)
	if equity == 0 {
		return 0.3
	}

	return equity
}

// classifyPostflopSDK uses the SDK for advanced postflop analysis
func (b *complexBot) classifyPostflopSDK() (string, float64) {
	if b.state.HoleCards.CountCards() != 2 || b.state.Board.CountCards() < 3 {
		return "unknown", 0.3
	}

	// Use pre-parsed cards
	if b.state.HoleCards == 0 || b.state.Board == 0 {
		return "unknown", 0.3
	}

	holeCards := b.state.HoleCards
	board := b.state.Board

	// Analyze board texture
	boardTexture := classification.AnalyzeBoardTexture(board)

	// Detect draws
	drawInfo := classification.DetectDraws(holeCards, board)

	// Calculate equity using Monte Carlo simulation (small sample for speed)
	equityResult := analysis.CalculateEquity(holeCards, board, b.state.ActiveCount-1, 1000, b.rng)
	equity := equityResult.Equity()

	// Enhanced classification based on draws and board texture
	class := "Air"

	// Check for strong draws
	if drawInfo.HasStrongDraw() {
		if drawInfo.IsComboDraw() {
			class = "ComboDraw"
			equity = math.Max(equity, 0.55) // Boost combo draws
		} else {
			class = "StrongDraw"
			equity = math.Max(equity, 0.40) // Boost strong draws
		}
	} else if drawInfo.HasWeakDraw() {
		class = "WeakDraw"
		equity = math.Max(equity, 0.25)
	}

	// Adjust equity based on board texture
	switch boardTexture {
	case classification.VeryWet:
		if !drawInfo.HasStrongDraw() {
			equity *= 0.85 // Reduce equity on very wet boards without draws
		}
	case classification.Wet:
		if !drawInfo.HasStrongDraw() {
			equity *= 0.90
		}
	case classification.Dry:
		if !drawInfo.HasStrongDraw() {
			equity *= 1.05 // Slight boost on dry boards
		}
	}

	// Use existing hand strength detection for made hands
	switch {
	case equity >= 0.75:
		class = "TripsPlus"
	case equity >= 0.65:
		class = "TwoPair"
	case equity >= 0.55:
		class = "TopPair"
	}

	// Adjust for multiway pots (more conservative reduction)
	if b.state.ActiveCount > 2 {
		// Only reduce by 2% per additional player instead of 3%
		equity *= (1.0 - float64(b.state.ActiveCount-2)*0.02)
	}

	b.logger.Debug().
		Str("board_texture", boardTexture.String()).
		Int("draw_outs", drawInfo.Outs).
		Bool("strong_draw", drawInfo.HasStrongDraw()).
		Str("classification", class).
		Float64("equity", equity).
		Msg("SDK postflop analysis")

	return class, math.Max(equity, 0.05)
}

// updatePreflopCounters maintains lightweight stats needed for simplified strategy.
func (b *complexBot) updatePreflopCounters(action protocol.PlayerAction) {
	switch action.Action {
	case "raise", "allin":
		b.state.raiseSeen = true
	case "call":
		if !b.state.raiseSeen {
			b.state.NumLimpers++
		}
	}
}

// Rest of the methods remain the same as the original complex bot
func (b *complexBot) getPosition() int {
	// Calculate position relative to button
	// 0 = button, 1 = cutoff, 2 = middle, 3+ = early
	if b.state.Button < 0 {
		return 2
	}

	activePlayers := []int{}
	for i, p := range b.state.Players {
		if !p.Folded && p.Chips > 0 {
			activePlayers = append(activePlayers, i)
		}
	}

	if len(activePlayers) <= 2 {
		// Heads-up: button = 0 (in position), other = 1 (out of position)
		if b.state.Seat == b.state.Button {
			return 0 // We have position
		}
		return 1 // We're out of position
	}

	// Find our position relative to button
	buttonIdx := -1
	ourIdx := -1
	for i, seat := range activePlayers {
		if seat == b.state.Button {
			buttonIdx = i
		}
		if seat == b.state.Seat {
			ourIdx = i
		}
	}

	if buttonIdx < 0 || ourIdx < 0 {
		return 2
	}

	distance := (ourIdx - buttonIdx + len(activePlayers)) % len(activePlayers)

	// Adjust position categorization based on table size
	// In 6-max or smaller, be more aggressive with position definitions
	if len(activePlayers) <= 6 {
		// 6-max adjustments:
		// 0 = button, 1 = cutoff, 2+ = early/middle (no true "early" in 6-max)
		if distance >= 2 {
			return 2 // Treat as middle position, not early
		}
	}

	return distance
}

func (b *complexBot) calculatePotOdds(req protocol.ActionRequest) float64 {
	if req.ToCall == 0 {
		return 1000.0 // Free to play
	}
	potAfterCall := req.Pot + req.ToCall
	return float64(potAfterCall) / float64(req.ToCall)
}

func (b *complexBot) makeStrategicDecision(req protocol.ActionRequest, handClass string, equity float64, position int, _ float64) (string, int) {
	// Preflop handled by a dedicated policy
	if b.state.Street == "preflop" {
		return b.preflopDecision(req, position)
	}

	// Postflop: use table-driven decision making
	// Protocol v2: "call" is used for checking (to_call=0)
	canCheck := slices.Contains(req.ValidActions, "call") && req.ToCall == 0
	if equity <= 0 {
		var eq float64
		handClass, eq = b.classifyPostflopSDK()
		if eq > 0 {
			equity = eq
		}
	}

	// For now, removing equity adjustments based on opponent tracking
	// These were causing more problems than benefits
	// TODO: Re-implement with more sophisticated range vs range analysis

	// First check fold threshold
	if b.shouldFold(req, equity) {
		if canCheck {
			return "call", 0 // Protocol v2: call for checking
		}
		return "fold", 0
	}

	// Calculate SPR for decision context
	spr := b.calcSPR(req)
	multiway := b.state.ActiveCount > 2

	// Special case: very low SPR with strong equity
	if spr < 2.0 && equity > 0.60 {
		if slices.Contains(req.ValidActions, "allin") {
			return "allin", 0
		}
	}

	// Look up action from postflop matrix
	action, sizePct := b.strategy.PostflopDecision(handClass, canCheck, spr, multiway)

	// Handle the action from the table
	switch action {
	case "bet":
		if canCheck {
			// Get board texture for sizing
			boardTexture := classification.AnalyzeBoardTexture(b.state.Board)
			sizePct = b.strategy.BetSize(b.state.Street, boardTexture.String(), getHandStrengthCategory(equity))
			return b.raiseOrJam(req, b.betSize(req, sizePct))
		}
		return "call", 0 // Protocol v2: call for checking

	case "raise":
		if slices.Contains(req.ValidActions, "raise") {
			return b.raiseOrJam(req, b.betSize(req, sizePct))
		}
		return "call", 0 // Call if can't raise

	case "call":
		if slices.Contains(req.ValidActions, "call") {
			return "call", 0
		}
		return "fold", 0 // Fold if can't call

	case "check":
		// Protocol v2: check action from strategy becomes call
		if canCheck {
			return "call", 0
		}
		return "fold", 0 // Fold if can't check

	case "fold":
		if canCheck {
			return "call", 0 // Protocol v2: call for checking
		}
		return "fold", 0
	}

	// Fallback
	if canCheck {
		return "call", 0 // Protocol v2: call for checking
	}
	return "fold", 0
}

// getHandStrengthCategory converts equity to a category for bet sizing
func getHandStrengthCategory(equity float64) string {
	if equity >= 0.75 {
		return HandStrengthStrong
	}
	if equity >= 0.50 {
		return HandStrengthMedium
	}
	return HandStrengthDraw
}

// Helper functions (keeping the same implementations as original)
func (b *complexBot) betSize(req protocol.ActionRequest, pct float64) int {
	size := max(min(max(int(float64(req.Pot)*pct), req.MinBet), b.state.Chips), 0)
	return size
}

func (b *complexBot) raiseOrJam(req protocol.ActionRequest, amt int) (string, int) {
	if amt < req.MinBet {
		if amt >= b.state.Chips {
			if slices.Contains(req.ValidActions, "allin") {
				return "allin", 0
			}
		}
		if slices.Contains(req.ValidActions, "call") {
			return "call", 0
		}
		// Protocol v2: check uses call with to_call=0
		if slices.Contains(req.ValidActions, "call") && req.ToCall == 0 {
			return "call", 0
		}
		return "fold", 0
	}
	return "raise", amt
}

func (b *complexBot) calcSPR(req protocol.ActionRequest) float64 {
	if req.Pot <= 0 {
		return 99.0
	}
	return float64(b.state.Chips) / float64(req.Pot)
}

func (b *complexBot) shouldFold(req protocol.ActionRequest, equity float64) bool {
	pot := req.Pot
	if pot <= 0 {
		pot = 1
	}
	if req.ToCall <= 0 {
		return false
	}
	betPct := float64(req.ToCall) / float64(pot)

	if equity < 0.20 && betPct > 0.60 {
		return true
	}

	minEquity := b.strategy.FoldThresholdValue(b.state.Street, betPct)
	return equity < minEquity
}

// Helper to check if our hand is in a range
func (b *complexBot) handInRange(r *analysis.Range) bool {
	if r == nil || len(b.state.HoleCardsStr) != 2 {
		return false
	}
	return r.Contains(b.state.HoleCardsStr[0], b.state.HoleCardsStr[1])
}

// Preflop decision logic using table-driven ranges
func hasAction(valid []string, target string) bool {
	return slices.Contains(valid, target)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (b *complexBot) preflopDecision(req protocol.ActionRequest, position int) (string, int) {
	// Validate hole cards
	if len(b.state.HoleCardsStr) != 2 {
		// Protocol v2: check uses call with to_call=0
		if hasAction(req.ValidActions, "call") && req.ToCall == 0 {
			return "call", 0
		}
		return "fold", 0
	}

	bb := b.bigBlind
	facing := req.ToCall
	inPosition := position <= 1

	// Adjust bet sizing based on table dynamics
	limperMultiplier := 1.0 + float64(b.state.NumLimpers)*0.5

	// Standard bet sizes (adjust for limpers)
	minR := max(req.MinRaise, req.MinBet)
	openSize := maxInt(minR, int(2.5*float64(bb)*limperMultiplier))
	threeBetIP := maxInt(minR, int(8.5*float64(bb)))
	threeBetOOP := maxInt(minR, int(10.0*float64(bb)))
	fourBetSize := maxInt(minR, int(22.0*float64(bb)))

	// Get relevant ranges for our position
	// Use special heads-up ranges when appropriate
	isHeadsUp := b.state.ActiveCount <= 2

	var openRange, defendRange *analysis.Range
	if isHeadsUp {
		openRange = b.strategy.PreflopRangeFor(position, ActionOpenHeadsUp)
		defendRange = b.strategy.PreflopRangeFor(position, ActionDefendHeadsUp)
	} else {
		openRange = b.strategy.PreflopRangeFor(position, ActionOpen)
		defendRange = b.strategy.PreflopRangeFor(position, ActionDefend)
	}

	value3BetRange := b.strategy.PreflopRangeFor(position, Action3BetValue)
	bluff3BetRange := b.strategy.PreflopRangeFor(position, Action3BetBluff)
	fourBetRange := b.strategy.PreflopRangeFor(position, Action4Bet)

	// Case 1: BB can check (to_call==0)
	// Protocol v2: use "call" for checking
	if facing == 0 && hasAction(req.ValidActions, "call") {
		// Optional BB iso-raise with strong hands
		if b.handInRange(openRange) && hasAction(req.ValidActions, "raise") {
			return b.raiseOrJam(req, openSize)
		}
		return "call", 0
	}

	// Case 2: Unopened / limped pot
	if facing <= bb {
		if hasAction(req.ValidActions, "raise") && b.handInRange(openRange) {
			return b.raiseOrJam(req, openSize)
		}
		// No limping - check if possible (to_call==0), else fold
		// Protocol v2: use "call" for checking
		if hasAction(req.ValidActions, "call") && facing == 0 {
			return "call", 0
		}
		return "fold", 0
	}

	// Case 3: Facing an open raise (2-3bb)
	if facing > bb && facing <= 3*bb {
		// 3-bet for value
		if hasAction(req.ValidActions, "raise") && b.handInRange(value3BetRange) {
			amt := threeBetOOP
			if inPosition {
				amt = threeBetIP
			}
			return b.raiseOrJam(req, min(amt, b.state.Chips))
		}

		// 3-bet bluff occasionally (25% frequency)
		if hasAction(req.ValidActions, "raise") && b.handInRange(bluff3BetRange) && b.rng.Float64() < 0.25 {
			amt := threeBetIP
			if !inPosition {
				amt = threeBetOOP
			}
			return b.raiseOrJam(req, min(amt, b.state.Chips))
		}

		// Defend by calling
		if hasAction(req.ValidActions, "call") && b.handInRange(defendRange) {
			return "call", 0
		}

		return "fold", 0
	}

	// Case 4: Facing a 3-bet or larger (>3bb)
	if facing > 3*bb {
		// 4-bet with premium range
		if b.handInRange(fourBetRange) {
			// Jam if low SPR
			if hasAction(req.ValidActions, "allin") && b.calcSPR(req) < 4.0 {
				return "allin", 0
			}
			// Otherwise standard 4-bet
			if hasAction(req.ValidActions, "raise") {
				return b.raiseOrJam(req, min(fourBetSize, b.state.Chips))
			}
		}

		// Sometimes flat strong hands in position (simplified to TT-JJ)
		if hasAction(req.ValidActions, "call") && inPosition {
			flatRange := b.strategy.FlatTrapRange
			if flatRange != nil && b.handInRange(flatRange) {
				return "call", 0
			}
		}

		return "fold", 0
	}

	return "fold", 0
}

func (b *complexBot) ownWinnerName() string {
	candidates := []string{b.id}
	if len(b.id) >= 8 {
		candidates = append(candidates, b.id[:8])
	}
	candidates = append(candidates, fmt.Sprintf("player-%d", b.state.Seat+1))
	candidates = append(candidates, fmt.Sprintf("bot-%d", b.state.Seat+1))
	// Prefer truncated ID if present
	for _, c := range candidates {
		if len(c) == 8 || c == b.id {
			return c
		}
	}
	return candidates[0]
}

// Handler is an alias for complexBot to satisfy the client.Handler interface
type Handler = complexBot

// NewHandler creates a new complex bot handler
func NewHandler() *Handler {
	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	return newComplexBot(logger)
}

// NewHandlerWithLogger creates a new complex bot handler with a custom logger
func NewHandlerWithLogger(logger zerolog.Logger) *Handler {
	return newComplexBot(logger)
}

// NewQuietHandler creates a new complex bot handler with logging disabled
func NewQuietHandler() *Handler {
	logger := zerolog.New(io.Discard)
	return newComplexBot(logger)
}

// Check it implements the client.Handler interface
var _ client.Handler = (*Handler)(nil)
