package game

import (
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/lox/holdem-cli/internal/deck"
	"github.com/lox/holdem-cli/internal/evaluator"
	"github.com/lox/holdem-cli/internal/gameid"
)

// RandSource interface for dependency injection of randomness
type RandSource interface {
	Intn(n int) int
}

// TableConfig holds configuration for creating a table
type TableConfig struct {
	MaxSeats   int
	SmallBlind int
	BigBlind   int
	RandSource RandSource
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
	MaxSeats   int // Maximum number of seats (6 or 9)
	SmallBlind int // Small blind amount
	BigBlind   int // Big blind amount

	// Players and positions
	Players        []*Player // All players at the table
	ActivePlayers  []*Player // Players currently in the hand
	DealerPosition int       // Current dealer button position (seat number)

	// Game state
	CurrentRound BettingRound // Current betting round
	State        GameState    // Overall game state
	HandID       string       // Current hand ID

	// Cards
	Deck           *deck.Deck  // The deck of cards
	CommunityCards []deck.Card // Community cards (flop, turn, river)

	// Betting
	Pot        int // Main pot
	CurrentBet int // Current bet to call
	MinRaise   int // Minimum raise amount
	ActionOn   int // Player index who needs to act

	// Hand tracking
	PlayersActed map[int]bool // Track which players have acted this round
	HandHistory  *HandHistory // Current hand history

	// Dependencies
	randSource RandSource // Random number generator
}

