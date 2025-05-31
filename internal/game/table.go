package game

import (
	"fmt"
	"math/rand"
	"sort"

	"github.com/lox/holdem-cli/internal/deck"
	"github.com/lox/holdem-cli/internal/evaluator"
)

// TableConfig holds configuration for creating a table
type TableConfig struct {
	MaxSeats   int
	SmallBlind int
	BigBlind   int
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
	MinRaise   int // Minimum raise amount (size of last raise, not always big blind)
	ActionOn   int // Player index who needs to act

	// Hand tracking
	PlayersActed map[int]bool // Track which players have acted this round
	HandHistory  *HandHistory // Current hand history

	// Dependencies
	rng      *rand.Rand // Random number generator
	eventBus EventBus   // Event bus for publishing game events
}

// NewTable creates a new poker table with custom configuration
func NewTable(rng *rand.Rand, config TableConfig, eventBus EventBus) *Table {
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
		Deck:           deck.NewDeck(rng),
		CommunityCards: make([]deck.Card, 0, 5),
		Pot:            0,
		CurrentBet:     0,
		MinRaise:       config.BigBlind,
		ActionOn:       -1,
		PlayersActed:   make(map[int]bool),
		rng:            rng,
		eventBus:       eventBus,
	}
}

// SetEventBus sets the event bus for the table
func (t *Table) SetEventBus(eventBus EventBus) {
	t.eventBus = eventBus
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

	t.HandID = GenerateGameID(t.rng)
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
		randomIndex := t.rng.Intn(len(t.ActivePlayers))
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
		// Small blind posting
		smallBlindPlayer.Call(amount)
		t.Pot += amount

		// Publish small blind event
		if t.eventBus != nil {
			event := NewPlayerActionEvent(smallBlindPlayer, Call, amount, PreFlop, "", t.Pot)
			t.eventBus.Publish(event)
		}
	}

	if bigBlindPlayer != nil {
		amount := min(t.BigBlind, bigBlindPlayer.Chips)
		// Big blind posting
		bigBlindPlayer.Call(amount)
		t.Pot += amount
		t.CurrentBet = amount

		// Publish big blind event
		if t.eventBus != nil {
			event := NewPlayerActionEvent(bigBlindPlayer, Call, amount, PreFlop, "", t.Pot)
			t.eventBus.Publish(event)
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
		// For 3+ players, find the first player after big blind to act
		// This could be UTG in larger games, or the button in 3-player games
		for _, player := range t.ActivePlayers {
			if player.Position == UnderTheGun {
				firstToAct = player
				break
			}
		}

		// If no UTG (e.g., 3-player game), find player after big blind
		if firstToAct == nil {
			// Find big blind player index
			bigBlindIndex := -1
			for i, player := range t.ActivePlayers {
				if player.Position == BigBlind {
					bigBlindIndex = i
					break
				}
			}

			// First player after big blind
			if bigBlindIndex != -1 {
				nextIndex := (bigBlindIndex + 1) % len(t.ActivePlayers)
				firstToAct = t.ActivePlayers[nextIndex]
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
	t.MinRaise = t.BigBlind // Reset to big blind for new betting round
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
		player := t.ActivePlayers[index]
		
		// Player can act if they're active, not folded, not all-in, AND
		// either they haven't acted yet OR they haven't matched the current bet
		if player.CanAct() && (!t.PlayersActed[player.ID] || player.BetThisRound < t.CurrentBet) {
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

			// A player has "acted properly" if:
			// 1. They're all-in (can't act in current round, automatically counts as acted), OR
			// 2. They have acted AND bet the current amount (normal case)
			if player.IsAllIn {
				playersActed++
			} else if t.PlayersActed[player.ID] && player.BetThisRound == t.CurrentBet {
				playersActed++
			}
		}
	}

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



// AwardPot awards the pot to winner(s), handling both simple splits and side pots
func (t *Table) AwardPot() {
	if t.Pot <= 0 {
		return
	}

	// Calculate side pots for multi-way all-in scenarios
	sidePots := CalculateSidePots(t.Players, t.Pot)
	
	if len(sidePots) == 0 {
		// Simple case: no side pots, just award to winners
		winners := t.FindWinners()
		t.splitPotWithButtonOrder(t.Pot, winners)
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

	t.Pot = 0 // Pot has been fully awarded
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
	buttonSeat := t.DealerPosition
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
		distance += t.MaxSeats // Wrap around
	}
	return distance
}

// FindWinners determines all winners of the hand (handles ties)
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
	if len(t.CommunityCards) < 3 {
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
		allCards = append(allCards, t.CommunityCards...)

		// Need at least 5 cards total
		if len(allCards) < 5 {
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

// FindWinner returns a single winner for backwards compatibility
func (t *Table) FindWinner() *Player {
	winners := t.FindWinners()
	if len(winners) > 0 {
		return winners[0]
	}
	return nil
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

// CreateTableState creates a read-only snapshot of the table state for decision making
func (t *Table) CreateTableState(actingPlayer *Player) TableState {
	players := make([]PlayerState, len(t.ActivePlayers))
	actingIdx := -1

	for i, p := range t.ActivePlayers {
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
		CurrentBet:      t.CurrentBet,
		Pot:             t.Pot,
		CurrentRound:    t.CurrentRound,
		CommunityCards:  t.CommunityCards,
		SmallBlind:      t.SmallBlind,
		BigBlind:        t.BigBlind,
		Players:         players,
		ActingPlayerIdx: actingIdx,
		HandHistory:     t.HandHistory,
	}
}

// GetValidActions calculates the valid actions for the current acting player
func (t *Table) GetValidActions() []ValidAction {
	currentPlayer := t.GetCurrentPlayer()
	if currentPlayer == nil || !currentPlayer.CanAct() {
		return []ValidAction{}
	}

	var actions []ValidAction

	// Fold is always available (except when checking is possible)
	callAmount := t.CurrentBet - currentPlayer.BetThisRound
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
	minRaise := t.CurrentBet + t.MinRaise
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

	// Validate decision against valid actions
	validActions := t.GetValidActions()
	valid := false
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

	if !valid {
		return "", fmt.Errorf("invalid action: %s", decision.Action)
	}

	// Apply the decision
	switch decision.Action {
	case Fold:
		currentPlayer.Fold()
	case Call:
		callAmount := t.CurrentBet - currentPlayer.BetThisRound
		if callAmount > 0 && callAmount <= currentPlayer.Chips {
			currentPlayer.Call(callAmount)
			t.Pot += callAmount
		} else {
			currentPlayer.Check()
		}
	case Check:
		currentPlayer.Check()
	case Raise:
		totalNeeded := decision.Amount - currentPlayer.BetThisRound
		if totalNeeded > 0 && totalNeeded <= currentPlayer.Chips {
			// Calculate the size of this raise for future minimum raise calculations
			raiseSize := decision.Amount - t.CurrentBet
			
			currentPlayer.Raise(totalNeeded)
			t.Pot += totalNeeded
			t.CurrentBet = decision.Amount
			
			// Update minimum raise to be the size of this raise
			// This is the correct Texas Hold'em rule
			if raiseSize > 0 {
				t.MinRaise = raiseSize
			}
		} else {
			return "", fmt.Errorf("insufficient chips for raise")
		}
	case AllIn:
		allInAmount := currentPlayer.Chips
		if currentPlayer.AllIn() {
			t.Pot += allInAmount
			
			// If this all-in raises the bet, update minimum raise
			if currentPlayer.TotalBet > t.CurrentBet {
				raiseSize := currentPlayer.TotalBet - t.CurrentBet
				t.CurrentBet = currentPlayer.TotalBet
				
				// Only update MinRaise if this all-in is a raise (not just a call)
				if raiseSize >= t.MinRaise {
					t.MinRaise = raiseSize
				}
			}
		}
	}

	// Note: Actions are now recorded via event system

	return decision.Reasoning, nil
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
