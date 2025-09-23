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

// complexBot implements advanced poker strategy with SDK components.
type complexBot struct {
	id       string
	logger   zerolog.Logger
	state    tableState
	rng      *rand.Rand
	handNum  int
	bigBlind int // Track the big blind amount
}

func newcomplexBot(logger zerolog.Logger) *complexBot {
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

	// Postflop: apply fold thresholds, SPR awareness, and standardized sizing
	canCheck := slices.Contains(req.ValidActions, "check")

	class, equity := b.classifyPostflopSDK()
	if equity <= 0 {
		equity = handStrength // fallback
	}
	if b.shouldFold(req, equity) {
		if canCheck {
			return "check", 0
		}
		return "fold", 0
	}

	// SPR based guardrails
	spr := b.calcSPR(req)
	// If very low SPR with strong equity, prefer jamming when available
	if spr < 2.0 && equity > 0.60 {
		if slices.Contains(req.ValidActions, "allin") {
			return "allin", 0
		}
	}

	// With high SPR and marginal equity, avoid raising
	avoidRaise := spr > 8.0 && equity < 0.55

	// Choose action and size
	if canCheck {
		// Multiway pot control unless strong
		if b.state.ActiveCount > 2 && equity < 0.60 {
			return "check", 0
		}
		// Decide to value bet / semi-bluff or pot control
		switch class {
		case "TripsPlus", "Overpair", "TwoPair", "TPTK":
			if !avoidRaise {
				return b.raiseOrJam(req, b.betSize(req, 0.50))
			}
		case "TopPair":
			if !avoidRaise {
				return b.raiseOrJam(req, b.betSize(req, 0.33))
			}
		case "ComboDraw", "StrongDraw":
			if !avoidRaise {
				return b.raiseOrJam(req, b.betSize(req, 0.33))
			}
		}
		// Pot control
		return "check", 0
	}

	// Facing a bet
	pot := req.Pot
	if pot <= 0 {
		pot = 1
	}
	betPct := float64(req.ToCall) / float64(pot)

	// Simple adjustment based on bet size
	// Against large bets, be more selective
	if req.ToCall > 0 && betPct >= 0.75 {
		switch class {
		case "TripsPlus", "Overpair", "TwoPair", "TPTK":
			// continue with strong hands
		default:
			// fold marginal hands vs large bets
			if equity < 0.45 {
				return "fold", 0
			}
		}
	}

	// Raise for value with very strong hands
	if equity >= 0.75 && !avoidRaise {
		if slices.Contains(req.ValidActions, "raise") {
			return b.raiseOrJam(req, b.betSize(req, 0.50))
		}
	}

	// Semi-bluff occasionally with strong draws vs small bets
	if (class == "ComboDraw" || class == "StrongDraw") && betPct <= 0.5 && !avoidRaise && position <= 1 {
		if b.rng.Float64() < 0.25 { // 25% frequency
			if slices.Contains(req.ValidActions, "raise") {
				return b.raiseOrJam(req, b.betSize(req, 0.33))
			}
		}
	}

	// Otherwise continue by calling if possible
	if slices.Contains(req.ValidActions, "call") {
		return "call", 0
	}

	// Fallback
	return "fold", 0
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

	switch b.state.Street {
	case "flop":
		if betPct <= 0.33 {
			return equity <= 0.15
		} else if betPct <= 0.66 {
			return equity <= 0.35
		}
		return equity <= 0.50
	case "turn":
		if betPct <= 0.50 {
			return equity <= 0.30
		} else if betPct < 1.00 {
			return equity <= 0.50
		}
		return equity < 0.60
	case "river":
		if betPct <= 0.25 {
			return equity <= 0.30
		}
		if betPct <= 0.50 {
			return equity <= 0.45
		}
		return equity <= 0.60
	default:
		return false
	}
}

