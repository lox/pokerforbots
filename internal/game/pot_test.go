package game

import (
	"reflect"
	"testing"

	"github.com/lox/pokerforbots/v2/poker"
)

func TestPotManagerBasics(t *testing.T) {
	t.Parallel()

	players := []*Player{
		{Seat: 0, Chips: 100, Folded: false},
		{Seat: 1, Chips: 100, Folded: false},
		{Seat: 2, Chips: 100, Folded: false},
	}

	pm := NewPotManager(players)

	// Initial pot should be empty with all players eligible
	pots := pm.GetPots()
	if len(pots) != 1 {
		t.Fatalf("Expected 1 pot, got %d", len(pots))
	}
	if pots[0].Amount != 0 {
		t.Errorf("Initial pot should be 0, got %d", pots[0].Amount)
	}
	if len(pots[0].Eligible) != 3 {
		t.Errorf("All 3 players should be eligible, got %d", len(pots[0].Eligible))
	}
}

func TestCollectBets(t *testing.T) {
	t.Parallel()

	players := []*Player{
		{Seat: 0, Chips: 80, Bet: 20, Folded: false},
		{Seat: 1, Chips: 70, Bet: 30, Folded: false},
		{Seat: 2, Chips: 60, Bet: 40, Folded: false},
	}

	pm := NewPotManager(players)
	pm.CollectBets(players)

	// Bets should be collected into pot
	if pm.Total() != 90 {
		t.Errorf("Pot should be 90 (20+30+40), got %d", pm.Total())
	}

	// Player bets should be cleared
	for i, p := range players {
		if p.Bet != 0 {
			t.Errorf("Player %d bet should be 0 after collection, got %d", i, p.Bet)
		}
	}
}

func TestGetPotsWithUncollected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		pots            []Pot
		playerBets      []int
		expectedAmounts []int
	}{
		{
			name: "single pot with uncollected bets",
			pots: []Pot{
				{Amount: 100, Eligible: []int{0, 1, 2}},
			},
			playerBets:      []int{10, 10, 10},
			expectedAmounts: []int{130}, // 100 + 30 uncollected
		},
		{
			name: "side pots with uncollected bets go to last pot",
			pots: []Pot{
				{Amount: 60, Eligible: []int{0, 1, 2}}, // Main pot (all-in player capped)
				{Amount: 40, Eligible: []int{1, 2}},    // Side pot for active players
			},
			playerBets:      []int{0, 20, 20}, // Player 0 all-in, 1&2 still betting
			expectedAmounts: []int{60, 80},    // Uncollected goes to side pot
		},
		{
			name: "multiple side pots - uncollected to last",
			pots: []Pot{
				{Amount: 30, Eligible: []int{0, 1, 2}},
				{Amount: 20, Eligible: []int{1, 2}},
				{Amount: 10, Eligible: []int{2}},
			},
			playerBets:      []int{0, 0, 25},
			expectedAmounts: []int{30, 20, 35}, // Only player 2 betting, goes to their pot
		},
		{
			name: "no uncollected bets",
			pots: []Pot{
				{Amount: 100, Eligible: []int{0, 1}},
			},
			playerBets:      []int{0, 0, 0},
			expectedAmounts: []int{100},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			players := make([]*Player, len(tc.playerBets))
			for i, bet := range tc.playerBets {
				players[i] = &Player{Seat: i, Bet: bet}
			}

			pm := &PotManager{pots: tc.pots}
			result := pm.GetPotsWithUncollected(players)

			if len(result) != len(tc.expectedAmounts) {
				t.Fatalf("Expected %d pots, got %d", len(tc.expectedAmounts), len(result))
			}

			for i, expected := range tc.expectedAmounts {
				if result[i].Amount != expected {
					t.Errorf("Pot %d: expected amount %d, got %d", i, expected, result[i].Amount)
				}
			}
		})
	}
}

func TestCalculateSidePots(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		players      []*Player
		expectedPots []Pot
	}{
		{
			name: "no all-ins, single pot",
			players: []*Player{
				{Seat: 0, TotalBet: 100, Folded: false, AllInFlag: false},
				{Seat: 1, TotalBet: 100, Folded: false, AllInFlag: false},
				{Seat: 2, TotalBet: 100, Folded: false, AllInFlag: false},
			},
			expectedPots: []Pot{
				{Amount: 300, Eligible: []int{0, 1, 2}},
			},
		},
		{
			name: "one player all-in creates side pot",
			players: []*Player{
				{Seat: 0, TotalBet: 50, Folded: false, AllInFlag: true},
				{Seat: 1, TotalBet: 100, Folded: false, AllInFlag: false},
				{Seat: 2, TotalBet: 100, Folded: false, AllInFlag: false},
			},
			expectedPots: []Pot{
				{Amount: 150, Eligible: []int{0, 1, 2}, MaxPerPlayer: 50}, // Main pot
				{Amount: 100, Eligible: []int{1, 2}},                      // Side pot
			},
		},
		{
			name: "multiple all-ins at different levels",
			players: []*Player{
				{Seat: 0, TotalBet: 25, Folded: false, AllInFlag: true},
				{Seat: 1, TotalBet: 75, Folded: false, AllInFlag: true},
				{Seat: 2, TotalBet: 150, Folded: false, AllInFlag: false},
			},
			expectedPots: []Pot{
				{Amount: 75, Eligible: []int{0, 1, 2}, MaxPerPlayer: 25}, // Smallest all-in
				{Amount: 100, Eligible: []int{1, 2}, MaxPerPlayer: 75},   // Next all-in
				{Amount: 75, Eligible: []int{2}},                         // Remainder
			},
		},
		{
			name: "folded players excluded from pots",
			players: []*Player{
				{Seat: 0, TotalBet: 50, Folded: true, AllInFlag: false},
				{Seat: 1, TotalBet: 100, Folded: false, AllInFlag: true},
				{Seat: 2, TotalBet: 100, Folded: false, AllInFlag: false},
			},
			expectedPots: []Pot{
				{Amount: 250, Eligible: []int{1, 2}, MaxPerPlayer: 100},
			},
		},
		{
			name: "all players all-in at same amount",
			players: []*Player{
				{Seat: 0, TotalBet: 100, Folded: false, AllInFlag: true},
				{Seat: 1, TotalBet: 100, Folded: false, AllInFlag: true},
				{Seat: 2, TotalBet: 100, Folded: false, AllInFlag: true},
			},
			expectedPots: []Pot{
				{Amount: 300, Eligible: []int{0, 1, 2}, MaxPerPlayer: 100},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pm := NewPotManager(tc.players)

			// Set up initial pot with total bets
			totalBets := 0
			for _, p := range tc.players {
				totalBets += p.TotalBet
			}
			pm.pots[0].Amount = totalBets

			pm.CalculateSidePots(tc.players)
			pots := pm.GetPots()

			if len(pots) != len(tc.expectedPots) {
				t.Fatalf("Expected %d pots, got %d", len(tc.expectedPots), len(pots))
			}

			for i, expected := range tc.expectedPots {
				if pots[i].Amount != expected.Amount {
					t.Errorf("Pot %d: expected amount %d, got %d", i, expected.Amount, pots[i].Amount)
				}
				if !reflect.DeepEqual(pots[i].Eligible, expected.Eligible) {
					t.Errorf("Pot %d: expected eligible %v, got %v", i, expected.Eligible, pots[i].Eligible)
				}
			}
		})
	}
}

