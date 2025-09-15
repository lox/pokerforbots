package game

import (
	"fmt"
)

// Street represents the betting round
type Street int

const (
	Preflop Street = iota
	Flop
	Turn
	River
	Showdown
)

func (s Street) String() string {
	return [...]string{"preflop", "flop", "turn", "river", "showdown"}[s]
}

// Action represents a player action
type Action int

const (
	Fold Action = iota
	Check
	Call
	Raise
	AllIn
)

func (a Action) String() string {
	return [...]string{"fold", "check", "call", "raise", "allin"}[a]
}

// Player represents a player in a hand
type Player struct {
	Seat      int
	Name      string
	Chips     int
	HoleCards Hand
	Folded    bool
	AllInFlag bool
	Bet       int // Current bet in this round
	TotalBet  int // Total bet in the hand
}

// Pot represents a pot (main or side)
type Pot struct {
	Amount      int
	Eligible    []int // Seat numbers eligible for this pot
	MaxPerPlayer int   // Maximum contribution per player
}

// HandState represents the state of a poker hand
type HandState struct {
	Players      []*Player
	Button       int
	CurrentBet   int
	MinRaise     int
	LastRaiser   int
	Street       Street
	Board        Hand
	Pots         []Pot
	ActivePlayer int
	Deck         *Deck
}

// NewHandState creates a new hand state
func NewHandState(playerNames []string, button int, smallBlind, bigBlind, startingChips int) *HandState {
	players := make([]*Player, len(playerNames))
	for i, name := range playerNames {
		players[i] = &Player{
			Seat:   i,
			Name:   name,
			Chips:  startingChips,
			Folded: false,
		}
	}

	h := &HandState{
		Players:    players,
		Button:     button,
		CurrentBet: 0,
		MinRaise:   bigBlind,
		Street:     Preflop,
		Deck:       NewDeck(),
		Pots:       []Pot{{Amount: 0, Eligible: makeEligible(players)}},
	}

	// Post blinds
	h.postBlinds(smallBlind, bigBlind)

	// Deal hole cards
	h.dealHoleCards()

	// Set first active player
	h.ActivePlayer = h.nextActivePlayer((button + 3) % len(players))

	return h
}

func makeEligible(players []*Player) []int {
	eligible := make([]int, 0, len(players))
	for _, p := range players {
		eligible = append(eligible, p.Seat)
	}
	return eligible
}

func (h *HandState) postBlinds(smallBlind, bigBlind int) {
	numPlayers := len(h.Players)

	// Small blind
	sbPos := (h.Button + 1) % numPlayers
	h.Players[sbPos].Bet = min(smallBlind, h.Players[sbPos].Chips)
	h.Players[sbPos].TotalBet = h.Players[sbPos].Bet
	h.Players[sbPos].Chips -= h.Players[sbPos].Bet

	// Big blind
	bbPos := (h.Button + 2) % numPlayers
	h.Players[bbPos].Bet = min(bigBlind, h.Players[bbPos].Chips)
	h.Players[bbPos].TotalBet = h.Players[bbPos].Bet
	h.Players[bbPos].Chips -= h.Players[bbPos].Bet

	h.CurrentBet = bigBlind
	h.Pots[0].Amount = h.Players[sbPos].Bet + h.Players[bbPos].Bet
}

func (h *HandState) dealHoleCards() {
	for _, p := range h.Players {
		cards := h.Deck.Deal(2)
		p.HoleCards = NewHand(cards...)
	}
}

// GetValidActions returns valid actions for the current player
func (h *HandState) GetValidActions() []Action {
	p := h.Players[h.ActivePlayer]
	actions := []Action{Fold}

	if h.CurrentBet == p.Bet {
		actions = append(actions, Check)
	} else {
		toCall := h.CurrentBet - p.Bet
		if toCall >= p.Chips {
			actions = append(actions, AllIn)
		} else {
			actions = append(actions, Call)
			if p.Chips > toCall+h.MinRaise {
				actions = append(actions, Raise)
			} else if p.Chips > toCall {
				actions = append(actions, AllIn)
			}
		}
	}

	return actions
}

// ProcessAction processes a player action
func (h *HandState) ProcessAction(action Action, amount int) error {
	p := h.Players[h.ActivePlayer]

	switch action {
	case Fold:
		p.Folded = true

	case Check:
		if h.CurrentBet != p.Bet {
			return fmt.Errorf("cannot check, must call %d", h.CurrentBet-p.Bet)
		}

	case Call:
		toCall := min(h.CurrentBet-p.Bet, p.Chips)
		p.Bet += toCall
		p.TotalBet += toCall
		p.Chips -= toCall
		h.Pots[0].Amount += toCall
		if p.Chips == 0 {
			p.AllInFlag = true
		}

	case Raise:
		if amount < h.CurrentBet+h.MinRaise {
			return fmt.Errorf("raise too small, minimum %d", h.CurrentBet+h.MinRaise)
		}
		if amount > p.Chips+p.Bet {
			return fmt.Errorf("insufficient chips")
		}

		raiseAmount := amount - p.Bet
		h.MinRaise = amount - h.CurrentBet
		h.CurrentBet = amount
		h.LastRaiser = h.ActivePlayer

		p.Chips -= raiseAmount
		h.Pots[0].Amount += raiseAmount
		p.Bet = amount
		p.TotalBet += raiseAmount

	case AllIn:
		allInAmount := p.Chips
		p.Chips = 0
		p.AllInFlag = true
		h.Pots[0].Amount += allInAmount
		p.Bet += allInAmount
		p.TotalBet += allInAmount

		if p.Bet > h.CurrentBet {
			h.MinRaise = p.Bet - h.CurrentBet
			h.CurrentBet = p.Bet
			h.LastRaiser = h.ActivePlayer
		}
	}

	// Move to next player
	h.ActivePlayer = h.nextActivePlayer(h.ActivePlayer + 1)

	// Check if betting round is complete
	if h.isBettingComplete() {
		h.nextStreet()
	}

	return nil
}

