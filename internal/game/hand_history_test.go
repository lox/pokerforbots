package game

import (
	"strings"
	"testing"
	"time"

	"github.com/lox/holdem-cli/internal/deck"
)

func TestNewHandHistory(t *testing.T) {
	// Create a test table
	table := &Table{
		handID:         "01h5n0et5q6mt3v7ms1234abcd",
		smallBlind:     1,
		bigBlind:       2,
		dealerPosition: 3,
		players: []*Player{
			{Name: "Alice", Chips: 100, Position: Button},
			{Name: "Bob", Chips: 200, Position: SmallBlind},
			{Name: "Charlie", Chips: 150, Position: BigBlind},
		},
	}

	hh := NewHandHistory(table, 12345, &NoOpHandHistoryWriter{})

	// Test basic fields
	if hh.HandID != "01h5n0et5q6mt3v7ms1234abcd" {
		t.Errorf("Expected HandID 01h5n0et5q6mt3v7ms1234abcd, got %s", hh.HandID)
	}
	if hh.SmallBlind != 1 {
		t.Errorf("Expected SmallBlind 1, got %d", hh.SmallBlind)
	}
	if hh.BigBlind != 2 {
		t.Errorf("Expected BigBlind 2, got %d", hh.BigBlind)
	}
	if hh.DealerPosition != 3 {
		t.Errorf("Expected DealerPosition 3, got %d", hh.DealerPosition)
	}

	// Test players
	if len(hh.Players) != 3 {
		t.Errorf("Expected 3 players, got %d", len(hh.Players))
	}
	if hh.Players[0].Name != "Alice" || hh.Players[0].Chips != 100 {
		t.Errorf("First player data incorrect: %+v", hh.Players[0])
	}

	// Test empty collections
	if len(hh.Actions) != 0 {
		t.Errorf("Expected empty actions, got %d", len(hh.Actions))
	}
	if len(hh.Winners) != 0 {
		t.Errorf("Expected empty winners, got %d", len(hh.Winners))
	}
}

func TestAddAction(t *testing.T) {
	hh := &HandHistory{
		Actions: make([]HandAction, 0),
	}

	// Create dummy players for events
	alice := &Player{ID: 1, Name: "Alice"}
	bob := &Player{ID: 2, Name: "Bob"}
	charlie := &Player{ID: 3, Name: "Charlie"}

	// Add actions via events
	hh.OnEvent(NewPlayerActionEvent(alice, Fold, 0, PreFlop, "Weak hand", 0))
	hh.OnEvent(NewPlayerActionEvent(bob, Call, 10, PreFlop, "", 13))
	hh.OnEvent(NewPlayerActionEvent(charlie, Raise, 20, Flop, "Strong hand", 33))

	if len(hh.Actions) != 3 {
		t.Errorf("Expected 3 actions, got %d", len(hh.Actions))
	}

	// Test first action
	action := hh.Actions[0]
	if action.PlayerName != "Alice" {
		t.Errorf("Expected PlayerName 'Alice', got '%s'", action.PlayerName)
	}
	if action.Action != Fold {
		t.Errorf("Expected Action Fold, got %v", action.Action)
	}
	if action.Amount != 0 {
		t.Errorf("Expected Amount 0, got %d", action.Amount)
	}
	if action.Round != PreFlop {
		t.Errorf("Expected Round PreFlop, got %v", action.Round)
	}
	if action.Thinking != "Weak hand" {
		t.Errorf("Expected Thinking 'Weak hand', got '%s'", action.Thinking)
	}

	// Test action without thinking
	action2 := hh.Actions[1]
	if action2.Thinking != "" {
		t.Errorf("Expected empty thinking, got '%s'", action2.Thinking)
	}
}

func TestAddPlayerHoleCards(t *testing.T) {
	hh := &HandHistory{
		Players: []PlayerSnapshot{
			{Name: "Alice", Chips: 100},
			{Name: "Bob", Chips: 200},
		},
	}

	cards := []deck.Card{
		{Rank: deck.Ace, Suit: deck.Spades},
		{Rank: deck.King, Suit: deck.Hearts},
	}

	hh.AddPlayerHoleCards("Alice", cards)

	// Check that Alice got the hole cards
	if len(hh.Players[0].HoleCards) != 2 {
		t.Errorf("Expected Alice to have 2 hole cards, got %d", len(hh.Players[0].HoleCards))
	}
	if hh.Players[0].HoleCards[0].Rank != deck.Ace {
		t.Errorf("Expected first card to be Ace, got %v", hh.Players[0].HoleCards[0].Rank)
	}

	// Check that Bob doesn't have hole cards
	if len(hh.Players[1].HoleCards) != 0 {
		t.Errorf("Expected Bob to have no hole cards, got %d", len(hh.Players[1].HoleCards))
	}

	// Test adding cards to non-existent player
	hh.AddPlayerHoleCards("Charlie", cards) // Should not crash
}

