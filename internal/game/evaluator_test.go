package game

import (
	"testing"
)

func parseCards(strs ...string) Hand {
	var hand Hand
	for _, s := range strs {
		card, _ := ParseCard(s)
		hand |= Hand(card)
	}
	return hand
}

func TestEvaluateHighCard(t *testing.T) {
	hand := parseCards("As", "Kh", "Qd", "Jc", "9s", "7h", "5d")
	rank := Evaluate7Cards(hand)

	if rank.Type() != HighCard {
		t.Errorf("Expected HighCard, got %s", rank.String())
	}
}

func TestEvaluatePair(t *testing.T) {
	hand := parseCards("As", "Ah", "Kd", "Qc", "Js", "9h", "7d")
	rank := Evaluate7Cards(hand)

	if rank.Type() != Pair {
		t.Errorf("Expected Pair, got %s", rank.String())
	}
}

func TestEvaluateTwoPair(t *testing.T) {
	hand := parseCards("As", "Ah", "Kd", "Kc", "Qs", "9h", "7d")
	rank := Evaluate7Cards(hand)

	if rank.Type() != TwoPair {
		t.Errorf("Expected TwoPair, got %s", rank.String())
	}
}

func TestEvaluateThreeOfAKind(t *testing.T) {
	hand := parseCards("As", "Ah", "Ad", "Kc", "Qs", "9h", "7d")
	rank := Evaluate7Cards(hand)

	if rank.Type() != ThreeOfAKind {
		t.Errorf("Expected ThreeOfAKind, got %s", rank.String())
	}
}

func TestEvaluateStraight(t *testing.T) {
	// Regular straight
	hand := parseCards("As", "Kh", "Qd", "Jc", "Ts", "9h", "7d")
	rank := Evaluate7Cards(hand)

	if rank.Type() != Straight {
		t.Errorf("Expected Straight, got %s", rank.String())
	}

	// Ace-low straight (wheel)
	hand2 := parseCards("As", "2h", "3d", "4c", "5s", "Kh", "Qd")
	rank2 := Evaluate7Cards(hand2)

	if rank2.Type() != Straight {
		t.Errorf("Expected Straight (wheel), got %s", rank2.String())
	}
}

func TestEvaluateFlush(t *testing.T) {
	hand := parseCards("As", "Ks", "Qs", "Js", "9s", "7h", "5d")
	rank := Evaluate7Cards(hand)

	if rank.Type() != Flush {
		t.Errorf("Expected Flush, got %s", rank.String())
	}
}

func TestEvaluateFullHouse(t *testing.T) {
	hand := parseCards("As", "Ah", "Ad", "Kc", "Kh", "9h", "7d")
	rank := Evaluate7Cards(hand)

	if rank.Type() != FullHouse {
		t.Errorf("Expected FullHouse, got %s", rank.String())
	}
}

func TestEvaluateFourOfAKind(t *testing.T) {
	hand := parseCards("As", "Ah", "Ad", "Ac", "Ks", "9h", "7d")
	rank := Evaluate7Cards(hand)

	if rank.Type() != FourOfAKind {
		t.Errorf("Expected FourOfAKind, got %s", rank.String())
	}
}

func TestEvaluateStraightFlush(t *testing.T) {
	hand := parseCards("As", "Ks", "Qs", "Js", "Ts", "9s", "7h")
	rank := Evaluate7Cards(hand)

	if rank.Type() != StraightFlush {
		t.Errorf("Expected StraightFlush, got %s", rank.String())
	}
}

func TestCompareHands(t *testing.T) {
	tests := []struct {
		name     string
		hand1    Hand
		hand2    Hand
		expected int // 1 if hand1 wins, -1 if hand2 wins, 0 for tie
	}{
		{
			name:     "Pair beats high card",
			hand1:    parseCards("As", "Ah", "Kd", "Qc", "Js", "9h", "7d"),
			hand2:    parseCards("As", "Kh", "Qd", "Jc", "9s", "7h", "5d"),
			expected: 1,
		},
		{
			name:     "Higher pair beats lower pair",
			hand1:    parseCards("As", "Ah", "Kd", "Qc", "Js", "9h", "7d"),
			hand2:    parseCards("Ks", "Kh", "Qd", "Jc", "9s", "7h", "5d"),
			expected: 1,
		},
		{
			name:     "Flush beats straight",
			hand1:    parseCards("As", "Ks", "Qs", "Js", "9s", "7h", "5d"),
			hand2:    parseCards("As", "Kh", "Qd", "Jc", "Ts", "9h", "7d"),
			expected: 1,
		},
		{
			name:     "Full house beats flush",
			hand1:    parseCards("As", "Ah", "Ad", "Kc", "Kh", "9h", "7d"),
			hand2:    parseCards("As", "Ks", "Qs", "Js", "9s", "7h", "5d"),
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rank1 := Evaluate7Cards(tt.hand1)
			rank2 := Evaluate7Cards(tt.hand2)
			result := CompareHands(rank1, rank2)

			if result != tt.expected {
				t.Errorf("Expected %d, got %d (hand1: %s, hand2: %s)",
					tt.expected, result, rank1.String(), rank2.String())
			}
		})
	}
}

func BenchmarkEvaluate7Cards(b *testing.B) {
	hand := parseCards("As", "Kh", "Qd", "Jc", "Ts", "9h", "7d")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Evaluate7Cards(hand)
	}
}

func BenchmarkEvaluateFlush(b *testing.B) {
	hand := parseCards("As", "Ks", "Qs", "Js", "9s", "7h", "5d")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Evaluate7Cards(hand)
	}
}

func BenchmarkEvaluateFullHouse(b *testing.B) {
	hand := parseCards("As", "Ah", "Ad", "Kc", "Kh", "9h", "7d")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Evaluate7Cards(hand)
	}
}