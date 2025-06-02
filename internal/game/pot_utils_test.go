package game

import (
	"testing"

	"github.com/lox/pokerforbots/internal/deck"
)

func TestCalculateSidePots_SimpleCase(t *testing.T) {
	// Simple case: no all-ins, everyone bets the same amount
	// Should NOT create side pots (use regular pot distribution)
	players := []*Player{
		{Name: "Alice", TotalBet: 100, IsActive: true, IsAllIn: false},
		{Name: "Bob", TotalBet: 100, IsActive: true, IsAllIn: false},
		{Name: "Charlie", TotalBet: 100, IsActive: true, IsAllIn: false},
	}

	sidePots := CalculateSidePots(players, 300)

	// Should NOT create side pots when no one is all-in
	if len(sidePots) != 0 {
		t.Errorf("Expected 0 side pots when no all-ins, got %d", len(sidePots))
	}
}

func TestCalculateSidePots_OneAllIn(t *testing.T) {
	// Alice goes all-in with small stack, others call
	players := []*Player{
		{Name: "Alice", TotalBet: 50, IsActive: true, IsAllIn: true},     // All-in with small stack
		{Name: "Bob", TotalBet: 100, IsActive: true, IsAllIn: false},     // Called the all-in and more
		{Name: "Charlie", TotalBet: 100, IsActive: true, IsAllIn: false}, // Called the all-in and more
	}

	sidePots := CalculateSidePots(players, 250)

	// Should create two side pots:
	// Main pot: 50 * 3 = 150 (all three eligible)
	// Side pot: 50 * 2 = 100 (Bob and Charlie only)
	if len(sidePots) != 2 {
		t.Errorf("Expected 2 side pots, got %d", len(sidePots))
	}

	// First side pot (main pot)
	if sidePots[0].Amount != 150 {
		t.Errorf("Expected main pot 150, got %d", sidePots[0].Amount)
	}
	if len(sidePots[0].EligiblePlayers) != 3 {
		t.Errorf("Expected 3 eligible players in main pot, got %d", len(sidePots[0].EligiblePlayers))
	}

	// Second side pot
	if sidePots[1].Amount != 100 {
		t.Errorf("Expected side pot 100, got %d", sidePots[1].Amount)
	}
	if len(sidePots[1].EligiblePlayers) != 2 {
		t.Errorf("Expected 2 eligible players in side pot, got %d", len(sidePots[1].EligiblePlayers))
	}
}

func TestCalculateSidePots_MultipleAllIns(t *testing.T) {
	// Complex scenario: multiple all-ins with different amounts
	players := []*Player{
		{Name: "Alice", TotalBet: 30, IsActive: true, IsAllIn: true},     // Smallest all-in
		{Name: "Bob", TotalBet: 70, IsActive: true, IsAllIn: true},       // Medium all-in
		{Name: "Charlie", TotalBet: 100, IsActive: true, IsAllIn: false}, // Largest bet
		{Name: "David", TotalBet: 100, IsActive: true, IsAllIn: false},   // Matched largest
	}

	sidePots := CalculateSidePots(players, 300)

	// Should create three side pots:
	// Main pot: 30 * 4 = 120 (all four eligible)
	// Side pot 1: 40 * 3 = 120 (Bob, Charlie, David)
	// Side pot 2: 30 * 2 = 60 (Charlie, David)
	if len(sidePots) != 3 {
		t.Errorf("Expected 3 side pots, got %d", len(sidePots))
	}

	// Verify amounts
	expectedAmounts := []int{120, 120, 60}
	expectedEligible := []int{4, 3, 2}

	for i, expected := range expectedAmounts {
		if sidePots[i].Amount != expected {
			t.Errorf("Side pot %d: expected amount %d, got %d", i, expected, sidePots[i].Amount)
		}
	}

	for i, expected := range expectedEligible {
		if len(sidePots[i].EligiblePlayers) != expected {
			t.Errorf("Side pot %d: expected %d eligible players, got %d", i, expected, len(sidePots[i].EligiblePlayers))
		}
	}
}

func TestCalculateSidePots_WithFoldedPlayers(t *testing.T) {
	// Scenario where some players have folded and there's an all-in
	players := []*Player{
		{Name: "Alice", TotalBet: 50, IsActive: false, IsFolded: true, IsAllIn: false}, // Folded
		{Name: "Bob", TotalBet: 50, IsActive: true, IsAllIn: true},                     // All-in
		{Name: "Charlie", TotalBet: 100, IsActive: true, IsAllIn: false},               // Regular bet
	}

	sidePots := CalculateSidePots(players, 200)

	// Even though Alice contributed, she's folded so not eligible
	if len(sidePots) != 2 {
		t.Errorf("Expected 2 side pots, got %d", len(sidePots))
	}

	// Main pot: 50 * 3 = 150 (includes Alice's contribution even though she's folded)
	if sidePots[0].Amount != 150 {
		t.Errorf("Expected main pot 150, got %d", sidePots[0].Amount)
	}
	if len(sidePots[0].EligiblePlayers) != 2 {
		t.Errorf("Expected 2 eligible players in main pot, got %d", len(sidePots[0].EligiblePlayers))
	}

	// Verify folded player is not in eligible list
	for _, player := range sidePots[0].EligiblePlayers {
		if player.Name == "Alice" {
			t.Error("Folded player should not be eligible for side pot")
		}
	}
}