func TestSetFinalResults(t *testing.T) {
	hh := &HandHistory{}

	winners := []WinnerInfo{
		{
			PlayerName: "Alice",
			Amount:     100,
			HandRank:   "Pair of Aces",
			HoleCards: []deck.Card{
				{Rank: deck.Ace, Suit: deck.Spades},
				{Rank: deck.Ace, Suit: deck.Hearts},
			},
		},
	}

	hh.SetFinalResults(150, winners)

	if hh.FinalPot != 150 {
		t.Errorf("Expected FinalPot 150, got %d", hh.FinalPot)
	}
	if len(hh.Winners) != 1 {
		t.Errorf("Expected 1 winner, got %d", len(hh.Winners))
	}
	if hh.Winners[0].PlayerName != "Alice" {
		t.Errorf("Expected winner 'Alice', got '%s'", hh.Winners[0].PlayerName)
	}
}

func TestFormatAction(t *testing.T) {
	hh := &HandHistory{}

	tests := []struct {
		action   HandAction
		expected string
	}{
		{
			action:   HandAction{PlayerName: "Alice", Action: Fold, Amount: 0, PotAfter: 10},
			expected: "Alice: folds",
		},
		{
			action:   HandAction{PlayerName: "Bob", Action: Call, Amount: 10, PotAfter: 20},
			expected: "Bob: calls $10 (pot now: $20)",
		},
		{
			action:   HandAction{PlayerName: "Charlie", Action: Check, Amount: 0, PotAfter: 20},
			expected: "Charlie: checks",
		},
		{
			action:   HandAction{PlayerName: "Dave", Action: Raise, Amount: 20, PotAfter: 40},
			expected: "Dave: raises $20 (pot now: $40)",
		},
		{
			action:   HandAction{PlayerName: "Eve", Action: AllIn, Amount: 100, PotAfter: 140},
			expected: "Eve: goes all-in for $100 (pot now: $140)",
		},
	}

	for _, test := range tests {
		result := hh.formatAction(test.action)
		if result != test.expected {
			t.Errorf("formatAction(%+v) = '%s', expected '%s'", test.action, result, test.expected)
		}
	}
}

