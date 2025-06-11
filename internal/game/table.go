package game

import (
	"fmt"
	"math/rand"
	"sort"

	"github.com/lox/pokerforbots/internal/deck"
	"github.com/lox/pokerforbots/internal/evaluator"
)

// TableConfig holds configuration for creating a table
type TableConfig struct {
	MaxSeats   int
	SmallBlind int
	BigBlind   int

	// Optional
	Seed              int64
	HandHistoryWriter HandHistoryWriter
}

// BettingRound represents the current betting round
type BettingRound int

const (
	PreFlop BettingRound = iota
	Flop
	Turn
	River
	Showdown
)

// String returns the string representation of a betting round
func (br BettingRound) String() string {
	switch br {
	case PreFlop:
		return "Pre-flop"
	case Flop:
		return "Flop"
	case Turn:
		return "Turn"
	case River:
		return "River"
	case Showdown:
		return "Showdown"
	default:
		return "Unknown"
	}
}

// GameState represents the overall state of the game
type GameState int

const (
	WaitingToStart GameState = iota
	InProgress
	HandComplete
	GameComplete
)

// String returns the string representation of a game state
func (gs GameState) String() string {
	switch gs {
	case WaitingToStart:
		return "Waiting to Start"
	case InProgress:
		return "In Progress"
	case HandComplete:
		return "Hand Complete"
	case GameComplete:
		return "Game Complete"
	default:
		return "Unknown"
	}
}

// Table represents a poker table
type Table struct {
	// Basic table info
	maxSeats   int // Maximum number of seats (6 or 9)
	smallBlind int // Small blind amount
	bigBlind   int // Big blind amount

	// Players and positions
	players        []*Player // All players at the table
	activePlayers  []*Player // Players currently in the hand
	dealerPosition int       // Current dealer button position (seat number)

	// Game state
	currentRound BettingRound // Current betting round
	state        GameState    // Overall game state
	handID       string       // Current hand ID

	// Cards
	deck           *deck.Deck  // The deck of cards
	communityCards []deck.Card // Community cards (flop, turn, river)

	// Betting
	pot        int // Main pot
	currentBet int // Current bet to call
	minRaise   int // Minimum raise amount (size of last raise, not always big blind)
	actionOn   int // Player index who needs to act

	// Hand tracking
	playersActed map[int]bool // Track which players have acted this round
	handHistory  *HandHistory // Current hand history
	seed         int64        // Original seed for reproduction

	// Dependencies
	rng               *rand.Rand        // Random number generator
	handHistoryWriter HandHistoryWriter // Writer for saving hand history
	eventBus          EventBus          // Event bus for publishing game events
}

// NewTable creates a new poker table with custom configuration
func NewTable(rng *rand.Rand, eventBus EventBus, config TableConfig) *Table {
	if config.HandHistoryWriter == nil {
		config.HandHistoryWriter = &NoOpHandHistoryWriter{}
	}

	return &Table{
		maxSeats:          config.MaxSeats,
		smallBlind:        config.SmallBlind,
		bigBlind:          config.BigBlind,
		players:           make([]*Player, 0, config.MaxSeats),
		activePlayers:     make([]*Player, 0, config.MaxSeats),
		dealerPosition:    -1, // Will be set randomly when first hand starts
		currentRound:      PreFlop,
		state:             WaitingToStart,
		handID:            "",
		deck:              deck.NewDeck(rng),
		communityCards:    make([]deck.Card, 0, 5),
		pot:               0,
		currentBet:        0,
		minRaise:          config.BigBlind,
		actionOn:          -1,
		playersActed:      make(map[int]bool),
		seed:              config.Seed,
		rng:               rng,
		handHistoryWriter: config.HandHistoryWriter,
		eventBus:          eventBus, // Table receives EventBus as dependency
	}
}

// GetEventBus returns the table's event bus for subscribing to events
func (t *Table) GetEventBus() EventBus {
	return t.eventBus
}

func (t *Table) BigBlind() int {
	return t.bigBlind
}

func (t *Table) SmallBlind() int {
	return t.smallBlind
}

func (t *Table) CurrentRound() BettingRound {
	return t.currentRound
}

