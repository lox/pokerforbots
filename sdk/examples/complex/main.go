package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/lox/pokerforbots/poker"
	"github.com/lox/pokerforbots/protocol"
	"github.com/lox/pokerforbots/sdk/analysis"
	"github.com/lox/pokerforbots/sdk/classification"
	"github.com/lox/pokerforbots/sdk/client"
	"github.com/lox/pokerforbots/sdk/config"
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
}

// ==========================================
// TABLE-DRIVEN STRATEGY DEFINITIONS
// ==========================================

// FoldThreshold defines minimum equity needed to continue at different bet sizes
type FoldThreshold struct {
	Street    string
	MaxBetPct float64 // Maximum bet/pot ratio this threshold applies to
	MinEquity float64 // Minimum equity needed to continue
}

// Sorted fold thresholds by street and bet size
var foldThresholds = []FoldThreshold{
	// Flop thresholds
	{StreetFlop, 0.33, 0.15},
	{StreetFlop, 0.66, 0.35},
	{StreetFlop, 999, 0.50},
	// Turn thresholds
	{StreetTurn, 0.50, 0.30},
	{StreetTurn, 1.00, 0.50},
	{StreetTurn, 999, 0.60},
	// River thresholds
	{StreetRiver, 0.25, 0.30},
	{StreetRiver, 0.50, 0.45},
	{StreetRiver, 999, 0.60},
}

// Position constants for preflop ranges
const (
	PositionButton = 0
	PositionCutoff = 1
	PositionMiddle = 2
	PositionEarly  = 3  // 3+ is treated as early position
	PositionAny    = -1 // Applies to any position
)

// Action constants for preflop ranges
const (
	ActionOpen      = "open"
	Action3BetValue = "3bet_value"
	Action3BetBluff = "3bet_bluff"
	ActionDefend    = "defend"
	Action4Bet      = "4bet"
)

// PreflopRange defines opening/3bet/4bet ranges by position
type PreflopRange struct {
	Position int    // Use Position* constants
	Action   string // Use Action* constants
	Range    string // Range notation like "AA,KK,AKs"
}

// Preflop ranges organized by position and action
var preflopRanges = []PreflopRange{
	// Early position (UTG/EP) opening range - tight
	{PositionEarly, ActionOpen, "77+,AJo+,KQo,A5s+,KTs+,QTs+,JTs,T9s"},

	// Middle position (MP/HJ) - slightly wider
	{PositionMiddle, ActionOpen, "55+,ATo+,KJo+,A2s+,K9s+,Q9s+,J9s+,T9s,98s,87s,76s"},

	// Cutoff - much wider
	{PositionCutoff, ActionOpen, "22+,A2+,K8o+,Q9o+,J9o+,T9o,K2s+,Q4s+,J7s+,T7s+,97s+,86s+,75s+,65s,54s"},

	// Button - widest opening range
	{PositionButton, ActionOpen, "22+,A2+,K5o+,Q8o+,J8o+,T8o+,98o,K2s+,Q2s+,J4s+,T6s+,96s+,85s+,74s+,64s+,53s+,43s"},

	// 3-bet value range (position-independent)
	{PositionAny, Action3BetValue, "TT+,AQs+,AKo"},

	// 3-bet bluff range (only from late position)
	{PositionButton, Action3BetBluff, "A5s-A2s,K9s,K8s,QTs,JTs,T9s,98s,87s,76s,65s"},
	{PositionCutoff, Action3BetBluff, "A5s-A2s,KTs,K9s,QTs,JTs"},

	// Defend vs 3-bet (call range)
	{PositionAny, ActionDefend, "99-22,AJs,KQs,QJs,JTs,T9s,98s,87s,76s"},

	// 4-bet range
	{PositionAny, Action4Bet, "QQ+,AK"},
}

// PostflopAction defines actions based on hand classification and situation
type PostflopAction struct {
	HandClass string  // "TripsPlus", "TwoPair", "TopPair", etc.
	CanCheck  bool    // Whether we have the option to check
	MaxSPR    float64 // Maximum SPR for this action
	Multiway  bool    // Whether pot has 3+ players
	Action    string  // "bet", "check", "call", "raise", "fold"
	SizePct   float64 // Bet size as percentage of pot
}

