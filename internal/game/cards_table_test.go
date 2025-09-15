package game

import (
	"testing"
)

func TestParseCardTable(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantCard    Card
		wantErr     bool
		description string
	}{
		{
			name:        "ace of spades",
			input:       "As",
			wantCard:    NewCard(12, 3), // Ace=12, Spades=3
			wantErr:     false,
			description: "Parse highest card",
		},
		{
			name:        "two of hearts",
			input:       "2h",
			wantCard:    NewCard(0, 2), // Two=0, Hearts=2
			wantErr:     false,
			description: "Parse lowest card",
		},
		{
			name:        "king of diamonds",
			input:       "Kd",
			wantCard:    NewCard(11, 1), // King=11, Diamonds=1
			wantErr:     false,
			description: "Parse face card",
		},
		{
			name:        "ten of clubs",
			input:       "Tc",
			wantCard:    NewCard(8, 0), // Ten=8, Clubs=0
			wantErr:     false,
			description: "Parse ten with T notation",
		},
		{
			name:        "invalid rank",
			input:       "Xs",
			wantCard:    0,
			wantErr:     true,
			description: "Should error on invalid rank",
		},
		{
			name:        "invalid suit",
			input:       "Ax",
			wantCard:    0,
			wantErr:     true,
			description: "Should error on invalid suit",
		},
		{
			name:        "empty string",
			input:       "",
			wantCard:    0,
			wantErr:     true,
			description: "Should error on empty string",
		},
		{
			name:        "too short",
			input:       "A",
			wantCard:    0,
			wantErr:     true,
			description: "Should error on single character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			card, err := ParseCard(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCard(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if card != tt.wantCard {
				t.Errorf("ParseCard(%q) = %v, want %v", tt.input, card, tt.wantCard)
			}
		})
	}
}

func TestCardOperations(t *testing.T) {
	tests := []struct {
		name string
		test func(t *testing.T)
	}{
		{
			name: "card creation and string conversion",
			test: func(t *testing.T) {
				testCases := []struct {
					rank uint8
					suit uint8
					want string
				}{
					{12, 3, "As"}, // Ace of spades (Ace=12, Spades=3)
					{0, 2, "2h"},  // Two of hearts (Two=0, Hearts=2)
					{11, 1, "Kd"}, // King of diamonds (King=11, Diamonds=1)
					{8, 0, "Tc"},  // Ten of clubs (Ten=8, Clubs=0)
				}

				for _, tc := range testCases {
					card := NewCard(tc.rank, tc.suit)
					got := card.String()
					if got != tc.want {
						t.Errorf("NewCard(%d, %d).String() = %q, want %q", tc.rank, tc.suit, got, tc.want)
					}
				}
			},
		},
		{
			name: "unique bit positions for all 52 cards",
			test: func(t *testing.T) {
				seen := make(map[Card]string)
				for suit := uint8(0); suit < 4; suit++ {
					for rank := uint8(0); rank < 13; rank++ {
						card := NewCard(rank, suit)
						cardStr := card.String()

						if prevStr, exists := seen[card]; exists {
							t.Errorf("Collision: %s and %s have same bit pattern", prevStr, cardStr)
						}
						seen[card] = cardStr

						// Verify single bit is set
						if bits := countBits(uint64(card)); bits != 1 {
							t.Errorf("Card %s has %d bits set, expected 1", cardStr, bits)
						}
					}
				}

				if len(seen) != 52 {
					t.Errorf("Expected 52 unique cards, got %d", len(seen))
				}
			},
		},
		{
			name: "hand operations",
			test: func(t *testing.T) {
				// Test combining cards into a hand
				aceSpades, _ := ParseCard("As")
				kingHearts, _ := ParseCard("Kh")

				hand := NewHand(aceSpades, kingHearts)

				if !hand.HasCard(aceSpades) {
					t.Error("Hand should contain Ace of Spades")
				}
				if !hand.HasCard(kingHearts) {
					t.Error("Hand should contain King of Hearts")
				}

				queenDiamonds, _ := ParseCard("Qd")
				if hand.HasCard(queenDiamonds) {
					t.Error("Hand should not contain Queen of Diamonds")
				}

				// Test card counting
				if count := hand.CountCards(); count != 2 {
					t.Errorf("Hand should have 2 cards, got %d", count)
				}

				// Test adding more cards
				hand |= Hand(queenDiamonds)
				if count := hand.CountCards(); count != 3 {
					t.Errorf("Hand should have 3 cards after adding, got %d", count)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.test)
	}
}

// Helper function to count set bits
func countBits(n uint64) int {
	count := 0
	for n != 0 {
		count++
		n &= n - 1
	}
	return count
}