func (t *Table) DealerPosition() int {
	return t.dealerPosition
}

func (t *Table) Pot() int {
	return t.pot
}

func (t *Table) CurrentBet() int {
	return t.currentBet
}

func (t *Table) MaxSeats() int {
	return t.maxSeats
}

func (t *Table) GetPlayers() []*Player {
	return t.players
}

func (t *Table) GetActivePlayers() []*Player {
	return t.activePlayers
}

// AddPlayer adds a player to the table
func (t *Table) AddPlayer(player *Player) bool {
	if len(t.players) >= t.maxSeats {
		return false // Table is full
	}

	// Check for duplicate player ID
	for _, p := range t.players {
		if p.ID == player.ID {
			return false // Player with this ID already exists
		}
	}

	// Find first available seat
	seatTaken := make(map[int]bool)
	for _, p := range t.players {
		seatTaken[p.SeatNumber] = true
	}

	for seat := 1; seat <= t.maxSeats; seat++ {
		if !seatTaken[seat] {
			player.SeatNumber = seat
			break
		}
	}

	t.players = append(t.players, player)

	// Sort players by seat number for consistent ordering
	sort.Slice(t.players, func(i, j int) bool {
		return t.players[i].SeatNumber < t.players[j].SeatNumber
	})

	return true
}

// StartNewHand starts a new hand
func (t *Table) StartNewHand() {
	if len(t.players) < 2 {
		return // Need at least 2 players
	}

	t.handID = GenerateGameID(t.rng)
	t.state = InProgress
	t.currentRound = PreFlop
	t.pot = 0
	t.currentBet = 0
	t.minRaise = t.bigBlind
	t.communityCards = t.communityCards[:0]
	t.playersActed = make(map[int]bool)

	// Reset all players for new hand
	t.activePlayers = make([]*Player, 0, len(t.players))
	for _, player := range t.players {
		if player.Chips > 0 { // Only include players with chips
			player.ResetForNewHand()
			t.activePlayers = append(t.activePlayers, player)
		}
	}

	// Set dealer position for the hand
	t.setDealerPosition()

	// Set positions
	t.setPositions()

	// Initialize hand history for this hand (after dealer position is set)
	t.handHistory = NewHandHistory(t, t.seed, t.handHistoryWriter)

	// Shuffle and deal
	t.deck.Reset()
	t.dealHoleCards()

	// Publish hand start event (before blind posting so TUI is ready)
	if t.eventBus != nil {
		handStartEvent := NewHandStartEvent(t.handID, t.players, t.activePlayers, t.smallBlind, t.bigBlind, 0)
		t.eventBus.Publish(handStartEvent)
	}

	// Post blinds
	t.postBlinds()

	// Set action on first player to act (UTG in heads-up, left of BB otherwise)
	t.setFirstToAct()
}

// setDealerPosition sets the dealer position for the hand
func (t *Table) setDealerPosition() {
	if len(t.activePlayers) < 2 {
		return
	}

	if t.dealerPosition == -1 {
		// First hand - set random starting position
		randomIndex := t.rng.Intn(len(t.activePlayers))
		t.dealerPosition = t.activePlayers[randomIndex].SeatNumber
	} else {
		// Move button to next active player
		t.moveButtonToNextPlayer()
	}
}

// moveButtonToNextPlayer moves the dealer button to the next active player
func (t *Table) moveButtonToNextPlayer() {
	if len(t.activePlayers) < 2 {
		return
	}

	// Find current dealer in active players
	currentDealerIndex := -1
	for i, player := range t.activePlayers {
		if player.SeatNumber == t.dealerPosition {
			currentDealerIndex = i
			break
		}
	}

	// If current dealer not found or not active, start from beginning
	if currentDealerIndex == -1 {
		t.dealerPosition = t.activePlayers[0].SeatNumber
		return
	}

	// Move to next active player
	nextIndex := (currentDealerIndex + 1) % len(t.activePlayers)
	t.dealerPosition = t.activePlayers[nextIndex].SeatNumber
}

// setPositions assigns positions to active players based on dealer button
func (t *Table) setPositions() {
	positions := calculatePositions(t.dealerPosition, t.activePlayers)

	// Apply the calculated positions
	for _, player := range t.activePlayers {
		if pos, exists := positions[player.SeatNumber]; exists {
			player.Position = pos
		}
	}
}