func TestAwardSidePots_BasicSplit(t *testing.T) {
	// Test awarding side pots with a simple split
	alice := &Player{Name: "Alice", Chips: 1000}
	bob := &Player{Name: "Bob", Chips: 1000}
	charlie := &Player{Name: "Charlie", Chips: 1000}

	sidePots := []SidePot{
		{
			Amount:          300,
			EligiblePlayers: []*Player{alice, bob, charlie},
		},
	}

	// Mock hand evaluator that returns all players as winners (tie)
	handEvaluator := func(players []*Player) []*Player {
		return players // Everyone ties
	}

	AwardSidePots(sidePots, handEvaluator)

	// Should split 300 three ways: 100 each
	if alice.Chips != 1100 {
		t.Errorf("Alice should have 1100 chips, got %d", alice.Chips)
	}
	if bob.Chips != 1100 {
		t.Errorf("Bob should have 1100 chips, got %d", bob.Chips)
	}
	if charlie.Chips != 1100 {
		t.Errorf("Charlie should have 1100 chips, got %d", charlie.Chips)
	}
}

func TestAwardSidePots_DifferentWinners(t *testing.T) {
	// Test awarding side pots where different players win different pots
	alice := &Player{Name: "Alice", Chips: 1000, HoleCards: deck.MustParseCards("AsKs")}
	bob := &Player{Name: "Bob", Chips: 1000, HoleCards: deck.MustParseCards("QhQd")}
	charlie := &Player{Name: "Charlie", Chips: 1000, HoleCards: deck.MustParseCards("JcJd")}

	sidePots := []SidePot{
		{
			Amount:          150, // Main pot - all eligible
			EligiblePlayers: []*Player{alice, bob, charlie},
		},
		{
			Amount:          100, // Side pot - only Bob and Charlie eligible
			EligiblePlayers: []*Player{bob, charlie},
		},
	}

	// Mock hand evaluator: Alice wins main pot, Bob wins side pot
	handEvaluator := func(players []*Player) []*Player {
		// Alice wins if she's in the list, otherwise Bob wins
		for _, p := range players {
			if p.Name == "Alice" {
				return []*Player{alice}
			}
		}
		return []*Player{bob}
	}

	AwardSidePots(sidePots, handEvaluator)

	// Alice wins main pot (150), Bob wins side pot (100), Charlie gets nothing
	if alice.Chips != 1150 {
		t.Errorf("Alice should have 1150 chips, got %d", alice.Chips)
	}
	if bob.Chips != 1100 {
		t.Errorf("Bob should have 1100 chips, got %d", bob.Chips)
	}
	if charlie.Chips != 1000 {
		t.Errorf("Charlie should have 1000 chips, got %d", charlie.Chips)
	}
}

func TestSplitPot_EdgeCases(t *testing.T) {
	// Test edge cases for pot splitting
	t.Run("Empty winners", func(t *testing.T) {
		var winners []*Player
		splitPot(100, winners) // Should not panic
	})

	t.Run("Zero pot", func(t *testing.T) {
		alice := &Player{Name: "Alice", Chips: 1000}
		winners := []*Player{alice}
		splitPot(0, winners)

		if alice.Chips != 1000 {
			t.Errorf("Alice should still have 1000 chips, got %d", alice.Chips)
		}
	})

	t.Run("Negative pot", func(t *testing.T) {
		alice := &Player{Name: "Alice", Chips: 1000}
		winners := []*Player{alice}
		splitPot(-50, winners)

		if alice.Chips != 1000 {
			t.Errorf("Alice should still have 1000 chips, got %d", alice.Chips)
		}
	})

	t.Run("One chip split among many", func(t *testing.T) {
		alice := &Player{Name: "Alice", Chips: 1000}
		bob := &Player{Name: "Bob", Chips: 1000}
		charlie := &Player{Name: "Charlie", Chips: 1000}
		winners := []*Player{alice, bob, charlie}

		splitPot(1, winners)

		// Alice gets the remainder
		if alice.Chips != 1001 {
			t.Errorf("Alice should have 1001 chips, got %d", alice.Chips)
		}
		if bob.Chips != 1000 {
			t.Errorf("Bob should have 1000 chips, got %d", bob.Chips)
		}
		if charlie.Chips != 1000 {
			t.Errorf("Charlie should have 1000 chips, got %d", charlie.Chips)
		}
	})
}
