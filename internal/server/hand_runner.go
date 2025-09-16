package server

import (
	"log"
	"time"

	"github.com/lox/pokerforbots/internal/game"
	"github.com/lox/pokerforbots/internal/protocol"
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

// NewHandRunner creates a new hand runner
func NewHandRunner(bots []*Bot, handID string, button int) *HandRunner {
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
	}
}

// Run executes the hand
func (hr *HandRunner) Run() {
	log.Printf("Hand %s starting with %d players", hr.handID, len(hr.bots))

	// Create player names and get buy-ins from bots
	playerNames := make([]string, len(hr.bots))
	chipCounts := make([]int, len(hr.bots))
	for i, bot := range hr.bots {
		// Use first 8 chars of ID as name, or full ID if shorter
		if len(bot.ID) >= 8 {
			playerNames[i] = bot.ID[:8]
		} else {
			playerNames[i] = bot.ID
		}
		// Get bot's buy-in (capped at 100)
		chipCounts[i] = bot.GetBuyIn()
	}

	// Initialize hand state with individual chip counts
	hr.handState = game.NewHandStateWithChips(
		playerNames,
		chipCounts,
		hr.button,
		defaultSmallBlind,
		defaultBigBlind,
	)

	// Store the actual buy-ins for P&L calculation later
	hr.seatBuyIns = chipCounts

	// Send hand start messages
	hr.broadcastHandStart()

	// Run betting rounds until hand is complete
	for !hr.handState.IsComplete() {
		// Get current player
		activePlayer := hr.handState.ActivePlayer
		if activePlayer == -1 {
			log.Printf("Hand %s: No active players, ending", hr.handID)
			break // No active players
		}

		// Get valid actions and verify they exist
		validActions := hr.handState.GetValidActions()
		if len(validActions) == 0 {
			log.Printf("Warning: No valid actions for player %d in hand %s", activePlayer, hr.handID)
			break // Invalid state, end hand
		}

		log.Printf("Hand %s: Player %d to act, street %s, valid actions: %v",
			hr.handID, activePlayer, hr.handState.Street, validActions)

		// Send action request to active bot
		bot := hr.bots[activePlayer]
		if err := hr.sendActionRequest(bot, activePlayer, validActions); err != nil {
			log.Printf("Failed to send action request: %v", err)
			// Auto-fold on error
			hr.processAction(activePlayer, game.Fold, 0)
			continue
		}

		// Wait for action with timeout
		action, amount := hr.waitForAction(activePlayer)

		// Process the action
		hr.processAction(activePlayer, action, amount)

		// Broadcast game update
		hr.broadcastGameUpdate()

		// Check for street change
		if hr.handState.Street != hr.getPreviousStreet() {
			hr.broadcastStreetChange()
		}
	}

	// Determine winners and distribute pots
	hr.resolveHand()

	// Send hand result
	hr.broadcastHandResult()

	// Update bot bankrolls based on actual P&L (final chips - buy-in)
	for i, bot := range hr.bots {
		finalChips := hr.handState.Players[i].Chips
		delta := finalChips - hr.seatBuyIns[i]
		bot.ApplyResult(delta)
	}

	// Clean up action channels
	for _, bot := range hr.bots {
		bot.ClearActionChannel()
	}

	log.Printf("Hand %s completed", hr.handID)
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
			log.Printf("Failed to send hand start to bot %s: %v", bot.ID, err)
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
		log.Printf("Bot %s timed out", hr.bots[botIndex].ID)
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
				log.Printf("Bot %s sent action during %s's turn - ignoring", envelope.BotID, expectedBotID)
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
func (hr *HandRunner) processAction(botIndex int, action game.Action, amount int) {
	err := hr.handState.ProcessAction(action, amount)
	if err != nil {
		log.Printf("Invalid action from bot %s: %v", hr.bots[botIndex].ID, err)
		// Force fold on invalid action
		hr.handState.ProcessAction(game.Fold, 0)
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
			log.Printf("Failed to send game update to bot %s: %v", bot.ID, err)
		}
	}
}

// broadcastStreetChange sends street change notification
func (hr *HandRunner) broadcastStreetChange() {
	// Convert board cards to strings
	boardCards := []string{}
	for i := 0; i < hr.handState.Board.CountCards(); i++ {
		card := hr.handState.Board.GetCard(i)
		if card != 0 {
			boardCards = append(boardCards, game.CardString(card))
		}
	}

	for _, bot := range hr.bots {
		msg := &protocol.StreetChange{
			Type:   "street_change",
			HandID: hr.handID,
			Street: hr.handState.Street.String(),
			Board:  boardCards,
		}

		if err := bot.SendMessage(msg); err != nil {
			log.Printf("Failed to send street change to bot %s: %v", bot.ID, err)
		}
	}
}

// resolveHand determines winners and distributes pots
func (hr *HandRunner) resolveHand() {
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

	// Get winners for each pot
	winners := hr.handState.GetWinners()

	// Distribute pots (simplified - just track for display)
	for potIdx, winnerSeats := range winners {
		if len(winnerSeats) == 0 {
			continue
		}

		pot := hr.handState.Pots[potIdx]
		share := pot.Amount / len(winnerSeats)

		for _, seat := range winnerSeats {
			hr.handState.Players[seat].Chips += share
		}
	}
}

// broadcastHandResult sends the final hand result
func (hr *HandRunner) broadcastHandResult() {
	// Build winner information
	winners := hr.handState.GetWinners()
	winnerInfo := []protocol.Winner{}

	for potIdx, winnerSeats := range winners {
		if len(winnerSeats) == 0 {
			continue
		}

		pot := hr.handState.Pots[potIdx]
		share := pot.Amount / len(winnerSeats)

		for _, seat := range winnerSeats {
			player := hr.handState.Players[seat]
			winnerInfo = append(winnerInfo, protocol.Winner{
				Name:   player.Name,
				Amount: share,
			})
		}
	}

	// Convert board to strings
	boardCards := []string{}
	for i := 0; i < hr.handState.Board.CountCards(); i++ {
		card := hr.handState.Board.GetCard(i)
		if card != 0 {
			boardCards = append(boardCards, game.CardString(card))
		}
	}

	for _, bot := range hr.bots {
		msg := &protocol.HandResult{
			Type:    "hand_result",
			HandID:  hr.handID,
			Winners: winnerInfo,
			Board:   boardCards,
		}

		if err := bot.SendMessage(msg); err != nil {
			log.Printf("Failed to send hand result to bot %s: %v", bot.ID, err)
		}
	}
}

// getPreviousStreet returns the previous street (helper for detecting changes)
func (hr *HandRunner) getPreviousStreet() game.Street {
	switch hr.handState.Street {
	case game.Flop:
		return game.Preflop
	case game.Turn:
		return game.Flop
	case game.River:
		return game.Turn
	case game.Showdown:
		return game.River
	default:
		return game.Preflop
	}
}

// GetHandState returns the current hand state (for testing)
func (hr *HandRunner) GetHandState() *game.HandState {
	return hr.handState
}