// calculatePositions is a pure function that determines poker positions
// This is extracted for easier testing
func calculatePositions(dealerSeat int, activePlayers []*Player) map[int]Position {
	positions := make(map[int]Position)
	numPlayers := len(activePlayers)

	if numPlayers < 2 {
		return positions
	}

	// Find dealer index in active players
	dealerIndex := -1
	for i, player := range activePlayers {
		if player.SeatNumber == dealerSeat {
			dealerIndex = i
			break
		}
	}

	// If dealer not found, use first player
	if dealerIndex == -1 {
		dealerIndex = 0
	}

	// Assign positions relative to dealer
	for i := 0; i < numPlayers; i++ {
		playerIndex := (dealerIndex + i) % numPlayers
		player := activePlayers[playerIndex]

		if numPlayers == 2 {
			// Heads-up: dealer is small blind
			if i == 0 {
				positions[player.SeatNumber] = SmallBlind
			} else {
				positions[player.SeatNumber] = BigBlind
			}
		} else {
			// Multi-way positions
			switch i {
			case 0:
				positions[player.SeatNumber] = Button
			case 1:
				positions[player.SeatNumber] = SmallBlind
			case 2:
				positions[player.SeatNumber] = BigBlind
			case 3:
				positions[player.SeatNumber] = UnderTheGun
			default:
				if i < numPlayers-2 {
					positions[player.SeatNumber] = EarlyPosition
				} else if i == numPlayers-2 {
					positions[player.SeatNumber] = Cutoff
				} else {
					positions[player.SeatNumber] = LatePosition
				}
			}
		}
	}

	return positions
}

// dealHoleCards deals 2 cards to each active player
func (t *Table) dealHoleCards() {
	for _, player := range t.activePlayers {
		holeCards := t.deck.DealN(2)
		player.DealHoleCards(holeCards)

		// Add hole cards to hand history
		if t.handHistory != nil {
			t.handHistory.AddPlayerHoleCards(player.Name, holeCards)
		}
	}
}

// DealFlop deals the flop (3 community cards)
func (t *Table) DealFlop() {
	if t.currentRound != PreFlop {
		return
	}

	// Burn one card
	_, _ = t.deck.Deal()

	// Deal 3 community cards
	flop := t.deck.DealN(3)
	t.communityCards = append(t.communityCards, flop...)

	t.currentRound = Flop
	t.startNewBettingRound()

	// Publish flop event
	if t.eventBus != nil {
		flopEvent := NewStreetChangeEvent(Flop, t.communityCards, t.currentBet)
		t.eventBus.Publish(flopEvent)
	}
}

// DealTurn deals the turn (4th community card)
func (t *Table) DealTurn() {
	if t.currentRound != Flop {
		return
	}

	// Burn one card
	_, _ = t.deck.Deal()

	// Deal turn card
	turnCard, _ := t.deck.Deal()
	t.communityCards = append(t.communityCards, turnCard)

	t.currentRound = Turn
	t.startNewBettingRound()

	// Publish turn event
	if t.eventBus != nil {
		turnEvent := NewStreetChangeEvent(Turn, t.communityCards, t.currentBet)
		t.eventBus.Publish(turnEvent)
	}
}

// DealRiver deals the river (5th community card)
func (t *Table) DealRiver() {
	if t.currentRound != Turn {
		return
	}

	// Burn one card
	_, _ = t.deck.Deal()

	// Deal river card
	riverCard, _ := t.deck.Deal()
	t.communityCards = append(t.communityCards, riverCard)

	t.currentRound = River
	t.startNewBettingRound()

	// Publish river event
	if t.eventBus != nil {
		riverEvent := NewStreetChangeEvent(River, t.communityCards, t.currentBet)
		t.eventBus.Publish(riverEvent)
	}
}

// EndHand ends the current hand and prepares for the next
func (t *Table) EndHand() {
	if t.state != InProgress {
		return
	}

	t.currentRound = Showdown
	t.state = HandComplete

	// Note: HandEndEvent is published by the GameEngine, not the table

	// Save hand history
	if t.handHistory != nil {
		_ = t.handHistory.SaveToFile() // Ignore error - this is best effort logging
	}
}

