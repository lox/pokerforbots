package main

import (
	"context"
	"encoding/json"
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

	"github.com/lox/pokerforbots/internal/protocol"
	"github.com/lox/pokerforbots/internal/server/statistics"
	"github.com/lox/pokerforbots/sdk"
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
	HoleCards    []string
	Board        []string
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
	LastAction  string
	LastStreet  string
}

// complexBot implements advanced poker strategy.
type complexBot struct {
	id        string
	logger    zerolog.Logger
	state     tableState
	opponents map[string]*opponentProfile
	rng       *rand.Rand
	stats     *statistics.Statistics
	handNum   int
	bigBlind  int // Track the big blind amount
}

// StatsSummary holds aggregated statistics
type StatsSummary struct {
	Hands           int     `json:"hands"`
	NetBB           float64 `json:"net_bb"`
	BB100           float64 `json:"bb_per_100"`
	Mean            float64 `json:"mean_bb_hand"`
	Median          float64 `json:"median_bb_hand"`
	StdDev          float64 `json:"std_dev"`
	CI95Low         float64 `json:"ci_95_low"`
	CI95High        float64 `json:"ci_95_high"`
	WinningHands    int     `json:"winning_hands"`
	WinRate         float64 `json:"win_rate_pct"`
	ShowdownWins    int     `json:"showdown_wins"`
	NonShowdownWins int     `json:"non_showdown_wins"`
	ShowdownWinRate float64 `json:"showdown_win_rate_pct"`
	ShowdownBB      float64 `json:"showdown_bb"`
	NonShowdownBB   float64 `json:"non_showdown_bb"`
	MaxPotBB        float64 `json:"max_pot_bb"`
	BigPots         int     `json:"big_pots_50bb_plus"`
}

// PositionSummary holds position-specific statistics
type PositionSummary struct {
	Name      string  `json:"name"`
	Hands     int     `json:"hands"`
	NetBB     float64 `json:"net_bb"`
	BBPerHand float64 `json:"bb_per_hand"`
	BB100     float64 `json:"bb_per_100"`
	WinRate   float64 `json:"win_rate_pct"`
}

// StreetSummary holds street-specific statistics
type StreetSummary struct {
	HandsEnded int     `json:"hands_ended"`
	NetBB      float64 `json:"net_bb"`
	BBPerHand  float64 `json:"bb_per_hand"`
	Wins       int     `json:"wins"`
	Losses     int     `json:"losses"`
}

// CategorySummary holds hand category statistics
type CategorySummary struct {
	Hands        int     `json:"hands"`
	NetBB        float64 `json:"net_bb"`
	BBPerHand    float64 `json:"bb_per_hand"`
	Wins         int     `json:"wins"`
	ShowdownWins int     `json:"showdown_wins"`
	ShowdownRate float64 `json:"showdown_rate_pct"`
}

