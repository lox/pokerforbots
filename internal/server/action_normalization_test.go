package server

import (
	"testing"

	"github.com/lox/pokerforbots/internal/game"
)

// TestNormalizeActionProtocolV2 verifies that the server correctly normalizes
// the simplified 4-action protocol (fold, call, raise, allin) to semantic actions
// (fold, check, call, bet, raise, allin) based on game state.
func TestNormalizeActionProtocolV2(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		clientAction  string
		clientAmount  int
		toCall        int
		playerChips   int
		currentBet    int
		minRaise      int
		wantAction    game.Action
		wantAmount    int
		wantBroadcast string // What gets broadcast in player_action
	}{
		{
			name:          "call with to_call=0 normalizes to check",
			clientAction:  "call",
			clientAmount:  0,
			toCall:        0,
			playerChips:   100,
			currentBet:    0,
			minRaise:      10,
			wantAction:    game.Check,
			wantAmount:    0,
			wantBroadcast: "check",
		},
		{
			name:          "call with to_call>0 stays call",
			clientAction:  "call",
			clientAmount:  0,
			toCall:        20,
			playerChips:   100,
			currentBet:    20,
			minRaise:      20,
			wantAction:    game.Call,
			wantAmount:    0,
			wantBroadcast: "call",
		},
		{
			name:          "raise with to_call=0 stays raise (game engine handles bet semantics)",
			clientAction:  "raise",
			clientAmount:  50,
			toCall:        0,
			playerChips:   100,
			currentBet:    0,
			minRaise:      10,
			wantAction:    game.Raise,
			wantAmount:    50,
			wantBroadcast: "raise", // Game engine doesn't distinguish bet vs raise
		},
		{
			name:          "raise with to_call>0 stays raise",
			clientAction:  "raise",
			clientAmount:  100,
			toCall:        20,
			playerChips:   200,
			currentBet:    20,
			minRaise:      20,
			wantAction:    game.Raise,
			wantAmount:    100,
			wantBroadcast: "raise",
		},
		{
			name:          "raise with amount >= stack normalizes to allin",
			clientAction:  "raise",
			clientAmount:  120,
			toCall:        20,
			playerChips:   120,
			currentBet:    20,
			minRaise:      20,
			wantAction:    game.AllIn,
			wantAmount:    120,
			wantBroadcast: "allin",
		},
		{
			name:          "fold stays fold",
			clientAction:  "fold",
			clientAmount:  0,
			toCall:        20,
			playerChips:   100,
			currentBet:    20,
			minRaise:      20,
			wantAction:    game.Fold,
			wantAmount:    0,
			wantBroadcast: "fold",
		},
		{
			name:          "allin stays allin",
			clientAction:  "allin",
			clientAmount:  0,
			toCall:        20,
			playerChips:   100,
			currentBet:    20,
			minRaise:      20,
			wantAction:    game.AllIn,
			wantAmount:    0,
			wantBroadcast: "allin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock hand runner with the necessary state
			player := &game.Player{
				Chips: tt.playerChips,
				Bet:   0,
			}
			betting := &game.BettingRound{
				CurrentBet: tt.currentBet,
				MinRaise:   tt.minRaise,
			}

			// This function doesn't exist yet - we're writing the failing test
			action, amount := normalizeAction(tt.clientAction, tt.clientAmount, player, betting)

			if action != tt.wantAction {
				t.Errorf("normalizeAction() action = %v, want %v", action, tt.wantAction)
			}
			if amount != tt.wantAmount {
				t.Errorf("normalizeAction() amount = %d, want %d", amount, tt.wantAmount)
			}

			// Verify the broadcast name matches semantic expectations
			broadcastName := action.String()
			if broadcastName != tt.wantBroadcast {
				t.Errorf("broadcast action name = %q, want %q", broadcastName, tt.wantBroadcast)
			}
		})
	}
}

// TestNormalizeActionRejectsOldProtocol verifies that the old action names
// (check, bet) are explicitly rejected with clear error messages.
func TestNormalizeActionRejectsOldProtocol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		clientAction string
		wantErr      string
	}{
		{
			name:         "check rejected - use call",
			clientAction: "check",
			wantErr:      "use 'call'",
		},
		{
			name:         "bet rejected - use raise",
			clientAction: "bet",
			wantErr:      "use 'raise'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			player := &game.Player{Chips: 100}
			betting := &game.BettingRound{CurrentBet: 0, MinRaise: 10}

			// This should return an error or invalid action
			action, _ := normalizeAction(tt.clientAction, 0, player, betting)

			// For now, we expect Fold (invalid action default)
			// TODO: Update when we add proper error handling
			if action != game.Fold {
				t.Errorf("normalizeAction(%q) should reject old protocol, got %v", tt.clientAction, action)
			}
		})
	}
}
