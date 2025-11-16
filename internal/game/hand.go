package game

import (
	"fmt"
	rand "math/rand/v2"

	"github.com/lox/pokerforbots/v2/poker"
)

// HandState represents the state of a poker hand
type HandState struct {
	Players      []*Player
	Button       int
	Street       Street
	Board        poker.Hand
	boardOrder   []poker.Card
	PotManager   *PotManager
	ActivePlayer int
	Deck         *poker.Deck
	Betting      *BettingRound // Encapsulates all betting state
}

// HandOption configures a HandState during creation.
type HandOption func(*handConfig)

// handConfig holds optional configuration for creating a hand.
type handConfig struct {
	chipCounts []int       // If nil, uses uniform starting chips
	startChips int         // Default: 1000
	deck       *poker.Deck // If provided, uses this deck (overrides RNG for deck creation)
}

// NewHandState creates a new hand state with required RNG and optional configuration.
// The RNG is required to make randomness explicit and testing deterministic.
//
// Example usage:
//
//	// Production - time-seeded RNG
//	rng := randutil.New(time.Now().UnixNano())
//	h := NewHandState(rng, []string{"Alice", "Bob"}, 0, 5, 10)
//
//	// Testing - deterministic RNG
//	rng := randutil.New(42)
//	h := NewHandState(rng, []string{"Alice", "Bob"}, 0, 5, 10)
//
//	// With options
//	h := NewHandState(rng, players, 0, 5, 10,
//	    WithChipsByPlayer([]int{1000, 800, 1200}))
func NewHandState(rng *rand.Rand, playerNames []string, button int, smallBlind, bigBlind int, opts ...HandOption) *HandState {
	if rng == nil {
		panic("rng is required for hand creation")
	}
	if len(playerNames) < 2 {
		panic("at least 2 players required")
	}
	if button < 0 || button >= len(playerNames) {
		panic("button position out of range")
	}

	cfg := &handConfig{
		startChips: 1000, // sensible default
	}

	// Apply options
	for _, opt := range opts {
		opt(cfg)
	}

	// Validation
	if cfg.chipCounts != nil && len(cfg.chipCounts) != len(playerNames) {
		panic("chip counts must match number of players")
	}

	// Build players
	players := make([]*Player, len(playerNames))
	for i, name := range playerNames {
		chips := cfg.startChips
		if cfg.chipCounts != nil {
			chips = cfg.chipCounts[i]
		}
		players[i] = &Player{
			Seat:   i,
			Name:   name,
			Chips:  chips,
			Folded: false,
		}
	}

	// Setup deck (deck option overrides RNG if provided)
	var deck *poker.Deck
	if cfg.deck != nil {
		deck = cfg.deck
	} else {
		deck = poker.NewDeck(rng)
	}

	// Create hand state
	h := &HandState{
		Players:    players,
		Button:     button,
		Street:     Preflop,
		Deck:       deck,
		PotManager: NewPotManager(players),
		Betting:    NewBettingRound(len(players), bigBlind),
	}

	// Initialize the hand
	h.postBlinds(smallBlind, bigBlind)
	h.dealHoleCards()

	// Set first active player
	if len(players) == 2 {
		// Heads-up: button acts first preflop
		h.ActivePlayer = button
	} else {
		// Regular: UTG (button+3) acts first
		h.ActivePlayer = h.nextActivePlayer((button + 3) % len(players))
	}

	return h
}

// Option Functions

// WithChips sets the same starting chips for all players.
// Default is 1000 if not specified.
func WithChips(chips int) HandOption {
	return func(c *handConfig) {
		c.startChips = chips
		c.chipCounts = nil // Clear any individual counts
	}
}

// WithChipsByPlayer sets individual chip counts for each player.
// The length must match the number of players.
func WithChipsByPlayer(chipCounts []int) HandOption {
	return func(c *handConfig) {
		c.chipCounts = chipCounts
	}
}

// WithDeck sets a specific pre-shuffled deck.
// This overrides the RNG for deck creation.
func WithDeck(deck *poker.Deck) HandOption {
	return func(c *handConfig) {
		c.deck = deck
	}
}

