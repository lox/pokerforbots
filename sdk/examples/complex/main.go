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
	"strings"
	"syscall"
	"time"

	"github.com/lox/pokerforbots/internal/server/statistics"
	"github.com/lox/pokerforbots/poker"
	"github.com/lox/pokerforbots/protocol"
	"github.com/lox/pokerforbots/sdk/analysis"
	"github.com/lox/pokerforbots/sdk/classification"
	"github.com/lox/pokerforbots/sdk/client"
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

	// For statistics tracking
	StartingChips int
	HandNum       int
	PreflopAction string
	FlopAction    string
	TurnAction    string
	RiverAction   string
}

// opponentProfile tracks opponent behavior
type opponentProfile struct {
	Name        string
	HandsSeen   int
	FoldCount   int
	CallCount   int
	RaiseCount  int
	AggroFactor float64 // (raises + bets) / calls
	VPIP        float64 // voluntary put in pot
	// internal counters
	vpipVoluntary int  // count of hands where player VPIP'd preflop
	vpipThisHand  bool // whether player VPIP'd this hand (preflop call/raise/all-in excluding blinds)
	LastAction    string
	LastStreet    string
}

// complexImprovedBot implements advanced poker strategy with SDK components.
type complexImprovedBot struct {
	id        string
	logger    zerolog.Logger
	state     tableState
	opponents map[string]*opponentProfile
	rng       *rand.Rand
	stats     *statistics.Statistics
	handNum   int
	bigBlind  int // Track the big blind amount
}

func newComplexImprovedBot(logger zerolog.Logger) *complexImprovedBot {
	id := fmt.Sprintf("complex-improved-%04d", rand.Intn(10000))
	return &complexImprovedBot{
		id:        id,
		logger:    logger.With().Str("bot_id", id).Logger(),
		opponents: make(map[string]*opponentProfile),
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
		stats:     statistics.NewStatistics(10), // Default to 10 chip big blind, will update when we get game info
		handNum:   0,
		bigBlind:  10, // Default big blind
	}
}

