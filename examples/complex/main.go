package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
	"github.com/lox/pokerforbots/internal/server/statistics"
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
	conn      *websocket.Conn
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

func (b *complexBot) connect(serverURL string) error {
	u, err := url.Parse(serverURL)
	if err != nil {
		return err
	}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	b.conn = conn

	connect := &protocol.Connect{Type: protocol.TypeConnect, Name: b.id, Role: "player"}
	payload, err := protocol.Marshal(connect)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, payload)
}

func (b *complexBot) run() error {
	for {
		msgType, data, err := b.conn.ReadMessage()
		if err != nil {
			return err
		}
		if msgType != websocket.BinaryMessage {
			continue
		}
		if err := b.handle(data); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			b.logger.Error().Err(err).Msg("handler error")
		}
	}
}

func (b *complexBot) handle(data []byte) error {
	if b.tryHandStart(data) || b.tryGameUpdate(data) || b.tryPlayerAction(data) || b.tryStreetChange(data) || b.tryHandResult(data) {
		return nil
	}

	var req protocol.ActionRequest
	if err := protocol.Unmarshal(data, &req); err == nil && req.Type == protocol.TypeActionRequest {
		return b.respond(req)
	}

	if handled, err := b.tryGameCompleted(data); handled {
		return err
	}
	return nil
}

func (b *complexBot) tryHandStart(data []byte) bool {
	var start protocol.HandStart
	if err := protocol.Unmarshal(data, &start); err != nil || start.Type != protocol.TypeHandStart {
		return false
	}
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

	b.logger.Info().
		Strs("holes", start.HoleCards).
		Int("position", b.getPosition()).
		Msg("hand start")
	return true
}

func (b *complexBot) tryGameUpdate(data []byte) bool {
	var update protocol.GameUpdate
	if err := protocol.Unmarshal(data, &update); err != nil || update.Type != protocol.TypeGameUpdate {
		return false
	}
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
	return true
}

func (b *complexBot) tryPlayerAction(data []byte) bool {
	var action protocol.PlayerAction
	if err := protocol.Unmarshal(data, &action); err != nil || action.Type != protocol.TypePlayerAction {
		return false
	}
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
	return true
}

func (b *complexBot) tryStreetChange(data []byte) bool {
	var street protocol.StreetChange
	if err := protocol.Unmarshal(data, &street); err != nil || street.Type != protocol.TypeStreetChange {
		return false
	}
	b.state.Street = street.Street
	b.state.Board = street.Board
	return true
}