// Preflop decision logic (keeping the same implementation)
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
	// Extract ranks/suited
	if len(b.state.HoleCardsStr) != 2 {
		if hasAction(req.ValidActions, "check") {
			return "check", 0
		}
		return "fold", 0
	}
	h1, h2 := b.state.HoleCardsStr[0], b.state.HoleCardsStr[1]
	r1, r2 := client.CardRank(h1), client.CardRank(h2)
	suited := client.IsSuited(h1, h2)
	low, high := r1, r2
	if low > high {
		low, high = high, low
	}

	bb := b.bigBlind
	// Detect scenario
	facing := req.ToCall

	// Open sizes (respect min raise requirements)
	minR := max(req.MinRaise, req.MinBet)
	openSize := maxInt(minR, int(2.5*float64(bb)))
	threeBetIP := maxInt(minR, int(8.5*float64(bb)))
	threeBetOOP := maxInt(minR, int(10.0*float64(bb)))
	fourBetSize := maxInt(minR, int(22.0*float64(bb)))

	inPosition := position <= 1 // approximate

	// Helper predicates
	isPair := r1 == r2
	pairAtLeast := func(min int) bool { return isPair && high >= min }
	anyAce := high == 14
	suitedBroadway := suited && high >= 11 && low >= 10 // KQs, QJs, KJs etc.
	offBroadway := !suited && high >= 13 && low >= 10   // KQo, KJo, QJo
	suitedConnector := suited && (high-low == 1) && high >= 8
	suitedWheelAce := suited && anyAce && low >= 2 && low <= 5 // A5s-A2s

	inOpenRange := func(pos int) bool {
		switch {
		case pos >= 3: // early (UTG/LJ)
			if pairAtLeast(7) {
				return true
			}
			if anyAce && !suited && low >= 11 {
				return true
			} // AJo+
			if offBroadway && high == 13 && low == 12 {
				return true
			} // KQo
			if anyAce && suited && low >= 5 {
				return true
			} // A5s+
			if suited && high == 13 && low >= 10 {
				return true
			} // KTs+
			if suited && high == 12 && low >= 10 {
				return true
			} // QTs+
			if suited && high == 11 && low == 10 {
				return true
			} // JTs
			if suited && high == 10 && low == 9 {
				return true
			} // T9s
			return false
		case pos == 2: // CO
			if pairAtLeast(5) {
				return true
			}
			if anyAce && suited && low >= 9 {
				return true
			} // A9s+
			if anyAce && !suited && low >= 11 {
				return true
			} // AJo+
			if !suited && high == 13 && low >= 11 {
				return true
			} // KJo+
			if suited && high == 13 && low >= 9 {
				return true
			} // K9s+
			if suited && high == 12 && low >= 10 {
				return true
			} // QTs+
			if suited && high == 11 && low == 10 {
				return true
			} // JTs
			if suited && high == 10 && low == 9 {
				return true
			} // T9s
			if suited && high == 9 && low == 8 {
				return true
			} // 98s
			return false
		default: // BTN and later (pos 0/1)
			if isPair {
				return true
			}
			if anyAce {
				return true
			}
			if suited && high == 13 && low >= 8 {
				return true
			} // K8s+
			if suited && high == 12 && low >= 9 {
				return true
			} // Q9s+
			if suited && high == 11 && low >= 9 {
				return true
			} // J9s+
			if suited && high >= 10 && low >= 8 {
				return true
			} // T8s+
			if suitedConnector {
				return true
			} // 65s+
			if !suited && high >= 13 && low >= 10 {
				return true
			} // KTo+, QTo+, JTo
			return false
		}
	}

	inValue3Bet := func() bool {
		if pairAtLeast(10) {
			return true
		} // TT+
		if anyAce && suited && low >= 12 {
			return true
		} // AQs+
		if anyAce && !suited && low == 13 {
			return true
		} // AKo
		return false
	}

	inBluff3Bet := func() bool {
		if position <= 1 { // BTN/CO only
			if suitedWheelAce {
				return true
			}
			if suited && high == 13 && low >= 9 {
				return true
			} // K9s+
			if suitedBroadway && high != 14 {
				return true
			} // QTs, KQs already covered by value sometimes
		}
		return false
	}

	inDefendCall := func() bool {
		// Simple defend set: small pairs, suited broadways, good suited connectors
		if pairAtLeast(2) && high <= 9 {
			return true
		} // 22-99
		if suitedBroadway {
			return true
		}
		if suitedConnector {
			return true
		}
		return false
	}

	// Case 1: BB can check
	if facing == 0 && hasAction(req.ValidActions, "check") {
		// Optional BB iso-raise with strong hands; keep simple: check most
		if inOpenRange(position) && hasAction(req.ValidActions, "raise") {
			return b.raiseOrJam(req, openSize)
		}
		return "check", 0
	}

	// Case 2: Unopened / limp-sized to call (treat as open spot)
	if facing <= bb {
		if hasAction(req.ValidActions, "raise") && inOpenRange(position) {
			return b.raiseOrJam(req, openSize)
		}
		// No limping strategy: fold if cannot/should not raise
		if hasAction(req.ValidActions, "check") {
			return "check", 0
		}
		return "fold", 0
	}

	// Case 3: Facing an open raise (≈2-3bb)
	if facing > bb && facing <= 3*bb {
		// 3-bet value / bluff, else call some, else fold
		if hasAction(req.ValidActions, "raise") && inValue3Bet() {
			amt := threeBetOOP
			if inPosition {
				amt = threeBetIP
			}
			if amt > b.state.Chips {
				amt = b.state.Chips
			}
			return b.raiseOrJam(req, amt)
		}
		// Only bluff 3-bet occasionally (25% of the time) to avoid being too aggressive
		if hasAction(req.ValidActions, "raise") && inBluff3Bet() && b.rng.Float64() < 0.25 {
			amt := threeBetIP
			if !inPosition {
				amt = threeBetOOP
			}
			if amt > b.state.Chips {
				amt = b.state.Chips
			}
			return b.raiseOrJam(req, amt)
		}
		if hasAction(req.ValidActions, "call") && inDefendCall() {
			return "call", 0
		}
		return "fold", 0
	}

	// Case 4: Facing a 3-bet (large ToCall) → 4-bet only with QQ+/AK
	if facing > 3*bb {
		qqPlusAK := (pairAtLeast(12) || (anyAce && (low == 13 || (suited && low >= 12))))
		if hasAction(req.ValidActions, "allin") && qqPlusAK && b.calcSPR(req) < 4.0 {
			return "allin", 0
		}
		if hasAction(req.ValidActions, "raise") && qqPlusAK {
			amt := min(fourBetSize, b.state.Chips)
			return b.raiseOrJam(req, amt)
		}
		if hasAction(req.ValidActions, "call") && pairAtLeast(10) && inPosition { // flats TT/JJ IP sometimes
			return "call", 0
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

	complexBot := newcomplexBot(logger)
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