// FindWinners determines the winner(s) of the hand
func (t *Table) FindWinners() []*Player {
	activePlayers := t.GetActivePlayers()
	if len(activePlayers) == 0 {
		return nil
	}

	// If only one player left, they win
	if len(activePlayers) == 1 {
		return activePlayers
	}

	// Check if we have enough cards for evaluation (need at least 5 total)
	if len(t.communityCards) < 3 {
		// Not enough community cards yet, return first player for now
		return []*Player{activePlayers[0]}
	}

	// Evaluate each player's best hand
	type playerHand struct {
		player *Player
		hand   evaluator.HandRank
	}

	var playerHands []playerHand
	for _, player := range activePlayers {
		// Combine hole cards with community cards
		allCards := make([]deck.Card, 0, 7)
		allCards = append(allCards, player.HoleCards...)
		allCards = append(allCards, t.communityCards...)

		// Need exactly 7 cards for Evaluate7 (2 hole + 5 community)
		if len(allCards) != 7 {
			continue
		}

		// Find the best 5-card hand
		playerHandScore := evaluator.Evaluate7(allCards)
		playerHands = append(playerHands, playerHand{player, playerHandScore})
	}

	// Fallback if no valid hands found
	if len(playerHands) == 0 {
		return []*Player{activePlayers[0]}
	}

	// Find all players with the best hand
	var winners []*Player
	bestHandScore := playerHands[0].hand

	for _, ph := range playerHands {
		comparison := ph.hand.Compare(bestHandScore)
		if comparison > 0 {
			// Found a better hand, start new winners list
			winners = []*Player{ph.player}
			bestHandScore = ph.hand
		} else if comparison == 0 {
			// Tie - add to winners list
			winners = append(winners, ph.player)
		}
	}

	return winners
}

// GetState returns the current game state
func (t *Table) GetState() GameState {
	return t.state
}

// GetHandID returns the current hand ID
func (t *Table) GetHandID() string {
	return t.handID
}

// GetCommunityCards returns the community cards
func (t *Table) GetCommunityCards() []deck.Card {
	return t.communityCards
}

// GetHandHistory returns the current hand history
func (t *Table) GetHandHistory() *HandHistory {
	return t.handHistory
}

// GetCurrentPlayer returns the current player whose turn it is to act
func (t *Table) GetCurrentPlayer() *Player {
	if t.actionOn >= 0 && t.actionOn < len(t.activePlayers) {
		return t.activePlayers[t.actionOn]
	}
	return nil
}

// GetCurrentRound returns the current betting round
func (t *Table) GetCurrentRound() BettingRound {
	return t.currentRound
}

// GetCurrentBet returns the current bet amount
func (t *Table) GetCurrentBet() int {
	return t.currentBet
}

// GetActionOn returns the current action index
func (t *Table) GetActionOn() int {
	return t.actionOn
}

// GetPot returns the current pot size
func (t *Table) GetPot() int {
	return t.pot
}

// AdvanceAction moves to the next player
func (t *Table) AdvanceAction() {
	if t.actionOn == -1 {
		return
	}

	currentPlayer := t.activePlayers[t.actionOn]
	t.playersActed[currentPlayer.ID] = true

	t.actionOn = t.findNextActivePlayer(t.actionOn)
}

// CreateTableState creates a snapshot of the table state for a specific acting player
func (t *Table) CreateTableState(actingPlayer *Player) TableState {
	players := make([]PlayerState, len(t.activePlayers))
	actingIdx := -1

	for i, p := range t.activePlayers {
		players[i] = PlayerState{
			Name:         p.Name,
			Chips:        p.Chips,
			Position:     p.Position,
			BetThisRound: p.BetThisRound,
			TotalBet:     p.TotalBet,
			IsActive:     p.IsActive,
			IsFolded:     p.IsFolded,
			IsAllIn:      p.IsAllIn,
			LastAction:   p.LastAction,
			// HoleCards only visible to the acting player
			HoleCards: func() []deck.Card {
				if p == actingPlayer {
					return p.HoleCards
				}
				return nil // Hidden from other players
			}(),
		}

		if p == actingPlayer {
			actingIdx = i
		}
	}

	return TableState{
		CurrentBet:      t.currentBet,
		Pot:             t.pot,
		CurrentRound:    t.currentRound,
		CommunityCards:  t.communityCards,
		SmallBlind:      t.smallBlind,
		BigBlind:        t.bigBlind,
		Players:         players,
		ActingPlayerIdx: actingIdx,
		HandHistory:     t.handHistory,
	}
}