func (b *complexBot) tryHandResult(data []byte) bool {
	var result protocol.HandResult
	if err := protocol.Unmarshal(data, &result); err != nil || result.Type != protocol.TypeHandResult {
		return false
	}

	// Calculate net result for this hand
	netChips := b.state.Chips - b.state.StartingChips
	netBB := float64(netChips) / float64(b.bigBlind)

	// Check if we won
	won := false
	wonAtShowdown := false
	for _, winner := range result.Winners {
		if winner.Name == b.id {
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

	b.logger.Info().
		Float64("net_bb", netBB).
		Bool("won", won).
		Bool("showdown", len(result.Showdown) > 0).
		Str("street", finalStreet).
		Msg("hand completed")

	return true
}

func (b *complexBot) tryGameCompleted(data []byte) (bool, error) {
	var completed protocol.GameCompleted
	if err := protocol.Unmarshal(data, &completed); err != nil || completed.Type != protocol.TypeGameCompleted {
		return false, nil
	}

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
				Float64("bb_per_100", ps.AvgPerHand*100).
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

	return true, io.EOF
}

func (b *complexBot) respond(req protocol.ActionRequest) error {
	// Calculate hand strength and make strategic decision
	handStrength := b.evaluateHandStrength()
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

	act := protocol.Action{
		Type:   protocol.TypeAction,
		Action: action,
		Amount: amount,
	}

	payload, err := protocol.Marshal(&act)
	if err != nil {
		return err
	}
	return b.conn.WriteMessage(websocket.BinaryMessage, payload)
}

func (b *complexBot) evaluateHandStrength() float64 {
	if len(b.state.HoleCards) != 2 {
		return 0.5
	}

	// Parse hole cards
	h1, h2 := b.state.HoleCards[0], b.state.HoleCards[1]
	r1, r2 := cardRank(h1), cardRank(h2)
	suited := h1[1] == h2[1]

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
		br := cardRank(boardCard)
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
	// Calculate required equity based on pot odds
	requiredEquity := 1.0 / (potOdds + 1.0)

	// Position-based adjustments
	positionBonus := 0.0
	if position <= 1 { // Late position
		positionBonus = 0.1
	} else if position >= 3 { // Early position
		positionBonus = -0.1
	}

	adjustedStrength := handStrength + positionBonus

	// Check if we can check
	canCheck := false
	for _, action := range req.ValidActions {
		if action == "check" {
			canCheck = true
			break
		}
	}

	// Decision logic
	if adjustedStrength > 0.85 || (adjustedStrength > 0.7 && b.state.Street != "preflop") {
		// Very strong hand - raise or go all in
		for _, action := range req.ValidActions {
			if action == "raise" {
				// Size bet based on pot
				betSize := req.Pot
				if b.state.Street == "river" {
					betSize = int(float64(req.Pot) * 1.5)
				}
				if betSize < req.MinBet {
					betSize = req.MinBet
				}
				if betSize > b.state.Chips {
					betSize = b.state.Chips
				}
				return "raise", betSize
			}
		}
		for _, action := range req.ValidActions {
			if action == "allin" {
				return "allin", 0
			}
		}
	}

	if adjustedStrength > requiredEquity {
		// Good odds to continue
		if canCheck {
			// Mix between checking and betting
			if b.rng.Float64() < 0.3 && adjustedStrength > 0.6 {
				// Sometimes bet with good hands
				for _, action := range req.ValidActions {
					if action == "raise" {
						betSize := req.Pot / 2
						if betSize < req.MinBet {
							betSize = req.MinBet
						}
						return "raise", betSize
					}
				}
			}
			return "check", 0
		}

		// Call if we can't check
		for _, action := range req.ValidActions {
			if action == "call" {
				return "call", 0
			}
		}
	}

	// Bluff occasionally in position
	if position <= 1 && canCheck && b.state.ActiveCount <= 3 && b.rng.Float64() < 0.15 {
		for _, action := range req.ValidActions {
			if action == "raise" {
				bluffSize := req.Pot / 3
				if bluffSize < req.MinBet {
					bluffSize = req.MinBet
				}
				if bluffSize <= b.state.Chips/4 { // Don't bluff more than 25% of stack
					return "raise", bluffSize
				}
			}
		}
	}

	// Weak hand - check or fold
	if canCheck {
		return "check", 0
	}

	// Only call very small bets with marginal hands
	if req.ToCall > 0 && float64(req.ToCall) < float64(req.Pot)*0.1 && adjustedStrength > 0.3 {
		for _, action := range req.ValidActions {
			if action == "call" {
				return "call", 0
			}
		}
	}

	return "fold", 0
}

func (b *complexBot) getOrCreateProfile(name string) *opponentProfile {
	if prof, ok := b.opponents[name]; ok {
		return prof
	}
	prof := &opponentProfile{Name: name}
	b.opponents[name] = prof
	return prof
}

func cardRank(card string) int {
	if len(card) < 1 {
		return 0
	}
	switch card[0] {
	case '2':
		return 2
	case '3':
		return 3
	case '4':
		return 4
	case '5':
		return 5
	case '6':
		return 6
	case '7':
		return 7
	case '8':
		return 8
	case '9':
		return 9
	case 'T':
		return 10
	case 'J':
		return 11
	case 'Q':
		return 12
	case 'K':
		return 13
	case 'A':
		return 14
	default:
		return 0
	}
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
	r1, r2 := cardRank(h1), cardRank(h2)
	suited := h1[1] == h2[1]

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

	bot := newComplexBot(logger)
	if err := bot.connect(*serverURL); err != nil {
		logger.Fatal().Err(err).Msg("connect failed")
	}
	logger.Info().Msg("complex bot connected")

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	runErr := make(chan error, 1)
	go func() { runErr <- bot.run() }()

	select {
	case <-interrupt:
		logger.Info().Msg("shutting down")
	case err := <-runErr:
		if err != nil {
			logger.Error().Err(err).Msg("run error")
		}
	}
}
