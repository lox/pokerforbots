package server

import (
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/lox/pokerforbots/internal/game"
	"github.com/lox/pokerforbots/internal/protocol"
	"github.com/rs/zerolog"
)

const (
	// Decision timeout for bot actions
	decisionTimeout = 100 * time.Millisecond

	// Small blind and big blind amounts
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

// NewHandRunner creates a new hand runner
func NewHandRunner(logger zerolog.Logger, bots []*Bot, handID string, button int, rng *rand.Rand) *HandRunner {
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
	}
}

// Run executes the hand
func (hr *HandRunner) Run() {
	hr.logger.Debug().Int("player_count", len(hr.bots)).Msg("Hand starting")

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
		// Get bot's buy-in (capped at 100)
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
		defaultSmallBlind,
		defaultBigBlind,
		deck,
	)
	hr.lastStreet = hr.handState.Street

	// Store the actual buy-ins for P&L calculation later
	hr.seatBuyIns = chipCounts

	// Send hand start messages
	hr.broadcastHandStart()

	// Run betting rounds until hand is complete
	for !hr.handState.IsComplete() {
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
		if err := hr.sendActionRequest(bot, activePlayer, validActions); err != nil {
			hr.logger.Error().Err(err).Msg("Failed to send action request")
			executed := hr.processAction(activePlayer, game.Fold, 0)
			hr.logPlayerAction(activePlayer, streetName, executed, 0, toCall)
			continue
		}

		// Wait for action with timeout
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
			SmallBlind: defaultSmallBlind,
			BigBlind:   defaultBigBlind,
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
		MinRaise:      hr.handState.MinRaise,
		ValidActions:  actions,
		TimeRemaining: int(decisionTimeout.Milliseconds()),
	}

	return bot.SendMessage(msg)
}

// waitForAction waits for a bot to send an action or times out
func (hr *HandRunner) waitForAction(botIndex int) (game.Action, int) {
	// Create a channel to signal when we're done
	done := make(chan struct{})
	defer close(done)

	timer := time.NewTimer(decisionTimeout)
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

	case <-timer.C:
		// Timeout - auto fold
		hr.logger.Warn().Str("bot_id", hr.bots[botIndex].ID).Msg("Bot timed out")
		return game.Fold, 0
	}
}

// listenForAction listens for an action from a specific bot
func (hr *HandRunner) listenForAction(botIndex int, done <-chan struct{}) {
	expectedBotID := hr.bots[botIndex].ID
	timeout := time.After(decisionTimeout)

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

// processAction processes a bot's action
func (hr *HandRunner) processAction(botIndex int, action game.Action, amount int) game.Action {
	if err := hr.handState.ProcessAction(action, amount); err != nil {
		hr.logger.Error().Err(err).Str("bot_id", hr.bots[botIndex].ID).Msg("Invalid action from bot")
		// Force fold on invalid action
		_ = hr.handState.ProcessAction(game.Fold, 0)
		return game.Fold
	}
	return action
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
	hr.logger.Debug().
		Int("seat", seat).
		Str("bot", hr.playerLabels[seat]).
		Str("street", street).
		Str("action", action.String()).
		Int("declared_amount", declaredAmount).
		Int("to_call", toCall).
		Int("pot", hr.totalPot()).
		Msg("Action resolved")
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
	boardCards := hr.boardStrings()
	hr.logger.Debug().
		Str("from", previous.String()).
		Str("to", hr.handState.Street.String()).
		Strs("board", boardCards).
		Msg("Street advanced")

	for _, bot := range hr.bots {
		msg := &protocol.StreetChange{
			Type:   "street_change",
			HandID: hr.handID,
			Street: hr.handState.Street.String(),
			Board:  boardCards,
		}

		if err := bot.SendMessage(msg); err != nil {
			hr.logger.Error().Err(err).Str("bot_id", bot.ID).Msg("Failed to send street change")
		}
	}
}

// resolveHand determines winners, distributes pots, and returns payout summaries
func (hr *HandRunner) resolveHand() []winnerSummary {
	// Force showdown if needed
	if hr.handState.Street != game.Showdown {
		// If everyone is all-in, just advance to showdown
		if hr.handState.ActivePlayer == -1 {
			// Deal remaining cards directly
			for hr.handState.Street != game.Showdown {
				hr.handState.NextStreet()
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

// broadcastHandResult sends the final hand result
func (hr *HandRunner) broadcastHandResult(winners []winnerSummary) {
	winnerInfo := make([]protocol.Winner, len(winners))
	for i, winner := range winners {
		winnerInfo[i] = protocol.Winner{
			Name:   winner.name,
			Amount: winner.amount,
		}
	}

	boardCards := hr.boardStrings()

	for _, bot := range hr.bots {
		msg := &protocol.HandResult{
			Type:    "hand_result",
			HandID:  hr.handID,
			Winners: winnerInfo,
			Board:   boardCards,
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