// Postflop action matrix
var postflopMatrix = []PostflopAction{
	// Strong hands - always bet/raise for value
	{"TripsPlus", true, 999, false, "bet", 0.50},
	{"TripsPlus", true, 999, true, "bet", 0.75}, // Size up multiway
	{"TripsPlus", false, 999, false, "raise", 0.50},
	{"TripsPlus", false, 999, true, "call", 0}, // Just call multiway when facing bet

	// Two pair
	{"TwoPair", true, 999, false, "bet", 0.50},
	{"TwoPair", true, 999, true, "check", 0}, // Pot control multiway
	{"TwoPair", false, 999, false, "call", 0},
	{"TwoPair", false, 999, true, "call", 0},

	// Top pair good kicker
	{"TPTK", true, 8.0, false, "bet", 0.33},
	{"TPTK", true, 999, false, "check", 0}, // High SPR pot control
	{"TPTK", true, 999, true, "check", 0},  // Multiway pot control
	{"TPTK", false, 999, false, "call", 0},
	{"TPTK", false, 999, true, "fold", 0}, // Fold multiway to aggression

	// Top pair weak kicker
	{"TopPair", true, 5.0, false, "bet", 0.25},
	{"TopPair", true, 999, false, "check", 0},
	{"TopPair", true, 999, true, "check", 0},
	{"TopPair", false, 999, false, "call", 0},
	{"TopPair", false, 999, true, "fold", 0},

	// Strong draws
	{"ComboDraw", true, 8.0, false, "bet", 0.33},
	{"ComboDraw", true, 999, false, "check", 0},
	{"ComboDraw", false, 999, false, "call", 0},
	{"ComboDraw", false, 999, true, "call", 0},

	{"StrongDraw", true, 5.0, false, "bet", 0.25},
	{"StrongDraw", true, 999, false, "check", 0},
	{"StrongDraw", false, 999, false, "call", 0},
	{"StrongDraw", false, 999, true, "fold", 0},

	// Weak draws and air
	{"WeakDraw", true, 999, false, "check", 0},
	{"WeakDraw", false, 999, false, "fold", 0},
	{"WeakDraw", false, 999, true, "fold", 0},

	{"Air", true, 999, false, "check", 0},
	{"Air", false, 999, false, "fold", 0},
	{"Air", false, 999, true, "fold", 0},
}

// Street constants for bet sizing
const (
	StreetFlop  = "flop"
	StreetTurn  = "turn"
	StreetRiver = "river"
)

// HandStrength categories for bet sizing
const (
	HandStrengthStrong = "strong"
	HandStrengthMedium = "medium"
	HandStrengthDraw   = "draw"
	HandStrengthAny    = "*"
)

// BoardTextureString constants matching classification.BoardTexture.String() output
const (
	BoardTextureDry     = "dry"
	BoardTextureSemiWet = "semi-wet"
	BoardTextureWet     = "wet"
	BoardTextureVeryWet = "very wet"
	BoardTextureAny     = "*"
)

// BetSizing defines bet sizes for different situations
type BetSizing struct {
	Street       string
	BoardTexture string // Use BoardTexture* constants
	HandStrength string // Use HandStrength* constants
	SizePct      float64
}

// Bet sizing table
var betSizingTable = []BetSizing{
	// Flop sizing based on board texture
	{StreetFlop, BoardTextureDry, HandStrengthAny, 0.33},
	{StreetFlop, BoardTextureSemiWet, HandStrengthAny, 0.50},
	{StreetFlop, BoardTextureWet, HandStrengthAny, 0.66},
	{StreetFlop, BoardTextureVeryWet, HandStrengthAny, 0.75},

	// Turn standard sizing
	{StreetTurn, BoardTextureAny, HandStrengthStrong, 0.66},
	{StreetTurn, BoardTextureAny, HandStrengthMedium, 0.50},
	{StreetTurn, BoardTextureAny, HandStrengthDraw, 0.50},

	// River sizing based on hand strength
	{StreetRiver, BoardTextureAny, HandStrengthStrong, 1.00},
	{StreetRiver, BoardTextureAny, HandStrengthMedium, 0.50},
	{StreetRiver, BoardTextureAny, HandStrengthDraw, 0.75}, // Bluff sizing
}

// ==========================================
// TABLE LOOKUP FUNCTIONS
// ==========================================

// lookupFoldThreshold finds the minimum equity needed to continue
func lookupFoldThreshold(street string, betPct float64) float64 {
	for _, threshold := range foldThresholds {
		if threshold.Street == street && betPct <= threshold.MaxBetPct {
			return threshold.MinEquity
		}
	}
	return 0.50 // Default threshold
}

// getPreflopRange returns the range for a given position and action
func getPreflopRange(position int, action string) *analysis.Range {
	for _, pr := range preflopRanges {
		// PositionAny means any position
		// PositionEarly applies to all positions >= 3
		positionMatches := pr.Position == PositionAny ||
			pr.Position == position ||
			(pr.Position == PositionEarly && position >= PositionEarly)

		if positionMatches && pr.Action == action {
			if r, err := analysis.ParseRange(pr.Range); err == nil {
				return r
			}
		}
	}
	return nil
}

