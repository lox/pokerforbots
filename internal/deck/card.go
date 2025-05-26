package deck

import (
	"fmt"
	"strings"
)

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

// ParseCards parses a string of card notation into a slice of cards.
// Format: "AsKsQsJsTs" where each card is [Rank][Suit]
// Ranks: A, K, Q, J, T, 9, 8, 7, 6, 5, 4, 3, 2
// Suits: s (spades), h (hearts), d (diamonds), c (clubs)
func ParseCards(s string) ([]Card, error) {
	s = strings.ReplaceAll(s, " ", "") // Remove any spaces
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("invalid card string length: %d (must be even)", len(s))
	}

	var cards []Card
	for i := 0; i < len(s); i += 2 {
		if i+1 >= len(s) {
			return nil, fmt.Errorf("incomplete card at position %d", i)
		}

		rankChar := s[i]
		suitChar := s[i+1]

		rank, err := parseRank(rankChar)
		if err != nil {
			return nil, fmt.Errorf("invalid rank '%c' at position %d: %w", rankChar, i, err)
		}

		suit, err := parseSuit(suitChar)
		if err != nil {
			return nil, fmt.Errorf("invalid suit '%c' at position %d: %w", suitChar, i+1, err)
		}

		cards = append(cards, Card{Rank: rank, Suit: suit})
	}

	return cards, nil
}

// MustParseCards parses cards and panics on error (for tests)
func MustParseCards(s string) []Card {
	cards, err := ParseCards(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse cards '%s': %v", s, err))
	}
	return cards
}

func parseRank(c byte) (Rank, error) {
	switch c {
	case 'A', 'a':
		return Ace, nil
	case 'K', 'k':
		return King, nil
	case 'Q', 'q':
		return Queen, nil
	case 'J', 'j':
		return Jack, nil
	case 'T', 't':
		return Ten, nil
	case '9':
		return Nine, nil
	case '8':
		return Eight, nil
	case '7':
		return Seven, nil
	case '6':
		return Six, nil
	case '5':
		return Five, nil
	case '4':
		return Four, nil
	case '3':
		return Three, nil
	case '2':
		return Two, nil
	default:
		return 0, fmt.Errorf("unknown rank '%c'", c)
	}
}

func parseSuit(c byte) (Suit, error) {
	switch c {
	case 's', 'S':
		return Spades, nil
	case 'h', 'H':
		return Hearts, nil
	case 'd', 'D':
		return Diamonds, nil
	case 'c', 'C':
		return Clubs, nil
	default:
		return 0, fmt.Errorf("unknown suit '%c'", c)
	}
}
