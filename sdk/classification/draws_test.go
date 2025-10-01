package classification

import (
	"testing"

	"github.com/lox/pokerforbots/v2/poker"
)

// Helper function to parse card strings for tests
func parseCards(cardStrs []string) poker.Hand {
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

func TestDrawTypeString(t *testing.T) {
	tests := []struct {
		drawType DrawType
		expected string
	}{
		{FlushDraw, "flush draw"},
		{NutFlushDraw, "nut flush draw"},
		{OpenEndedStraightDraw, "open-ended straight draw"},
		{Gutshot, "gutshot"},
		{ComboDraw, "combo draw"},
		{NoDraw, "no draw"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.drawType.String()
			if result != tt.expected {
				t.Errorf("DrawType.String() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDrawInfoMethods(t *testing.T) {
	t.Run("HasStrongDraw", func(t *testing.T) {
		tests := []struct {
			name     string
			draws    []DrawType
			expected bool
		}{
			{
				name:     "flush draw is strong",
				draws:    []DrawType{FlushDraw},
				expected: true,
			},
			{
				name:     "nut flush draw is strong",
				draws:    []DrawType{NutFlushDraw},
				expected: true,
			},
			{
				name:     "OESD is strong",
				draws:    []DrawType{OpenEndedStraightDraw},
				expected: true,
			},
			{
				name:     "combo draw is strong",
				draws:    []DrawType{ComboDraw},
				expected: true,
			},
			{
				name:     "gutshot is not strong",
				draws:    []DrawType{Gutshot},
				expected: false,
			},
			{
				name:     "no draw is not strong",
				draws:    []DrawType{NoDraw},
				expected: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				info := DrawInfo{Draws: tt.draws}
				result := info.HasStrongDraw()
				if result != tt.expected {
					t.Errorf("HasStrongDraw() = %v, want %v", result, tt.expected)
				}
			})
		}
	})

	t.Run("HasWeakDraw", func(t *testing.T) {
		tests := []struct {
			name     string
			draws    []DrawType
			expected bool
		}{
			{
				name:     "gutshot is weak",
				draws:    []DrawType{Gutshot},
				expected: true,
			},
			{
				name:     "backdoor flush is weak",
				draws:    []DrawType{BackdoorFlush},
				expected: true,
			},
			{
				name:     "overcards is weak",
				draws:    []DrawType{Overcards},
				expected: true,
			},
			{
				name:     "flush draw is not weak",
				draws:    []DrawType{FlushDraw},
				expected: false,
			},
			{
				name:     "no draw is not weak",
				draws:    []DrawType{NoDraw},
				expected: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				info := DrawInfo{Draws: tt.draws}
				result := info.HasWeakDraw()
				if result != tt.expected {
					t.Errorf("HasWeakDraw() = %v, want %v", result, tt.expected)
				}
			})
		}
	})

	t.Run("IsComboDraw", func(t *testing.T) {
		tests := []struct {
			name     string
			draws    []DrawType
			outs     int
			expected bool
		}{
			{
				name:     "multiple draws with many outs",
				draws:    []DrawType{FlushDraw, OpenEndedStraightDraw},
				outs:     15,
				expected: true,
			},
			{
				name:     "multiple draws with few outs",
				draws:    []DrawType{Gutshot, Overcards},
				outs:     7,
				expected: false,
			},
			{
				name:     "single draw with many outs",
				draws:    []DrawType{FlushDraw},
				outs:     9,
				expected: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				info := DrawInfo{Draws: tt.draws, Outs: tt.outs}
				result := info.IsComboDraw()
				if result != tt.expected {
					t.Errorf("IsComboDraw() = %v, want %v", result, tt.expected)
				}
			})
		}
	})
}

func TestDetectDraws(t *testing.T) {
	tests := []struct {
		name       string
		holeCards  []string
		board      []string
		hasFlush   bool
		hasOESD    bool
		hasGutshot bool
	}{
		{
			name:       "no draws",
			holeCards:  []string{"As", "7h"},
			board:      []string{"2c", "9d", "Kh"},
			hasFlush:   false,
			hasOESD:    false,
			hasGutshot: false,
		},
		{
			name:       "flush draw",
			holeCards:  []string{"As", "7s"},
			board:      []string{"2s", "9d", "Kh"},
			hasFlush:   true,
			hasOESD:    false,
			hasGutshot: false,
		},
		{
			name:       "open ended straight draw",
			holeCards:  []string{"8h", "9c"},
			board:      []string{"Ts", "Jd", "2h"},
			hasFlush:   false,
			hasOESD:    true,
			hasGutshot: false,
		},
		{
			name:       "gutshot",
			holeCards:  []string{"8h", "6c"},
			board:      []string{"5s", "9d", "2h"},
			hasFlush:   false,
			hasOESD:    false,
			hasGutshot: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			holeCards := parseCards(tt.holeCards)
			board := parseCards(tt.board)
			result := DetectDraws(holeCards, board)

			hasFlush := false
			hasOESD := false
			hasGutshot := false

			for _, draw := range result.Draws {
				switch draw {
				case FlushDraw, NutFlushDraw:
					hasFlush = true
				case OpenEndedStraightDraw:
					hasOESD = true
				case Gutshot:
					hasGutshot = true
				}
			}

			if hasFlush != tt.hasFlush {
				t.Errorf("Expected flush draw: %v, got: %v", tt.hasFlush, hasFlush)
			}

			if hasOESD != tt.hasOESD {
				t.Errorf("Expected OESD: %v, got: %v", tt.hasOESD, hasOESD)
			}

			if hasGutshot != tt.hasGutshot {
				t.Errorf("Expected gutshot: %v, got: %v", tt.hasGutshot, hasGutshot)
			}
		})
	}
}

func TestDetectFlushDraw(t *testing.T) {
	tests := []struct {
		name          string
		holeCards     []string
		board         []string
		expectedFlush bool
		expectedNut   bool
	}{
		{
			name:          "no flush draw",
			holeCards:     []string{"As", "7h"},
			board:         []string{"2c", "9d", "Kh"},
			expectedFlush: false,
			expectedNut:   false,
		},
		{
			name:          "flush draw",
			holeCards:     []string{"As", "7s"},
			board:         []string{"2s", "9d", "Kh"},
			expectedFlush: true,
			expectedNut:   true, // Ace high flush draw
		},
		{
			name:          "non-nut flush draw",
			holeCards:     []string{"7s", "6s"},
			board:         []string{"2s", "9d", "Kh"},
			expectedFlush: true,
			expectedNut:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			holeCards := parseCards(tt.holeCards)
			board := parseCards(tt.board)
			result := detectFlushDraw(holeCards, board)

			if result.HasFlushDraw != tt.expectedFlush {
				t.Errorf("HasFlushDraw = %v, want %v", result.HasFlushDraw, tt.expectedFlush)
			}

			if result.IsNutFlushDraw != tt.expectedNut {
				t.Errorf("IsNutFlushDraw = %v, want %v", result.IsNutFlushDraw, tt.expectedNut)
			}
		})
	}
}