func newComplexBot(logger zerolog.Logger) *complexBot {
	id := fmt.Sprintf("complex-%04d", rand.Intn(10000))
	return &complexBot{
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
func (b *complexBot) OnHandStart(state *sdk.GameState, start protocol.HandStart) error {
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
	b.state.HoleCards = start.HoleCards
	b.state.Board = nil
	b.state.Street = "preflop"
	b.state.Button = start.Button
	b.state.BetsThisHand = 0
	b.state.HandNum = b.handNum
	// Reset action tracking
	b.state.PreflopAction = ""
	b.state.FlopAction = ""
	b.state.TurnAction = ""
	b.state.RiverAction = ""

	// Count active players
	active := 0
	for _, p := range start.Players {
		if !p.Folded && p.Chips > 0 {
			active++
		}
	}
	b.state.ActiveCount = active

	b.logger.Debug().
		Strs("holes", state.HoleCards).
		Int("position", b.getPosition()).
		Msg("hand start")
	return nil
}

func (b *complexBot) OnActionRequest(state *sdk.GameState, req protocol.ActionRequest) (string, int, error) {
	// Calculate hand strength and make strategic decision
	handStrength := b.evaluateHandStrength()
	if b.state.Street != "preflop" {
		_, eq := b.classifyPostflop()
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

func (b *complexBot) OnGameUpdate(state *sdk.GameState, update protocol.GameUpdate) error {
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

func (b *complexBot) OnPlayerAction(state *sdk.GameState, action protocol.PlayerAction) error {
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
		case "raise", "allin":
			prof.RaiseCount++
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

func (b *complexBot) OnStreetChange(state *sdk.GameState, street protocol.StreetChange) error {
	b.state.Street = street.Street
	b.state.Board = street.Board
	return nil
}

func (b *complexBot) OnHandResult(state *sdk.GameState, result protocol.HandResult) error {
	// Calculate net result for this hand (use SDK state which includes final payout)
	netChips := state.Chips - state.StartingChips
	netBB := float64(netChips) / float64(b.bigBlind)

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
		HoleCards:      strings.Join(b.state.HoleCards, ""),
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

func (b *complexBot) OnGameCompleted(state *sdk.GameState, completed protocol.GameCompleted) error {
	// Save results to JSON file
	type BotResults struct {
		Timestamp      time.Time                      `json:"timestamp"`
		BotID          string                         `json:"bot_id"`
		GameID         string                         `json:"game_id"`
		HandsCompleted uint64                         `json:"hands_completed"`
		Seed           int64                          `json:"seed"`
		MyStats        *protocol.GameCompletedPlayer  `json:"my_stats"`
		AllPlayers     []protocol.GameCompletedPlayer `json:"all_players"`
		Won            bool                           `json:"won"`

		// Detailed statistics
		Statistics        *StatsSummary              `json:"statistics"`
		PositionStats     map[string]PositionSummary `json:"position_stats"`
		StreetStats       map[string]StreetSummary   `json:"street_stats"`
		HandCategoryStats map[string]CategorySummary `json:"hand_category_stats"`
	}

	var myStats *protocol.GameCompletedPlayer
	won := false

	for i, ps := range completed.Players {
		if ps.DisplayName == b.id {
			myStats = &completed.Players[i]
			// Check if we won (highest net chips)
			won = true
			for _, other := range completed.Players {
				if other.DisplayName != b.id && other.NetChips > ps.NetChips {
					won = false
					break
				}
			}

			b.logger.Info().
				Str("game_id", completed.GameID).
				Float64("bb_per_100", (ps.AvgPerHand/float64(b.bigBlind))*100).
				Int64("net_chips", ps.NetChips).
				Uint64("hands", completed.HandsCompleted).
				Bool("won", won).
				Msg("game completed - my results")
			break
		}
	}

	// Compile statistics
	statsSummary := &StatsSummary{}
	hands := b.stats.Hands()
	if hands > 0 {
		hands, sumBB, winningHands, _, showdownWins, nonShowdownWins, showdownLosses, showdownBB, nonShowdownBB, maxPotBB, bigPots, _, _, _ := b.stats.GetStats()
		low, high := b.stats.ConfidenceInterval95()
		winRate := 0.0
		if hands > 0 {
			winRate = float64(winningHands) / float64(hands) * 100
		}
		showdownWinRate := 0.0
		totalShowdowns := showdownWins + showdownLosses
		if totalShowdowns > 0 {
			showdownWinRate = float64(showdownWins) / float64(totalShowdowns) * 100
		}

		statsSummary = &StatsSummary{
			Hands:           hands,
			NetBB:           sumBB,
			BB100:           b.stats.BB100(),
			Mean:            b.stats.Mean(),
			Median:          b.stats.Median(),
			StdDev:          b.stats.StdDev(),
			CI95Low:         low,
			CI95High:        high,
			WinningHands:    winningHands,
			WinRate:         winRate,
			ShowdownWins:    showdownWins,
			NonShowdownWins: nonShowdownWins,
			ShowdownWinRate: showdownWinRate,
			ShowdownBB:      showdownBB,
			NonShowdownBB:   nonShowdownBB,
			MaxPotBB:        maxPotBB,
			BigPots:         bigPots,
		}
	}

	// Position statistics
	positionStats := make(map[string]PositionSummary)
	buttonDistResults := b.stats.ButtonDistanceResults()
	for dist := 0; dist < 6; dist++ {
		bd := buttonDistResults[dist]
		if bd.Hands > 0 {
			posMean := b.stats.ButtonDistanceMean(dist)
			winRate := 0.0
			if bd.Hands > 0 {
				winRate = float64(bd.Wins) / float64(bd.Hands) * 100
			}
			positionStats[statistics.GetPositionName(dist)] = PositionSummary{
				Name:      statistics.GetPositionName(dist),
				Hands:     bd.Hands,
				NetBB:     bd.SumBB,
				BBPerHand: posMean,
				BB100:     posMean * 100,
				WinRate:   winRate,
			}
		}
	}

	// Street statistics
	streetStats := make(map[string]StreetSummary)
	for street, stat := range b.stats.StreetStats() {
		if stat.HandsReached > 0 {
			streetStats[street] = StreetSummary{
				HandsEnded: stat.HandsReached,
				NetBB:      stat.NetBB,
				BBPerHand:  stat.NetBB / float64(stat.HandsReached),
				Wins:       stat.Wins,
				Losses:     stat.Losses,
			}
		}
	}

	// Hand category statistics
	categoryStats := make(map[string]CategorySummary)
	for cat, stat := range b.stats.CategoryStats() {
		if stat.Hands > 0 {
			showdownRate := 0.0
			if stat.WentToShowdown > 0 {
				showdownRate = float64(stat.WentToShowdown) / float64(stat.Hands) * 100
			}
			categoryStats[cat] = CategorySummary{
				Hands:        stat.Hands,
				NetBB:        stat.NetBB,
				BBPerHand:    stat.NetBB / float64(stat.Hands),
				Wins:         stat.Wins,
				ShowdownWins: stat.ShowdownWins,
				ShowdownRate: showdownRate,
			}
		}
	}

	results := BotResults{
		Timestamp:         time.Now(),
		BotID:             b.id,
		GameID:            completed.GameID,
		HandsCompleted:    completed.HandsCompleted,
		Seed:              completed.Seed,
		MyStats:           myStats,
		AllPlayers:        completed.Players,
		Won:               won,
		Statistics:        statsSummary,
		PositionStats:     positionStats,
		StreetStats:       streetStats,
		HandCategoryStats: categoryStats,
	}

	// Write to JSON file
	// Print summary to console
	if b.stats.Hands() > 0 {
		fmt.Println(b.stats.Summary())
	}

	// Save detailed results
	filename := fmt.Sprintf("complex-bot-results-%s-%d.json", b.id, time.Now().Unix())
	file, err := os.Create(filename)
	if err != nil {
		b.logger.Error().Err(err).Msg("failed to create results file")
	} else {
		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(results); err != nil {
			b.logger.Error().Err(err).Msg("failed to write results")
		} else {
			b.logger.Info().Str("file", filename).Msg("results saved to file")
		}
		file.Close()
	}

	return io.EOF
}

func (b *complexBot) evaluateHandStrength() float64 {
	if len(b.state.HoleCards) != 2 {
		return 0.5
	}

	// Parse hole cards
	h1, h2 := b.state.HoleCards[0], b.state.HoleCards[1]
	r1, r2 := sdk.CardRank(h1), sdk.CardRank(h2)
	suited := sdk.IsSuited(h1, h2)

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

	// Post-flop: simplified strength based on board texture
	// This is a placeholder - real implementation would evaluate actual hand ranking
	strength := 0.3

	// Check for pairs with board
	for _, boardCard := range b.state.Board {
		br := sdk.CardRank(boardCard)
		if br == r1 || br == r2 {
			strength += 0.2
		}
	}

	// High card bonus
	if r1 == 14 || r2 == 14 {
		strength += 0.1
	}

	// Adjust based on number of opponents
	if b.state.ActiveCount > 2 {
		strength *= (1.0 - float64(b.state.ActiveCount-2)*0.05)
	}

	return math.Min(math.Max(strength, 0.1), 0.9)
}

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

func (b *complexBot) makeStrategicDecision(req protocol.ActionRequest, handStrength float64, position int, potOdds float64) (string, int) {
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

	class, equity := b.classifyPostflop()
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
				return "raise", b.betSize(req, 0.50)
			}
		case "TopPair":
			if !avoidRaise {
				return "raise", b.betSize(req, 0.33)
			}
		case "ComboDraw", "StrongDraw":
			if !avoidRaise {
				return "raise", b.betSize(req, 0.33)
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

	// Raise for value with very strong hands
	if equity >= 0.75 && !avoidRaise {
		for _, a := range req.ValidActions {
			if a == "raise" {
				return "raise", b.betSize(req, 0.50)
			}
		}
	}

	// Semi-bluff occasionally with strong draws vs small bets
	if (class == "ComboDraw" || class == "StrongDraw") && betPct <= 0.5 && !avoidRaise && position <= 1 {
		if b.rng.Float64() < 0.25 { // 25% frequency
			for _, a := range req.ValidActions {
				if a == "raise" {
					return "raise", b.betSize(req, 0.33)
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

// --- Helpers for Patch 2: postflop classification (coarse buckets) ---

// classifyPostflop returns a coarse class and equity bucket in [0,1]
func (b *complexBot) classifyPostflop() (string, float64) {
	// Basic info
	if len(b.state.HoleCards) != 2 {
		return "unknown", 0.3
	}
	h1, h2 := b.state.HoleCards[0], b.state.HoleCards[1]
	hr1, hr2 := sdk.CardRank(h1), sdk.CardRank(h2)
	s1, s2 := sdk.CardSuit(h1), sdk.CardSuit(h2)
	board := b.state.Board

	// Build board ranks/suits
	boardRanks := make([]int, 0, len(board))
	boardSuits := make([]byte, 0, len(board))
	for _, c := range board {
		boardRanks = append(boardRanks, sdk.CardRank(c))
		boardSuits = append(boardSuits, sdk.CardSuit(c))
	}
	// Frequency maps and texture
	rankCount := map[int]int{}
	suitCount := map[byte]int{}
	minBoard := 99
	maxBoard := 0
	for i, r := range boardRanks {
		rankCount[r]++
		if r < minBoard {
			minBoard = r
		}
		if r > maxBoard {
			maxBoard = r
		}
		suitCount[boardSuits[i]]++
	}
	numDistinctSuits := len(suitCount)
	monotone := numDistinctSuits == 1 && len(boardSuits) >= 3
	twoTone := numDistinctSuits == 2 && len(boardSuits) >= 3
	pairedBoard := false
	for _, cnt := range rankCount {
		if cnt >= 2 {
			pairedBoard = true
			break
		}
	}
	wet := len(boardRanks) >= 3 && (maxBoard-minBoard) <= 4
	_ = monotone

	// Combined counts for made hands
	combinedCount := map[int]int{}
	for _, r := range boardRanks {
		combinedCount[r]++
	}
	combinedCount[hr1]++
	combinedCount[hr2]++

	// Detect trips+ and two pair
	for r, cnt := range combinedCount {
		if cnt >= 4 { // quads/full house
			return "TripsPlus", 0.85
		}
		_ = r
	}
	pairs := 0
	for _, cnt := range combinedCount {
		if cnt >= 2 {
			pairs++
		}
	}
	if pairs >= 2 { // two pair or better
		return "TwoPair", 0.70
	}

	// Overpair: pocket pair higher than any board rank
	if hr1 == hr2 {
		maxBoard := 0
		for _, r := range boardRanks {
			if r > maxBoard {
				maxBoard = r
			}
		}
		if hr1 > maxBoard {
			return "Overpair", 0.80
		}
	}

	// Top pair vs second pair
	maxBoard = 0
	secondBoard := 0
	for _, r := range boardRanks {
		if r > maxBoard {
			maxBoard = r
		}
	}
	for _, r := range boardRanks {
		if r > secondBoard && r < maxBoard {
			secondBoard = r
		}
	}
	isTopPair := (hr1 == maxBoard || hr2 == maxBoard)
	isSecondPair := (hr1 == secondBoard || hr2 == secondBoard)
	if isTopPair {
		kicker := hr1
		if hr1 == maxBoard {
			kicker = hr2
		}
		if kicker >= 13 { // K or A kicker ~ TPTK
			eq := 0.65
			if wet || twoTone {
				eq -= 0.05
			}
			if eq < 0.55 {
				eq = 0.55
			}
			return "TPTK", eq
		}
		eq := 0.55
		if wet || twoTone {
			eq -= 0.05
		}
		if pairedBoard {
			eq -= 0.03
		}
		if eq < 0.45 {
			eq = 0.45
		}
		return "TopPair", eq
	}
	if isSecondPair {
		eq := 0.42
		if wet {
			eq -= 0.03
		}
		if eq < 0.35 {
			eq = 0.35
		}
		return "SecondPair", eq
	}

	// Draws: flush draw
	fd := false
	suitCounts := map[byte]int{}
	suitCounts[s1]++
	suitCounts[s2]++
	for _, s := range boardSuits {
		suitCounts[s]++
	}
	for _, c := range suitCounts {
		if c >= 4 {
			fd = true
			break
		}
	}

	// Straight draws (approx): check for 4 out of 5 consecutive ranks
	uniq := map[int]bool{}
	for _, r := range boardRanks {
		uniq[r] = true
	}
	uniq[hr1] = true
	uniq[hr2] = true
	// simple scan
	oesd := false
	gut := false
	for start := 2; start <= 10; start++ {
		need := 0
		have := 0
		for d := 0; d < 5; d++ {
			r := start + d
			if uniq[r] {
				have++
			} else {
				need++
			}
		}
		if have >= 4 && need == 1 {
			gut = true
		}
		if have >= 4 && (uniq[start] && uniq[start+4]) { // ends present, closer to OESD
			oesd = true
		}
	}

	// Combo draws weighting
	if fd && oesd {
		return "ComboDraw", 0.55
	}
	if fd || oesd {
		return "StrongDraw", 0.40
	}
	if gut {
		return "WeakDraw", 0.25
	}

	// Air fallback
	class := "Air"
	eq := 0.10
	// Multiway adjust
	if b.state.ActiveCount > 2 {
		eq -= 0.05 * float64(b.state.ActiveCount-2)
		if eq < 0.05 {
			eq = 0.05
		}
	}
	return class, eq
}

// --- Helpers for Patch 1: sizing, thresholds, preflop policy ---

func (b *complexBot) betSize(req protocol.ActionRequest, pct float64) int {
	size := int(float64(req.Pot) * pct)
	minRequired := req.MinBet
	if req.MinRaise > minRequired {
		minRequired = req.MinRaise
	}
	if size < minRequired {
		size = minRequired
	}
	if size > b.state.Chips {
		size = b.state.Chips
	}
	if size < 0 {
		size = 0
	}
	return size
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

// --- Preflop decision policy (tighten & size) ---

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

func (b *complexBot) preflopDecision(req protocol.ActionRequest, position int) (string, int) {
	// Extract ranks/suited
	if len(b.state.HoleCards) != 2 {
		if hasAction(req.ValidActions, "check") {
			return "check", 0
		}
		return "fold", 0
	}
	h1, h2 := b.state.HoleCards[0], b.state.HoleCards[1]
	r1, r2 := sdk.CardRank(h1), sdk.CardRank(h2)
	suited := sdk.IsSuited(h1, h2)
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
			return "raise", openSize
		}
		return "check", 0
	}

	// Case 2: Unopened / limp-sized to call (treat as open spot)
	if facing <= bb {
		if hasAction(req.ValidActions, "raise") && inOpenRange(position) {
			return "raise", openSize
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
			return "raise", amt
		}
		if hasAction(req.ValidActions, "raise") && inBluff3Bet() {
			amt := threeBetIP
			if !inPosition {
				amt = threeBetOOP
			}
			if amt > b.state.Chips {
				amt = b.state.Chips
			}
			return "raise", amt
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
			amt := fourBetSize
			if amt > b.state.Chips {
				amt = b.state.Chips
			}
			return "raise", amt
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

func (b *complexBot) getOrCreateProfile(name string) *opponentProfile {
	if prof, ok := b.opponents[name]; ok {
		return prof
	}
	prof := &opponentProfile{Name: name}
	b.opponents[name] = prof
	return prof
}

// trackAction records the action taken for statistics
func (b *complexBot) trackAction(action string) {
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
func (b *complexBot) calculateButtonDistance() int {
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
func (b *complexBot) categorizeHoleCards() string {
	if len(b.state.HoleCards) != 2 {
		return "unknown"
	}

	h1, h2 := b.state.HoleCards[0], b.state.HoleCards[1]
	r1, r2 := sdk.CardRank(h1), sdk.CardRank(h2)
	suited := sdk.IsSuited(h1, h2)

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
func (b *complexBot) determineFinalStreet() string {
	if b.state.RiverAction != "" || len(b.state.Board) >= 5 {
		return "River"
	}
	if b.state.TurnAction != "" || len(b.state.Board) >= 4 {
		return "Turn"
	}
	if b.state.FlopAction != "" || len(b.state.Board) >= 3 {
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

	complexBot := newComplexBot(logger)
	bot := sdk.New(complexBot.id, complexBot, logger)

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
