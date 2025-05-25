package evaluator

import (
	"fmt"
	"strings"

	"github.com/lox/holdem-cli/internal/deck"
)

// ParseCards parses a string of card notation into a slice of cards.
// Format: "AsKsQsJsTs" where each card is [Rank][Suit]
// Ranks: A, K, Q, J, T, 9, 8, 7, 6, 5, 4, 3, 2
// Suits: s (spades), h (hearts), d (diamonds), c (clubs)
func ParseCards(s string) ([]deck.Card, error) {
	s = strings.ReplaceAll(s, " ", "") // Remove any spaces
	if len(s)%2 != 0 {
		return nil, fmt.Errorf("invalid card string length: %d (must be even)", len(s))
	}

	var cards []deck.Card
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

		cards = append(cards, deck.Card{Rank: rank, Suit: suit})
	}

	return cards, nil
}

// MustParseCards parses cards and panics on error (for tests)
func MustParseCards(s string) []deck.Card {
	cards, err := ParseCards(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse cards '%s': %v", s, err))
	}
	return cards
}

func parseRank(c byte) (deck.Rank, error) {
	switch c {
	case 'A', 'a':
		return deck.Ace, nil
	case 'K', 'k':
		return deck.King, nil
	case 'Q', 'q':
		return deck.Queen, nil
	case 'J', 'j':
		return deck.Jack, nil
	case 'T', 't':
		return deck.Ten, nil
	case '9':
		return deck.Nine, nil
	case '8':
		return deck.Eight, nil
	case '7':
		return deck.Seven, nil
	case '6':
		return deck.Six, nil
	case '5':
		return deck.Five, nil
	case '4':
		return deck.Four, nil
	case '3':
		return deck.Three, nil
	case '2':
		return deck.Two, nil
	default:
		return 0, fmt.Errorf("unknown rank '%c'", c)
	}
}

func parseSuit(c byte) (deck.Suit, error) {
	switch c {
	case 's', 'S':
		return deck.Spades, nil
	case 'h', 'H':
		return deck.Hearts, nil
	case 'd', 'D':
		return deck.Diamonds, nil
	case 'c', 'C':
		return deck.Clubs, nil
	default:
		return 0, fmt.Errorf("unknown suit '%c'", c)
	}
}