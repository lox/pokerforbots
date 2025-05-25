package deck

import "fmt"

// Suit represents a card suit
type Suit int

const (
	Spades Suit = iota
	Hearts
	Diamonds
	Clubs
)

// String returns the string representation of a suit
func (s Suit) String() string {
	switch s {
	case Spades:
		return "♠"
	case Hearts:
		return "♥"
	case Diamonds:
		return "♦"
	case Clubs:
		return "♣"
	default:
		return "?"
	}
}

// IsRed returns true if the suit is red (Hearts or Diamonds)
func (s Suit) IsRed() bool {
	return s == Hearts || s == Diamonds
}

// Rank represents a card rank
type Rank int

const (
	Two Rank = iota + 2
	Three
	Four
	Five
	Six
	Seven
	Eight
	Nine
	Ten
	Jack
	Queen
	King
	Ace
)

// String returns the string representation of a rank
func (r Rank) String() string {
	switch r {
	case Two:
		return "2"
	case Three:
		return "3"
	case Four:
		return "4"
	case Five:
		return "5"
	case Six:
		return "6"
	case Seven:
		return "7"
	case Eight:
		return "8"
	case Nine:
		return "9"
	case Ten:
		return "T"
	case Jack:
		return "J"
	case Queen:
		return "Q"
	case King:
		return "K"
	case Ace:
		return "A"
	default:
		return "?"
	}
}

// Card represents a playing card
type Card struct {
	Suit Suit
	Rank Rank
}

// NewCard creates a new card
func NewCard(suit Suit, rank Rank) Card {
	return Card{Suit: suit, Rank: rank}
}

// String returns the string representation of a card (e.g., "A♠")
func (c Card) String() string {
	return fmt.Sprintf("%s%s", c.Rank, c.Suit)
}

// IsRed returns true if the card is red
func (c Card) IsRed() bool {
	return c.Suit.IsRed()
}

// Value returns the numeric value of the card for comparison
// Aces are high (14), but can be used as low (1) in specific contexts
func (c Card) Value() int {
	return int(c.Rank)
}

// IsAce returns true if the card is an Ace
func (c Card) IsAce() bool {
	return c.Rank == Ace
}

// IsFaceCard returns true if the card is a face card (J, Q, K)
func (c Card) IsFaceCard() bool {
	return c.Rank >= Jack && c.Rank <= King
}