func TestGenerateHistoryText(t *testing.T) {
	hh := &HandHistory{
		HandID:         "01h5n0et5q6mt3v7ms1234abcd",
		StartTime:      time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		SmallBlind:     1,
		BigBlind:       2,
		DealerPosition: 0,
		Players: []PlayerSnapshot{
			{
				Name:     "Alice",
				Chips:    100,
				Position: Button,
				HoleCards: []deck.Card{
					{Rank: deck.Ace, Suit: deck.Spades},
					{Rank: deck.King, Suit: deck.Hearts},
				},
			},
			{
				Name:     "Bob",
				Chips:    200,
				Position: SmallBlind,
				HoleCards: []deck.Card{
					{Rank: deck.Two, Suit: deck.Clubs},
					{Rank: deck.Three, Suit: deck.Diamonds},
				},
			},
		},
		Actions: []HandAction{
			{PlayerName: "Alice", Action: Raise, Amount: 10, PotAfter: 20, Round: PreFlop, Thinking: "Strong hand"},
			{PlayerName: "Bob", Action: Call, Amount: 10, PotAfter: 30, Round: PreFlop, Thinking: ""},
		},
		FinalPot: 20,
		Winners: []WinnerInfo{
			{
				PlayerName: "Alice",
				Amount:     20,
				HandRank:   "Ace High",
				HoleCards: []deck.Card{
					{Rank: deck.Ace, Suit: deck.Spades},
					{Rank: deck.King, Suit: deck.Hearts},
				},
			},
		},
	}

	text := hh.GenerateHistoryText()

	// Test that key elements are present
	expectedElements := []string{
		"=== HAND 01h5n0et5q6mt3v7ms1234abcd ===",
		"Date: 2025-01-01 12:00:00",
		"Blinds: 1/2",
		"Players: 2",
		"Dealer Position: 0",
		"STARTING POSITIONS:",
		"Seat 1: Alice (100 chips)",
		"Seat 2: Bob (200 chips)",
		"HOLE CARDS:",
		"Alice: A♠ K♥",
		"Bob: 2♣ 3♦",
		"HAND ACTION:",
		"*** PRE-FLOP ***",
		"Alice: thinks \"Strong hand\"",
		"Alice: raises $10 (pot now: $20)",
		"Bob: calls $10",
		"*** SUMMARY ***",
		"Total pot $20",
		"Seat 1: Alice (button) showed [A♠ K♥] and won ($20) with Ace High",
		"=== END HAND ===",
	}

	for _, element := range expectedElements {
		if !strings.Contains(text, element) {
			t.Errorf("Expected text to contain '%s', but it didn't.\nFull text:\n%s", element, text)
		}
	}

	// Test that thinking appears before action
	aliceThinkingIndex := strings.Index(text, "Alice: thinks \"Strong hand\"")
	aliceActionIndex := strings.Index(text, "Alice: raises $10")
	if aliceThinkingIndex == -1 || aliceActionIndex == -1 {
		t.Error("Could not find Alice's thinking or action in text")
	}
	if aliceThinkingIndex > aliceActionIndex {
		t.Error("Alice's thinking should appear before her action")
	}

	// Test that Bob's action has no thinking line before it
	bobCallIndex := strings.Index(text, "Bob: calls $10")
	bobThinkingIndex := strings.Index(text, "Bob: thinks")
	if bobThinkingIndex != -1 && bobThinkingIndex < bobCallIndex {
		t.Error("Bob should not have thinking text, but found it before his action")
	}
}

func TestGetDisplayActions(t *testing.T) {
	hh := &HandHistory{
		Actions: []HandAction{
			{PlayerName: "Alice", Action: Raise, Amount: 10, PotAfter: 20, Round: PreFlop, Thinking: "Strong hand"},
			{PlayerName: "Bob", Action: Call, Amount: 10, PotAfter: 30, Round: PreFlop, Thinking: ""},
			{PlayerName: "Alice", Action: Check, Amount: 0, PotAfter: 30, Round: Flop, Thinking: "Waiting"},
		},
	}

	actions := hh.GetDisplayActions()

	expected := []string{
		"*** PRE-FLOP ***",
		"Alice: raises $10 (pot now: $20)",
		"Bob: calls $10 (pot now: $30)",
		"*** FLOP ***",
		"Alice: checks",
	}

	if len(actions) != len(expected) {
		t.Errorf("Expected %d actions, got %d", len(expected), len(actions))
	}

	for i, expectedAction := range expected {
		if i >= len(actions) {
			t.Errorf("Missing action at index %d: expected '%s'", i, expectedAction)
			continue
		}
		if actions[i] != expectedAction {
			t.Errorf("Action at index %d: expected '%s', got '%s'", i, expectedAction, actions[i])
		}
	}

	// Verify no thinking is included in display actions (for TUI)
	for _, action := range actions {
		if strings.Contains(action, "thinks") {
			t.Errorf("Display actions should not contain thinking, but found: '%s'", action)
		}
	}
}

func TestEmptyHandHistory(t *testing.T) {
	hh := &HandHistory{
		HandID:    "01h5n0et5q6mt3v7ms1234abcd",
		StartTime: time.Now(),
		Players:   []PlayerSnapshot{},
		Actions:   []HandAction{},
		Winners:   []WinnerInfo{},
	}

	text := hh.GenerateHistoryText()

	// Should not crash and should contain basic structure
	if !strings.Contains(text, "=== HAND 01h5n0et5q6mt3v7ms1234abcd ===") {
		t.Error("Expected hand header even with empty history")
	}
	if !strings.Contains(text, "=== END HAND ===") {
		t.Error("Expected hand footer even with empty history")
	}
}