// lookupPostflopAction finds the best action for current situation
func lookupPostflopAction(handClass string, canCheck bool, spr float64, multiway bool) (string, float64) {
	for _, action := range postflopMatrix {
		if action.HandClass != handClass {
			continue
		}
		if action.CanCheck != canCheck {
			continue
		}
		if spr > action.MaxSPR {
			continue
		}
		if action.Multiway != multiway {
			continue
		}
		return action.Action, action.SizePct
	}
	// Default fallback
	if canCheck {
		return "check", 0
	}
	return "fold", 0
}

// lookupBetSizing returns the bet size for a situation
func lookupBetSizing(street, boardTexture, handStrength string) float64 {
	for _, sizing := range betSizingTable {
		if sizing.Street != street {
			continue
		}
		if sizing.BoardTexture != BoardTextureAny && sizing.BoardTexture != boardTexture {
			continue
		}
		if sizing.HandStrength != HandStrengthAny && sizing.HandStrength != handStrength {
			continue
		}
		return sizing.SizePct
	}
	return 0.50 // Default sizing
}

// complexBot implements advanced poker strategy with SDK components.
type complexBot struct {
	id       string
	logger   zerolog.Logger
	state    tableState
	rng      *rand.Rand
	handNum  int
	bigBlind int // Track the big blind amount
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
	rng := rand.New(rand.NewSource(seed))

	// Check if bot ID is provided by server, otherwise generate one
	var id string
	if err == nil && cfg.BotID != "" {
		// Use the server-provided ID with a complex bot prefix
		id = fmt.Sprintf("complex-%s", cfg.BotID)
	} else {
		// Generate our own ID if not provided (e.g., when run standalone)
		id = fmt.Sprintf("complex-improved-%04d", rng.Intn(10000))
	}

	logger.Debug().Int64("seed", seed).Str("bot_id", id).Msg("Bot initialized with seed")

	return &complexBot{
		id:       id,
		logger:   logger.With().Str("bot_id", id).Logger(),
		rng:      rng,
		handNum:  0,
		bigBlind: 10, // Default big blind
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

	b.logger.Debug().
		Strs("holes", b.state.HoleCardsStr).
		Int("position", b.getPosition()).
		Msg("hand start")
	return nil
}

func (b *complexBot) OnActionRequest(state *client.GameState, req protocol.ActionRequest) (string, int, error) {
	// Calculate hand strength using SDK components
	handStrength := b.evaluateHandStrength()
	if b.state.Street != "preflop" {
		_, eq := b.classifyPostflopSDK()
		if eq > 0 {
			handStrength = eq
		}
	}
	position := b.getPosition()
	potOdds := b.calculatePotOdds(req)

	action, amount := b.makeStrategicDecision(req, handStrength, position, potOdds)

	b.logger.Debug().
		Float64("hand_strength", handStrength).
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

	// Just track if we bet/raised for simple logic
	if action.Seat == b.state.Seat && (action.Action == "raise" || action.Action == "allin") {
		b.state.BetsThisHand++
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

func (b *complexBot) evaluateHandStrength() float64 {
	if b.state.HoleCards.CountCards() != 2 {
		return 0.5
	}

	// Use string format for preflop calculations
	if len(b.state.HoleCardsStr) != 2 {
		return 0.5
	}

	// Pre-flop: Use accurate Monte Carlo equity table
	if b.state.Street == "preflop" {
		// Get hand category (e.g., "AA", "AKs", "72o")
		category := analysis.GetHandCategory(b.state.HoleCardsStr[0], b.state.HoleCardsStr[1])
		if category == "" {
			return 0.5 // Default if can't categorize
		}

		// Get equity based on number of active opponents
		opponents := max(b.state.ActiveCount-1, 1)

		equity := analysis.GetPreflopEquity(category, opponents)
		if equity == 0 {
			// Fallback to basic estimate if not in table
			return 0.3
		}

		return equity
	}

	// Post-flop: Use SDK equity calculator for accurate strength evaluation
	if b.state.Board.CountCards() >= 3 {
		// Use pre-parsed cards for equity calculation
		if b.state.HoleCards == 0 || b.state.Board == 0 {
			return 0.3 // Default strength if parsing failed
		}

		equityResult := analysis.CalculateEquity(b.state.HoleCards, b.state.Board, b.state.ActiveCount-1, 1000, b.rng)
		return equityResult.Equity()
	}

	// Should not reach here but return default
	return 0.3
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

	// Adjust for multiway pots
	if b.state.ActiveCount > 2 {
		equity *= (1.0 - float64(b.state.ActiveCount-2)*0.03)
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
		return 0 // Heads up
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

func (b *complexBot) makeStrategicDecision(req protocol.ActionRequest, handStrength float64, position int, _ float64) (string, int) {
	// Preflop handled by a dedicated policy
	if b.state.Street == "preflop" {
		return b.preflopDecision(req, position)
	}

	// Postflop: use table-driven decision making
	canCheck := slices.Contains(req.ValidActions, "check")
	class, equity := b.classifyPostflopSDK()
	if equity <= 0 {
		equity = handStrength // fallback
	}

	// First check fold threshold
	if b.shouldFold(req, equity) {
		if canCheck {
			return "check", 0
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
	action, sizePct := lookupPostflopAction(class, canCheck, spr, multiway)

	// Handle the action from the table
	switch action {
	case "bet":
		if canCheck {
			// Get board texture for sizing
			boardTexture := classification.AnalyzeBoardTexture(b.state.Board)
			sizePct = lookupBetSizing(b.state.Street, boardTexture.String(), getHandStrengthCategory(equity))
			return b.raiseOrJam(req, b.betSize(req, sizePct))
		}
		return "check", 0 // Can't bet if we can't check

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
		if canCheck {
			return "check", 0
		}
		return "fold", 0 // Fold if can't check

	case "fold":
		if canCheck {
			return "check", 0 // Check if it's free
		}
		return "fold", 0
	}

	// Fallback
	if canCheck {
		return "check", 0
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
		if slices.Contains(req.ValidActions, "check") {
			return "check", 0
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
	betPct := float64(req.ToCall) / float64(pot)

	// Fallback guard
	if equity < 0.20 && betPct > 0.60 {
		return true
	}

	// Use table lookup instead of switch
	minEquity := lookupFoldThreshold(b.state.Street, betPct)
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
		if hasAction(req.ValidActions, "check") {
			return "check", 0
		}
		return "fold", 0
	}

	bb := b.bigBlind
	facing := req.ToCall
	inPosition := position <= 1

	// Standard bet sizes
	minR := max(req.MinRaise, req.MinBet)
	openSize := maxInt(minR, int(2.5*float64(bb)))
	threeBetIP := maxInt(minR, int(8.5*float64(bb)))
	threeBetOOP := maxInt(minR, int(10.0*float64(bb)))
	fourBetSize := maxInt(minR, int(22.0*float64(bb)))

	// Get relevant ranges for our position
	openRange := getPreflopRange(position, ActionOpen)
	value3BetRange := getPreflopRange(position, Action3BetValue)
	bluff3BetRange := getPreflopRange(position, Action3BetBluff)
	defendRange := getPreflopRange(position, ActionDefend)
	fourBetRange := getPreflopRange(position, Action4Bet)

	// Case 1: BB can check
	if facing == 0 && hasAction(req.ValidActions, "check") {
		// Optional BB iso-raise with strong hands
		if b.handInRange(openRange) && hasAction(req.ValidActions, "raise") {
			return b.raiseOrJam(req, openSize)
		}
		return "check", 0
	}

	// Case 2: Unopened / limped pot
	if facing <= bb {
		if hasAction(req.ValidActions, "raise") && b.handInRange(openRange) {
			return b.raiseOrJam(req, openSize)
		}
		// No limping - check if possible, else fold
		if hasAction(req.ValidActions, "check") {
			return "check", 0
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
			// Check if we have TT or JJ
			flatRange, _ := analysis.ParseRange("TT,JJ")
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

func main() {
	serverURL := flag.String("server", "ws://localhost:8080/ws", "WebSocket server URL")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	// Parse configuration from environment
	cfg, err := config.FromEnv()
	if err == nil {
		// Use environment config if available
		*serverURL = cfg.ServerURL
	}

	level := zerolog.InfoLevel
	if *debug {
		level = zerolog.DebugLevel
	}
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).Level(level).With().Timestamp().Logger()

	complexBot := newComplexBot(logger)
	bot := client.New(complexBot.id, complexBot, logger)

	if err := bot.Connect(*serverURL); err != nil {
		logger.Fatal().Err(err).Msg("connect failed")
	}
	logger.Info().Msg("complex bot connected")

	// Handle shutdown gracefully
	ctx, cancel := context.WithCancel(context.Background())
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	runErr := make(chan error, 1)
	go func() { runErr <- bot.Run(ctx) }()

	select {
	case <-interrupt:
		logger.Info().Msg("shutting down")
		cancel()
	case err := <-runErr:
		if err != nil {
			logger.Error().Err(err).Msg("run error")
		}
	}
}
