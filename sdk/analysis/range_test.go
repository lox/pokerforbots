package analysis

import (
	"testing"

	"github.com/lox/pokerforbots/poker"
)

func TestParseRange(t *testing.T) {
	tests := []struct {
		name     string
		notation string
		wantSize int
		wantErr  bool
	}{
		{
			name:     "pocket aces",
			notation: "AA",
			wantSize: 6, // 6 combinations
		},
		{
			name:     "ace king suited",
			notation: "AKs",
			wantSize: 4, // 4 suited combinations
		},
		{
			name:     "ace king offsuit",
			notation: "AKo",
			wantSize: 12, // 12 offsuit combinations
		},
		{
			name:     "ace king any",
			notation: "AK",
			wantSize: 16, // 4 suited + 12 offsuit
		},
		{
			name:     "multiple hands",
			notation: "AA,KK,AKs",
			wantSize: 16, // 6 + 6 + 4
		},
		{
			name:     "pocket pairs range",
			notation: "TT+",
			wantSize: 30, // TT,JJ,QQ,KK,AA = 5 * 6
		},
		{
			name:     "suited range plus",
			notation: "ATs+",
			wantSize: 16, // AT,AJ,AQ,AK suited = 4 * 4
		},
		{
			name:     "offsuit range plus",
			notation: "KJo+",
			wantSize: 24, // KJ,KQ offsuit = 2 * 12
		},
		{
			name:     "dash range pairs",
			notation: "22-55",
			wantSize: 24, // 22,33,44,55 = 4 * 6
		},
		{
			name:     "dash range suited",
			notation: "A5s-A2s",
			wantSize: 16, // A5s,A4s,A3s,A2s = 4 * 4
		},
		{
			name:     "complex range",
			notation: "TT+,AJs+,KQs",
			wantSize: 46, // 30 + 12 + 4 (TT+=30, AJs/AQs/AKs=12, KQs=4)
		},
		{
			name:     "invalid notation",
			notation: "XX",
			wantErr:  true,
		},
		{
			name:     "invalid modifier",
			notation: "AKx",
			wantErr:  true,
		},
		{
			name:     "pocket pair with modifier",
			notation: "AAs",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := ParseRange(tt.notation)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRange() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && r.Size() != tt.wantSize {
				t.Errorf("ParseRange() size = %v, want %v", r.Size(), tt.wantSize)
			}
		})
	}
}

func TestRangeContains(t *testing.T) {
	r, err := ParseRange("AA,KK,AKs")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		card1 string
		card2 string
		want  bool
	}{
		{"Ah", "As", true},  // AA
		{"Kh", "Kd", true},  // KK
		{"Ah", "Kh", true},  // AKs
		{"Ah", "Kd", false}, // AKo not in range
		{"Qh", "Qd", false}, // QQ not in range
	}

	for _, tt := range tests {
		got := r.Contains(tt.card1, tt.card2)
		if got != tt.want {
			t.Errorf("Contains(%s,%s) = %v, want %v", tt.card1, tt.card2, got, tt.want)
		}
	}
}

func TestRangeContainsCards(t *testing.T) {
	r, err := ParseRange("AA,KK,AKs")
	if err != nil {
		t.Fatal(err)
	}

	// Test with poker.Card types
	aceHearts := poker.NewCard(12, 2)    // Ace=12, Hearts=2
	aceSpades := poker.NewCard(12, 3)    // Ace=12, Spades=3
	kingHearts := poker.NewCard(11, 2)   // King=11, Hearts=2
	kingDiamonds := poker.NewCard(11, 1) // King=11, Diamonds=1
	queenHearts := poker.NewCard(10, 2)  // Queen=10, Hearts=2

	tests := []struct {
		card1 poker.Card
		card2 poker.Card
		want  bool
	}{
		{aceHearts, aceSpades, true},      // AA
		{kingHearts, kingDiamonds, true},  // KK
		{aceHearts, kingHearts, true},     // AKs
		{aceHearts, kingDiamonds, false},  // AKo not in range
		{queenHearts, queenHearts, false}, // QQ not in range
	}

	for _, tt := range tests {
		got := r.ContainsCards(tt.card1, tt.card2)
		if got != tt.want {
			t.Errorf("ContainsCards(%v,%v) = %v, want %v", tt.card1, tt.card2, got, tt.want)
		}
	}
}

func TestRangeDashNotation(t *testing.T) {
	// Test pocket pair ranges
	r1, err := ParseRange("22-44")
	if err != nil {
		t.Fatal(err)
	}
	if r1.Size() != 18 { // 3 pairs * 6 combos each
		t.Errorf("22-44 should have 18 combos, got %d", r1.Size())
	}

	// Test suited ranges
	r2, err := ParseRange("K9s-K6s")
	if err != nil {
		t.Fatal(err)
	}
	if r2.Size() != 16 { // 4 hands * 4 suited combos
		t.Errorf("K9s-K6s should have 16 combos, got %d", r2.Size())
	}

	// Test offsuit ranges
	r3, err := ParseRange("A5o-A2o")
	if err != nil {
		t.Fatal(err)
	}
	if r3.Size() != 48 { // 4 hands * 12 offsuit combos
		t.Errorf("A5o-A2o should have 48 combos, got %d", r3.Size())
	}
}

func TestRangePlusNotation(t *testing.T) {
	// Test pocket pairs plus
	r1, err := ParseRange("JJ+")
	if err != nil {
		t.Fatal(err)
	}
	if r1.Size() != 24 { // JJ,QQ,KK,AA = 4 * 6
		t.Errorf("JJ+ should have 24 combos, got %d", r1.Size())
	}

	// Test suited plus
	r2, err := ParseRange("K9s+")
	if err != nil {
		t.Fatal(err)
	}
	if r2.Size() != 16 { // K9s,KTs,KJs,KQs = 4 * 4
		t.Errorf("K9s+ should have 16 combos, got %d", r2.Size())
	}

	// Test any plus
	r3, err := ParseRange("AT+")
	if err != nil {
		t.Fatal(err)
	}
	if r3.Size() != 64 { // AT,AJ,AQ,AK = 4 * 16
		t.Errorf("AT+ should have 64 combos, got %d", r3.Size())
	}
}

func TestRangeHands(t *testing.T) {
	r, err := ParseRange("AA")
	if err != nil {
		t.Fatal(err)
	}

	hands := r.Hands()
	if len(hands) != 6 {
		t.Errorf("AA should have 6 hands, got %d", len(hands))
	}

	// Verify all hands are pocket aces
	for _, hand := range hands {
		if hand.CountCards() != 2 {
			t.Errorf("Expected 2 cards, got %d", hand.CountCards())
			continue
		}
		card1 := hand.GetCard(0)
		card2 := hand.GetCard(1)
		if card1.Rank() != 12 || card2.Rank() != 12 { // Ace = 12
			t.Errorf("Expected pocket aces, got %v %v", card1, card2)
		}
	}
}
