package game

import (
	"reflect"
	"strings"
	"testing"
)

type playerConfig struct {
	chips    int
	bet      int
	totalBet int
	folded   bool
	allIn    bool
	acted    bool
}

func buildHandState(configs []playerConfig, currentBet, minRaise, activePlayer int) *HandState {
	players := make([]*Player, len(configs))
	eligible := make([]int, 0, len(configs))
	potAmount := 0

	for i, cfg := range configs {
		totalBet := cfg.totalBet
		if totalBet == 0 {
			totalBet = cfg.bet
		}
		players[i] = &Player{
			Seat:      i,
			Chips:     cfg.chips,
			Bet:       cfg.bet,
			TotalBet:  totalBet,
			Folded:    cfg.folded,
			AllInFlag: cfg.allIn,
		}
		if !cfg.folded {
			eligible = append(eligible, i)
		}
		potAmount += cfg.bet
	}

	state := &HandState{
		Players:      players,
		Button:       0,
		Street:       Preflop,
		PotManager:   NewPotManager(players),
		ActivePlayer: activePlayer,
		Betting: &BettingRound{
			CurrentBet:     currentBet,
			MinRaise:       minRaise,
			LastRaiser:     -1,
			ActedThisRound: make([]bool, len(players)),
		},
	}
	// Don't put money in pot - it's in player.Bet fields
	state.PotManager.pots = []Pot{{Amount: 0, Eligible: eligible}}

	for i, cfg := range configs {
		state.Betting.ActedThisRound[i] = cfg.acted
	}

	return state
}

func TestGetValidActionsScenarios(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		state  *HandState
		expect []Action
	}

	cases := []testCase{
		{
			name: "can raise when deep and facing zero to call",
			state: buildHandState(
				[]playerConfig{
					{chips: 200, bet: 10, acted: true},
					{chips: 90, bet: 10, acted: true},
					{chips: 90, bet: 10, acted: true},
				},
				10,
				10,
				0,
			),
			expect: []Action{Fold, Check, Raise},
		},
		{
			name: "short stack can only shove over free check",
			state: buildHandState(
				[]playerConfig{
					{chips: 8, bet: 10, acted: true},
					{chips: 90, bet: 10, acted: true},
					{chips: 90, bet: 10, acted: true},
				},
				10,
				12,
				0,
			),
			expect: []Action{Fold, Check, AllIn},
		},
		{
			name: "player with no chips can only fold or check",
			state: buildHandState(
				[]playerConfig{
					{chips: 0, bet: 10, acted: true},
					{chips: 90, bet: 10, acted: true},
					{chips: 90, bet: 10, acted: true},
				},
				10,
				10,
				0,
			),
			expect: []Action{Fold, Check},
		},
		{
			name: "facing a bet with enough chips to raise",
			state: buildHandState(
				[]playerConfig{
					{chips: 200, bet: 10, acted: false},
					{chips: 90, bet: 30, acted: true},
					{chips: 90, bet: 30, acted: true},
				},
				30,
				20,
				0,
			),
			expect: []Action{Fold, Call, Raise},
		},
		{
			name: "short stack facing bet can only call or shove",
			state: buildHandState(
				[]playerConfig{
					{chips: 30, bet: 10, acted: false},
					{chips: 90, bet: 30, acted: true},
					{chips: 90, bet: 30, acted: true},
				},
				30,
				20,
				0,
			),
			expect: []Action{Fold, Call, AllIn},
		},
		{
			name: "player covered by bet can only fold or shove",
			state: buildHandState(
				[]playerConfig{
					{chips: 15, bet: 10, acted: false},
					{chips: 90, bet: 30, acted: true},
					{chips: 90, bet: 30, acted: true},
				},
				30,
				20,
				0,
			),
			expect: []Action{Fold, AllIn},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.state.GetValidActions()
			if !reflect.DeepEqual(got, tc.expect) {
				t.Fatalf("GetValidActions mismatch:\nwant %v\n got %v", tc.expect, got)
			}
		})
	}
}

