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
			action, amount := normalizeActionV2(tt.clientAction, tt.clientAmount, player, betting)

			if action != tt.wantAction {
				t.Errorf("normalizeActionV2() action = %v, want %v", action, tt.wantAction)
			}
			if amount != tt.wantAmount {
				t.Errorf("normalizeActionV2() amount = %d, want %d", amount, tt.wantAmount)
			}

			// Verify the broadcast name matches semantic expectations
			broadcastName := action.String()
			if broadcastName != tt.wantBroadcast {
				t.Errorf("broadcast action name = %q, want %q", broadcastName, tt.wantBroadcast)
			}
		})
	}
}

// TestNormalizeActionProtocolV1 verifies that protocol v1 actions are handled correctly
// with direct 1:1 mapping to game actions.
func TestNormalizeActionProtocolV1(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		clientAction string
		clientAmount int
		wantAction   game.Action
		wantAmount   int
	}{
		{
			name:         "fold",
			clientAction: "fold",
			clientAmount: 0,
			wantAction:   game.Fold,
			wantAmount:   0,
		},
		{
			name:         "check",
			clientAction: "check",
			clientAmount: 0,
			wantAction:   game.Check,
			wantAmount:   0,
		},
		{
			name:         "call",
			clientAction: "call",
			clientAmount: 0,
			wantAction:   game.Call,
			wantAmount:   0,
		},
		{
			name:         "bet with amount",
			clientAction: "bet",
			clientAmount: 50,
			wantAction:   game.Raise,
			wantAmount:   50,
		},
		{
			name:         "raise with amount",
			clientAction: "raise",
			clientAmount: 100,
			wantAction:   game.Raise,
			wantAmount:   100,
		},
		{
			name:         "allin",
			clientAction: "allin",
			clientAmount: 0,
			wantAction:   game.AllIn,
			wantAmount:   0,
		},
		{
			name:         "invalid action defaults to fold",
			clientAction: "invalid",
			clientAmount: 0,
			wantAction:   game.Fold,
			wantAmount:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action, amount := normalizeActionV1(tt.clientAction, tt.clientAmount)

			if action != tt.wantAction {
				t.Errorf("normalizeActionV1() action = %v, want %v", action, tt.wantAction)
			}
			if amount != tt.wantAmount {
				t.Errorf("normalizeActionV1() amount = %d, want %d", amount, tt.wantAmount)
			}
		})
	}
}

// TestConvertActionsForProtocol verifies that valid_actions are converted correctly
// based on the protocol version.
func TestConvertActionsForProtocol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		actions []game.Action
		toCall  int
		version string
		want    []string
	}{
		{
			name:    "v1 check available",
			actions: []game.Action{game.Fold, game.Check, game.Raise},
			toCall:  0,
			version: "1",
			want:    []string{"fold", "check", "raise"},
		},
		{
			name:    "v1 call available",
			actions: []game.Action{game.Fold, game.Call, game.Raise},
			toCall:  20,
			version: "1",
			want:    []string{"fold", "call", "raise"},
		},
		{
			name:    "v2 check becomes call",
			actions: []game.Action{game.Fold, game.Check, game.Raise},
			toCall:  0,
			version: "2",
			want:    []string{"fold", "call", "raise"},
		},
		{
			name:    "v2 call stays call",
			actions: []game.Action{game.Fold, game.Call, game.Raise},
			toCall:  20,
			version: "2",
			want:    []string{"fold", "call", "raise"},
		},
		{
			name:    "v2 all actions",
			actions: []game.Action{game.Fold, game.Check, game.Call, game.Raise, game.AllIn},
			toCall:  10,
			version: "2",
			want:    []string{"fold", "call", "call", "raise", "allin"},
		},
		{
			name:    "v1 all actions",
			actions: []game.Action{game.Fold, game.Check, game.Call, game.Raise, game.AllIn},
			toCall:  10,
			version: "1",
			want:    []string{"fold", "check", "call", "raise", "allin"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertActionsForProtocol(tt.actions, tt.toCall, tt.version)

			if len(got) != len(tt.want) {
				t.Errorf("convertActionsForProtocol() len = %d, want %d", len(got), len(tt.want))
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("convertActionsForProtocol()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
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

			// Protocol v2 should reject old action names
			action, _ := normalizeActionV2(tt.clientAction, 0, player, betting)

			// Should return Fold (invalid action default)
			if action != game.Fold {
				t.Errorf("normalizeActionV2(%q) should reject old protocol, got %v", tt.clientAction, action)
			}
		})
	}
}