// RemovePlayer removes a player from the table and handles any necessary cleanup
func (t *Table) RemovePlayer(playerName string) error {
	playerIndex := -1
	activeIndex := -1

	// Find player in main players list
	for i, player := range t.players {
		if player.Name == playerName {
			playerIndex = i
			break
		}
	}

	if playerIndex == -1 {
		return fmt.Errorf("player not found: %s", playerName)
	}

	player := t.players[playerIndex]
	removedChips := player.Chips

	// Find player in active players list
	for i, activePlayer := range t.activePlayers {
		if activePlayer.Name == playerName {
			activeIndex = i
			break
		}
	}

	// If player is in active hand, fold them
	if activeIndex != -1 && player.IsInHand() {
		player.Fold()

		// If this was the current acting player, advance action
		if t.actionOn == activeIndex {
			t.AdvanceAction()
		} else if t.actionOn > activeIndex {
			// Adjust action index since we're removing a player before current action
			t.actionOn--
		}

		// Remove from active players list
		t.activePlayers = append(t.activePlayers[:activeIndex], t.activePlayers[activeIndex+1:]...)
	}

	// Remove from main players list
	t.players = append(t.players[:playerIndex], t.players[playerIndex+1:]...)

	// Track removed chips for conservation validation if we have tracking capability
	// This is a simple workaround - chips are removed from the system when players disconnect
	_ = removedChips // Acknowledge we're intentionally removing chips

	return nil
}

// GetValidActions calculates the valid actions for the current acting player
func (t *Table) GetValidActions() []ValidAction {
	currentPlayer := t.GetCurrentPlayer()
	if currentPlayer == nil || !currentPlayer.CanAct() {
		return []ValidAction{}
	}

	var actions []ValidAction

	// Fold is always available (except when checking is possible)
	callAmount := t.currentBet - currentPlayer.BetThisRound
	if callAmount > 0 {
		actions = append(actions, ValidAction{
			Action:    Fold,
			MinAmount: 0,
			MaxAmount: 0,
		})
	}

	// Check is available when no bet to call
	if callAmount == 0 {
		actions = append(actions, ValidAction{
			Action:    Check,
			MinAmount: 0,
			MaxAmount: 0,
		})
	}

	// Call is available when there's a bet to call and player has chips
	if callAmount > 0 && callAmount <= currentPlayer.Chips {
		actions = append(actions, ValidAction{
			Action:    Call,
			MinAmount: callAmount,
			MaxAmount: callAmount,
		})
	}

	// Raise is available if player has enough chips for minimum raise
	minRaise := t.currentBet + t.minRaise
	totalNeeded := minRaise - currentPlayer.BetThisRound
	if totalNeeded <= currentPlayer.Chips {
		actions = append(actions, ValidAction{
			Action:    Raise,
			MinAmount: minRaise,
			MaxAmount: currentPlayer.BetThisRound + currentPlayer.Chips, // All-in amount
		})
	}

	// All-in is available if player has chips and isn't already all-in
	if currentPlayer.Chips > 0 && !currentPlayer.IsAllIn {
		allInAmount := currentPlayer.BetThisRound + currentPlayer.Chips
		actions = append(actions, ValidAction{
			Action:    AllIn,
			MinAmount: allInAmount,
			MaxAmount: allInAmount,
		})
	}

	return actions
}