func (h *HandState) nextActivePlayer(from int) int {
	numPlayers := len(h.Players)
	for i := 0; i < numPlayers; i++ {
		pos := (from + i) % numPlayers
		if !h.Players[pos].Folded && !h.Players[pos].AllInFlag {
			return pos
		}
	}
	return -1 // No active players
}

func (h *HandState) isBettingComplete() bool {
	// Count active players (not folded, not all-in)
	activePlayers := 0
	for _, p := range h.Players {
		if !p.Folded && !p.AllInFlag {
			activePlayers++
		}
	}

	if activePlayers <= 1 {
		return true
	}

	// Check if all active players have acted and matched the current bet
	for _, p := range h.Players {
		if !p.Folded && !p.AllInFlag && p.Bet != h.CurrentBet {
			return false
		}
	}

	// Special case for preflop: big blind gets option
	if h.Street == Preflop {
		bbPos := (h.Button + 2) % len(h.Players)
		if h.ActivePlayer == bbPos && h.LastRaiser == -1 {
			return false
		}
	}

	return true
}

func (h *HandState) nextStreet() {
	// Calculate side pots if needed
	h.calculateSidePots()

	// Reset bets for new street
	for _, p := range h.Players {
		p.Bet = 0
	}
	h.CurrentBet = 0
	h.LastRaiser = -1

	// Deal community cards
	switch h.Street {
	case Preflop:
		h.Street = Flop
		cards := h.Deck.Deal(3)
		for _, c := range cards {
			h.Board |= Hand(c)
		}
	case Flop:
		h.Street = Turn
		cards := h.Deck.Deal(1)
		h.Board |= Hand(cards[0])
	case Turn:
		h.Street = River
		cards := h.Deck.Deal(1)
		h.Board |= Hand(cards[0])
	case River:
		h.Street = Showdown
		return
	}

	// Set first active player for new street
	h.ActivePlayer = h.nextActivePlayer((h.Button + 1) % len(h.Players))
}

func (h *HandState) calculateSidePots() {
	// Get all-in amounts
	allInAmounts := make(map[int]int)
	for _, p := range h.Players {
		if p.AllInFlag && !p.Folded {
			allInAmounts[p.TotalBet] = p.Seat
		}
	}

	if len(allInAmounts) == 0 {
		return
	}

	// Sort amounts
	amounts := make([]int, 0, len(allInAmounts))
	for amount := range allInAmounts {
		amounts = append(amounts, amount)
	}

	// Simple bubble sort for small arrays
	for i := 0; i < len(amounts); i++ {
		for j := i + 1; j < len(amounts); j++ {
			if amounts[i] > amounts[j] {
				amounts[i], amounts[j] = amounts[j], amounts[i]
			}
		}
	}

	// Create side pots
	newPots := []Pot{}
	lastAmount := 0

	for _, maxAmount := range amounts {
		pot := Pot{
			Amount:       0,
			Eligible:     []int{},
			MaxPerPlayer: maxAmount,
		}

		contribution := maxAmount - lastAmount

		for _, p := range h.Players {
			if !p.Folded && p.TotalBet >= maxAmount {
				pot.Amount += contribution
				pot.Eligible = append(pot.Eligible, p.Seat)
			} else if !p.Folded && p.TotalBet > lastAmount {
				pot.Amount += p.TotalBet - lastAmount
				pot.Eligible = append(pot.Eligible, p.Seat)
			}
		}

		if pot.Amount > 0 {
			newPots = append(newPots, pot)
		}
		lastAmount = maxAmount
	}

	// Handle remaining pot
	mainPot := Pot{
		Amount:   0,
		Eligible: []int{},
	}

	for _, p := range h.Players {
		if !p.Folded && p.TotalBet > lastAmount {
			mainPot.Amount += p.TotalBet - lastAmount
			if !p.AllInFlag {
				mainPot.Eligible = append(mainPot.Eligible, p.Seat)
			}
		}
	}

	if mainPot.Amount > 0 {
		newPots = append(newPots, mainPot)
	}

	if len(newPots) > 0 {
		h.Pots = newPots
	}
}

// IsComplete returns true if the hand is complete
func (h *HandState) IsComplete() bool {
	// Count non-folded players
	activePlayers := 0
	for _, p := range h.Players {
		if !p.Folded {
			activePlayers++
		}
	}

	return h.Street == Showdown || activePlayers <= 1
}

// GetWinners determines the winners of each pot
func (h *HandState) GetWinners() map[int][]int {
	winners := make(map[int][]int) // pot index -> winner seats

	for potIdx, pot := range h.Pots {
		if len(pot.Eligible) == 0 {
			continue
		}

		// If only one player eligible, they win
		if len(pot.Eligible) == 1 {
			winners[potIdx] = pot.Eligible
			continue
		}

		// Evaluate hands
		bestRank := HandRank(0)
		bestPlayers := []int{}

		for _, seat := range pot.Eligible {
			p := h.Players[seat]
			if p.Folded {
				continue
			}

			// Combine hole cards and board
			fullHand := p.HoleCards | h.Board
			rank := Evaluate7Cards(fullHand)

			cmp := CompareHands(rank, bestRank)
			if cmp > 0 {
				bestRank = rank
				bestPlayers = []int{seat}
			} else if cmp == 0 {
				bestPlayers = append(bestPlayers, seat)
			}
		}

		winners[potIdx] = bestPlayers
	}

	return winners
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}