// NewTable creates a new poker table with default configuration
func NewTable(maxSeats int, smallBlind, bigBlind int) *Table {
	config := TableConfig{
		MaxSeats:   maxSeats,
		SmallBlind: smallBlind,
		BigBlind:   bigBlind,
		RandSource: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	return NewTableWithConfig(config)
}

// NewTableWithConfig creates a new poker table with custom configuration
func NewTableWithConfig(config TableConfig) *Table {
	return &Table{
		MaxSeats:       config.MaxSeats,
		SmallBlind:     config.SmallBlind,
		BigBlind:       config.BigBlind,
		Players:        make([]*Player, 0, config.MaxSeats),
		ActivePlayers:  make([]*Player, 0, config.MaxSeats),
		DealerPosition: -1, // Will be set randomly when first hand starts
		CurrentRound:   PreFlop,
		State:          WaitingToStart,
		HandID:         "",
		Deck:           deck.NewDeck(),
		CommunityCards: make([]deck.Card, 0, 5),
		Pot:            0,
		CurrentBet:     0,
		MinRaise:       config.BigBlind,
		ActionOn:       -1,
		PlayersActed:   make(map[int]bool),
		randSource:     config.RandSource,
	}
}

// AddPlayer adds a player to the table
func (t *Table) AddPlayer(player *Player) bool {
	if len(t.Players) >= t.MaxSeats {
		return false // Table is full
	}

	// Find first available seat
	seatTaken := make(map[int]bool)
	for _, p := range t.Players {
		seatTaken[p.SeatNumber] = true
	}

	for seat := 1; seat <= t.MaxSeats; seat++ {
		if !seatTaken[seat] {
			player.SeatNumber = seat
			break
		}
	}

	t.Players = append(t.Players, player)

	// Sort players by seat number for consistent ordering
	sort.Slice(t.Players, func(i, j int) bool {
		return t.Players[i].SeatNumber < t.Players[j].SeatNumber
	})

	return true
}

// StartNewHand starts a new hand
func (t *Table) StartNewHand() {
	if len(t.Players) < 2 {
		return // Need at least 2 players
	}

	t.HandID = gameid.GenerateWithRandSource(t.randSource)
	t.State = InProgress
	t.CurrentRound = PreFlop
	t.Pot = 0
	t.CurrentBet = 0
	t.MinRaise = t.BigBlind
	t.CommunityCards = t.CommunityCards[:0]
	t.PlayersActed = make(map[int]bool)

	// Reset all players for new hand
	t.ActivePlayers = make([]*Player, 0, len(t.Players))
	for _, player := range t.Players {
		if player.Chips > 0 { // Only include players with chips
			player.ResetForNewHand()
			t.ActivePlayers = append(t.ActivePlayers, player)
		}
	}

	// Set dealer position for the hand
	t.setDealerPosition()

	// Set positions
	t.setPositions()

	// Initialize hand history for this hand (after dealer position is set)
	t.HandHistory = NewHandHistory(t)

	// Shuffle and deal
	t.Deck.Reset()
	t.dealHoleCards()

	// Post blinds
	t.postBlinds()

	// Set action on first player to act (UTG in heads-up, left of BB otherwise)
	t.setFirstToAct()
}

// setDealerPosition sets the dealer position for the hand
func (t *Table) setDealerPosition() {
	if len(t.ActivePlayers) < 2 {
		return
	}

	if t.DealerPosition == -1 {
		// First hand - set random starting position
		randomIndex := t.randSource.Intn(len(t.ActivePlayers))
		t.DealerPosition = t.ActivePlayers[randomIndex].SeatNumber
	} else {
		// Move button to next active player
		t.moveButtonToNextPlayer()
	}
}

// moveButtonToNextPlayer moves the dealer button to the next active player
func (t *Table) moveButtonToNextPlayer() {
	if len(t.ActivePlayers) < 2 {
		return
	}

	// Find current dealer in active players
	currentDealerIndex := -1
	for i, player := range t.ActivePlayers {
		if player.SeatNumber == t.DealerPosition {
			currentDealerIndex = i
			break
		}
	}

	// If current dealer not found or not active, start from beginning
	if currentDealerIndex == -1 {
		t.DealerPosition = t.ActivePlayers[0].SeatNumber
		return
	}

	// Move to next active player
	nextIndex := (currentDealerIndex + 1) % len(t.ActivePlayers)
	t.DealerPosition = t.ActivePlayers[nextIndex].SeatNumber
}

// setPositions assigns positions to active players based on dealer button
func (t *Table) setPositions() {
	positions := calculatePositions(t.DealerPosition, t.ActivePlayers)

	// Apply the calculated positions
	for _, player := range t.ActivePlayers {
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
	for _, player := range t.ActivePlayers {
		holeCards := t.Deck.DealN(2)
		player.DealHoleCards(holeCards)

		// Add hole cards to hand history
		if t.HandHistory != nil {
			t.HandHistory.AddPlayerHoleCards(player.Name, holeCards)
		}
	}
}

// postBlinds posts the small and big blinds
func (t *Table) postBlinds() {
	var smallBlindPlayer, bigBlindPlayer *Player

	// Find blind players
	for _, player := range t.ActivePlayers {
		switch player.Position {
		case SmallBlind:
			smallBlindPlayer = player
		case BigBlind:
			bigBlindPlayer = player
		}
	}

	// Post blinds
	if smallBlindPlayer != nil {
		amount := min(t.SmallBlind, smallBlindPlayer.Chips)
		smallBlindPlayer.Call(amount)
		t.Pot += amount

		// Record small blind posting in hand history
		if t.HandHistory != nil {
			t.HandHistory.AddAction(smallBlindPlayer.Name, Call, amount, t.Pot, PreFlop, "")
		}
	}

	if bigBlindPlayer != nil {
		amount := min(t.BigBlind, bigBlindPlayer.Chips)
		bigBlindPlayer.Call(amount)
		t.Pot += amount
		t.CurrentBet = amount

		// Record big blind posting in hand history
		if t.HandHistory != nil {
			t.HandHistory.AddAction(bigBlindPlayer.Name, Call, amount, t.Pot, PreFlop, "")
		}
	}
}

// setFirstToAct determines who acts first preflop
func (t *Table) setFirstToAct() {
	numPlayers := len(t.ActivePlayers)
	if numPlayers < 2 {
		return
	}

	// In heads-up, big blind acts first preflop
	// In multi-way, first player after big blind acts first
	var firstToAct *Player

	if numPlayers == 2 {
		for _, player := range t.ActivePlayers {
			if player.Position == BigBlind {
				firstToAct = player
				break
			}
		}
	} else {
		for _, player := range t.ActivePlayers {
			if player.Position == UnderTheGun {
				firstToAct = player
				break
			}
		}
	}

	// Find the index of first to act
	for i, player := range t.ActivePlayers {
		if player == firstToAct {
			t.ActionOn = i
			break
		}
	}
}

// DealFlop deals the flop (3 community cards)
func (t *Table) DealFlop() {
	if t.CurrentRound != PreFlop {
		return
	}

	t.Deck.Deal() // Burn card
	flop := t.Deck.DealN(3)
	t.CommunityCards = append(t.CommunityCards, flop...)
	t.CurrentRound = Flop
	t.startNewBettingRound()
}

// DealTurn deals the turn (4th community card)
func (t *Table) DealTurn() {
	if t.CurrentRound != Flop {
		return
	}

	t.Deck.Deal() // Burn card
	turn, _ := t.Deck.Deal()
	t.CommunityCards = append(t.CommunityCards, turn)
	t.CurrentRound = Turn
	t.startNewBettingRound()
}

// DealRiver deals the river (5th community card)
func (t *Table) DealRiver() {
	if t.CurrentRound != Turn {
		return
	}

	t.Deck.Deal() // Burn card
	river, _ := t.Deck.Deal()
	t.CommunityCards = append(t.CommunityCards, river)
	t.CurrentRound = River
	t.startNewBettingRound()
}

// startNewBettingRound starts a new betting round
func (t *Table) startNewBettingRound() {
	t.CurrentBet = 0
	t.MinRaise = t.BigBlind
	t.PlayersActed = make(map[int]bool)

	// Reset all players for new round
	for _, player := range t.ActivePlayers {
		if player.IsInHand() {
			player.ResetForNewRound()
		}
	}

	// Find first active player after dealer
	t.ActionOn = t.findNextActivePlayer(t.getDealerIndex())
}

// getDealerIndex returns the index of the dealer in active players
func (t *Table) getDealerIndex() int {
	for i, player := range t.ActivePlayers {
		if player.Position == Button || (len(t.ActivePlayers) == 2 && player.Position == SmallBlind) {
			return i
		}
	}
	return 0
}

// findNextActivePlayer finds the next player who can act
func (t *Table) findNextActivePlayer(startIndex int) int {
	for i := 1; i <= len(t.ActivePlayers); i++ {
		index := (startIndex + i) % len(t.ActivePlayers)
		if t.ActivePlayers[index].CanAct() {
			return index
		}
	}
	return -1 // No active players
}

// GetCurrentPlayer returns the player who should act
func (t *Table) GetCurrentPlayer() *Player {
	if t.ActionOn >= 0 && t.ActionOn < len(t.ActivePlayers) {
		return t.ActivePlayers[t.ActionOn]
	}
	return nil
}

// AdvanceAction moves to the next player
func (t *Table) AdvanceAction() {
	if t.ActionOn == -1 {
		return
	}

	currentPlayer := t.ActivePlayers[t.ActionOn]
	t.PlayersActed[currentPlayer.ID] = true

	t.ActionOn = t.findNextActivePlayer(t.ActionOn)
}

// IsBettingRoundComplete checks if the current betting round is complete
func (t *Table) IsBettingRoundComplete() bool {
	playersInHand := 0
	playersActed := 0
	playersAllIn := 0

	for _, player := range t.ActivePlayers {
		if player.IsInHand() {
			playersInHand++
			if player.IsAllIn {
				playersAllIn++
			}
			if t.PlayersActed[player.ID] && player.BetThisRound == t.CurrentBet {
				playersActed++
			}
		}
	}

	// Round is complete if all players have acted and matched the current bet,
	// or if only one player remains, or if all but one are all-in
	return playersActed == playersInHand || playersInHand <= 1 || playersInHand-playersAllIn <= 1
}

// String returns a string representation of the table state
func (t *Table) String() string {
	return fmt.Sprintf("Hand %s - %s - Pot: $%d - Action on: %s",
		t.HandID, t.CurrentRound, t.Pot,
		func() string {
			if player := t.GetCurrentPlayer(); player != nil {
				return player.Name
			}
			return "None"
		}())
}

// AwardPot awards the entire pot to the specified winner
func (t *Table) AwardPot(winner *Player) {
	if winner != nil && t.Pot > 0 {
		winner.Chips += t.Pot
		t.Pot = 0
	}
}

// FindWinner determines the winner of the hand using proper hand evaluation
func (t *Table) FindWinner() *Player {
	activePlayers := t.GetActivePlayers()
	if len(activePlayers) == 0 {
		return nil
	}

	// If only one player left, they win
	if len(activePlayers) == 1 {
		return activePlayers[0]
	}

	// Check if we have enough cards for evaluation (need at least 5 total)
	// This can happen if FindWinner is called before all community cards are dealt
	if len(t.CommunityCards) < 3 {
		// Not enough community cards yet, return first player for now
		// In a real game, this shouldn't happen during showdown
		return activePlayers[0]
	}

	// Evaluate each player's best hand
	var bestPlayer *Player
	var bestHandScore evaluator.HandRank

	for i, player := range activePlayers {
		// Combine hole cards with community cards
		allCards := make([]deck.Card, 0, 7)
		allCards = append(allCards, player.HoleCards...)
		allCards = append(allCards, t.CommunityCards...)

		// Need at least 5 cards total
		if len(allCards) < 5 {
			continue
		}

		// Find the best 5-card hand
		playerHandScore := evaluator.Evaluate7(allCards)

		// Compare with current best using HandRank.Compare
		if i == 0 || playerHandScore.Compare(bestHandScore) > 0 {
			bestPlayer = player
			bestHandScore = playerHandScore
		}
	}

	// Fallback if no valid hands found
	if bestPlayer == nil {
		return activePlayers[0]
	}

	return bestPlayer
}

// GetActivePlayers returns players who are still in the hand (not folded)
func (t *Table) GetActivePlayers() []*Player {
	var active []*Player
	for _, player := range t.ActivePlayers {
		if player.IsInHand() {
			active = append(active, player)
		}
	}
	return active
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
