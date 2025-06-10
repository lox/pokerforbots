package tui

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/lox/pokerforbots/internal/client"
	"github.com/lox/pokerforbots/internal/deck"
	"github.com/lox/pokerforbots/internal/game"
	"github.com/lox/pokerforbots/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParserFunctions(t *testing.T) {
	t.Run("parseActionFromString", func(t *testing.T) {
		tests := []struct {
			input    string
			expected game.Action
		}{
			{"fold", game.Fold},
			{"call", game.Call},
			{"check", game.Check},
			{"raise", game.Raise},
			{"allin", game.AllIn},
			{"unknown", game.Fold}, // default case
		}

		for _, test := range tests {
			result := parseActionFromString(test.input)
			assert.Equal(t, test.expected, result, "input: %s", test.input)
		}
	})

	t.Run("parsePositionFromString", func(t *testing.T) {
		tests := []struct {
			input    string
			expected game.Position
		}{
			{"small_blind", game.SmallBlind},
			{"big_blind", game.BigBlind},
			{"under_the_gun", game.UnderTheGun},
			{"middle_position", game.MiddlePosition},
			{"cutoff", game.Cutoff},
			{"button", game.Button},
			{"unknown", game.MiddlePosition}, // default case
		}

		for _, test := range tests {
			result := parsePositionFromString(test.input)
			assert.Equal(t, test.expected, result, "input: %s", test.input)
		}
	})

	t.Run("parseRoundFromString", func(t *testing.T) {
		tests := []struct {
			input    string
			expected game.BettingRound
		}{
			{"Pre-flop", game.PreFlop},
			{"Flop", game.Flop},
			{"Turn", game.Turn},
			{"River", game.River},
			{"Showdown", game.Showdown},
			{"unknown", game.PreFlop}, // default case
		}

		for _, test := range tests {
			result := parseRoundFromString(test.input)
			assert.Equal(t, test.expected, result, "input: %s", test.input)
		}
	})

	t.Run("actionToNetworkString", func(t *testing.T) {
		tests := []struct {
			input    game.Action
			expected string
		}{
			{game.Fold, "fold"},
			{game.Call, "call"},
			{game.Check, "check"},
			{game.Raise, "raise"},
			{game.AllIn, "allin"},
		}

		for _, test := range tests {
			result := actionToNetworkString(test.input)
			assert.Equal(t, test.expected, result, "input: %v", test.input)
		}
	})
}

func TestProcessUserAction(t *testing.T) {
	logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})
	tui := NewTUIModelWithOptions(logger, true)

	// Set up comprehensive valid actions for testing basic parsing
	validActions := []game.ValidAction{
		{Action: game.Fold, MinAmount: 0, MaxAmount: 0},
		{Action: game.Call, MinAmount: 0, MaxAmount: 100},
		{Action: game.Check, MinAmount: 0, MaxAmount: 0},
		{Action: game.Raise, MinAmount: 1, MaxAmount: 1000},
		{Action: game.AllIn, MinAmount: 0, MaxAmount: 1000},
	}
	tui.UpdateValidActions(validActions)

	tests := []struct {
		name           string
		action         string
		args           []string
		expectedAction game.Action
		expectedAmount int
		expectError    bool
	}{
		{"fold", "f", []string{}, game.Fold, 0, false},
		{"fold full", "fold", []string{}, game.Fold, 0, false},
		{"call", "c", []string{}, game.Call, 0, false},
		{"call full", "call", []string{}, game.Call, 0, false},
		{"check", "k", []string{}, game.Check, 0, false},
		{"check full", "check", []string{}, game.Check, 0, false},
		{"raise with amount", "r", []string{"50"}, game.Raise, 50, false},
		{"raise full with amount", "raise", []string{"100"}, game.Raise, 100, false},
		{"allin", "a", []string{}, game.AllIn, 0, false},
		{"allin full", "allin", []string{}, game.AllIn, 0, false},
		{"all shorthand", "all", []string{}, game.AllIn, 0, false},
		{"raise without amount", "raise", []string{}, game.Check, 0, true},
		{"raise invalid amount", "raise", []string{"invalid"}, game.Check, 0, true},
		{"unknown action", "unknown", []string{}, game.Check, 0, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Clear any previous log entries
			tui.capturedLog = []string{}

			decision := processUserAction(test.action, test.args, tui)

			assert.Equal(t, test.expectedAction, decision.Action)
			assert.Equal(t, test.expectedAmount, decision.Amount)

			if test.expectError {
				assert.Equal(t, "Invalid input - try again", decision.Reasoning)
				// Should have added an error message to log
				assert.NotEmpty(t, tui.GetCapturedLog())
			} else {
				assert.NotEqual(t, "Invalid input - try again", decision.Reasoning)
			}
		})
	}
}

