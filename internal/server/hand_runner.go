package server

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/lox/pokerforbots/internal/game"
	"github.com/lox/pokerforbots/internal/protocol"
	"github.com/rs/zerolog"
)

const (
	// Default decision timeout for bot actions
	defaultDecisionTimeout = 100 * time.Millisecond

	// Default blind amounts
	defaultSmallBlind = 5
	defaultBigBlind   = 10
	defaultStartChips = 1000
)

// HandRunner manages the execution of a single poker hand
type HandRunner struct {
	bots          []*Bot
	handState     *game.HandState
	button        int
	handID        string
	actions       chan BotAction
	botActionChan chan ActionEnvelope // Channel to receive actions from bots with ID verification
	seatBuyIns    []int               // Track actual buy-in per seat for accurate P&L
	playerLabels  []string
	lastStreet    game.Street
	logger        zerolog.Logger
	rng           *rand.Rand
	pool          *BotPool // Reference to pool for metrics
	config        Config   // Server configuration
}

// ActionEnvelope wraps an action with the sender's bot ID for verification
type ActionEnvelope struct {
	BotID  string
	Action protocol.Action
}

// BotAction represents an action from a bot
type BotAction struct {
	botIndex int
	action   protocol.Action
}

type winnerSummary struct {
	seat   int
	name   string
	amount int
}

// NewHandRunner creates a new hand runner with default config
func NewHandRunner(logger zerolog.Logger, bots []*Bot, handID string, button int, rng *rand.Rand) *HandRunner {
	config := Config{
		SmallBlind: defaultSmallBlind,
		BigBlind:   defaultBigBlind,
		StartChips: defaultStartChips,
		Timeout:    defaultDecisionTimeout,
	}
	return NewHandRunnerWithConfig(logger, bots, handID, button, rng, config)
}

// NewHandRunnerWithConfig creates a new hand runner with config
func NewHandRunnerWithConfig(logger zerolog.Logger, bots []*Bot, handID string, button int, rng *rand.Rand, config Config) *HandRunner {
	actionChan := make(chan ActionEnvelope, len(bots))

	// Set action channel for all bots
	for _, bot := range bots {
		bot.SetActionChannel(actionChan)
	}

	return &HandRunner{
		bots:          bots,
		handID:        handID,
		button:        button,
		actions:       make(chan BotAction, 1),
		botActionChan: actionChan,
		lastStreet:    game.Preflop,
		logger:        logger.With().Str("component", "hand_runner").Str("hand_id", handID).Logger(),
		rng:           rng,
		config:        config,
	}
}

// SetPool sets the pool reference for metrics tracking
func (hr *HandRunner) SetPool(pool *BotPool) {
	hr.pool = pool
}

