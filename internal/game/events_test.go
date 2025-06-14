package game

import (
	"strings"
	"testing"
	"time"

	"github.com/lox/pokerforbots/internal/deck"
)

func TestEventFormatter_FormatPlayerAction(t *testing.T) {
	tests := []struct {
		name     string
		opts     FormattingOptions
		event    PlayerActionEvent
		expected string
	}{
		{
			name: "basic fold",
			opts: FormattingOptions{},
			event: PlayerActionEvent{
				Player:    &Player{Name: "Alice"},
				Action:    Fold,
				Amount:    0,
				PotAfter:  100,
				Reasoning: "Bad cards",
				timestamp: time.Now(),
			},
			expected: "Alice: folds",
		},
		{
			name: "timeout fold with timeout display enabled",
			opts: FormattingOptions{ShowTimeouts: true},
			event: PlayerActionEvent{
				Player:    &Player{Name: "Bob"},
				Action:    Fold,
				Amount:    0,
				PotAfter:  100,
				Reasoning: "Decision timeout",
				timestamp: time.Now(),
			},
			expected: "Bob: times out and folds",
		},
		{
			name: "call with pot info",
			opts: FormattingOptions{},
			event: PlayerActionEvent{
				Player:    &Player{Name: "Charlie"},
				Action:    Call,
				Amount:    20,
				PotAfter:  120,
				Reasoning: "Good odds",
				timestamp: time.Now(),
			},
			expected: "Charlie: calls $20 (pot now: $120)",
		},
		{
			name: "small blind posting",
			opts: FormattingOptions{},
			event: PlayerActionEvent{
				Player:    &Player{Name: "Diana", Position: SmallBlind},
				Action:    Call,
				Amount:    5,
				Round:     PreFlop,
				PotAfter:  5,
				Reasoning: "small blind",
				timestamp: time.Now(),
			},
			expected: "Diana: posts small blind $5",
		},
		{
			name: "big blind posting",
			opts: FormattingOptions{},
			event: PlayerActionEvent{
				Player:    &Player{Name: "Eve", Position: BigBlind},
				Action:    Call,
				Amount:    10,
				Round:     PreFlop,
				PotAfter:  15,
				Reasoning: "big blind",
				timestamp: time.Now(),
			},
			expected: "Eve: posts big blind $10",
		},
		{
			name: "check",
			opts: FormattingOptions{},
			event: PlayerActionEvent{
				Player:    &Player{Name: "Frank"},
				Action:    Check,
				Amount:    0,
				PotAfter:  100,
				Reasoning: "No bet to call",
				timestamp: time.Now(),
			},
			expected: "Frank: checks",
		},
		{
			name: "timeout check with timeout display",
			opts: FormattingOptions{ShowTimeouts: true},
			event: PlayerActionEvent{
				Player:    &Player{Name: "Grace"},
				Action:    Check,
				Amount:    0,
				PotAfter:  100,
				Reasoning: "timeout",
				timestamp: time.Now(),
			},
			expected: "Grace: times out and checks",
		},
		{
			name: "raise",
			opts: FormattingOptions{},
			event: PlayerActionEvent{
				Player:    &Player{Name: "Henry"},
				Action:    Raise,
				Amount:    50,
				PotAfter:  200,
				Reasoning: "Strong hand",
				timestamp: time.Now(),
			},
			expected: "Henry: raises $50 (pot now: $200)",
		},
		{
			name: "all-in",
			opts: FormattingOptions{},
			event: PlayerActionEvent{
				Player:    &Player{Name: "Iris"},
				Action:    AllIn,
				Amount:    100,
				PotAfter:  250,
				Reasoning: "Desperate move",
				timestamp: time.Now(),
			},
			expected: "Iris: goes all-in for $100 (pot now: $250)",
		},
		{
			name: "action with reasoning displayed",
			opts: FormattingOptions{ShowReasonings: true},
			event: PlayerActionEvent{
				Player:    &Player{Name: "Jack"},
				Action:    Raise,
				Amount:    30,
				PotAfter:  130,
				Reasoning: "Bluff attempt",
				timestamp: time.Now(),
			},
			expected: "Jack: raises $30 (pot now: $130) (Bluff attempt)",
		},
		{
			name: "sit out action",
			opts: FormattingOptions{},
			event: PlayerActionEvent{
				Player:    &Player{Name: "Kate"},
				Action:    SitOut,
				Amount:    0,
				PotAfter:  100,
				Reasoning: "Taking a break",
				timestamp: time.Now(),
			},
			expected: "Kate: sits out",
		},
		{
			name: "sit in action",
			opts: FormattingOptions{},
			event: PlayerActionEvent{
				Player:    &Player{Name: "Liam"},
				Action:    SitIn,
				Amount:    0,
				PotAfter:  100,
				Reasoning: "Ready to play",
				timestamp: time.Now(),
			},
			expected: "Liam: sits back in",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewEventFormatter(tt.opts)
			result := formatter.FormatPlayerAction(tt.event)
			if result != tt.expected {
				t.Errorf("FormatPlayerAction() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestEventFormatter_FormatStreetChange(t *testing.T) {
	// Create test cards
	cards := []deck.Card{
		{Rank: deck.Ace, Suit: deck.Hearts},
		{Rank: deck.King, Suit: deck.Spades},
		{Rank: deck.Queen, Suit: deck.Diamonds},
		{Rank: deck.Jack, Suit: deck.Clubs},
		{Rank: deck.Ten, Suit: deck.Hearts},
	}

	tests := []struct {
		name     string
		event    StreetChangeEvent
		expected string
	}{
		{
			name: "flop with cards",
			event: StreetChangeEvent{
				Round:          Flop,
				CommunityCards: cards[:3],
				CurrentBet:     0,
				timestamp:      time.Now(),
			},
			expected: "\n\033[1m*** FLOP ***\033[0m [\033[31mA♥\033[0m \033[30mK♠\033[0m \033[31mQ♦\033[0m]",
		},
		{
			name: "turn with cards",
			event: StreetChangeEvent{
				Round:          Turn,
				CommunityCards: cards[:4],
				CurrentBet:     0,
				timestamp:      time.Now(),
			},
			expected: "\n\033[1m*** TURN ***\033[0m [\033[31mA♥\033[0m \033[30mK♠\033[0m \033[31mQ♦\033[0m] \033[30mJ♣\033[0m",
		},
		{
			name: "river with cards",
			event: StreetChangeEvent{
				Round:          River,
				CommunityCards: cards[:5],
				CurrentBet:     0,
				timestamp:      time.Now(),
			},
			expected: "\n\033[1m*** RIVER ***\033[0m [\033[31mA♥\033[0m \033[30mK♠\033[0m \033[31mQ♦\033[0m \033[30mJ♣\033[0m] \033[31mT♥\033[0m",
		},
		{
			name: "showdown with cards",
			event: StreetChangeEvent{
				Round:          Showdown,
				CommunityCards: cards[:5],
				CurrentBet:     0,
				timestamp:      time.Now(),
			},
			expected: "\n\033[1m*** SHOWDOWN ***\033[0m [\033[31mA♥\033[0m \033[30mK♠\033[0m \033[31mQ♦\033[0m \033[30mJ♣\033[0m \033[31mT♥\033[0m]",
		},
		{
			name: "flop without cards",
			event: StreetChangeEvent{
				Round:          Flop,
				CommunityCards: []deck.Card{},
				CurrentBet:     0,
				timestamp:      time.Now(),
			},
			expected: "\n\033[1m*** FLOP ***\033[0m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewEventFormatter(FormattingOptions{})
			result := formatter.FormatStreetChange(tt.event)
			if result != tt.expected {
				t.Errorf("FormatStreetChange() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestEventFormatter_FormatHandStart(t *testing.T) {
	players := []*Player{
		{Name: "Alice", Chips: 1000},
		{Name: "Bob", Chips: 1000},
		{Name: "Charlie", Chips: 1000},
	}

	event := HandStartEvent{
		HandID:        "test-hand-123",
		Players:       players,
		ActivePlayers: players,
		SmallBlind:    5,
		BigBlind:      10,
		InitialPot:    0,
		timestamp:     time.Now(),
	}

	formatter := NewEventFormatter(FormattingOptions{})
	result := formatter.FormatHandStart(event)

	// Check that the result contains the expected information
	if !strings.Contains(result, "Hand test-hand-123") {
		t.Errorf("FormatHandStart() should contain hand ID, got %q", result)
	}
	if !strings.Contains(result, "3 players • $5/$10") {
		t.Errorf("FormatHandStart() should contain player count and stakes, got %q", result)
	}

	// Check that it has the simple structure
	lines := strings.Split(result, "\n")
	if len(lines) != 2 {
		t.Errorf("FormatHandStart() should have 2 lines (hand, details), got %d", len(lines))
	}

	// Check that the hand line includes bold formatting
	if !strings.Contains(result, "\033[1m") || !strings.Contains(result, "\033[0m") {
		t.Errorf("FormatHandStart() should include bold formatting for Hand, got %q", result)
	}
}

func TestEventFormatter_FormatHandEnd(t *testing.T) {
	holeCards := []deck.Card{
		{Rank: deck.Ace, Suit: deck.Hearts},
		{Rank: deck.King, Suit: deck.Spades},
	}

	tests := []struct {
		name     string
		opts     FormattingOptions
		event    HandEndEvent
		expected []string // Expected strings that should be present
	}{
		{
			name: "hand end without hole cards",
			opts: FormattingOptions{ShowHoleCards: false},
			event: HandEndEvent{
				HandID:  "test-hand-456",
				PotSize: 200,
				Winners: []WinnerInfo{
					{
						PlayerName: "Alice",
						Amount:     200,
						HandRank:   "Pair of Aces",
						HoleCards:  holeCards,
					},
				},
				timestamp: time.Now(),
			},
			expected: []string{
				"=== Hand test-hand-456 Complete ===",
				"Pot: $200",
				"Winner: Alice ($200) - Pair of Aces",
			},
		},
		{
			name: "hand end with hole cards",
			opts: FormattingOptions{ShowHoleCards: true},
			event: HandEndEvent{
				HandID:  "test-hand-789",
				PotSize: 150,
				Winners: []WinnerInfo{
					{
						PlayerName: "Bob",
						Amount:     150,
						HandRank:   "High Card",
						HoleCards:  holeCards,
					},
				},
				timestamp: time.Now(),
			},
			expected: []string{
				"=== Hand test-hand-789 Complete ===",
				"Pot: $150",
				"Winner: Bob ($150) - High Card [A♥ K♠]",
			},
		},
		{
			name: "multiple winners",
			opts: FormattingOptions{},
			event: HandEndEvent{
				HandID:  "test-hand-999",
				PotSize: 300,
				Winners: []WinnerInfo{
					{PlayerName: "Alice", Amount: 150, HandRank: "Pair of Kings"},
					{PlayerName: "Bob", Amount: 150, HandRank: "Pair of Kings"},
				},
				timestamp: time.Now(),
			},
			expected: []string{
				"=== Hand test-hand-999 Complete ===",
				"Pot: $300",
				"Winner: Alice ($150) - Pair of Kings",
				"Winner: Bob ($150) - Pair of Kings",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewEventFormatter(tt.opts)
			result := formatter.FormatHandEnd(tt.event)

			for _, expectedStr := range tt.expected {
				if !strings.Contains(result, expectedStr) {
					t.Errorf("FormatHandEnd() result missing expected string %q\nGot: %q", expectedStr, result)
				}
			}
		})
	}
}

func TestEventFormatter_FormatHoleCards(t *testing.T) {
	holeCards := []deck.Card{
		{Rank: deck.Ace, Suit: deck.Hearts},
		{Rank: deck.King, Suit: deck.Spades},
	}

	tests := []struct {
		name       string
		opts       FormattingOptions
		playerName string
		cards      []deck.Card
		expected   string
	}{
		{
			name:       "show cards for perspective player",
			opts:       FormattingOptions{Perspective: "Alice"},
			playerName: "Alice",
			cards:      holeCards,
			expected:   "Dealt to Alice: [\033[31mA♥\033[0m \033[30mK♠\033[0m]",
		},
		{
			name:       "hide cards for non-perspective player",
			opts:       FormattingOptions{Perspective: "Alice"},
			playerName: "Bob",
			cards:      holeCards,
			expected:   "",
		},
		{
			name:       "show all cards when ShowHoleCards is true",
			opts:       FormattingOptions{ShowHoleCards: true, Perspective: "Alice"},
			playerName: "Bob",
			cards:      holeCards,
			expected:   "Dealt to Bob: [\033[31mA♥\033[0m \033[30mK♠\033[0m]",
		},
		{
			name:       "empty cards",
			opts:       FormattingOptions{ShowHoleCards: true},
			playerName: "Charlie",
			cards:      []deck.Card{},
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewEventFormatter(tt.opts)
			result := formatter.FormatHoleCards(tt.playerName, tt.cards)
			if result != tt.expected {
				t.Errorf("FormatHoleCards() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestEventFormatter_isBlindPosting(t *testing.T) {
	tests := []struct {
		name     string
		event    PlayerActionEvent
		expected bool
	}{
		{
			name: "small blind posting with reasoning",
			event: PlayerActionEvent{
				Player:    &Player{Position: SmallBlind},
				Action:    Call,
				Round:     PreFlop,
				Reasoning: "small blind",
			},
			expected: true,
		},
		{
			name: "big blind posting with reasoning",
			event: PlayerActionEvent{
				Player:    &Player{Position: BigBlind},
				Action:    Call,
				Round:     PreFlop,
				Reasoning: "big blind",
			},
			expected: true,
		},
		{
			name: "regular call from small blind position without blind reasoning",
			event: PlayerActionEvent{
				Player:    &Player{Position: SmallBlind},
				Action:    Call,
				Round:     PreFlop,
				Reasoning: "good odds",
			},
			expected: false,
		},
		{
			name: "call on flop",
			event: PlayerActionEvent{
				Player: &Player{Position: SmallBlind},
				Action: Call,
				Round:  Flop,
			},
			expected: false,
		},
		{
			name: "raise action (not call)",
			event: PlayerActionEvent{
				Player:    &Player{Position: SmallBlind},
				Action:    Raise,
				Round:     PreFlop,
				Reasoning: "small blind",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewEventFormatter(FormattingOptions{})
			result := formatter.isBlindPosting(tt.event)
			if result != tt.expected {
				t.Errorf("isBlindPosting() = %v, expected %v", result, tt.expected)
			}
		})
	}
}