// ApplyDecision applies a decision to the table state and returns the reasoning
func (t *Table) ApplyDecision(decision Decision) (string, error) {
	currentPlayer := t.GetCurrentPlayer()
	if currentPlayer == nil {
		return "", fmt.Errorf("no current player")
	}

	if !currentPlayer.CanAct() {
		return "Player cannot act", nil
	}

	// Validate decision against valid actions (Quit and SitOut/SitIn are always valid)
	validActions := t.GetValidActions()
	valid := decision.Action == Quit || decision.Action == SitOut || decision.Action == SitIn

	if !valid {
		for _, validAction := range validActions {
			if validAction.Action == decision.Action {
				if decision.Action == Raise &&
					(decision.Amount < validAction.MinAmount || decision.Amount > validAction.MaxAmount) {
					return "", fmt.Errorf("invalid raise amount: %d (valid range: %d-%d)",
						decision.Amount, validAction.MinAmount, validAction.MaxAmount)
				}
				valid = true
				break
			}
		}
	}

	if !valid {
		return "", fmt.Errorf("invalid action: %s", decision.Action)
	}

	// Apply the decision
	switch decision.Action {
	case Fold:
		currentPlayer.Fold()
	case Call:
		callAmount := t.currentBet - currentPlayer.BetThisRound
		if callAmount > 0 && callAmount <= currentPlayer.Chips {
			currentPlayer.Call(callAmount)
			t.pot += callAmount
		} else {
			currentPlayer.Check()
		}
	case Check:
		currentPlayer.Check()
	case Raise:
		totalNeeded := decision.Amount - currentPlayer.BetThisRound
		if totalNeeded > 0 && totalNeeded <= currentPlayer.Chips {
			// Calculate the size of this raise for future minimum raise calculations
			raiseSize := decision.Amount - t.currentBet

			currentPlayer.Raise(totalNeeded)
			t.pot += totalNeeded
			t.currentBet = decision.Amount

			// Update minimum raise to be the size of this raise
			// This is the correct Texas Hold'em rule
			if raiseSize > 0 {
				t.minRaise = raiseSize
			}
		} else {
			return "", fmt.Errorf("insufficient chips for raise")
		}
	case AllIn:
		allInAmount := currentPlayer.Chips
		if currentPlayer.AllIn() {
			t.pot += allInAmount

			// If this all-in raises the bet, update minimum raise
			if currentPlayer.TotalBet > t.currentBet {
				raiseSize := currentPlayer.TotalBet - t.currentBet
				t.currentBet = currentPlayer.TotalBet

				// Only update MinRaise if this all-in is a raise (not just a call)
				if raiseSize >= t.minRaise {
					t.minRaise = raiseSize
				}
			}
		}
	case Quit:
		// Player wants to quit - this will be handled at the engine level
		// For now, just set the action on the player
		currentPlayer.LastAction = Quit
		return decision.Reasoning, nil
	case SitOut:
		// Player wants to sit out - fold current hand and mark as sitting out
		currentPlayer.SitOut()

		// Publish sit-out event
		if t.eventBus != nil {
			event := NewPlayerActionEvent(currentPlayer, SitOut, 0, t.currentRound, decision.Reasoning, t.pot)
			t.eventBus.Publish(event)
		}

		return decision.Reasoning, nil
	case SitIn:
		// Player wants to return from sitting out
		currentPlayer.SitIn()

		// Publish sit-in event
		if t.eventBus != nil {
			event := NewPlayerActionEvent(currentPlayer, SitIn, 0, t.currentRound, decision.Reasoning, t.pot)
			t.eventBus.Publish(event)
		}

		return decision.Reasoning, nil
	}

	return decision.Reasoning, nil
}

// ValidateChipConservation ensures that the total chips in the game equals the expected amount
// This is a critical invariant - chips should never be created or destroyed
func (t *Table) ValidateChipConservation(expectedTotal int) error {
	actualTotal := 0

	// Count chips held by all players
	for _, player := range t.players {
		actualTotal += player.Chips
	}

	// Add chips currently in the pot (if any)
	actualTotal += t.pot

	if actualTotal != expectedTotal {
		return fmt.Errorf("chip conservation violation: expected %d total chips, but found %d (difference: %d)",
			expectedTotal, actualTotal, actualTotal-expectedTotal)
	}

	return nil
}

// GetTotalChips returns the current total chips in the game (player chips + pot)
func (t *Table) GetTotalChips() int {
	total := t.pot
	for _, player := range t.players {
		total += player.Chips
	}
	return total
}

