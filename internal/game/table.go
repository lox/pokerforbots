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
	MinRaise   int // Minimum raise amount
	ActionOn   int // Player index who needs to act

	// Hand tracking
	PlayersActed map[int]bool // Track which players have acted this round
	HandHistory  *HandHistory // Current hand history

	// Dependencies
	rng *rand.Rand // Random number generator
}

// NewTable creates a new poker table with custom configuration
func NewTable(rng *rand.Rand, config TableConfig) *Table {
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

		// Record small blind posting in hand history
		if t.HandHistory != nil {
			t.HandHistory.AddAction(smallBlindPlayer.Name, Call, amount, t.Pot, PreFlop, "")
		}
	}

	if bigBlindPlayer != nil {
		amount := min(t.BigBlind, bigBlindPlayer.Chips)
		// Big blind posting
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

// splitPot splits a pot amount among multiple winners, with remainder going to first winner
func splitPot(potAmount int, winners []*Player) {
	if len(winners) == 0 || potAmount <= 0 {
		return
	}

	// Integer division for each player
	sharePerPlayer := potAmount / len(winners)
	remainder := potAmount % len(winners)

	// Give each player their share
	for i, winner := range winners {
		winner.Chips += sharePerPlayer
		// First winner gets any remainder chip
		if i == 0 {
			winner.Chips += remainder
		}
	}
}

// AwardPot awards the pot to winner(s), splitting in case of ties
func (t *Table) AwardPot() {
	// Enhanced pot award handling with side pots and ties.
	// Build a list of all players that contributed chips to the pot (TotalBet > 0).
	if t.Pot <= 0 {
		return
	}

	// Gather contributors with their total contribution this hand.
	type contributor struct {
		player *Player
		bet    int
	}

	var contributors []contributor
	for _, p := range t.Players {
		if p.TotalBet > 0 {
			contributors = append(contributors, contributor{player: p, bet: p.TotalBet})
		}
	}

	if len(contributors) == 0 {
		// Fallback to old behaviour if we somehow lost tracking information.
		winners := t.FindWinners()
		splitPot(t.Pot, winners)
		t.Pot = 0
		return
	}

	// Check if differences are only due to blinds (no actual betting disparity)
	// If all players have seen all streets and betting, side pots are legitimate
	// But if only blinds were posted, all active players should share the pot equally
	sort.Slice(contributors, func(i, j int) bool {
		return contributors[i].bet < contributors[j].bet
	})

	// If the highest bet is small blind + big blind or less, and we have exactly 2-3 active players,
	// this is likely just blind posting without real betting action - award to all winners
	activePlayers := t.GetActivePlayers()
	maxBet := contributors[len(contributors)-1].bet

	// Simple heuristic: if max bet <= big blind and we have <= 3 active players,
	// treat as blind-only scenario
	if len(activePlayers) <= 3 && maxBet <= t.BigBlind {
		winners := t.FindWinners()
		splitPot(t.Pot, winners)
		t.Pot = 0
		return
	}

	// Pre-compute the overall hand winners once so that we don't re-evaluate multiple times.
	allWinners := t.FindWinners()
	winnerSet := make(map[*Player]struct{}, len(allWinners))
	for _, w := range allWinners {
		winnerSet[w] = struct{}{}
	}

	remainingPot := t.Pot
	prevLevel := 0

	for idx := 0; idx < len(contributors); idx++ {
		// Determine current side-pot bet level (contributors[idx].bet)
		levelBet := contributors[idx].bet

		// Players that have contributed at least this much are eligible for this pot slice.
		eligible := contributors[idx:]
		numEligible := len(eligible)
		if numEligible == 0 {
			continue
		}

		// Amount in this side pot is (levelBet - prevLevel) * numEligible
		sidePot := (levelBet - prevLevel) * numEligible
		if sidePot > remainingPot {
			sidePot = remainingPot // safety – shouldn't really happen
		}

		// Determine winners among eligible players (could be single or multiple).
		var sideWinners []*Player
		for _, c := range eligible {
			if _, ok := winnerSet[c.player]; ok {
				sideWinners = append(sideWinners, c.player)
			}
		}

		if len(sideWinners) == 0 {
			// This should never happen, but fall back to all winners to avoid locking up.
			sideWinners = allWinners
		}

		splitPot(sidePot, sideWinners)
		remainingPot -= sidePot
		prevLevel = levelBet

		if remainingPot == 0 {
			break
		}
	}

	// Any chips that weren't allocated (edge cases) go to overall winners.
	if remainingPot > 0 {
		splitPot(remainingPot, allWinners)
		remainingPot = 0
	}

	t.Pot = 0 // Pot has been fully awarded.
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
			currentPlayer.Raise(totalNeeded)
			t.Pot += totalNeeded
			t.CurrentBet = decision.Amount
		} else {
			return "", fmt.Errorf("insufficient chips for raise")
		}
	case AllIn:
		allInAmount := currentPlayer.Chips
		if currentPlayer.AllIn() {
			t.Pot += allInAmount
			if currentPlayer.TotalBet > t.CurrentBet {
				t.CurrentBet = currentPlayer.TotalBet
			}
		}
	}

	// Record action in hand history
	if t.HandHistory != nil {
		t.HandHistory.AddAction(currentPlayer.Name, decision.Action,
			currentPlayer.ActionAmount, t.Pot, t.CurrentRound, decision.Reasoning)
	}

	return decision.Reasoning, nil
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