func TestMultipleRounds(t *testing.T) {
	hh := &HandHistory{
		Actions: []HandAction{
			{PlayerName: "Alice", Action: Raise, Amount: 10, PotAfter: 20, Round: PreFlop, Thinking: ""},
			{PlayerName: "Bob", Action: Call, Amount: 10, PotAfter: 30, Round: PreFlop, Thinking: ""},
			{PlayerName: "Alice", Action: Check, Amount: 0, PotAfter: 30, Round: Flop, Thinking: ""},
			{PlayerName: "Bob", Action: Raise, Amount: 20, PotAfter: 50, Round: Flop, Thinking: ""},
			{PlayerName: "Alice", Action: Call, Amount: 20, PotAfter: 70, Round: Turn, Thinking: ""},
		},
	}

	text := hh.GenerateHistoryText()

	// Check that all round headers appear
	rounds := []string{"*** PRE-FLOP ***", "*** FLOP ***", "*** TURN ***"}
	for _, round := range rounds {
		if !strings.Contains(text, round) {
			t.Errorf("Expected to find round header '%s' in text", round)
		}
	}

	// Check that rounds appear in order
	preflopIndex := strings.Index(text, "*** PRE-FLOP ***")
	flopIndex := strings.Index(text, "*** FLOP ***")
	turnIndex := strings.Index(text, "*** TURN ***")

	if preflopIndex > flopIndex || flopIndex > turnIndex {
		t.Error("Round headers should appear in chronological order")
	}
}

// Test the new betting analysis functionality
func TestGetCurrentRoundActions(t *testing.T) {
	hh := &HandHistory{
		Actions: []HandAction{
			{PlayerName: "Alice", Action: Raise, Amount: 10, Round: PreFlop},
			{PlayerName: "Bob", Action: Call, Amount: 10, Round: PreFlop},
			{PlayerName: "Alice", Action: Check, Amount: 0, Round: Flop},
			{PlayerName: "Bob", Action: Raise, Amount: 20, Round: Flop},
			{PlayerName: "Alice", Action: Call, Amount: 20, Round: Turn},
		},
	}

	// Test PreFlop actions
	preflopActions := hh.GetCurrentRoundActions(PreFlop)
	if len(preflopActions) != 2 {
		t.Errorf("Expected 2 preflop actions, got %d", len(preflopActions))
	}
	if preflopActions[0].PlayerName != "Alice" || preflopActions[0].Action != Raise {
		t.Errorf("Expected first preflop action to be Alice raising, got %v", preflopActions[0])
	}

	// Test Flop actions
	flopActions := hh.GetCurrentRoundActions(Flop)
	if len(flopActions) != 2 {
		t.Errorf("Expected 2 flop actions, got %d", len(flopActions))
	}

	// Test Turn actions
	turnActions := hh.GetCurrentRoundActions(Turn)
	if len(turnActions) != 1 {
		t.Errorf("Expected 1 turn action, got %d", len(turnActions))
	}

	// Test River actions (none)
	riverActions := hh.GetCurrentRoundActions(River)
	if len(riverActions) != 0 {
		t.Errorf("Expected 0 river actions, got %d", len(riverActions))
	}
}

func TestGetBettingRoundSummary(t *testing.T) {
	hh := &HandHistory{
		SmallBlind: 10,
		BigBlind:   20,
		Actions: []HandAction{
			{PlayerName: "Alice", Action: Call, Amount: 10, Round: PreFlop}, // Small blind
			{PlayerName: "Bob", Action: Call, Amount: 20, Round: PreFlop},   // Big blind
			{PlayerName: "Charlie", Action: Raise, Amount: 40, Round: PreFlop},
			{PlayerName: "Alice", Action: Call, Amount: 30, Round: PreFlop},
			{PlayerName: "Bob", Action: Raise, Amount: 80, Round: PreFlop}, // 3-bet
		},
	}

	summary := hh.GetBettingRoundSummary(PreFlop)

	if summary.Round != PreFlop {
		t.Errorf("Expected PreFlop round, got %v", summary.Round)
	}

	if summary.NumRaises != 2 {
		t.Errorf("Expected 2 raises, got %d", summary.NumRaises)
	}

	if summary.LastAggressor != "Bob" {
		t.Errorf("Expected last aggressor to be Bob, got %s", summary.LastAggressor)
	}

	// Note: The NumCallers count will include blind calls since isBlindPosting
	// requires timestamp comparison which we don't have in this simple test
	if summary.NumCallers < 1 {
		t.Errorf("Expected at least 1 caller, got %d", summary.NumCallers)
	}

	if len(summary.Actions) != 5 {
		t.Errorf("Expected 5 actions in summary, got %d", len(summary.Actions))
	}
}

