package deck

import "testing"

func TestParseCards(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []Card
		wantErr  bool
	}{
		{
			name:  "royal flush",
			input: "AsKsQsJsTs",
			expected: []Card{
				{Suit: Spades, Rank: Ace},
				{Suit: Spades, Rank: King},
				{Suit: Spades, Rank: Queen},
				{Suit: Spades, Rank: Jack},
				{Suit: Spades, Rank: Ten},
			},
		},
		{
			name:  "mixed suits",
			input: "AhKdQcJs9s",
			expected: []Card{
				{Suit: Hearts, Rank: Ace},
				{Suit: Diamonds, Rank: King},
				{Suit: Clubs, Rank: Queen},
				{Suit: Spades, Rank: Jack},
				{Suit: Spades, Rank: Nine},
			},
		},
		{
			name:  "low cards",
			input: "5h4d3c2s",
			expected: []Card{
				{Suit: Hearts, Rank: Five},
				{Suit: Diamonds, Rank: Four},
				{Suit: Clubs, Rank: Three},
				{Suit: Spades, Rank: Two},
			},
		},
		{
			name:  "case insensitive",
			input: "asKHqDjc",
			expected: []Card{
				{Suit: Spades, Rank: Ace},
				{Suit: Hearts, Rank: King},
				{Suit: Diamonds, Rank: Queen},
				{Suit: Clubs, Rank: Jack},
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
			expected: []Card{},
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
	expected := []Card{
		{Suit: Spades, Rank: Ace},
		{Suit: Spades, Rank: King},
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

func cardsEqual(a, b []Card) bool {
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