// Run executes the hand
func (hr *HandRunner) Run() {
	startTime := time.Now()
	hr.logger.Info().Int("player_count", len(hr.bots)).Msg("Hand starting")

	// Create player names and get buy-ins from bots
	playerNames := make([]string, len(hr.bots))
	chipCounts := make([]int, len(hr.bots))
	hr.playerLabels = make([]string, len(hr.bots))
	for i, bot := range hr.bots {
		// Use first 8 chars of ID as name, or full ID if shorter
		if len(bot.ID) >= 8 {
			playerNames[i] = bot.ID[:8]
		} else {
			playerNames[i] = bot.ID
		}
		hr.playerLabels[i] = playerNames[i]
		// Get bot's buy-in (capped at table starting stack)
		chipCounts[i] = bot.GetBuyIn()
	}

	// Initialize hand state with individual chip counts and deterministic deck
	// Clone the RNG to avoid concurrent access issues
	deckRNG := rand.New(rand.NewSource(hr.rng.Int63()))
	deck := game.NewDeck(deckRNG)
	hr.handState = game.NewHandStateWithChipsAndDeck(
		playerNames,
		chipCounts,
		hr.button,
		hr.config.SmallBlind,
		hr.config.BigBlind,
		deck,
	)
	hr.lastStreet = hr.handState.Street

	// Store the actual buy-ins for P&L calculation later
	hr.seatBuyIns = chipCounts

	// Send hand start messages
	hr.broadcastHandStart()

	// Broadcast blind posts
	hr.broadcastBlindPosts()

	// Run betting rounds until hand is complete
	for !hr.handState.IsComplete() {
		if hr.foldDisconnectedPlayers(-1) {
			// State changed (street may have advanced); re-evaluate hand completion
			if hr.handState.IsComplete() {
				break
			}
			continue
		}
		// Get current player
		activePlayer := hr.handState.ActivePlayer
		if activePlayer == -1 {
			hr.logger.Info().Msg("No active players, ending hand")
			break // No active players
		}

		// Get valid actions and verify they exist
		validActions := hr.handState.GetValidActions()
		if len(validActions) == 0 {
			hr.logger.Warn().Int("player", activePlayer).Msg("No valid actions for player")
			break // Invalid state, end hand
		}

		// Convert actions to strings for logging
		actionStrs := make([]string, len(validActions))
		for i, a := range validActions {
			actionStrs[i] = a.String()
		}
		streetName := hr.handState.Street.String()
		toCall := hr.handState.CurrentBet - hr.handState.Players[activePlayer].Bet
		hr.logger.Debug().
			Int("seat", activePlayer).
			Str("bot", hr.playerLabels[activePlayer]).
			Str("street", streetName).
			Strs("valid_actions", actionStrs).
			Int("to_call", toCall).
			Msg("Player to act")

		// Send action request to active bot
		bot := hr.bots[activePlayer]
		if hr.foldDisconnectedPlayers(activePlayer) {
			// Active player disconnected before acting, loop to pick next player
			continue
		}
		if err := hr.sendActionRequest(bot, activePlayer, validActions); err != nil {
			hr.logger.Error().Err(err).Msg("Failed to send action request")
			executed := hr.processAction(activePlayer, game.Fold, 0)
			hr.logPlayerAction(activePlayer, streetName, executed, 0, toCall)
			continue
		}

		// Wait for action with timeout or disconnect
		action, amount := hr.waitForAction(activePlayer)

		// Process the action and record outcome
		executed := hr.processAction(activePlayer, action, amount)
		hr.logPlayerAction(activePlayer, streetName, executed, amount, toCall)

		// Broadcast game update
		hr.broadcastGameUpdate()

		// Check for street change
		if hr.handState.Street != hr.lastStreet {
			previousStreet := hr.lastStreet
			hr.broadcastStreetChange(previousStreet)
			hr.lastStreet = hr.handState.Street
		}
	}

	// Determine winners and distribute pots
	winners := hr.resolveHand()

	// Send hand result
	hr.broadcastHandResult(winners)

	// Log aggregated hand summary and update bankrolls
	hr.logHandSummary(winners)

	// Log hand completion time
	elapsed := time.Since(startTime)
	hr.logger.Info().
		Dur("duration_ms", elapsed).
		Msg("Hand completed")

	// Clean up action channels
	for _, bot := range hr.bots {
		bot.ClearActionChannel()
	}

}

// broadcastHandStart sends the initial hand information to all bots
func (hr *HandRunner) broadcastHandStart() {
	for i, bot := range hr.bots {
		player := hr.handState.Players[i]

		// Create player list for this bot's perspective
		players := make([]protocol.Player, len(hr.bots))
		for j, p := range hr.handState.Players {
			players[j] = protocol.Player{
				Name:  p.Name,
				Chips: p.Chips,
				Seat:  p.Seat,
			}
		}

		msg := &protocol.HandStart{
			Type:     "hand_start",
			HandID:   hr.handID,
			Players:  players,
			Button:   hr.button,
			YourSeat: i,
			HoleCards: []string{
				game.CardString(player.HoleCards.GetCard(0)),
				game.CardString(player.HoleCards.GetCard(1)),
			},
			SmallBlind: hr.config.SmallBlind,
			BigBlind:   hr.config.BigBlind,
		}

		if err := bot.SendMessage(msg); err != nil {
			hr.logger.Error().Err(err).Str("bot_id", bot.ID).Msg("Failed to send hand start")
		}
	}
}

