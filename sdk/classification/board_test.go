package classification

import (
	"testing"

	"github.com/lox/pokerforbots/v2/poker"
)

// Helper function to parse board cards for tests
func parseBoard(cardStrs []string) poker.Hand {
	var hand poker.Hand
	for _, cardStr := range cardStrs {
		card, err := poker.ParseCard(cardStr)
		if err != nil {
			panic(err) // Test helper - should not fail
		}
		hand.AddCard(card)
	}
	return hand
}

func TestBoardTexture(t *testing.T) {
	tests := []struct {
		name     string
		board    []string
		expected BoardTexture
	}{
		{
			name:     "dry board",
			board:    []string{"As", "7h", "2c"},
			expected: Dry,
		},
		{
			name:     "semi wet board",
			board:    []string{"Kh", "Qh", "7c"},
			expected: SemiWet,
		},
		{
			name:     "wet board",
			board:    []string{"9h", "8h", "7s"},
			expected: Wet,
		},
		{
			name:     "very wet board",
			board:    []string{"Th", "9h", "8h"},
			expected: VeryWet,
		},
		{
			name:     "paired board",
			board:    []string{"As", "Ah", "7c"},
			expected: SemiWet, // Pair adds wetness
		},
		{
			name:     "rainbow dry",
			board:    []string{"As", "7h", "2c", "9d"},
			expected: Dry,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			board := parseBoard(tt.board)
			result := AnalyzeBoardTexture(board)
			if result != tt.expected {
				t.Errorf("AnalyzeBoardTexture(%v) = %v, want %v", tt.board, result, tt.expected)
			}
		})
	}
}

func TestBoardTextureString(t *testing.T) {
	tests := []struct {
		texture  BoardTexture
		expected string
	}{
		{Dry, "dry"},
		{SemiWet, "semi-wet"},
		{Wet, "wet"},
		{VeryWet, "very wet"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.texture.String()
			if result != tt.expected {
				t.Errorf("BoardTexture.String() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestAnalyzeFlushPotential(t *testing.T) {
	spades := poker.Spades
	tests := []struct {
		name     string
		board    []string
		expected FlushInfo
	}{
		{
			name:  "no flush draw",
			board: []string{"As", "7h", "2c"},
			expected: FlushInfo{
				MaxSuitCount: 1,
				DominantSuit: &spades,
				IsMonotone:   false,
				IsRainbow:    true,
			},
		},
		{
			name:  "two suited",
			board: []string{"As", "7s", "2c"},
			expected: FlushInfo{
				MaxSuitCount: 2,
				DominantSuit: &spades,
				IsMonotone:   false,
				IsRainbow:    false,
			},
		},
		{
			name:  "monotone flop",
			board: []string{"As", "7s", "2s"},
			expected: FlushInfo{
				MaxSuitCount: 3,
				DominantSuit: &spades,
				IsMonotone:   true,
				IsRainbow:    false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			board := parseBoard(tt.board)
			result := AnalyzeFlushPotential(board)

			if result.MaxSuitCount != tt.expected.MaxSuitCount {
				t.Errorf("MaxSuitCount = %v, want %v", result.MaxSuitCount, tt.expected.MaxSuitCount)
			}

			if result.IsMonotone != tt.expected.IsMonotone {
				t.Errorf("IsMonotone = %v, want %v", result.IsMonotone, tt.expected.IsMonotone)
			}

			if result.IsRainbow != tt.expected.IsRainbow {
				t.Errorf("IsRainbow = %v, want %v", result.IsRainbow, tt.expected.IsRainbow)
			}

			if tt.expected.DominantSuit != nil && result.DominantSuit != nil {
				if *result.DominantSuit != *tt.expected.DominantSuit {
					t.Errorf("DominantSuit = %v, want %v", *result.DominantSuit, *tt.expected.DominantSuit)
				}
			}
		})
	}
}

func TestAnalyzeStraightPotential(t *testing.T) {
	tests := []struct {
		name     string
		board    []string
		expected StraightInfo
	}{
		{
			name:  "disconnected",
			board: []string{"As", "7h", "2c"},
			expected: StraightInfo{
				ConnectedCards: 1,
				Gaps:           10, // Large gaps between A, 7, 2
				HasAce:         true,
				BroadwayCards:  1, // Just the ace
			},
		},
		{
			name:  "connected straight draw",
			board: []string{"9h", "8s", "7c"},
			expected: StraightInfo{
				ConnectedCards: 3,
				Gaps:           0,
				HasAce:         false,
				BroadwayCards:  0,
			},
		},
		{
			name:  "broadway draw",
			board: []string{"Kh", "Qs", "Jc"},
			expected: StraightInfo{
				ConnectedCards: 3,
				Gaps:           0,
				HasAce:         false,
				BroadwayCards:  3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			board := parseBoard(tt.board)
			result := AnalyzeStraightPotential(board)

			if result.ConnectedCards != tt.expected.ConnectedCards {
				t.Errorf("ConnectedCards = %v, want %v", result.ConnectedCards, tt.expected.ConnectedCards)
			}

			if result.HasAce != tt.expected.HasAce {
				t.Errorf("HasAce = %v, want %v", result.HasAce, tt.expected.HasAce)
			}

			if result.BroadwayCards != tt.expected.BroadwayCards {
				t.Errorf("BroadwayCards = %v, want %v", result.BroadwayCards, tt.expected.BroadwayCards)
			}
		})
	}
}

func TestHelperFunctions(t *testing.T) {
	t.Run("getSuit", func(t *testing.T) {
		tests := []struct {
			card     string
			expected uint8
		}{
			{"As", poker.Spades},
			{"Kh", poker.Hearts},
			{"Qd", poker.Diamonds},
			{"Jc", poker.Clubs},
		}

		for _, tt := range tests {
			card, err := poker.ParseCard(tt.card)
			if err != nil {
				t.Errorf("poker.ParseCard(%v) error: %v", tt.card, err)
				return
			}
			result := card.Suit()
			if result != tt.expected {
				t.Errorf("getSuit(%v) = %v, want %v", tt.card, result, tt.expected)
			}
		}
	})

	t.Run("getRank", func(t *testing.T) {
		tests := []struct {
			card     string
			expected int
		}{
			{"2s", 2},
			{"9h", 9},
			{"Td", 10},
			{"Jc", 11},
			{"Qh", 12},
			{"Ks", 13},
			{"As", 14},
		}

		for _, tt := range tests {
			card, err := poker.ParseCard(tt.card)
			if err != nil {
				t.Errorf("poker.ParseCard(%v) error: %v", tt.card, err)
				return
			}
			result := int(card.Rank()) + 2 // Convert to expected test values
			if result != tt.expected {
				t.Errorf("getRank(%v) = %v, want %v", tt.card, result, tt.expected)
			}
		}
	})

	t.Run("countBoardPairs", func(t *testing.T) {
		tests := []struct {
			board    []string
			expected int
		}{
			{[]string{"As", "7h", "2c"}, 0},
			{[]string{"As", "Ah", "2c"}, 1},
			{[]string{"As", "Ah", "2c", "2d"}, 2},
		}

		for _, tt := range tests {
			board := parseBoard(tt.board)
			result := countBoardPairs(board)
			if result != tt.expected {
				t.Errorf("countBoardPairs(%v) = %v, want %v", tt.board, result, tt.expected)
			}
		}
	})
}
