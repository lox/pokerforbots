package poker

import (
	"testing"
)

func TestCategorizeHoleCards(t *testing.T) {
	tests := []struct {
		name     string
		card1    string
		card2    string
		expected HoleCardCategory
	}{
		// Premium hands
		{"Pocket Aces", "As", "Ah", CategoryPremium},
		{"Pocket Kings", "Kh", "Kd", CategoryPremium},
		{"Pocket Queens", "Qc", "Qs", CategoryPremium},
		{"Pocket Jacks", "Jh", "Jd", CategoryPremium},
		{"Ace King suited", "As", "Ks", CategoryPremium},
		{"Ace King offsuit", "Ac", "Kh", CategoryPremium},

		// Strong hands
		{"Pocket Tens", "Tc", "Th", CategoryStrong},
		{"Ace Queen suited", "As", "Qs", CategoryStrong},
		{"Ace Queen offsuit", "Ac", "Qh", CategoryStrong},
		{"Ace Jack suited", "As", "Js", CategoryStrong},
		{"Ace Jack offsuit", "Ad", "Jc", CategoryStrong},

		// Medium hands
		{"Pocket Nines", "9c", "9h", CategoryMedium},
		{"Pocket Eights", "8d", "8s", CategoryMedium},
		{"Pocket Sevens", "7h", "7c", CategoryMedium},
		{"King Queen suited", "Ks", "Qs", CategoryMedium},
		{"King Jack suited", "Kh", "Jh", CategoryMedium},
		{"Queen Jack suited", "Qd", "Jd", CategoryMedium},

		// Weak hands
		{"Pocket Sixes", "6c", "6h", CategoryWeak},
		{"Pocket Fives", "5d", "5s", CategoryWeak},
		{"Pocket Fours", "4h", "4c", CategoryWeak},
		{"Pocket Threes", "3s", "3d", CategoryWeak},
		{"Pocket Twos", "2c", "2h", CategoryWeak},
		{"Suited connectors 76s", "7h", "6h", CategoryWeak},
		{"Suited connectors 54s", "5d", "4d", CategoryWeak},

		// Trash hands
		{"Seven Two offsuit", "7c", "2h", CategoryTrash},
		{"Nine Three offsuit", "9d", "3s", CategoryTrash},
		{"Jack Four offsuit", "Jh", "4c", CategoryTrash},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			card1, err1 := ParseCard(tt.card1)
			card2, err2 := ParseCard(tt.card2)
			if err1 != nil || err2 != nil {
				t.Fatalf("Failed to parse cards: %v, %v", err1, err2)
			}

			result := CategorizeHoleCards(card1, card2)
			if result != tt.expected {
				t.Errorf("CategorizeHoleCards(%s, %s) = %s, want %s",
					tt.card1, tt.card2, result, tt.expected)
			}
		})
	}
}

func TestCategorizeHoleCardsFromStrings(t *testing.T) {
	tests := []struct {
		name     string
		cards    []string
		expected string
	}{
		{"Premium AA", []string{"As", "Ah"}, "Premium"},
		{"Strong AQ", []string{"As", "Qh"}, "Strong"},
		{"Medium 88", []string{"8c", "8h"}, "Medium"},
		{"Weak 22", []string{"2c", "2h"}, "Weak"},
		{"Trash 72o", []string{"7c", "2h"}, "Trash"},
		{"Invalid - too many cards", []string{"As", "Ah", "Ac"}, "Unknown"},
		{"Invalid - too few cards", []string{"As"}, "Unknown"},
		{"Invalid - bad card format", []string{"XX", "YY"}, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CategorizeHoleCardsFromStrings(tt.cards)
			if result != tt.expected {
				t.Errorf("CategorizeHoleCardsFromStrings(%v) = %s, want %s",
					tt.cards, result, tt.expected)
			}
		})
	}
}
