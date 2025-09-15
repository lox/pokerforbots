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

func TestEvaluateHandTypes(t *testing.T) {
	tests := []struct {
		name     string
		cards    []string
		expected HandRank
	}{
		{
			name:     "high card",
			cards:    []string{"As", "Kh", "Qd", "Jc", "9s", "7h", "5d"},
			expected: HighCard,
		},
		{
			name:     "pair",
			cards:    []string{"As", "Ah", "Kd", "Qc", "Js", "9h", "7d"},
			expected: Pair,
		},
		{
			name:     "two pair",
			cards:    []string{"As", "Ah", "Kd", "Kc", "Qs", "9h", "7d"},
			expected: TwoPair,
		},
		{
			name:     "three of a kind",
			cards:    []string{"As", "Ah", "Ad", "Kc", "Qs", "9h", "7d"},
			expected: ThreeOfAKind,
		},
		{
			name:     "straight - broadway",
			cards:    []string{"As", "Kh", "Qd", "Jc", "Ts", "9h", "7d"},
			expected: Straight,
		},
		{
			name:     "straight - wheel",
			cards:    []string{"As", "2h", "3d", "4c", "5s", "Kh", "Qd"},
			expected: Straight,
		},
		{
			name:     "flush",
			cards:    []string{"As", "Ks", "Qs", "Js", "9s", "7h", "5d"},
			expected: Flush,
		},
		{
			name:     "full house",
			cards:    []string{"As", "Ah", "Ad", "Kc", "Kh", "9h", "7d"},
			expected: FullHouse,
		},
		{
			name:     "four of a kind",
			cards:    []string{"As", "Ah", "Ad", "Ac", "Ks", "9h", "7d"},
			expected: FourOfAKind,
		},
		{
			name:     "straight flush",
			cards:    []string{"As", "Ks", "Qs", "Js", "Ts", "9s", "7h"},
			expected: StraightFlush,
		},
		{
			name:     "royal flush",
			cards:    []string{"As", "Ks", "Qs", "Js", "Ts", "9h", "7d"},
			expected: StraightFlush, // Royal flush is just the highest straight flush
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hand := parseCards(tt.cards...)
			rank := Evaluate7Cards(hand)
			if rank.Type() != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, rank.Type())
			}
		})
	}
}

func TestCompareHands(t *testing.T) {
	tests := []struct {
		name           string
		hand1Cards     []string
		hand2Cards     []string
		expectedResult int // 1 if hand1 wins, -1 if hand2 wins, 0 if tie
	}{
		{
			name:           "pair beats high card",
			hand1Cards:     []string{"As", "Ah", "Kd", "Qc", "Js", "9h", "7d"},
			hand2Cards:     []string{"As", "Kh", "Qd", "Jc", "9s", "7h", "5d"},
			expectedResult: 1,
		},
		{
			name:           "higher pair beats lower pair",
			hand1Cards:     []string{"As", "Ah", "Kd", "Qc", "Js", "9h", "7d"},
			hand2Cards:     []string{"Ks", "Kh", "Qd", "Jc", "9s", "7h", "5d"},
			expectedResult: 1,
		},
		{
			name:           "flush beats straight",
			hand1Cards:     []string{"As", "Ks", "Qs", "Js", "9s", "7h", "5d"},
			hand2Cards:     []string{"As", "Kh", "Qd", "Jc", "Ts", "9h", "7d"},
			expectedResult: 1,
		},
		{
			name:           "full house beats flush",
			hand1Cards:     []string{"As", "Ah", "Ad", "Kc", "Kh", "9h", "7d"},
			hand2Cards:     []string{"As", "Ks", "Qs", "Js", "9s", "7h", "5d"},
			expectedResult: 1,
		},
		{
			name:           "kicker matters in pairs",
			hand1Cards:     []string{"As", "Ah", "Kd", "Qc", "Js", "9h", "7d"},
			hand2Cards:     []string{"Ac", "Ad", "Kh", "Qs", "Td", "9c", "7h"},
			expectedResult: 1, // Jack kicker beats Ten kicker
		},
		{
			name:           "identical hands tie",
			hand1Cards:     []string{"As", "Ks", "Qs", "Js", "Ts", "9h", "7d"},
			hand2Cards:     []string{"Ah", "Kh", "Qh", "Jh", "Th", "9c", "7s"},
			expectedResult: 0, // Both have K-high straights
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hand1 := parseCards(tt.hand1Cards...)
			hand2 := parseCards(tt.hand2Cards...)
			rank1 := Evaluate7Cards(hand1)
			rank2 := Evaluate7Cards(hand2)
			result := CompareHands(rank1, rank2)

			if result > 0 && tt.expectedResult != 1 {
				t.Errorf("Expected hand2 to win or tie, but hand1 won")
			} else if result < 0 && tt.expectedResult != -1 {
				t.Errorf("Expected hand1 to win or tie, but hand2 won")
			} else if result == 0 && tt.expectedResult != 0 {
				t.Errorf("Expected a winner, but got a tie")
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