// sendActionRequest sends an action request to the active bot
func (hr *HandRunner) sendActionRequest(bot *Bot, seat int, validActions []game.Action) error {
	// Convert game actions to protocol actions
	actions := make([]string, len(validActions))
	for i, a := range validActions {
		actions[i] = a.String()
	}

	// Calculate pot and amounts to call
	pot := 0
	for _, p := range hr.handState.Pots {
		pot += p.Amount
	}

	toCall := hr.handState.CurrentBet - hr.handState.Players[seat].Bet

	msg := &protocol.ActionRequest{
		Type:          "action_request",
		HandID:        hr.handID,
		Pot:           pot,
		ToCall:        toCall,
		MinBet:        hr.handState.CurrentBet + hr.handState.MinRaise,
		MinRaise:      hr.handState.MinRaise,
		ValidActions:  actions,
		TimeRemaining: int(hr.config.Timeout.Milliseconds()),
	}

	return bot.SendMessage(msg)
}

// waitForAction waits for a bot to send an action or times out
func (hr *HandRunner) waitForAction(botIndex int) (game.Action, int) {
	// Create a channel to signal when we're done
	done := make(chan struct{})
	defer close(done)

	timer := time.NewTimer(hr.config.Timeout)
	defer timer.Stop()

	// Start goroutine to listen for action
	go hr.listenForAction(botIndex, done)

	select {
	case action := <-hr.actions:
		if action.botIndex == botIndex {
			return hr.convertAction(action.action)
		}
		// Wrong bot sent action, auto-fold
		return game.Fold, 0

	case <-hr.bots[botIndex].Done():
		hr.logger.Warn().Str("bot_id", hr.bots[botIndex].ID).Msg("Bot disconnected during action window")
		return game.Fold, 0

	case <-timer.C:
		// Timeout - auto fold
		hr.logger.Warn().Str("bot_id", hr.bots[botIndex].ID).Msg("Bot timed out")
		if hr.pool != nil {
			hr.pool.IncrementTimeoutCounter()
		}
		return game.Fold, 0
	}
}

// listenForAction listens for an action from a specific bot
func (hr *HandRunner) listenForAction(botIndex int, done <-chan struct{}) {
	expectedBotID := hr.bots[botIndex].ID
	timeout := time.After(hr.config.Timeout)

	// Keep draining the channel until we get the right bot's action or timeout
	for {
		select {
		case envelope := <-hr.botActionChan:
			// Verify this action is from the expected bot
			if envelope.BotID == expectedBotID {
				// Correct bot - forward the action
				select {
				case hr.actions <- BotAction{
					botIndex: botIndex,
					action:   envelope.Action,
				}:
					return // Successfully sent action
				case <-done:
					// Parent function has returned, stop
					return
				}
			} else {
				// Wrong bot sent action - log and ignore
				hr.logger.Warn().
					Str("sender_bot_id", envelope.BotID).
					Str("expected_bot_id", expectedBotID).
					Msg("SECURITY: Bot sent action during another bot's turn - REJECTED")
				// Continue draining to prevent channel poisoning
				continue
			}

		case <-timeout:
			// Timeout - send fold action
			select {
			case hr.actions <- BotAction{
				botIndex: botIndex,
				action: protocol.Action{
					Type:   "action",
					Action: "fold",
				},
			}:
				return
			case <-done:
				// Parent function has returned, stop
				return
			}

		case <-done:
			// Parent function has timed out or completed
			return
		}
	}
}

// convertAction converts a protocol action to a game action
func (hr *HandRunner) convertAction(action protocol.Action) (game.Action, int) {
	switch action.Action {
	case "fold":
		return game.Fold, 0
	case "check":
		return game.Check, 0
	case "call":
		return game.Call, 0
	case "raise":
		return game.Raise, action.Amount
	case "allin":
		return game.AllIn, 0
	default:
		return game.Fold, 0 // Invalid action = fold
	}
}

// processAction processes a bot's action and broadcasts it
func (hr *HandRunner) processAction(botIndex int, action game.Action, amount int) game.Action {
	// Track the player's bet before the action
	playerBetBefore := hr.handState.Players[botIndex].Bet

	if err := hr.handState.ProcessAction(action, amount); err != nil {
		hr.logger.Error().
			Err(err).
			Str("bot_id", hr.bots[botIndex].ID).
			Str("action", action.String()).
			Int("amount", amount).
			Int("seat", botIndex).
			Msg("Invalid action from bot - forcing fold")
		// Force fold on invalid action
		_ = hr.handState.ProcessAction(game.Fold, 0)

		// Broadcast the forced fold
		hr.broadcastPlayerAction(botIndex, "timeout_fold", 0)
		return game.Fold
	}

	// Calculate amount paid (difference in bet)
	playerBetAfter := hr.handState.Players[botIndex].Bet
	amountPaid := playerBetAfter - playerBetBefore

	// Map action to string for broadcast
	actionStr := action.String()
	if action == game.AllIn {
		actionStr = "allin"
	} else {
		actionStr = strings.ToLower(actionStr)
	}

	// Broadcast the player action
	hr.broadcastPlayerAction(botIndex, actionStr, amountPaid)

	return action
}

