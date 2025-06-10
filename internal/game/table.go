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
func NewTable(rng *rand.Rand, config TableConfig) *Table {
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
		eventBus:          NewEventBus(), // Table owns its own EventBus
	}
}

func (t *Table) SetEventBus(eventBus EventBus) {
	t.eventBus = eventBus
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

// String returns a string representation of the table state
func (t *Table) String() string {
	return fmt.Sprintf("Hand %s - %s - Pot: $%d - Action on: %s",
		t.handID, t.currentRound, t.pot,
		func() string {
			if player := t.GetCurrentPlayer(); player != nil {
				return player.Name
			}
			return "None"
		}())
}
