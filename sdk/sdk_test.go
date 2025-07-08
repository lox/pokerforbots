package sdk

import (
	"encoding/json"
	"testing"

	"github.com/lox/pokerforbots/sdk/deck"
)

func TestMessageSerialization(t *testing.T) {
	// Test creating and serializing a message
	authData := AuthData{
		PlayerName: "TestBot",
	}

	msg, err := NewMessage(MessageTypeAuth, authData)
	if err != nil {
		t.Fatalf("Failed to create message: %v", err)
	}

	// Serialize to JSON
	jsonData, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal message: %v", err)
	}

	// Deserialize back
	var decoded Message
	if err := json.Unmarshal(jsonData, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal message: %v", err)
	}

	if decoded.Type != MessageTypeAuth {
		t.Errorf("Expected message type %s, got %s", MessageTypeAuth, decoded.Type)
	}
}

func TestActionEnums(t *testing.T) {
	// Test action string conversion
	tests := []struct {
		action Action
		want   string
	}{
		{ActionFold, "fold"},
		{ActionCheck, "check"},
		{ActionCall, "call"},
		{ActionRaise, "raise"},
		{ActionAllIn, "all-in"},
	}

	for _, tt := range tests {
		got := tt.action.String()
		if got != tt.want {
			t.Errorf("Action.String() = %s, want %s", got, tt.want)
		}

		// Test reverse conversion
		parsed := ActionFromString(tt.want)
		if parsed != tt.action {
			t.Errorf("ActionFromString(%s) = %v, want %v", tt.want, parsed, tt.action)
		}
	}
}

func TestDecisionHelpers(t *testing.T) {
	// Test decision creation helpers
	foldDecision := NewFoldDecision("Testing fold")
	if foldDecision.Action != ActionFold {
		t.Errorf("NewFoldDecision() action = %v, want %v", foldDecision.Action, ActionFold)
	}
	if foldDecision.Reasoning != "Testing fold" {
		t.Errorf("NewFoldDecision() reasoning = %s, want %s", foldDecision.Reasoning, "Testing fold")
	}

	raiseDecision := NewRaiseDecision(100, "Big raise")
	if raiseDecision.Action != ActionRaise {
		t.Errorf("NewRaiseDecision() action = %v, want %v", raiseDecision.Action, ActionRaise)
	}
	if raiseDecision.Amount != 100 {
		t.Errorf("NewRaiseDecision() amount = %d, want %d", raiseDecision.Amount, 100)
	}
}

func TestTableStateHelpers(t *testing.T) {
	// Test table state helper methods
	ts := TableState{
		CurrentRound: RoundFlop,
		Players: []BotPlayerState{
			{Name: "Bot1", HoleCards: []deck.Card{{Rank: deck.Ace, Suit: deck.Spades}, {Rank: deck.King, Suit: deck.Spades}}, IsActive: true},
			{Name: "Bot2", IsActive: true, IsFolded: false},
			{Name: "Bot3", IsActive: true, IsFolded: true},
		},
		ActingPlayerIdx: 1,
		CommunityCards: []deck.Card{
			{Rank: deck.Queen, Suit: deck.Spades},
			{Rank: deck.Jack, Suit: deck.Spades},
			{Rank: deck.Ten, Suit: deck.Spades},
		},
	}

	// Test GetBotPlayer
	bot := ts.GetBotPlayer()
	if bot == nil || bot.Name != "Bot1" {
		t.Errorf("GetBotPlayer() failed to find bot with hole cards")
	}

	// Test GetActingPlayer
	acting := ts.GetActingPlayer()
	if acting == nil || acting.Name != "Bot2" {
		t.Errorf("GetActingPlayer() failed to find acting player")
	}

	// Test GetActivePlayers
	active := ts.GetActivePlayers()
	if len(active) != 2 {
		t.Errorf("GetActivePlayers() returned %d players, want 2", len(active))
	}

	// Test round helpers
	if !ts.IsFlop() {
		t.Errorf("IsFlop() = false, want true")
	}

	if ts.GetCommunityCardCount() != 3 {
		t.Errorf("GetCommunityCardCount() = %d, want 3", ts.GetCommunityCardCount())
	}
}