func TestConvertPlayersFromServer(t *testing.T) {
	serverPlayers := []server.PlayerState{
		{
			Name:         "Alice",
			Chips:        1000,
			Position:     "small_blind",
			BetThisRound: 10,
			TotalBet:     10,
			HoleCards:    []deck.Card{deck.MustParseCards("AsKh")[0], deck.MustParseCards("AsKh")[1]},
			IsActive:     true,
			IsFolded:     false,
			IsAllIn:      false,
			SeatNumber:   1,
		},
		{
			Name:         "Bob",
			Chips:        950,
			Position:     "big_blind",
			BetThisRound: 20,
			TotalBet:     20,
			HoleCards:    []deck.Card{},
			IsActive:     true,
			IsFolded:     false,
			IsAllIn:      false,
			SeatNumber:   2,
		},
	}

	players := convertPlayersFromServer(serverPlayers)

	require.Len(t, players, 2)

	// Check first player
	assert.Equal(t, "Alice", players[0].Name)
	assert.Equal(t, 1000, players[0].Chips)
	assert.Equal(t, game.SmallBlind, players[0].Position)
	assert.Equal(t, 10, players[0].BetThisRound)
	assert.Equal(t, 10, players[0].TotalBet)
	assert.Len(t, players[0].HoleCards, 2)
	assert.True(t, players[0].IsActive)
	assert.False(t, players[0].IsFolded)
	assert.False(t, players[0].IsAllIn)
	assert.Equal(t, 1, players[0].SeatNumber)

	// Check second player
	assert.Equal(t, "Bob", players[1].Name)
	assert.Equal(t, 950, players[1].Chips)
	assert.Equal(t, game.BigBlind, players[1].Position)
	assert.Equal(t, 20, players[1].BetThisRound)
	assert.Equal(t, 20, players[1].TotalBet)
	assert.Empty(t, players[1].HoleCards)
	assert.True(t, players[1].IsActive)
	assert.False(t, players[1].IsFolded)
	assert.False(t, players[1].IsAllIn)
	assert.Equal(t, 2, players[1].SeatNumber)
}

func TestFormatCards(t *testing.T) {
	t.Run("empty cards", func(t *testing.T) {
		result := formatCards([]deck.Card{})
		assert.Equal(t, "", result)
	})

	t.Run("single card", func(t *testing.T) {
		cards := deck.MustParseCards("As")
		result := formatCards(cards)
		// Should contain the card symbol wrapped in brackets with ANSI codes
		assert.Contains(t, result, "A♠")
		assert.Contains(t, result, "[")
		assert.Contains(t, result, "]")
	})

	t.Run("multiple cards", func(t *testing.T) {
		cards := deck.MustParseCards("AsKh")
		result := formatCards(cards)
		// Should contain both card symbols
		assert.Contains(t, result, "A♠")
		assert.Contains(t, result, "K♥")
		assert.Contains(t, result, "[")
		assert.Contains(t, result, "]")
	})
}