func TestProcessActionInvalidInputs(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name    string
		state   *HandState
		action  Action
		amount  int
		wantErr string
	}

	cases := []testCase{
		{
			name: "cannot check when facing bet",
			state: buildHandState(
				[]playerConfig{
					{chips: 100, bet: 0, acted: false},
					{chips: 90, bet: 20, acted: true},
					{chips: 90, bet: 20, acted: true},
				},
				20,
				20,
				0,
			),
			action:  Check,
			wantErr: "cannot check",
		},
		{
			name: "raise smaller than minimum",
			state: buildHandState(
				[]playerConfig{
					{chips: 100, bet: 20, totalBet: 20, acted: true},
					{chips: 90, bet: 20, totalBet: 20, acted: true},
					{chips: 90, bet: 20, totalBet: 20, acted: true},
				},
				20,
				20,
				0,
			),
			action:  Raise,
			amount:  35,
			wantErr: "minimum",
		},
		{
			name: "raise exceeding stack",
			state: buildHandState(
				[]playerConfig{
					{chips: 100, bet: 20, totalBet: 20, acted: true},
					{chips: 90, bet: 20, totalBet: 20, acted: true},
					{chips: 90, bet: 20, totalBet: 20, acted: true},
				},
				20,
				20,
				0,
			),
			action:  Raise,
			amount:  200,
			wantErr: "insufficient",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			player := tc.state.Players[tc.state.ActivePlayer]
			beforeBet := player.Bet
			beforeChips := player.Chips
			beforeTotal := player.TotalBet

			err := tc.state.ProcessAction(tc.action, tc.amount)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("unexpected error: %v", err)
			}

			if player.Bet != beforeBet {
				t.Fatalf("player bet changed on error: got %d want %d", player.Bet, beforeBet)
			}
			if player.Chips != beforeChips {
				t.Fatalf("player chips changed on error: got %d want %d", player.Chips, beforeChips)
			}
			if player.TotalBet != beforeTotal {
				t.Fatalf("player total bet changed on error: got %d want %d", player.TotalBet, beforeTotal)
			}
			if tc.state.ActivePlayer != 0 {
				t.Fatalf("active player advanced on error: got %d want 0", tc.state.ActivePlayer)
			}
		})
	}
}

func TestProcessActionAllInUpdatesState(t *testing.T) {
	t.Parallel()

	state := buildHandState(
		[]playerConfig{
			{chips: 40, bet: 10, totalBet: 10, acted: false},
			{chips: 90, bet: 30, totalBet: 30, acted: true},
			{chips: 90, bet: 30, totalBet: 30, acted: true},
		},
		30,
		20,
		0,
	)

	if err := state.ProcessAction(AllIn, 0); err != nil {
		t.Fatalf("all-in should succeed, got error: %v", err)
	}

	player := state.Players[0]
	if !player.AllInFlag {
		t.Fatal("player should be marked all-in")
	}
	if player.Chips != 0 {
		t.Fatalf("expected chips 0 after all-in, got %d", player.Chips)
	}
	if player.Bet != 50 {
		t.Fatalf("expected bet to be 50 after shove, got %d", player.Bet)
	}
	if state.Betting.CurrentBet != 50 {
		t.Fatalf("current bet not updated: got %d want 50", state.Betting.CurrentBet)
	}
	if state.Betting.MinRaise != 20 {
		t.Fatalf("min raise not updated: got %d want 20", state.Betting.MinRaise)
	}
	if state.Betting.LastRaiser != 0 {
		t.Fatalf("last raiser should be player 0, got %d", state.Betting.LastRaiser)
	}
	// Collect bets to update pots
	state.PotManager.CollectBets(state.Players)
	if state.GetPots()[0].Amount != 110 {
		t.Fatalf("pot should include shove amount, got %d", state.GetPots()[0].Amount)
	}
}

func TestIsBettingCompleteHonorsBigBlindOption(t *testing.T) {
	t.Parallel()

	state := buildHandState(
		[]playerConfig{
			{chips: 90, bet: 10, acted: true},  // Button/UTG
			{chips: 95, bet: 10, acted: true},  // Small blind
			{chips: 90, bet: 10, acted: false}, // Big blind has not acted
		},
		10,
		10,
		2,
	)

	state.Button = 0
	state.Street = Preflop
	state.Betting.BBActed = false
	state.Betting.LastRaiser = -1

	if state.Betting.IsBettingComplete(state.Players, state.Street, state.Button) {
		t.Fatal("expected betting to remain open for big blind option")
	}

	state.Betting.BBActed = true
	state.Betting.ActedThisRound[2] = true

	if !state.Betting.IsBettingComplete(state.Players, state.Street, state.Button) {
		t.Fatal("expected betting to complete once big blind acts")
	}
}

func TestForceFoldOutOfTurn(t *testing.T) {
	t.Parallel()

	state := buildHandState(
		[]playerConfig{
			{chips: 90, bet: 10, acted: true},
			{chips: 95, bet: 10, acted: false},
			{chips: 90, bet: 10, acted: false},
		},
		10,
		10,
		0,
	)

	state.ForceFold(2)

	if !state.Players[2].Folded {
		t.Fatal("expected seat 2 to be folded")
	}
	if state.ActivePlayer != 0 {
		t.Fatalf("active player changed unexpectedly: got %d want 0", state.ActivePlayer)
	}
}

func TestForceFoldActiveSeat(t *testing.T) {
	t.Parallel()

	state := buildHandState(
		[]playerConfig{
			{chips: 90, bet: 10, acted: true},
			{chips: 95, bet: 10, acted: false},
			{chips: 90, bet: 10, acted: false},
		},
		10,
		10,
		1,
	)

	state.ForceFold(1)

	if !state.Players[1].Folded {
		t.Fatal("expected seat 1 to be folded")
	}
	if state.ActivePlayer == 1 {
		t.Fatal("active player should advance after folding active seat")
	}
}
