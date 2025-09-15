package game

import (
	"fmt"
	"math/bits"
	"math/rand"
)

// Card represents a single card as a bit position in a uint64
// Each card is one bit in a 64-bit integer
// Layout: [13 spades][13 hearts][13 diamonds][13 clubs]
// This matches the Zig implementation for fast bitwise operations
type Card uint64

// Hand is also a uint64 but can contain multiple cards
// Multiple cards are represented by multiple bits set
type Hand uint64

// Suit constants
const (
	Clubs    uint8 = 0
	Diamonds uint8 = 1
	Hearts   uint8 = 2
	Spades   uint8 = 3
)

// Rank constants (0-12 for 2-A)
const (
	Two   uint8 = 0
	Three uint8 = 1
	Four  uint8 = 2
	Five  uint8 = 3
	Six   uint8 = 4
	Seven uint8 = 5
	Eight uint8 = 6
	Nine  uint8 = 7
	Ten   uint8 = 8
	Jack  uint8 = 9
	Queen uint8 = 10
	King  uint8 = 11
	Ace   uint8 = 12
)

// Bit offsets for each suit in the bitset
const (
	ClubsOffset    = 0
	DiamondsOffset = 13
	HeartsOffset   = 26
	SpadesOffset   = 39
	RankMask       = 0x1FFF // 13 bits for ranks
)

// NewCard creates a card from rank and suit (like makeCard in Zig)
func NewCard(rank, suit uint8) Card {
	offset := suit*13 + rank
	return Card(1) << offset
}

// GetBitPosition returns which bit position this card occupies (0-51)
func (c Card) GetBitPosition() uint8 {
	if c == 0 {
		return 255 // Invalid card
	}
	return uint8(bits.TrailingZeros64(uint64(c)))
}

// Rank returns the rank of the card (0-12)
func (c Card) Rank() uint8 {
	pos := c.GetBitPosition()
	if pos == 255 {
		return 255
	}
	return pos % 13
}

// Suit returns the suit of the card (0-3)
func (c Card) Suit() uint8 {
	pos := c.GetBitPosition()
	if pos == 255 {
		return 255
	}
	return pos / 13
}

// String returns the string representation (e.g., "As", "Kh")
func (c Card) String() string {
	ranks := "23456789TJQKA"
	suits := "cdhs"

	rank := c.Rank()
	suit := c.Suit()

	if rank > 12 || suit > 3 {
		return "??"
	}

	return string(ranks[rank]) + string(suits[suit])
}

// ParseCard parses a string like "As" into a Card
func ParseCard(s string) (Card, error) {
	if len(s) != 2 {
		return 0, fmt.Errorf("invalid card string: %s", s)
	}

	var rank uint8
	switch s[0] {
	case '2': rank = Two
	case '3': rank = Three
	case '4': rank = Four
	case '5': rank = Five
	case '6': rank = Six
	case '7': rank = Seven
	case '8': rank = Eight
	case '9': rank = Nine
	case 'T', 't': rank = Ten
	case 'J', 'j': rank = Jack
	case 'Q', 'q': rank = Queen
	case 'K', 'k': rank = King
	case 'A', 'a': rank = Ace
	default:
		return 0, fmt.Errorf("invalid rank: %c", s[0])
	}

	var suit uint8
	switch s[1] {
	case 'c', 'C': suit = Clubs
	case 'd', 'D': suit = Diamonds
	case 'h', 'H': suit = Hearts
	case 's', 'S': suit = Spades
	default:
		return 0, fmt.Errorf("invalid suit: %c", s[1])
	}

	return NewCard(rank, suit), nil
}

// NewHand creates a hand from multiple cards
func NewHand(cards ...Card) Hand {
	var h Hand
	for _, c := range cards {
		h |= Hand(c)
	}
	return h
}

// AddCard adds a card to the hand
func (h *Hand) AddCard(c Card) {
	*h |= Hand(c)
}

// HasCard checks if the hand contains a specific card
func (h Hand) HasCard(c Card) bool {
	return (h & Hand(c)) != 0
}

// CountCards returns the number of cards in the hand
func (h Hand) CountCards() int {
	return bits.OnesCount64(uint64(h))
}

// GetSuitMask returns the cards of a specific suit as a bitmask
func (h Hand) GetSuitMask(suit uint8) uint16 {
	offset := suit * 13
	return uint16((h >> offset) & 0x1FFF)
}

// GetRankMask returns a bitmask of which ranks are present (for straight detection)
func (h Hand) GetRankMask() uint16 {
	mask := uint16(0)
	for suit := uint8(0); suit < 4; suit++ {
		mask |= h.GetSuitMask(suit)
	}
	// Handle ace-low straight (A-2-3-4-5)
	if mask&(1<<12) != 0 {
		mask |= 1 << 13 // Add ace as high
	}
	return mask
}

// Deck represents a standard 52-card deck
type Deck struct {
	cards [52]Card // Fixed size array
	next  int
}

// NewDeck creates a new shuffled deck
func NewDeck() *Deck {
	d := &Deck{
		next: 0,
	}

	// Create all 52 cards
	i := 0
	for suit := uint8(0); suit < 4; suit++ {
		for rank := uint8(0); rank < 13; rank++ {
			d.cards[i] = NewCard(rank, suit)
			i++
		}
	}

	// Shuffle
	d.Shuffle()
	return d
}

// Shuffle shuffles the deck using Fisher-Yates
func (d *Deck) Shuffle() {
	d.next = 0
	for i := len(d.cards) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		d.cards[i], d.cards[j] = d.cards[j], d.cards[i]
	}
}

// Deal deals n cards from the deck
func (d *Deck) Deal(n int) []Card {
	if d.next+n > len(d.cards) {
		return nil
	}
	cards := d.cards[d.next : d.next+n]
	d.next += n
	return cards
}

// Reset resets and reshuffles the deck
func (d *Deck) Reset() {
	d.Shuffle()
}