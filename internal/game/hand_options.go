package game

import (
	"math/rand"

	"github.com/lox/pokerforbots/poker"
)

// HandOption configures a HandState during creation.
type HandOption func(*handConfig)

// handConfig holds all configuration for creating a hand.
type handConfig struct {
	// Required fields (set via NewHand)
	rng         *rand.Rand
	playerNames []string
	button      int
	smallBlind  int
	bigBlind    int

	// Optional fields (set via options)
	chipCounts []int       // If nil, uses uniform starting chips
	startChips int         // Default: 1000
	deck       *poker.Deck // If provided, uses this deck (overrides RNG for deck creation)
}

// NewHand creates a new hand state with required RNG and optional configuration.
// The RNG is required to make randomness explicit and testing deterministic.
//
// Example usage:
//
//	// Production - time-seeded RNG
//	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
//	h := NewHand(rng, []string{"Alice", "Bob"}, 0, 5, 10)
//
//	// Testing - deterministic RNG
//	rng := rand.New(rand.NewSource(42))
//	h := NewHand(rng, []string{"Alice", "Bob"}, 0, 5, 10)
//
//	// With options
//	h := NewHand(rng, players, 0, 5, 10,
//	    WithChips([]int{1000, 800, 1200}))
func NewHand(rng *rand.Rand, playerNames []string, button int, smallBlind, bigBlind int, opts ...HandOption) *HandState {
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
		rng:         rng,
		playerNames: playerNames,
		button:      button,
		smallBlind:  smallBlind,
		bigBlind:    bigBlind,
		startChips:  1000, // sensible default
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
		deck = poker.NewDeck(cfg.rng)
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

// WithUniformChips sets the same starting chips for all players.
// Default is 1000 if not specified.
func WithUniformChips(chips int) HandOption {
	return func(c *handConfig) {
		c.startChips = chips
		c.chipCounts = nil // Clear any individual counts
	}
}

// WithChips sets individual chip counts for each player.
// The length must match the number of players.
func WithChips(chipCounts []int) HandOption {
	return func(c *handConfig) {
		c.chipCounts = chipCounts
	}
}

// WithDeck sets a specific pre-shuffled deck.
// This overrides the RNG for deck creation but the RNG
// may still be used for other randomness.
func WithDeck(deck *poker.Deck) HandOption {
	return func(c *handConfig) {
		c.deck = deck
	}
}