// foldDisconnectedPlayers scans for closed bot connections (excluding skipSeat) and force-folds them.
// Returns true if any folds occurred.
func (hr *HandRunner) foldDisconnectedPlayers(skipSeat int) bool {
	if hr.handState == nil {
		return false
	}
	changed := false
	for seat, bot := range hr.bots {
		if seat == skipSeat {
			continue
		}
		if bot.IsClosed() {
			if hr.forceFoldSeat(seat) {
				changed = true
			}
		}
	}
	return changed
}

// forceFoldSeat immediately folds the given seat and broadcasts state changes.
// Returns true if the player was folded.
func (hr *HandRunner) forceFoldSeat(seat int) bool {
	if hr.handState == nil || seat < 0 || seat >= len(hr.handState.Players) {
		return false
	}
	player := hr.handState.Players[seat]
	if player.Folded {
		return false
	}
	prevStreet := hr.handState.Street
	hr.logger.Warn().
		Int("seat", seat).
		Str("bot", hr.playerLabels[seat]).
		Msg("Bot disconnected - forcing fold")
	hr.handState.ForceFold(seat)
	hr.broadcastPlayerAction(seat, "timeout_fold", 0)
	hr.broadcastGameUpdate()
	if hr.handState.Street != prevStreet {
		hr.broadcastStreetChange(prevStreet)
		hr.lastStreet = hr.handState.Street
	}
	return true
}

// broadcastBlindPosts sends blind posting actions
func (hr *HandRunner) broadcastBlindPosts() {
	numPlayers := len(hr.handState.Players)
	var sbPos, bbPos int

	if numPlayers == 2 {
		// Heads-up: button posts small blind
		sbPos = hr.button
		bbPos = (hr.button + 1) % numPlayers
	} else {
		// Regular: button+1 posts small blind, button+2 posts big blind
		sbPos = (hr.button + 1) % numPlayers
		bbPos = (hr.button + 2) % numPlayers
	}

	// Broadcast small blind post
	sbPlayer := hr.handState.Players[sbPos]
	hr.broadcastPlayerAction(sbPos, "post_small_blind", sbPlayer.Bet)

	// Broadcast big blind post
	bbPlayer := hr.handState.Players[bbPos]
	hr.broadcastPlayerAction(bbPos, "post_big_blind", bbPlayer.Bet)
}

// broadcastPlayerAction sends detailed action information to all bots
func (hr *HandRunner) broadcastPlayerAction(seat int, action string, amountPaid int) {
	player := hr.handState.Players[seat]
	pot := hr.totalPot()

	msg := &protocol.PlayerAction{
		Type:        "player_action",
		HandID:      hr.handID,
		Street:      hr.handState.Street.String(),
		Seat:        seat,
		PlayerName:  player.Name,
		Action:      action,
		AmountPaid:  amountPaid,
		PlayerBet:   player.Bet,
		PlayerChips: player.Chips,
		Pot:         pot,
	}

	for _, bot := range hr.bots {
		if err := bot.SendMessage(msg); err != nil {
			hr.logger.Error().Err(err).Str("bot_id", bot.ID).Msg("Failed to send player action")
		}
	}
}

// broadcastGameUpdate sends game state updates to all bots
func (hr *HandRunner) broadcastGameUpdate() {
	for _, bot := range hr.bots {
		// Get pot total
		pot := 0
		for _, p := range hr.handState.Pots {
			pot += p.Amount
		}

		// Create player states
		players := make([]protocol.Player, len(hr.handState.Players))
		for i, p := range hr.handState.Players {
			players[i] = protocol.Player{
				Name:   p.Name,
				Chips:  p.Chips,
				Bet:    p.Bet,
				Folded: p.Folded,
				AllIn:  p.AllInFlag,
			}
		}

		msg := &protocol.GameUpdate{
			Type:    "game_update",
			HandID:  hr.handID,
			Pot:     pot,
			Players: players,
		}

		if err := bot.SendMessage(msg); err != nil {
			hr.logger.Error().Err(err).Str("bot_id", bot.ID).Msg("Failed to send game update")
		}
	}
}