// IsBettingRoundComplete checks if the current betting round is complete
func (t *Table) IsBettingRoundComplete() bool {
	playersInHand := 0
	playersActed := 0
	playersAllIn := 0
	playersCanAct := 0

	for _, player := range t.activePlayers {
		if player.IsInHand() {
			playersInHand++
			if player.IsAllIn {
				playersAllIn++
			}
			if player.CanAct() {
				playersCanAct++
			}

			// A player has "acted properly" if:
			// 1. They're all-in (can't act in current round, automatically counts as acted), OR
			// 2. They have acted AND bet the current amount (normal case)
			if player.IsAllIn {
				playersActed++
			} else if t.playersActed[player.ID] && player.BetThisRound == t.currentBet {
				playersActed++
			}
		}
	}

	// Betting round is complete when:
	// 1. All players in hand have acted properly, OR
	// 2. Only one or fewer players remain in hand
	allActed := playersActed == playersInHand
	onlyOneInHand := playersInHand <= 1

	return allActed || onlyOneInHand
}

// startNewBettingRound initializes a new betting round
func (t *Table) startNewBettingRound() {
	t.currentBet = 0
	t.minRaise = t.bigBlind // Reset to big blind for new betting round
	t.playersActed = make(map[int]bool)

	// Reset all players for new round
	for _, player := range t.activePlayers {
		if player.IsInHand() {
			player.ResetForNewRound()
		}
	}

	// Find first active player after dealer
	t.actionOn = t.findNextActivePlayer(t.getDealerIndex())
}

// postBlinds posts small and big blinds
func (t *Table) postBlinds() {
	var smallBlindPlayer, bigBlindPlayer *Player

	// Find blind players (include sitting out players for blind posting)
	for _, player := range t.players {
		if player.Chips > 0 { // Only consider players with chips
			switch player.Position {
			case SmallBlind:
				smallBlindPlayer = player
			case BigBlind:
				bigBlindPlayer = player
			}
		}
	}

	// Post blinds
	if smallBlindPlayer != nil {
		amount := min(t.smallBlind, smallBlindPlayer.Chips)
		// Small blind posting
		smallBlindPlayer.Call(amount)
		t.pot += amount

		// Publish small blind event
		if t.eventBus != nil {
			event := NewPlayerActionEvent(smallBlindPlayer, Call, amount, PreFlop, "", t.pot)
			t.eventBus.Publish(event)
		}
	}

	if bigBlindPlayer != nil {
		amount := min(t.bigBlind, bigBlindPlayer.Chips)
		// Big blind posting
		bigBlindPlayer.Call(amount)
		t.pot += amount
		t.currentBet = amount

		// Publish big blind event
		if t.eventBus != nil {
			event := NewPlayerActionEvent(bigBlindPlayer, Call, amount, PreFlop, "", t.pot)
			t.eventBus.Publish(event)
		}
	}
}

// setFirstToAct determines who acts first preflop
func (t *Table) setFirstToAct() {
	numPlayers := len(t.activePlayers)
	if numPlayers < 2 {
		return
	}

	// In heads-up, big blind acts first preflop
	// In multi-way, first player after big blind acts first
	var firstToAct *Player

	if numPlayers == 2 {
		for _, player := range t.activePlayers {
			if player.Position == BigBlind {
				firstToAct = player
				break
			}
		}
	} else {
		// For 3+ players, find the first player after big blind to act
		// This could be UTG in larger games, or the button in 3-player games
		for _, player := range t.activePlayers {
			if player.Position == UnderTheGun {
				firstToAct = player
				break
			}
		}

		// If no UTG (e.g., 3-player game), find player after big blind
		if firstToAct == nil {
			// Find big blind player index
			bigBlindIndex := -1
			for i, player := range t.activePlayers {
				if player.Position == BigBlind {
					bigBlindIndex = i
					break
				}
			}

			// First player after big blind
			if bigBlindIndex != -1 {
				nextIndex := (bigBlindIndex + 1) % len(t.activePlayers)
				firstToAct = t.activePlayers[nextIndex]
			}
		}
	}

	// Find the index of first to act
	for i, player := range t.activePlayers {
		if player == firstToAct {
			t.actionOn = i
			break
		}
	}
}