func TestBridgeEventHandling(t *testing.T) {
	logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})
	tui := NewTUIModelWithOptions(logger, true)

	// Create a properly initialized client for testing
	mockClient := client.NewClient("ws://localhost:8080", logger)

	bridge := NewBridge(mockClient, tui, 200)

	t.Run("handlePlayerAction", func(t *testing.T) {
		// Create a mock player action message
		actionData := server.PlayerActionData{
			Player:    "Alice",
			Action:    "call",
			Amount:    20,
			PotAfter:  40,
			Round:     "Pre-flop",
			Reasoning: "Player called",
		}

		msgData, err := json.Marshal(actionData)
		require.NoError(t, err)

		msg := &server.Message{
			Type: server.MessageTypePlayerAction,
			Data: msgData,
		}

		// Handle the message
		bridge.handlePlayerAction(msg)

		// Check that TUI state was updated
		assert.Equal(t, 40, tui.currentPot)

		// Check that log entry was added
		captured := tui.GetCapturedLog()
		assert.NotEmpty(t, captured)
	})

	t.Run("handleStreetChange", func(t *testing.T) {
		// Create a mock street change message
		streetData := server.StreetChangeData{
			Round:          "Flop",
			CurrentBet:     0,
			CommunityCards: deck.MustParseCards("AhKsQd"),
		}

		msgData, err := json.Marshal(streetData)
		require.NoError(t, err)

		msg := &server.Message{
			Type: server.MessageTypeStreetChange,
			Data: msgData,
		}

		// Handle the message
		bridge.handleStreetChange(msg)

		// Check that TUI state was updated
		assert.Equal(t, 0, tui.currentBet)
		assert.Equal(t, "Flop", tui.currentRound)
		assert.Len(t, tui.communityCards, 3)

		// Check that log entry was added
		captured := tui.GetCapturedLog()
		assert.NotEmpty(t, captured)
		// Should contain street change formatting
		found := false
		for _, entry := range captured {
			if strings.Contains(entry, "*** FLOP ***") && strings.Contains(entry, "A♥") {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected flop street change entry in log")
	})

	t.Run("handleHandEnd", func(t *testing.T) {
		// Create a mock hand end message
		handEndData := server.HandEndData{
			HandID:  "hand123",
			PotSize: 100,
			Winners: []game.WinnerInfo{
				{
					PlayerName: "Alice",
					Amount:     100,
					HandRank:   "Pair of Aces",
				},
			},
		}

		msgData, err := json.Marshal(handEndData)
		require.NoError(t, err)

		msg := &server.Message{
			Type: server.MessageTypeHandEnd,
			Data: msgData,
		}

		// Handle the message
		bridge.handleHandEnd(msg)

		// Check that log entries were added
		captured := tui.GetCapturedLog()
		assert.NotEmpty(t, captured)

		// Should contain hand completion info
		found := false
		for _, entry := range captured {
			if entry == "=== Hand hand123 Complete ===" {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected hand completion entry in log")
	})
}

func TestProcessUserActionWithValidation(t *testing.T) {
	logger := log.NewWithOptions(io.Discard, log.Options{Level: log.ErrorLevel})
	tui := NewTUIModelWithOptions(logger, true)

	t.Run("respects valid actions when set", func(t *testing.T) {
		// Setup: facing a bet, so only fold/call/raise are valid
		validActions := []game.ValidAction{
			{Action: game.Fold, MinAmount: 0, MaxAmount: 0},
			{Action: game.Call, MinAmount: 10, MaxAmount: 10},
			{Action: game.Raise, MinAmount: 20, MaxAmount: 200},
		}
		tui.UpdateValidActions(validActions)

		// Valid actions should work
		decision := processUserAction("call", []string{}, tui)
		assert.Equal(t, game.Call, decision.Action, "Valid call should be accepted")
		assert.Equal(t, 10, decision.Amount, "Should use amount from valid action")

		decision = processUserAction("fold", []string{}, tui)
		assert.Equal(t, game.Fold, decision.Action, "Valid fold should be accepted")

		decision = processUserAction("raise", []string{"50"}, tui)
		assert.Equal(t, game.Raise, decision.Action, "Valid raise should be accepted")
		assert.Equal(t, 50, decision.Amount, "Should use specified amount")

		// Invalid actions should be rejected
		decision = processUserAction("check", []string{}, tui)
		assert.Contains(t, decision.Reasoning, "Invalid input - try again", "Invalid check should be rejected")

		// Invalid raise amounts should be rejected
		decision = processUserAction("raise", []string{"5"}, tui)
		assert.Contains(t, decision.Reasoning, "Invalid input - try again", "Raise below minimum should be rejected")

		decision = processUserAction("raise", []string{"300"}, tui)
		assert.Contains(t, decision.Reasoning, "Invalid input - try again", "Raise above maximum should be rejected")
	})
}