func (h *HandState) postBlinds(smallBlind, bigBlind int) {
	numPlayers := len(h.Players)

	var sbPos, bbPos int

	if numPlayers == 2 {
		// Heads-up: button posts small blind
		sbPos = h.Button
		bbPos = (h.Button + 1) % numPlayers
	} else {
		// Regular: button+1 posts small blind, button+2 posts big blind
		sbPos = (h.Button + 1) % numPlayers
		bbPos = (h.Button + 2) % numPlayers
	}

	// Small blind
	h.Players[sbPos].Bet = min(smallBlind, h.Players[sbPos].Chips)
	h.Players[sbPos].TotalBet = h.Players[sbPos].Bet
	h.Players[sbPos].Chips -= h.Players[sbPos].Bet

	// Big blind
	h.Players[bbPos].Bet = min(bigBlind, h.Players[bbPos].Chips)
	h.Players[bbPos].TotalBet = h.Players[bbPos].Bet
	h.Players[bbPos].Chips -= h.Players[bbPos].Bet

	h.Betting.CurrentBet = bigBlind
	// Don't collect bets yet - they stay in player.Bet until NextStreet
}

func (h *HandState) dealHoleCards() {
	for _, p := range h.Players {
		cards := h.Deck.Deal(2)
		p.HoleCards = poker.NewHand(cards...)
	}
}

// GetValidActions returns valid actions for the current player
func (h *HandState) GetValidActions() []Action {
	if h.ActivePlayer < 0 || h.ActivePlayer >= len(h.Players) {
		return []Action{} // No active player
	}
	return h.Betting.GetValidActions(h.Players[h.ActivePlayer])
}

// ProcessAction processes a player action
func (h *HandState) ProcessAction(action Action, amount int) error {
	p := h.Players[h.ActivePlayer]

	// Mark player as having acted in this round
	h.Betting.MarkPlayerActed(h.ActivePlayer)

	// Track if BB is acting preflop
	if h.Street == Preflop {
		var bbPos int
		if len(h.Players) == 2 {
			// Heads-up: button+1 is BB
			bbPos = (h.Button + 1) % len(h.Players)
		} else {
			// Regular: button+2 is BB
			bbPos = (h.Button + 2) % len(h.Players)
		}
		if h.ActivePlayer == bbPos {
			h.Betting.BBActed = true
		}
	}

	switch action {
	case Fold:
		p.Folded = true

	case Check:
		if h.Betting.CurrentBet != p.Bet {
			return fmt.Errorf("cannot check, must call %d", h.Betting.CurrentBet-p.Bet)
		}

	case Call:
		toCall := min(h.Betting.CurrentBet-p.Bet, p.Chips)
		p.Bet += toCall
		p.TotalBet += toCall
		p.Chips -= toCall
		if p.Chips == 0 {
			p.AllInFlag = true
		}

	case Raise:
		// Check if player has enough chips for this raise
		playerTotalChips := p.Chips + p.Bet

		// If player is trying to raise more than they have, that's an error
		if amount > playerTotalChips {
			return fmt.Errorf("insufficient chips")
		}

		// If player has enough chips, enforce minimum raise
		// But if they're going all-in with less than min raise, allow it
		if amount < h.Betting.CurrentBet+h.Betting.MinRaise {
			// Check if this is an all-in (player is putting in all their chips)
			if amount < playerTotalChips {
				// Player has more chips but trying to raise below minimum
				return fmt.Errorf("raise too small, minimum %d", h.Betting.CurrentBet+h.Betting.MinRaise)
			}
			// Player is going all-in with less than min raise - this is allowed
		}

		raiseAmount := amount - p.Bet
		h.Betting.MinRaise = amount - h.Betting.CurrentBet
		h.Betting.CurrentBet = amount
		h.Betting.LastRaiser = h.ActivePlayer

		p.Chips -= raiseAmount
		p.Bet = amount
		p.TotalBet += raiseAmount

		// Mark player as all-in if they have no chips left
		if p.Chips == 0 {
			p.AllInFlag = true
		}

		// Reset acted flags when someone raises (everyone needs to act again)
		for i := range h.Betting.ActedThisRound {
			h.Betting.ActedThisRound[i] = false
		}
		h.Betting.ActedThisRound[h.ActivePlayer] = true

	case AllIn:
		allInAmount := p.Chips
		p.Chips = 0
		p.AllInFlag = true
		p.Bet += allInAmount
		p.TotalBet += allInAmount

		if p.Bet > h.Betting.CurrentBet {
			h.Betting.MinRaise = p.Bet - h.Betting.CurrentBet
			h.Betting.CurrentBet = p.Bet
			h.Betting.LastRaiser = h.ActivePlayer

			// Reset acted flags when all-in acts as a raise
			for i := range h.Betting.ActedThisRound {
				h.Betting.ActedThisRound[i] = false
			}
			h.Betting.ActedThisRound[h.ActivePlayer] = true
		}
	}

	// Move to next player
	h.ActivePlayer = h.nextActivePlayer(h.ActivePlayer + 1)

	// Check if betting round is complete
	// Note: ActivePlayer will be -1 if no active players left
	if h.ActivePlayer == -1 || h.Betting.IsBettingComplete(h.Players, h.Street, h.Button) {
		h.NextStreet()
	}

	return nil
}

