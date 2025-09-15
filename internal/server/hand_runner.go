package server

import (
	"log"
	"sync"
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
	wg            sync.WaitGroup
	botActionChan chan protocol.Action // Channel to receive actions from bots
}

// BotAction represents an action from a bot
type BotAction struct {
	botIndex int
	action   protocol.Action
}

// NewHandRunner creates a new hand runner
func NewHandRunner(bots []*Bot, handID string, button int) *HandRunner {
	actionChan := make(chan protocol.Action, len(bots))

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

	// Create player names from bot IDs
	playerNames := make([]string, len(hr.bots))
	for i, bot := range hr.bots {
		// Use first 8 chars of ID as name, or full ID if shorter
		if len(bot.ID) >= 8 {
			playerNames[i] = bot.ID[:8]
		} else {
			playerNames[i] = bot.ID
		}
	}

	// Initialize hand state
	hr.handState = game.NewHandState(
		playerNames,
		hr.button,
		defaultSmallBlind,
		defaultBigBlind,
		defaultStartChips,
	)

	// Send hand start messages
	hr.broadcastHandStart()

	// Run betting rounds until hand is complete
	for !hr.handState.IsComplete() {
		// Get current player
		activePlayer := hr.handState.ActivePlayer
		if activePlayer == -1 {
			break // No active players
		}

		// Send action request to active bot
		bot := hr.bots[activePlayer]
		validActions := hr.handState.GetValidActions()

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
	timer := time.NewTimer(decisionTimeout)
	defer timer.Stop()

	// Start goroutine to listen for action
	go hr.listenForAction(botIndex)

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
func (hr *HandRunner) listenForAction(botIndex int) {
	// Listen for actions from the bot action channel
	for {
		select {
		case action := <-hr.botActionChan:
			// We received an action, but need to verify it's from the right bot
			// For now, we accept any action since we can't easily identify which bot sent it
			// In a production system, we'd include bot ID in the action
			hr.actions <- BotAction{
				botIndex: botIndex,
				action:   action,
			}
			return
		case <-time.After(100 * time.Millisecond):
			// Timeout - send fold action
			hr.actions <- BotAction{
				botIndex: botIndex,
				action: protocol.Action{
					Type:   "action",
					Action: "fold",
				},
			}
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
		// Deal remaining cards
		for hr.handState.Street != game.Showdown {
			hr.handState.ProcessAction(game.Check, 0)
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
