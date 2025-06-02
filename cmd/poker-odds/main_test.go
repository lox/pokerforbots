package main

import (
	"math/rand"
	"testing"

	"github.com/lox/holdem-cli/internal/deck"
)

func TestParseHands(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected int
		hasError bool
	}{
		{
			name:     "Single hand",
			input:    []string{"AcKh"},
			expected: 1,
			hasError: false,
		},
		{
			name:     "Multiple hands",
			input:    []string{"AcKh", "KdQs"},
			expected: 2,
			hasError: false,
		},
		{
			name:     "Hand with spaces",
			input:    []string{"Ac Kh"},
			expected: 1,
			hasError: false,
		},
		{
			name:     "Invalid hand - too many cards",
			input:    []string{"AcKhQd"},
			expected: 0,
			hasError: true,
		},
		{
			name:     "Invalid hand - too few cards",
			input:    []string{"Ac"},
			expected: 0,
			hasError: true,
		},
		{
			name:     "Invalid card format",
			input:    []string{"AcXy"},
			expected: 0,
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hands, err := parseHands(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if len(hands) != tt.expected {
				t.Errorf("Expected %d hands, got %d", tt.expected, len(hands))
			}

			for _, hand := range hands {
				if len(hand) != 2 {
					t.Errorf("Each hand should have exactly 2 cards, got %d", len(hand))
				}
			}
		})
	}
}

func TestValidateNoDuplicates(t *testing.T) {
	tests := []struct {
		name     string
		hands    [][]deck.Card
		board    []deck.Card
		hasError bool
	}{
		{
			name: "No duplicates",
			hands: [][]deck.Card{
				{deck.NewCard(deck.Spades, deck.Ace), deck.NewCard(deck.Hearts, deck.King)},
				{deck.NewCard(deck.Diamonds, deck.King), deck.NewCard(deck.Spades, deck.Queen)},
			},
			board:    []deck.Card{deck.NewCard(deck.Diamonds, deck.Ten), deck.NewCard(deck.Spades, deck.Seven)},
			hasError: false,
		},
		{
			name: "Duplicate in hands",
			hands: [][]deck.Card{
				{deck.NewCard(deck.Spades, deck.Ace), deck.NewCard(deck.Hearts, deck.King)},
				{deck.NewCard(deck.Spades, deck.Ace), deck.NewCard(deck.Spades, deck.Queen)},
			},
			board:    []deck.Card{},
			hasError: true,
		},
		{
			name: "Duplicate between hand and board",
			hands: [][]deck.Card{
				{deck.NewCard(deck.Spades, deck.Ace), deck.NewCard(deck.Hearts, deck.King)},
			},
			board:    []deck.Card{deck.NewCard(deck.Spades, deck.Ace)},
			hasError: true,
		},
		{
			name: "Duplicate in board",
			hands: [][]deck.Card{
				{deck.NewCard(deck.Spades, deck.Ace), deck.NewCard(deck.Hearts, deck.King)},
			},
			board:    []deck.Card{deck.NewCard(deck.Diamonds, deck.Ten), deck.NewCard(deck.Diamonds, deck.Ten)},
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateNoDuplicates(tt.hands, tt.board)

			if tt.hasError && err == nil {
				t.Errorf("Expected error but got none")
			}

			if !tt.hasError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestSelectRandomIndices(t *testing.T) {
	// Use a fixed seed for deterministic testing
	mockRng := rand.New(rand.NewSource(42))

	tests := []struct {
		name     string
		max      int
		count    int
		expected int
	}{
		{
			name:     "Select subset",
			max:      10,
			count:    3,
			expected: 3,
		},
		{
			name:     "Select all",
			max:      5,
			count:    5,
			expected: 5,
		},
		{
			name:     "Select more than available",
			max:      3,
			count:    5,
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			indices := selectRandomIndices(tt.max, tt.count, mockRng)

			if len(indices) != tt.expected {
				t.Errorf("Expected %d indices, got %d", tt.expected, len(indices))
			}

			// Check for duplicates
			seen := make(map[int]bool)
			for _, idx := range indices {
				if seen[idx] {
					t.Errorf("Duplicate index found: %d", idx)
				}
				seen[idx] = true

				if idx < 0 || idx >= tt.max {
					t.Errorf("Index out of range: %d (max: %d)", idx, tt.max)
				}
			}
		})
	}
}

func TestFormatCards(t *testing.T) {
	cards := []deck.Card{
		deck.NewCard(deck.Spades, deck.Ace),
		deck.NewCard(deck.Hearts, deck.King),
		deck.NewCard(deck.Diamonds, deck.Queen),
	}

	result := formatCards(cards)
	expected := "A♠ K♥ Q♦"

	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}