func TestDetectOvercards(t *testing.T) {
	tests := []struct {
		name         string
		holeCards    []string
		board        []string
		hasOvercards bool
		expectedOuts int
	}{
		{
			name:         "no overcards",
			holeCards:    []string{"5s", "7h"},
			board:        []string{"Ac", "Kd", "Qh"},
			hasOvercards: false,
			expectedOuts: 0,
		},
		{
			name:         "one overcard",
			holeCards:    []string{"As", "7h"},
			board:        []string{"Tc", "9d", "8h"},
			hasOvercards: true,
			expectedOuts: 3, // 3 remaining Aces
		},
		{
			name:         "two overcards",
			holeCards:    []string{"As", "Kh"},
			board:        []string{"Tc", "9d", "8h"},
			hasOvercards: true,
			expectedOuts: 6, // 3 Aces + 3 Kings
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			holeCards := parseCards(tt.holeCards)
			board := parseCards(tt.board)
			allCards := holeCards | board
			result := detectOvercards(holeCards, board, allCards)

			if result.HasOvercards != tt.hasOvercards {
				t.Errorf("HasOvercards = %v, want %v", result.HasOvercards, tt.hasOvercards)
			}

			outs := result.OutsMask.CountCards()
			if outs != tt.expectedOuts {
				t.Errorf("Outs = %v, want %v", outs, tt.expectedOuts)
			}
		})
	}
}