// getDealerIndex returns the index of the dealer in active players
func (t *Table) getDealerIndex() int {
	for i, player := range t.activePlayers {
		if player.Position == Button || (len(t.activePlayers) == 2 && player.Position == SmallBlind) {
			return i
		}
	}
	return 0
}

// findNextActivePlayer finds the next player who can act
func (t *Table) findNextActivePlayer(startIndex int) int {
	for i := 1; i <= len(t.activePlayers); i++ {
		index := (startIndex + i) % len(t.activePlayers)
		player := t.activePlayers[index]

		// Player can act if they're active, not folded, not all-in, AND
		// either they haven't acted yet OR they haven't matched the current bet
		if player.CanAct() && (!t.playersActed[player.ID] || player.BetThisRound < t.currentBet) {
			return index
		}
	}
	return -1 // No active players
}

// AwardPot awards the pot to winner(s), handling both simple splits and side pots
func (t *Table) AwardPot() {
	if t.pot <= 0 {
		return
	}

	// Calculate side pots for multi-way all-in scenarios
	sidePots := CalculateSidePots(t.players, t.pot)

	if len(sidePots) == 0 {
		// Simple case: no side pots, just award to winners
		winners := t.FindWinners()
		t.splitPotWithButtonOrder(t.pot, winners)
	} else {
		// Complex case: award each side pot to eligible winners
		for _, sidePot := range sidePots {
			if len(sidePot.EligiblePlayers) == 0 || sidePot.Amount <= 0 {
				continue
			}

			// Find winners among eligible players
			allWinners := t.FindWinners()
			var winnersInSidePot []*Player

			eligibleSet := make(map[*Player]bool)
			for _, p := range sidePot.EligiblePlayers {
				eligibleSet[p] = true
			}

			for _, winner := range allWinners {
				if eligibleSet[winner] {
					winnersInSidePot = append(winnersInSidePot, winner)
				}
			}

			// If no winners in this side pot, award to first eligible player
			if len(winnersInSidePot) == 0 && len(sidePot.EligiblePlayers) > 0 {
				winnersInSidePot = []*Player{sidePot.EligiblePlayers[0]}
			}

			// Use table's button-order split function for proper remainder distribution
			t.splitPotWithButtonOrder(sidePot.Amount, winnersInSidePot)
		}
	}

	t.pot = 0 // Pot has been fully awarded
}

// splitPotWithButtonOrder splits pot giving remainder to player closest clockwise to button
func (t *Table) splitPotWithButtonOrder(potAmount int, winners []*Player) {
	if len(winners) == 0 || potAmount <= 0 {
		return
	}

	// Integer division for each player
	sharePerPlayer := potAmount / len(winners)
	remainder := potAmount % len(winners)

	// Give each player their share
	for _, winner := range winners {
		winner.Chips += sharePerPlayer
	}

	// Give remainder to player closest clockwise to button
	if remainder > 0 {
		closestToButton := t.findClosestToButton(winners)
		if closestToButton != nil {
			closestToButton.Chips += remainder
		} else {
			// Fallback: give to first winner
			winners[0].Chips += remainder
		}
	}
}

// findClosestToButton finds the player closest clockwise to the button among the given players
func (t *Table) findClosestToButton(players []*Player) *Player {
	if len(players) == 0 {
		return nil
	}

	// Find button position
	buttonSeat := t.dealerPosition
	if buttonSeat <= 0 {
		return players[0] // Fallback
	}

	// Find the player with the smallest clockwise distance from button
	closest := players[0]
	minDistance := t.clockwiseDistance(buttonSeat, closest.SeatNumber)

	for _, player := range players[1:] {
		distance := t.clockwiseDistance(buttonSeat, player.SeatNumber)
		if distance < minDistance {
			minDistance = distance
			closest = player
		}
	}

	return closest
}

// clockwiseDistance calculates clockwise distance from start to end seat
func (t *Table) clockwiseDistance(startSeat, endSeat int) int {
	if startSeat <= 0 || endSeat <= 0 {
		return 999 // Invalid seats get max distance
	}

	distance := endSeat - startSeat
	if distance <= 0 {
		distance += t.maxSeats // Wrap around
	}
	return distance
}
