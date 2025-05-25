package game

import (
	"fmt"
	"sort"

	"github.com/lox/holdem-cli/internal/deck"
)

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
	HandNumber   int          // Current hand number

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
}

// NewTable creates a new poker table
func NewTable(maxSeats int, smallBlind, bigBlind int) *Table {
	return &Table{
		MaxSeats:       maxSeats,
		SmallBlind:     smallBlind,
		BigBlind:       bigBlind,
		Players:        make([]*Player, 0, maxSeats),
		ActivePlayers:  make([]*Player, 0, maxSeats),
		DealerPosition: 1, // Start with seat 1 as dealer
		CurrentRound:   PreFlop,
		State:          WaitingToStart,
		HandNumber:     0,
		Deck:           deck.NewDeck(),
		CommunityCards: make([]deck.Card, 0, 5),
		Pot:            0,
		CurrentBet:     0,
		MinRaise:       bigBlind,
		ActionOn:       -1,
		PlayersActed:   make(map[int]bool),
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

	t.HandNumber++
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

	// Set positions
	t.setPositions()

	// Shuffle and deal
	t.Deck.Reset()
	t.dealHoleCards()

	// Post blinds
	t.postBlinds()

	// Set action on first player to act (UTG in heads-up, left of BB otherwise)
	t.setFirstToAct()
}

// setPositions assigns positions to active players based on dealer button
func (t *Table) setPositions() {
	numPlayers := len(t.ActivePlayers)
	if numPlayers < 2 {
		return
	}

	// Find dealer in active players
	dealerIndex := -1
	for i, player := range t.ActivePlayers {
		if player.SeatNumber == t.DealerPosition {
			dealerIndex = i
			break
		}
	}

	// If dealer not found or not active, move button
	if dealerIndex == -1 {
		dealerIndex = 0
		t.DealerPosition = t.ActivePlayers[0].SeatNumber
	}

	// Assign positions
	for i := 0; i < numPlayers; i++ {
		playerIndex := (dealerIndex + i) % numPlayers
		player := t.ActivePlayers[playerIndex]

		if numPlayers == 2 {
			// Heads-up: dealer is small blind
			if i == 0 {
				player.Position = SmallBlind
			} else {
				player.Position = BigBlind
			}
		} else {
			// Multi-way
			switch i {
			case 0:
				player.Position = Button
			case 1:
				player.Position = SmallBlind
			case 2:
				player.Position = BigBlind
			case 3:
				player.Position = UnderTheGun
			default:
				if i < numPlayers-2 {
					player.Position = EarlyPosition
				} else if i == numPlayers-2 {
					player.Position = Cutoff
				} else {
					player.Position = LatePosition
				}
			}
		}
	}
}

// dealHoleCards deals 2 cards to each active player
func (t *Table) dealHoleCards() {
	for _, player := range t.ActivePlayers {
		holeCards := t.Deck.DealN(2)
		player.DealHoleCards(holeCards)
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
	}

	if bigBlindPlayer != nil {
		amount := min(t.BigBlind, bigBlindPlayer.Chips)
		bigBlindPlayer.Call(amount)
		t.Pot += amount
		t.CurrentBet = amount
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
	return fmt.Sprintf("Hand #%d - %s - Pot: $%d - Action on: %s",
		t.HandNumber, t.CurrentRound, t.Pot,
		func() string {
			if player := t.GetCurrentPlayer(); player != nil {
				return player.Name
			}
			return "None"
		}())
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
