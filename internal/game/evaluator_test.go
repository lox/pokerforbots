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

// TestEvaluatorBugs tests specific bugs found by oracle review
func TestEvaluatorBugs(t *testing.T) {
	t.Run("overlapping bit fields bug - pair rankings", func(t *testing.T) {
		// Pair of 2s with kickers A-K-Q should lose to pair of 3s with any kickers
		// But due to bit overlap, the Ace/King bits can interfere with pair ranking
		hand1 := parseCards("2s", "2h", "As", "Kd", "Qc", "9h", "7d") // Pair of 2s with A-K-Q
		hand2 := parseCards("3s", "3h", "8d", "7c", "6s", "5h", "4d") // Pair of 3s with 8-7-6

		rank1 := Evaluate7Cards(hand1)
		rank2 := Evaluate7Cards(hand2)

		// Hand2 should win (higher pair)
		if CompareHands(rank1, rank2) > 0 {
			t.Errorf("Pair of 3s should beat pair of 2s, but got rank1=%d > rank2=%d", rank1, rank2)
			t.Logf("Hand1 (2s pair): %032b", uint32(rank1))
			t.Logf("Hand2 (3s pair): %032b", uint32(rank2))
		}
	})

	t.Run("trips over trips full house bug", func(t *testing.T) {
		// 7-7-7-5-5-5-K should be a full house (sevens full of fives)
		// But evaluator only looks for pairs as the second component
		hand := parseCards("7s", "7h", "7d", "5s", "5h", "5d", "Kc")
		rank := Evaluate7Cards(hand)

		if rank.Type() != FullHouse {
			t.Errorf("Expected full house for trips over trips, got %v", rank.Type())
		}
	})

	t.Run("kicker ordering bug - high card", func(t *testing.T) {
		// A-K-Q-J-9 should beat A-K-Q-T-8 
		// But bitset comparison loses lexicographic order
		hand1 := parseCards("As", "Kh", "Qd", "Jc", "9s", "7h", "2d") // A-K-Q-J-9
		hand2 := parseCards("Ah", "Kd", "Qs", "Tc", "8h", "6s", "3d") // A-K-Q-T-8

		rank1 := Evaluate7Cards(hand1)
		rank2 := Evaluate7Cards(hand2)

		// Hand1 should win (Jack beats Ten as 4th kicker)
		if CompareHands(rank1, rank2) <= 0 {
			t.Errorf("A-K-Q-J-9 should beat A-K-Q-T-8, but got rank1=%d <= rank2=%d", rank1, rank2)
			t.Logf("Hand1 (A-K-Q-J-9): %032b", uint32(rank1))
			t.Logf("Hand2 (A-K-Q-T-8): %032b", uint32(rank2))
		}
	})

	t.Run("kicker ordering bug - flush", func(t *testing.T) {
		// A♠-K♠-Q♠-J♠-9♠ should beat A♠-K♠-Q♠-T♠-8♠
		// But bitset loses ordering
		hand1 := parseCards("As", "Ks", "Qs", "Js", "9s", "7h", "2d") // A-K-Q-J-9 flush
		hand2 := parseCards("Ah", "Kh", "Qh", "Th", "8h", "6s", "3d") // A-K-Q-T-8 flush

		rank1 := Evaluate7Cards(hand1)
		rank2 := Evaluate7Cards(hand2)

		// Hand1 should win (Jack beats Ten as 4th card)
		if CompareHands(rank1, rank2) <= 0 {
			t.Errorf("A-K-Q-J-9 flush should beat A-K-Q-T-8 flush, but got rank1=%d <= rank2=%d", rank1, rank2)
		}
	})

	t.Run("multiple flush suits bug", func(t *testing.T) {
		// If two suits both have flushes, should pick the better one
		// This is hard to test with 7 cards having 2 different 5-card flushes
		// So we'll test the checkFlush function behavior - it returns first suit found
		hand1 := parseCards("As", "Ks", "Qs", "Ts", "8s", "Ah", "9h") // 5 spades (A-K-Q-T-8), 2 hearts
		hand2 := parseCards("Ah", "Kh", "Qh", "Jh", "9h", "As", "8s") // 5 hearts (A-K-Q-J-9), 2 spades

		rank1 := Evaluate7Cards(hand1)
		rank2 := Evaluate7Cards(hand2)

		// Both should be flushes but rank2 should be higher (better flush)
		if rank1.Type() != Flush || rank2.Type() != Flush {
			t.Errorf("Both should be flushes, got %v and %v", rank1.Type(), rank2.Type())
		}
		if CompareHands(rank2, rank1) <= 0 {
			t.Errorf("Better flush should win, but got rank2=%d <= rank1=%d", rank2, rank1)
		}
	})

	t.Run("pair kicker overflow bug", func(t *testing.T) {
		// Test that kickers for pairs don't overflow into pair rank bits
		hand1 := parseCards("2s", "2h", "As", "Ks", "Qs", "Jd", "9c") // Pair of 2s
		hand2 := parseCards("3s", "3h", "4d", "5c", "6s", "7h", "8d") // Pair of 3s

		rank1 := Evaluate7Cards(hand1)
		rank2 := Evaluate7Cards(hand2)

		// Pair of 3s should always beat pair of 2s regardless of kickers
		if CompareHands(rank2, rank1) <= 0 {
			t.Errorf("Pair of 3s should beat pair of 2s, got rank2=%d <= rank1=%d", rank2, rank1)
		}
	})

	t.Run("two pair kicker overflow bug", func(t *testing.T) {
		// Similar test for two pair - kickers shouldn't affect pair rankings
		hand1 := parseCards("2s", "2h", "3d", "3c", "As", "Ks", "Qd") // 2s and 3s with A kicker
		hand2 := parseCards("4s", "4h", "5d", "5c", "6s", "7h", "8d") // 4s and 5s with 8 kicker

		rank1 := Evaluate7Cards(hand1)
		rank2 := Evaluate7Cards(hand2)

		// Higher two pair should win regardless of kicker
		if CompareHands(rank2, rank1) <= 0 {
			t.Errorf("Higher two pair should win, got rank2=%d <= rank1=%d", rank2, rank1)
		}
	})
}