// SDK Handler interface implementation
func (b *complexImprovedBot) OnHandStart(state *client.GameState, start protocol.HandStart) error {
	b.handNum++
	// Update big blind if provided (it should be in every hand)
	if start.BigBlind > 0 {
		b.bigBlind = start.BigBlind
		// Update statistics package with new big blind if it changed
		if b.handNum == 1 {
			b.stats = statistics.NewStatistics(b.bigBlind)
		}
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
	// Reset action tracking
	b.state.PreflopAction = ""
	b.state.FlopAction = ""
	b.state.TurnAction = ""
	b.state.RiverAction = ""

	// Reset per-hand opponent VPIP flags
	for _, prof := range b.opponents {
		prof.vpipThisHand = false
	}

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

func (b *complexImprovedBot) OnActionRequest(state *client.GameState, req protocol.ActionRequest) (string, int, error) {
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

	// Track the action for statistics
	switch action {
	case "fold":
		b.trackAction("fold")
	case "call":
		if req.ToCall == 0 {
			b.trackAction("check")
		} else {
			b.trackAction("call")
		}
	case "raise", "allin":
		b.trackAction("raise")
	}

	b.logger.Debug().
		Float64("hand_strength", handStrength).
		Int("position", position).
		Float64("pot_odds", potOdds).
		Str("action", action).
		Int("amount", amount).
		Msg("decision")

	return action, amount, nil
}

func (b *complexImprovedBot) OnGameUpdate(state *client.GameState, update protocol.GameUpdate) error {
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

func (b *complexImprovedBot) OnPlayerAction(state *client.GameState, action protocol.PlayerAction) error {
	b.state.LastAction = action

	// Track opponent behavior
	if action.Seat != b.state.Seat {
		prof := b.getOrCreateProfile(action.PlayerName)
		prof.LastAction = action.Action
		prof.LastStreet = action.Street

		switch action.Action {
		case "fold":
			prof.FoldCount++
		case "call", "post_big_blind", "post_small_blind":
			prof.CallCount++
			// VPIP tracking: preflop voluntary money excludes blinds
			if action.Street == "preflop" && action.Action == "call" {
				prof.vpipThisHand = true
			}
		case "raise", "allin":
			prof.RaiseCount++
			// VPIP: any preflop raise or all-in counts as voluntary
			if action.Street == "preflop" {
				prof.vpipThisHand = true
			}
			if action.Seat == b.state.Seat {
				b.state.BetsThisHand++
			}
		}

		// Update aggression factor
		if prof.CallCount > 0 {
			prof.AggroFactor = float64(prof.RaiseCount) / float64(prof.CallCount)
		}
	}
	return nil
}

func (b *complexImprovedBot) OnStreetChange(state *client.GameState, street protocol.StreetChange) error {
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

func (b *complexImprovedBot) OnHandResult(state *client.GameState, result protocol.HandResult) error {
	// Calculate net result for this hand (use SDK state which includes final payout)
	netChips := state.Chips - state.StartingChips
	netBB := float64(netChips) / float64(b.bigBlind)

	// Update opponent VPIP stats for this hand
	for _, p := range b.state.Players {
		if p.Seat == b.state.Seat {
			continue
		}
		prof := b.getOrCreateProfile(p.Name)
		prof.HandsSeen++
		if prof.vpipThisHand {
			prof.vpipVoluntary++
		}
		if prof.HandsSeen > 0 {
			prof.VPIP = float64(prof.vpipVoluntary) / float64(prof.HandsSeen)
		}
		// Reset per hand flag
		prof.vpipThisHand = false
	}

	// Check if we won (server sends winner.Name as our perspective display name: first 8 chars of ID)
	won := false
	wonAtShowdown := false
	myWinnerName := b.ownWinnerName()
	for _, winner := range result.Winners {
		if winner.Name == myWinnerName {
			won = true
			if len(result.Showdown) > 0 {
				wonAtShowdown = true
			}
			break
		}
	}

	// Calculate button distance
	buttonDist := b.calculateButtonDistance()

	// Categorize hole cards
	handCategory := b.categorizeHoleCards()

	// Get final street
	finalStreet := b.determineFinalStreet()

	// Count opponents
	numOpponents := 0
	for _, p := range b.state.Players {
		if p.Name != b.id && !p.Folded {
			numOpponents++
		}
	}

	// Create hand result for statistics
	handResult := statistics.HandResult{
		HandNum:        b.state.HandNum,
		NetBB:          netBB,
		Position:       b.state.Seat,
		ButtonDistance: buttonDist,
		WentToShowdown: len(result.Showdown) > 0,
		WonAtShowdown:  wonAtShowdown,
		FinalPotBB:     float64(b.state.Pot) / float64(b.bigBlind),
		StreetReached:  finalStreet,
		HoleCards:      strings.Join(b.state.HoleCardsStr, ""),
		HandCategory:   handCategory,
		PreflopAction:  b.state.PreflopAction,
		FlopAction:     b.state.FlopAction,
		TurnAction:     b.state.TurnAction,
		RiverAction:    b.state.RiverAction,
		NumOpponents:   numOpponents,
	}

	// Add to statistics
	if err := b.stats.Add(handResult); err != nil {
		b.logger.Error().Err(err).Msg("failed to add hand result to statistics")
	}

	b.logger.Debug().
		Float64("net_bb", netBB).
		Bool("won", won).
		Bool("showdown", len(result.Showdown) > 0).
		Str("street", finalStreet).
		Msg("hand completed")

	return nil
}

func (b *complexImprovedBot) OnGameCompleted(state *client.GameState, completed protocol.GameCompleted) error {
	// Stop the bot on game completion; server handles stats aggregation/printing.
	return io.EOF
}

func (b *complexImprovedBot) evaluateHandStrength() float64 {
	if b.state.HoleCards.CountCards() != 2 {
		return 0.5
	}

	// Use string format for preflop calculations (until we refactor those helpers)
	if len(b.state.HoleCardsStr) != 2 {
		return 0.5
	}
	h1, h2 := b.state.HoleCardsStr[0], b.state.HoleCardsStr[1]
	r1, r2 := client.CardRank(h1), client.CardRank(h2)
	suited := client.IsSuited(h1, h2)

	// Pre-flop strength calculation
	if b.state.Street == "preflop" {
		strength := 0.0

		// Pocket pairs
		if r1 == r2 {
			strength = 0.5 + float64(r1)*0.035
			if r1 >= 10 { // TT+
				strength += 0.2
			}
		} else {
			// High cards
			maxRank := math.Max(float64(r1), float64(r2))
			minRank := math.Min(float64(r1), float64(r2))
			strength = 0.15 + maxRank*0.025 + minRank*0.015

			// Suited bonus
			if suited {
				strength += 0.1
			}

			// Connected cards
			gap := math.Abs(float64(r1) - float64(r2))
			switch gap {
			case 1:
				strength += 0.08
			case 2:
				strength += 0.04
			}

			// Premium hands
			if (r1 == 14 && r2 >= 10) || (r2 == 14 && r1 >= 10) {
				strength += 0.15
			}
		}

		return math.Min(strength, 0.95)
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
func (b *complexImprovedBot) classifyPostflopSDK() (string, float64) {
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
func (b *complexImprovedBot) getPosition() int {
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

func (b *complexImprovedBot) calculatePotOdds(req protocol.ActionRequest) float64 {
	if req.ToCall == 0 {
		return 1000.0 // Free to play
	}
	potAfterCall := req.Pot + req.ToCall
	return float64(potAfterCall) / float64(req.ToCall)
}

func (b *complexImprovedBot) makeStrategicDecision(req protocol.ActionRequest, handStrength float64, position int, potOdds float64) (string, int) {
	// Preflop handled by a dedicated policy
	if b.state.Street == "preflop" {
		return b.preflopDecision(req, position)
	}

	// Postflop: apply fold thresholds, SPR awareness, and standardized sizing
	canCheck := false
	for _, a := range req.ValidActions {
		if a == "check" {
			canCheck = true
			break
		}
	}

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
		for _, a := range req.ValidActions {
			if a == "allin" {
				return "allin", 0
			}
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

	// Opponent exploitation based on last aggressor profile
	if req.ToCall > 0 {
		last := b.state.LastAction
		if (last.Action == "raise" || last.Action == "allin") && last.PlayerName != "" && last.Seat != b.state.Seat {
			prof := b.getOrCreateProfile(last.PlayerName)
			isPassive := prof.AggroFactor > 0 && prof.AggroFactor <= 0.7
			isAggro := prof.AggroFactor >= 1.7
			// If passive villain makes a big bet and we are marginal, overfold
			if isPassive && betPct >= 0.5 {
				switch class {
				case "TripsPlus", "Overpair", "TwoPair", "TPTK":
					// continue with normal flow
				default:
					return "fold", 0
				}
			}
			// If aggro villain uses small bets, apply pressure more often
			if isAggro && betPct <= 0.33 && !avoidRaise {
				if class == "ComboDraw" || class == "StrongDraw" || equity >= 0.50 {
					for _, a := range req.ValidActions {
						if a == "raise" {
							return b.raiseOrJam(req, b.betSize(req, 0.33))
						}
					}
				}
			}
		}
	}

	// Raise for value with very strong hands
	if equity >= 0.75 && !avoidRaise {
		for _, a := range req.ValidActions {
			if a == "raise" {
				return b.raiseOrJam(req, b.betSize(req, 0.50))
			}
		}
	}

	// Semi-bluff occasionally with strong draws vs small bets
	if (class == "ComboDraw" || class == "StrongDraw") && betPct <= 0.5 && !avoidRaise && position <= 1 {
		if b.rng.Float64() < 0.25 { // 25% frequency
			for _, a := range req.ValidActions {
				if a == "raise" {
					return b.raiseOrJam(req, b.betSize(req, 0.33))
				}
			}
		}
	}

	// Otherwise continue by calling if possible
	for _, a := range req.ValidActions {
		if a == "call" {
			return "call", 0
		}
	}

	// Fallback
	return "fold", 0
}

// Helper functions (keeping the same implementations as original)
func (b *complexImprovedBot) betSize(req protocol.ActionRequest, pct float64) int {
	size := int(float64(req.Pot) * pct)
	if size < req.MinBet {
		size = req.MinBet
	}
	if size > b.state.Chips {
		size = b.state.Chips
	}
	if size < 0 {
		size = 0
	}
	return size
}

func (b *complexImprovedBot) raiseOrJam(req protocol.ActionRequest, amt int) (string, int) {
	if amt < req.MinBet {
		if amt >= b.state.Chips {
			for _, a := range req.ValidActions {
				if a == "allin" {
					return "allin", 0
				}
			}
		}
		for _, a := range req.ValidActions {
			if a == "call" {
				return "call", 0
			}
		}
		for _, a := range req.ValidActions {
			if a == "check" {
				return "check", 0
			}
		}
		return "fold", 0
	}
	return "raise", amt
}

func (b *complexImprovedBot) calcSPR(req protocol.ActionRequest) float64 {
	if req.Pot <= 0 {
		return 99.0
	}
	return float64(b.state.Chips) / float64(req.Pot)
}

func (b *complexImprovedBot) shouldFold(req protocol.ActionRequest, equity float64) bool {
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
	for _, a := range valid {
		if a == target {
			return true
		}
	}
	return false
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (b *complexImprovedBot) preflopDecision(req protocol.ActionRequest, position int) (string, int) {
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
	minR := req.MinRaise
	if minR < req.MinBet {
		minR = req.MinBet
	}
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
		// Identify opener profile if available
		opener := b.state.LastAction
		prof := &opponentProfile{}
		if opener.Action == "raise" && opener.PlayerName != "" && opener.Seat != b.state.Seat {
			prof = b.getOrCreateProfile(opener.PlayerName)
		}
		openerNit := prof.VPIP > 0 && prof.VPIP <= 0.18
		openerLoose := prof.VPIP >= 0.40
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
		if hasAction(req.ValidActions, "raise") && inBluff3Bet() && !openerNit {
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
			if openerNit {
				// tighten vs nits: prefer pairs 77+ or suited broadways
				if !pairAtLeast(7) && !suitedBroadway {
					return "fold", 0
				}
			}
			// slightly widen vs loose openers
			if openerLoose && !inDefendCall() && hasAction(req.ValidActions, "raise") && inBluff3Bet() && inPosition {
				// occasionally apply pressure
				if b.rng.Float64() < 0.25 {
					amt := threeBetIP
					if amt > b.state.Chips {
						amt = b.state.Chips
					}
					return b.raiseOrJam(req, amt)
				}
			}
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
			amt := fourBetSize
			if amt > b.state.Chips {
				amt = b.state.Chips
			}
			return b.raiseOrJam(req, amt)
		}
		if hasAction(req.ValidActions, "call") && pairAtLeast(10) && inPosition { // flats TT/JJ IP sometimes
			return "call", 0
		}
		return "fold", 0
	}

	return "fold", 0
}

func (b *complexImprovedBot) ownWinnerName() string {
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

func (b *complexImprovedBot) getOrCreateProfile(name string) *opponentProfile {
	if prof, ok := b.opponents[name]; ok {
		return prof
	}
	prof := &opponentProfile{Name: name}
	b.opponents[name] = prof
	return prof
}

// trackAction records the action taken for statistics
func (b *complexImprovedBot) trackAction(action string) {
	switch b.state.Street {
	case "preflop":
		b.state.PreflopAction = action
	case "flop":
		b.state.FlopAction = action
	case "turn":
		b.state.TurnAction = action
	case "river":
		b.state.RiverAction = action
	}
}

// calculateButtonDistance returns distance from button (0=button, 1=CO, etc)
func (b *complexImprovedBot) calculateButtonDistance() int {
	numPlayers := 0
	for _, p := range b.state.Players {
		if p.Chips > 0 {
			numPlayers++
		}
	}

	if numPlayers == 0 {
		return 0
	}

	distance := b.state.Seat - b.state.Button
	if distance <= 0 {
		distance += numPlayers
	}
	return distance - 1
}

// categorizeHoleCards categorizes preflop hand strength
func (b *complexImprovedBot) categorizeHoleCards() string {
	if len(b.state.HoleCardsStr) != 2 {
		return "unknown"
	}

	h1, h2 := b.state.HoleCardsStr[0], b.state.HoleCardsStr[1]
	r1, r2 := client.CardRank(h1), client.CardRank(h2)
	suited := client.IsSuited(h1, h2)

	if r1 > r2 {
		r1, r2 = r2, r1 // Ensure r1 <= r2
	}

	// Premium hands
	if (r1 >= 12 && r2 >= 12) || // AA, KK, QQ, AK
		(r1 == 11 && r2 == 11) { // JJ
		return "Premium"
	}

	// Strong hands
	if (r1 >= 10 && r2 >= 10) || // TT+, AQ, AJ
		(r1 >= 12 && r2 >= 10) ||
		(r1 == 9 && r2 == 9) || // 99
		(suited && r1 >= 11 && r2 >= 11) { // KQs, QJs
		return "Strong"
	}

	// Medium hands
	if (r1 >= 7 && r2 >= 7) || // 77+
		(r1 >= 12 && r2 >= 8) || // A9+
		(suited && r1 >= 9 && r2 >= 10) || // Suited connectors/broadways
		(r1 >= 10 && r2 >= 11) { // KJ, QJ, JT
		return "Medium"
	}

	// Weak playable hands
	if (r1 >= 5 && r2 >= 5) || // 55+
		(suited && math.Abs(float64(r1-r2)) <= 2) || // Suited connectors
		(r1 >= 12) { // Any ace
		return "Weak"
	}

	return "Trash"
}

// determineFinalStreet returns the furthest street reached
func (b *complexImprovedBot) determineFinalStreet() string {
	if b.state.RiverAction != "" || b.state.Board.CountCards() >= 5 {
		return "River"
	}
	if b.state.TurnAction != "" || b.state.Board.CountCards() >= 4 {
		return "Turn"
	}
	if b.state.FlopAction != "" || b.state.Board.CountCards() >= 3 {
		return "Flop"
	}
	return "Preflop"
}

func main() {
	serverURL := flag.String("server", "ws://localhost:8080/ws", "WebSocket server URL")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	level := zerolog.InfoLevel
	if *debug {
		level = zerolog.DebugLevel
	}
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).Level(level).With().Timestamp().Logger()

	complexBot := newComplexImprovedBot(logger)
	bot := client.New(complexBot.id, complexBot, logger)

	if err := bot.Connect(*serverURL); err != nil {
		logger.Fatal().Err(err).Msg("connect failed")
	}
	logger.Info().Msg("complex improved bot connected")

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