func (hr *HandRunner) boardStrings() []string {
	boardCards := make([]string, 0, hr.handState.Board.CountCards())
	for i := 0; i < hr.handState.Board.CountCards(); i++ {
		card := hr.handState.Board.GetCard(i)
		if card != 0 {
			boardCards = append(boardCards, game.CardString(card))
		}
	}
	return boardCards
}

func (hr *HandRunner) totalPot() int {
	total := 0
	for _, pot := range hr.handState.Pots {
		total += pot.Amount
	}
	return total
}

func (hr *HandRunner) logPlayerAction(seat int, street string, action game.Action, declaredAmount int, toCall int) {
	player := hr.handState.Players[seat]
	hr.logger.Info().
		Int("seat", seat).
		Str("bot", hr.playerLabels[seat]).
		Str("street", street).
		Str("action", action.String()).
		Int("declared_amount", declaredAmount).
		Int("to_call", toCall).
		Int("pot", hr.totalPot()).
		Int("chips", player.Chips).
		Int("bet", player.Bet).
		Int("total_bet", player.TotalBet).
		Msg("Player action")
}

func (hr *HandRunner) logHandSummary(winners []winnerSummary) {
	boardCards := hr.boardStrings()
	totalPot := hr.totalPot()

	initialStacks := make([]string, len(hr.seatBuyIns))
	finalStacks := make([]string, len(hr.seatBuyIns))
	pnlSummary := make([]string, len(hr.seatBuyIns))

	for i := range hr.bots {
		finalChips := hr.handState.Players[i].Chips
		delta := finalChips - hr.seatBuyIns[i]
		label := hr.playerLabels[i]
		initialStacks[i] = fmt.Sprintf("seat%d/%s/%d", i, label, hr.seatBuyIns[i])
		finalStacks[i] = fmt.Sprintf("seat%d/%s/%d", i, label, finalChips)
		pnlSummary[i] = fmt.Sprintf("seat%d/%s/%+d", i, label, delta)
		hr.bots[i].ApplyResult(delta)
	}

	winnerSummaries := make([]string, len(winners))
	for i, winner := range winners {
		label := hr.playerLabels[winner.seat]
		winnerSummaries[i] = fmt.Sprintf("seat%d/%s/%d", winner.seat, label, winner.amount)
	}

	hr.logger.Info().
		Int("player_count", len(hr.bots)).
		Int("button_seat", hr.button).
		Int("pot_final", totalPot).
		Strs("board", boardCards).
		Strs("initial_stacks", initialStacks).
		Strs("final_stacks", finalStacks).
		Strs("winners", winnerSummaries).
		Strs("pnls", pnlSummary).
		Msg("Hand summary")
}

// broadcastStreetChange sends street change notification
func (hr *HandRunner) broadcastStreetChange(previous game.Street) {
	hr.broadcastSpecificStreet(previous, hr.handState.Street, hr.boardStrings())
}

// resolveHand determines winners, distributes pots, and returns payout summaries
func (hr *HandRunner) resolveHand() []winnerSummary {
	// Force showdown if needed
	if hr.handState.Street != game.Showdown {
		// If everyone is all-in, fast-forward streets
		if hr.handState.ActivePlayer == -1 {
			for hr.handState.Street != game.Showdown {
				prev := hr.handState.Street
				hr.handState.NextStreet()
				if hr.handState.Street == game.Showdown {
					hr.broadcastRemainingStreets(prev)
				} else {
					hr.broadcastStreetChange(prev)
					hr.lastStreet = hr.handState.Street
				}
			}
		} else {
			// Deal remaining cards by checking
			for hr.handState.Street != game.Showdown {
				hr.handState.ProcessAction(game.Check, 0)
			}
		}
	}

	// Get winners for each pot and accumulate payouts per seat
	payouts := make(map[int]int)
	winners := hr.handState.GetWinners()

	for potIdx, winnerSeats := range winners {
		if len(winnerSeats) == 0 {
			continue
		}

		pot := hr.handState.Pots[potIdx]
		share := pot.Amount / len(winnerSeats)

		for _, seat := range winnerSeats {
			hr.handState.Players[seat].Chips += share
			payouts[seat] += share
		}
	}

	summaries := make([]winnerSummary, 0, len(payouts))
	for seat, amount := range payouts {
		player := hr.handState.Players[seat]
		summaries = append(summaries, winnerSummary{
			seat:   seat,
			name:   player.Name,
			amount: amount,
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].seat < summaries[j].seat
	})

	return summaries
}