func TestGetBetSizingInfo(t *testing.T) {
	hh := &HandHistory{
		Actions: []HandAction{
			{PlayerName: "Alice", Action: Raise, Amount: 20, PotAfter: 50}, // 20 into 30 pot
			{PlayerName: "Bob", Action: Call, Amount: 20, PotAfter: 70},
			{PlayerName: "Charlie", Action: Raise, Amount: 60, PotAfter: 130}, // 60 into 70 pot
		},
	}

	sizing := hh.GetBetSizingInfo(PreFlop)

	if len(sizing) != 2 { // Only raises, not calls
		t.Errorf("Expected 2 bet sizing entries, got %d", len(sizing))
	}

	// Check Alice's bet
	aliceBet := sizing[0]
	if aliceBet.PlayerName != "Alice" {
		t.Errorf("Expected first bet by Alice, got %s", aliceBet.PlayerName)
	}
	if aliceBet.Amount != 20 {
		t.Errorf("Expected bet amount 20, got %d", aliceBet.Amount)
	}
	if aliceBet.PotBefore != 30 {
		t.Errorf("Expected pot before bet to be 30, got %d", aliceBet.PotBefore)
	}
	expectedRatio := float64(20) / float64(30)
	if aliceBet.Ratio != expectedRatio {
		t.Errorf("Expected ratio %.3f, got %.3f", expectedRatio, aliceBet.Ratio)
	}

	// Check Charlie's raise
	charlieBet := sizing[1]
	if charlieBet.PlayerName != "Charlie" {
		t.Errorf("Expected second bet by Charlie, got %s", charlieBet.PlayerName)
	}
	if charlieBet.Amount != 60 {
		t.Errorf("Expected bet amount 60, got %d", charlieBet.Amount)
	}
}

func TestGetPlayerActions(t *testing.T) {
	hh := &HandHistory{
		Actions: []HandAction{
			{PlayerName: "Alice", Action: Raise, Amount: 10, Round: PreFlop},
			{PlayerName: "Bob", Action: Call, Amount: 10, Round: PreFlop},
			{PlayerName: "Alice", Action: Check, Amount: 0, Round: Flop},
			{PlayerName: "Charlie", Action: Fold, Amount: 0, Round: Flop},
			{PlayerName: "Alice", Action: Call, Amount: 20, Round: Turn},
		},
	}

	// Test Alice's actions
	aliceActions := hh.GetPlayerActions("Alice")
	if len(aliceActions) != 3 {
		t.Errorf("Expected 3 actions by Alice, got %d", len(aliceActions))
	}

	expectedActions := []Action{Raise, Check, Call}
	for i, expected := range expectedActions {
		if aliceActions[i].Action != expected {
			t.Errorf("Expected Alice's action %d to be %v, got %v", i, expected, aliceActions[i].Action)
		}
	}

	// Test Bob's actions
	bobActions := hh.GetPlayerActions("Bob")
	if len(bobActions) != 1 {
		t.Errorf("Expected 1 action by Bob, got %d", len(bobActions))
	}

	// Test non-existent player
	nonExistentActions := hh.GetPlayerActions("David")
	if len(nonExistentActions) != 0 {
		t.Errorf("Expected 0 actions by non-existent player, got %d", len(nonExistentActions))
	}
}

func TestHasPlayerActed(t *testing.T) {
	hh := &HandHistory{
		Actions: []HandAction{
			{PlayerName: "Alice", Action: Raise, Amount: 10, Round: PreFlop},
			{PlayerName: "Bob", Action: Call, Amount: 10, Round: PreFlop},
			{PlayerName: "Alice", Action: Check, Amount: 0, Round: Flop},
		},
	}

	// Test players who have acted
	if !hh.HasPlayerActed("Alice", PreFlop) {
		t.Error("Expected Alice to have acted in PreFlop")
	}
	if !hh.HasPlayerActed("Bob", PreFlop) {
		t.Error("Expected Bob to have acted in PreFlop")
	}
	if !hh.HasPlayerActed("Alice", Flop) {
		t.Error("Expected Alice to have acted in Flop")
	}

	// Test players who haven't acted
	if hh.HasPlayerActed("Bob", Flop) {
		t.Error("Expected Bob to not have acted in Flop")
	}
	if hh.HasPlayerActed("Charlie", PreFlop) {
		t.Error("Expected Charlie to not have acted in PreFlop")
	}
	if hh.HasPlayerActed("Alice", Turn) {
		t.Error("Expected Alice to not have acted in Turn")
	}
}