// ForceFold marks the specified seat as folded immediately, regardless of turn order.
// Used for exceptional conditions like disconnects and protocol violations.
func (h *HandState) ForceFold(seat int) {
	if seat < 0 || seat >= len(h.Players) {
		return
	}

	player := h.Players[seat]
	if player.Folded {
		return
	}

	player.Folded = true
	h.Betting.MarkPlayerActed(seat)

	// If the folding player was the big blind preflop, mark that they have acted to avoid hanging the round.
	if h.Street == Preflop {
		var bbPos int
		if len(h.Players) == 2 {
			bbPos = (h.Button + 1) % len(h.Players)
		} else {
			bbPos = (h.Button + 2) % len(h.Players)
		}
		if seat == bbPos {
			h.Betting.BBActed = true
		}
	}

	if h.Betting.LastRaiser == seat {
		h.Betting.LastRaiser = -1
	}

	// Advance the active player if the disconnected bot was due to act next.
	if seat == h.ActivePlayer {
		h.ActivePlayer = h.nextActivePlayer(seat + 1)
	}

	if h.ActivePlayer == -1 || h.Betting.IsBettingComplete(h.Players, h.Street, h.Button) {
		h.NextStreet()
	}
}

func (h *HandState) nextActivePlayer(from int) int {
	numPlayers := len(h.Players)
	for i := range numPlayers {
		pos := (from + i) % numPlayers
		if !h.Players[pos].Folded && !h.Players[pos].AllInFlag {
			return pos
		}
	}
	return -1 // No active players
}

// NextStreet advances to the next betting street
func (h *HandState) NextStreet() {
	// Collect all bets into pots and calculate side pots if needed
	h.PotManager.CollectBets(h.Players)
	h.PotManager.CalculateSidePots(h.Players)

	// Reset bets for new street
	for _, p := range h.Players {
		p.Bet = 0
	}
	h.Betting.ResetForNewRound(len(h.Players))

	if h.contestingPlayerCount() <= 1 {
		h.Street = Showdown
		h.ActivePlayer = -1
		return
	}

	// Move to next street and deal community cards
	switch h.Street {
	case Preflop:
		h.Street = Flop
		cards := h.Deck.Deal(3)
		for _, c := range cards {
			h.boardOrder = append(h.boardOrder, c)
			h.Board |= poker.Hand(c)
		}
	case Flop:
		h.Street = Turn
		cards := h.Deck.Deal(1)
		h.boardOrder = append(h.boardOrder, cards[0])
		h.Board |= poker.Hand(cards[0])
	case Turn:
		h.Street = River
		cards := h.Deck.Deal(1)
		h.boardOrder = append(h.boardOrder, cards[0])
		h.Board |= poker.Hand(cards[0])
	case River:
		h.Street = Showdown
	case Showdown:
		return
	}

	// Set first active player for new street
	h.ActivePlayer = h.nextActivePlayer((h.Button + 1) % len(h.Players))

	// If no active players (all non-folded players are all-in), keep advancing to showdown
	if h.ActivePlayer == -1 && h.Street != Showdown {
		if h.contestingPlayerCount() > 0 {
			h.NextStreet()
		}
	}
}

func (h *HandState) contestingPlayerCount() int {
	count := 0
	for _, p := range h.Players {
		if !p.Folded {
			count++
		}
	}
	return count
}

// BoardCards returns the community cards in the order they were dealt.
func (h *HandState) BoardCards() []poker.Card {
	if len(h.boardOrder) == 0 {
		return nil
	}
	cards := make([]poker.Card, len(h.boardOrder))
	copy(cards, h.boardOrder)
	return cards
}

// GetPots returns the current pots including uncollected bets
func (h *HandState) GetPots() []Pot {
	return h.PotManager.GetPotsWithUncollected(h.Players)
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

	for potIdx, pot := range h.GetPots() {
		if len(pot.Eligible) == 0 {
			continue
		}

		// If only one player eligible, they win
		if len(pot.Eligible) == 1 {
			winners[potIdx] = pot.Eligible
			continue
		}

		// Evaluate hands
		bestRank := poker.HandRank(0)
		bestPlayers := []int{}

		for _, seat := range pot.Eligible {
			p := h.Players[seat]
			if p.Folded {
				continue
			}

			// Combine hole cards and board
			fullHand := p.HoleCards | h.Board
			rank := poker.Evaluate7Cards(fullHand)

			cmp := poker.CompareHands(rank, bestRank)
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