func (hr *HandRunner) broadcastSpecificStreet(previous, current game.Street, board []string) {
	hr.logger.Debug().
		Str("from", previous.String()).
		Str("to", current.String()).
		Strs("board", board).
		Msg("Street advanced")

	for _, bot := range hr.bots {
		msg := &protocol.StreetChange{
			Type:   "street_change",
			HandID: hr.handID,
			Street: current.String(),
			Board:  board,
		}

		if err := bot.SendMessage(msg); err != nil {
			hr.logger.Error().Err(err).Str("bot_id", bot.ID).Msg("Failed to send street change")
		}
	}
}

func (hr *HandRunner) broadcastRemainingStreets(from game.Street) {
	board := hr.boardStrings()
	order := []struct {
		street game.Street
		needed int
	}{
		{game.Flop, 3},
		{game.Turn, 4},
		{game.River, 5},
	}
	prev := from
	for _, step := range order {
		if streetOrder(step.street) <= streetOrder(prev) {
			continue
		}
		if len(board) < step.needed {
			continue
		}
		hr.broadcastSpecificStreet(prev, step.street, board[:step.needed])
		prev = step.street
	}
	hr.lastStreet = prev
	if hr.handState.Street == game.Showdown {
		hr.lastStreet = game.Showdown
	}
}

func streetOrder(s game.Street) int {
	switch s {
	case game.Preflop:
		return 0
	case game.Flop:
		return 1
	case game.Turn:
		return 2
	case game.River:
		return 3
	case game.Showdown:
		return 4
	default:
		return -1
	}
}

// broadcastHandResult sends the final hand result with showdown details
func (hr *HandRunner) broadcastHandResult(winners []winnerSummary) {
	// Evaluate all hands for showdown info
	boardCards := hr.boardStrings()

	// Prepare winner info with hole cards and hand ranks
	winnerInfo := make([]protocol.Winner, len(winners))
	winnerSeats := make(map[int]bool)
	for i, winner := range winners {
		player := hr.handState.Players[winner.seat]
		holeCards := []string{
			game.CardString(player.HoleCards.GetCard(0)),
			game.CardString(player.HoleCards.GetCard(1)),
		}

		// Evaluate the hand
		fullHand := player.HoleCards | hr.handState.Board
		handRank := game.Evaluate7Cards(fullHand)

		winnerInfo[i] = protocol.Winner{
			Name:      winner.name,
			Amount:    winner.amount,
			HoleCards: holeCards,
			HandRank:  handRank.String(),
		}
		winnerSeats[winner.seat] = true
	}

	// Collect showdown hands from non-winners who made it to showdown
	var showdownHands []protocol.ShowdownHand
	if hr.handState.Street == game.Showdown {
		for _, player := range hr.handState.Players {
			// Skip if folded, winner, or no hole cards
			if player.Folded || winnerSeats[player.Seat] || player.HoleCards == 0 {
				continue
			}

			// Only include if player was still in at showdown (not folded)
			holeCards := []string{
				game.CardString(player.HoleCards.GetCard(0)),
				game.CardString(player.HoleCards.GetCard(1)),
			}

			fullHand := player.HoleCards | hr.handState.Board
			handRank := game.Evaluate7Cards(fullHand)

			showdownHands = append(showdownHands, protocol.ShowdownHand{
				Name:      player.Name,
				HoleCards: holeCards,
				HandRank:  handRank.String(),
			})
		}
	}

	for _, bot := range hr.bots {
		msg := &protocol.HandResult{
			Type:     "hand_result",
			HandID:   hr.handID,
			Winners:  winnerInfo,
			Board:    boardCards,
			Showdown: showdownHands,
		}

		if err := bot.SendMessage(msg); err != nil {
			hr.logger.Error().Err(err).Str("bot_id", bot.ID).Msg("Failed to send hand result")
		}
	}
}

// GetHandState returns the current hand state (for testing)
func (hr *HandRunner) GetHandState() *game.HandState {
	return hr.handState
}