func TestSidePotsWithOngoingBetting(t *testing.T) {
	t.Parallel()

	// Scenario: Player 0 goes all-in for 50 on flop
	// Players 1 and 2 continue betting on turn and river

	// After flop (all bets collected)
	players := []*Player{
		{Seat: 0, Chips: 0, TotalBet: 50, Folded: false, AllInFlag: true, HoleCards: poker.Hand(0)},
		{Seat: 1, Chips: 100, TotalBet: 50, Folded: false, AllInFlag: false, HoleCards: poker.Hand(0)},
		{Seat: 2, Chips: 100, TotalBet: 50, Folded: false, AllInFlag: false, HoleCards: poker.Hand(0)},
	}

	pm := NewPotManager(players)
	pm.pots[0].Amount = 150 // Initial collection
	pm.CalculateSidePots(players)

	// Should have main pot only at this point
	pots := pm.GetPots()
	if len(pots) != 1 {
		t.Fatalf("After flop all-in, expected 1 pot, got %d", len(pots))
	}
	if pots[0].Amount != 150 {
		t.Errorf("Main pot should be 150, got %d", pots[0].Amount)
	}

	// Turn: Players 1 and 2 bet 30 each (uncollected)
	players[1].Bet = 30
	players[2].Bet = 30

	// Get pots with uncollected bets
	potsWithUncollected := pm.GetPotsWithUncollected(players)
	if len(potsWithUncollected) != 1 {
		t.Fatalf("Expected 1 pot with uncollected, got %d", len(potsWithUncollected))
	}

	// With only one pot, uncollected should go there
	if potsWithUncollected[0].Amount != 210 { // 150 + 60
		t.Errorf("Pot with uncollected should be 210, got %d", potsWithUncollected[0].Amount)
	}

	// Now collect the turn bets and recalculate
	pm.CollectBets(players)
	players[1].TotalBet += 30
	players[2].TotalBet += 30
	pm.CalculateSidePots(players)

	// Should now have main pot and side pot
	pots = pm.GetPots()
	if len(pots) != 2 {
		t.Fatalf("After turn betting, expected 2 pots, got %d", len(pots))
	}
	if pots[0].Amount != 150 {
		t.Errorf("Main pot should be 150, got %d", pots[0].Amount)
	}
	if pots[1].Amount != 60 {
		t.Errorf("Side pot should be 60, got %d", pots[1].Amount)
	}

	// River: Players 1 and 2 bet 40 each (uncollected)
	players[1].Bet = 40
	players[2].Bet = 40

	// Get pots with uncollected - should add to LAST pot
	potsWithUncollected = pm.GetPotsWithUncollected(players)
	if len(potsWithUncollected) != 2 {
		t.Fatalf("Expected 2 pots with uncollected, got %d", len(potsWithUncollected))
	}
	if potsWithUncollected[0].Amount != 150 {
		t.Errorf("Main pot should stay 150, got %d", potsWithUncollected[0].Amount)
	}
	if potsWithUncollected[1].Amount != 140 { // 60 + 80
		t.Errorf("Side pot with uncollected should be 140, got %d", potsWithUncollected[1].Amount)
	}
}

func TestPotManagerTotal(t *testing.T) {
	t.Parallel()

	pm := &PotManager{
		pots: []Pot{
			{Amount: 100},
			{Amount: 50},
			{Amount: 25},
		},
	}

	if pm.Total() != 175 {
		t.Errorf("Total should be 175, got %d", pm.Total())
	}
}

func TestMakeEligible(t *testing.T) {
	t.Parallel()

	players := []*Player{
		{Seat: 0, Folded: false},
		{Seat: 1, Folded: true},
		{Seat: 2, Folded: false},
		{Seat: 3, Folded: false},
		{Seat: 4, Folded: true},
	}

	eligible := makeEligible(players)
	expected := []int{0, 2, 3}

	if !reflect.DeepEqual(eligible, expected) {
		t.Errorf("Expected eligible %v, got %v", expected, eligible)
	}
}
