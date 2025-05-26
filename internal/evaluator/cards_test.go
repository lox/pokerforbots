package evaluator

import (
	"testing"

	"github.com/lox/holdem-cli/internal/deck"
)

func TestParseCards(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []deck.Card
		wantErr  bool
	}{
		{
			name:  "royal flush",
			input: "AsKsQsJsTs",
			expected: []deck.Card{
				{Suit: deck.Spades, Rank: deck.Ace},
				{Suit: deck.Spades, Rank: deck.King},
				{Suit: deck.Spades, Rank: deck.Queen},
				{Suit: deck.Spades, Rank: deck.Jack},
				{Suit: deck.Spades, Rank: deck.Ten},
			},
		},
		{
			name:  "mixed suits",
			input: "AhKdQcJs9s",
			expected: []deck.Card{
				{Suit: deck.Hearts, Rank: deck.Ace},
				{Suit: deck.Diamonds, Rank: deck.King},
				{Suit: deck.Clubs, Rank: deck.Queen},
				{Suit: deck.Spades, Rank: deck.Jack},
				{Suit: deck.Spades, Rank: deck.Nine},
			},
		},
		{
			name:  "low cards",
			input: "5h4d3c2s",
			expected: []deck.Card{
				{Suit: deck.Hearts, Rank: deck.Five},
				{Suit: deck.Diamonds, Rank: deck.Four},
				{Suit: deck.Clubs, Rank: deck.Three},
				{Suit: deck.Spades, Rank: deck.Two},
			},
		},
		{
			name:  "case insensitive",
			input: "asKHqDjc",
			expected: []deck.Card{
				{Suit: deck.Spades, Rank: deck.Ace},
				{Suit: deck.Hearts, Rank: deck.King},
				{Suit: deck.Diamonds, Rank: deck.Queen},
				{Suit: deck.Clubs, Rank: deck.Jack},
			},
		},
		{
			name:    "invalid rank",
			input:   "XsKs",
			wantErr: true,
		},
		{
			name:    "invalid suit",
			input:   "AsKx",
			wantErr: true,
		},
		{
			name:    "odd length",
			input:   "AsK",
			wantErr: true,
		},
		{
			name:     "empty string",
			input:    "",
			expected: []deck.Card{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCards(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCards() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !cardsEqual(got, tt.expected) {
				t.Errorf("ParseCards() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMustParseCards(t *testing.T) {
	// Test successful parsing
	cards := MustParseCards("AsKs")
	expected := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Spades, Rank: deck.King},
	}
	if !cardsEqual(cards, expected) {
		t.Errorf("MustParseCards() = %v, want %v", cards, expected)
	}

	// Test panic on invalid input
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustParseCards() should panic on invalid input")
		}
	}()
	MustParseCards("invalid")
}

func cardsEqual(a, b []deck.Card) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Rank != b[i].Rank || a[i].Suit != b[i].Suit {
			return false
		}
	}
	return true
}
