package poker

import (
	"math/bits"
	"math/rand"
	"testing"
)

func TestCardCreation(t *testing.T) {
	t.Parallel()
	// Test card creation
	aceSpades := NewCard(Ace, Spades)
	if aceSpades.Rank() != Ace {
		t.Errorf("Expected rank Ace, got %d", aceSpades.Rank())
	}
	if aceSpades.Suit() != Spades {
		t.Errorf("Expected suit Spades, got %d", aceSpades.Suit())
	}

	// Test string representation
	if aceSpades.String() != "As" {
		t.Errorf("Expected 'As', got %s", aceSpades.String())
	}

	// Test two of clubs (lowest card)
	twoClubs := NewCard(Two, Clubs)
	if twoClubs.String() != "2c" {
		t.Errorf("Expected '2c', got %s", twoClubs.String())
	}
}

func TestParseCard(t *testing.T) {
	t.Parallel()
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
			name:        "nine of spades",
			input:       "9s",
			wantCard:    NewCard(7, 3), // Nine=7, Spades=3
			wantErr:     false,
			description: "Parse number card",
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
		{
			name:        "too long",
			input:       "Asd",
			wantCard:    0,
			wantErr:     true,
			description: "Should error on too many characters",
		},
	}

	for _, testCase := range tests {
		tc := testCase
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			card, err := ParseCard(tc.input)
			if (err != nil) != tc.wantErr {
				return
			}
			if card != tc.wantCard {
				t.Errorf("ParseCard(%q) = %v, want %v", tc.input, card, tc.wantCard)
			}
		})
	}
}

func TestAll52Cards(t *testing.T) {
	t.Parallel()
	// Test all 52 cards encode/decode correctly
	cards := make(map[string]bool)

	for suit := uint8(0); suit < 4; suit++ {
		for rank := uint8(0); rank < 13; rank++ {
			card := NewCard(rank, suit)
			str := card.String()

			// Check no duplicates
			if cards[str] {
				t.Errorf("Duplicate card: %s", str)
			}
			cards[str] = true

			// Test round-trip
			parsed, err := ParseCard(str)
			if err != nil {
				t.Errorf("Failed to parse %s: %v", str, err)
			}
			if parsed != card {
				t.Errorf("Round-trip failed for %s", str)
			}
		}
	}

	if len(cards) != 52 {
		t.Errorf("Expected 52 unique cards, got %d", len(cards))
	}
}

func TestHandOperations(t *testing.T) {
	t.Parallel()
	aceSpades, _ := ParseCard("As")
	kingHearts, _ := ParseCard("Kh")
	queenDiamonds, _ := ParseCard("Qd")

	// Test creating hand from cards
	hand := NewHand(aceSpades, kingHearts)

	if !hand.HasCard(aceSpades) {
		t.Error("Hand should contain Ace of Spades")
	}
	if !hand.HasCard(kingHearts) {
		t.Error("Hand should contain King of Hearts")
	}
	if hand.HasCard(queenDiamonds) {
		t.Error("Hand should not contain Queen of Diamonds")
	}

	// Test card count
	if hand.CountCards() != 2 {
		t.Errorf("Hand should have 2 cards, got %d", hand.CountCards())
	}

	// Add another card
	hand.AddCard(queenDiamonds)
	if !hand.HasCard(queenDiamonds) {
		t.Error("Hand should now contain Queen of Diamonds")
	}
	if hand.CountCards() != 3 {
		t.Errorf("Hand should have 3 cards, got %d", hand.CountCards())
	}
}

func TestHandBitset(t *testing.T) {
	t.Parallel()
	// Test that different cards use different bits
	aceSpades, _ := ParseCard("As")
	aceHearts, _ := ParseCard("Ah")
	twoClubs, _ := ParseCard("2c")

	// Cards should be single bits
	if bits.OnesCount64(uint64(aceSpades)) != 1 {
		t.Error("Card should be a single bit")
	}

	// No overlap between different cards
	if aceSpades&aceHearts != 0 {
		t.Error("Different cards should not share bits")
	}
	if aceSpades&twoClubs != 0 {
		t.Error("Different cards should not share bits")
	}
	if aceHearts&twoClubs != 0 {
		t.Error("Different cards should not share bits")
	}

	// Combining should preserve all cards
	combined := Hand(aceSpades) | Hand(aceHearts) | Hand(twoClubs)
	if combined.CountCards() != 3 {
		t.Errorf("Combined hand should have 3 cards, got %d", combined.CountCards())
	}
}

func TestGetSuitMask(t *testing.T) {
	t.Parallel()
	// Create a hand with specific cards
	cards := []Card{}

	// Add all spades
	for rank := uint8(0); rank < 13; rank++ {
		cards = append(cards, NewCard(rank, Spades))
	}

	hand := NewHand(cards...)

	// Check spades mask has all 13 bits set
	spadesMask := hand.GetSuitMask(Spades)
	if spadesMask != 0x1FFF { // 13 bits all set
		t.Errorf("Expected all spades, got mask %016b", spadesMask)
	}

	// Check other suits are empty
	if hand.GetSuitMask(Hearts) != 0 {
		t.Error("Hearts should be empty")
	}
}

func TestDeck(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(42))
	deck := NewDeck(rng)

	// Deal some cards
	cards1 := deck.Deal(2)
	if len(cards1) != 2 {
		t.Errorf("Expected 2 cards, got %d", len(cards1))
	}

	cards2 := deck.Deal(3)
	if len(cards2) != 3 {
		t.Errorf("Expected 3 cards, got %d", len(cards2))
	}

	// Cards should be different
	for _, c1 := range cards1 {
		for _, c2 := range cards2 {
			if c1 == c2 {
				t.Error("Dealt same card twice")
			}
		}
	}

	// Deal remaining cards
	remaining := deck.Deal(47)
	if len(remaining) != 47 {
		t.Errorf("Expected 47 remaining cards, got %d", len(remaining))
	}

	// Should not be able to deal more
	extra := deck.Deal(1)
	if extra != nil {
		t.Error("Should not be able to deal from empty deck")
	}

	// Reset and deal again
	deck.Reset()
	newCards := deck.Deal(2)
	if len(newCards) != 2 {
		t.Error("Should be able to deal after reset")
	}
}

func BenchmarkCardCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewCard(Ace, Spades)
	}
}

func BenchmarkCardString(b *testing.B) {
	card := NewCard(Ace, Spades)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = card.String()
	}
}

func BenchmarkParseCard(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = ParseCard("As")
	}
}

func BenchmarkHandOperations(b *testing.B) {
	c1 := NewCard(Ace, Spades)
	c2 := NewCard(King, Hearts)
	c3 := NewCard(Queen, Diamonds)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hand := NewHand(c1, c2)
		hand.AddCard(c3)
		_ = hand.CountCards()
		_ = hand.HasCard(c1)
	}
}
