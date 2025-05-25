package game

import (
	"fmt"
	"testing"
)

func TestNewTable(t *testing.T) {
	table := NewTable(6, 1, 2)

	if table.MaxSeats != 6 {
		t.Errorf("Expected 6 seats, got %d", table.MaxSeats)
	}

	if table.SmallBlind != 1 {
		t.Errorf("Expected small blind 1, got %d", table.SmallBlind)
	}

	if table.BigBlind != 2 {
		t.Errorf("Expected big blind 2, got %d", table.BigBlind)
	}

	if table.State != WaitingToStart {
		t.Errorf("Expected WaitingToStart state, got %s", table.State)
	}
}

func TestAddPlayer(t *testing.T) {
	table := NewTable(6, 1, 2)

	player1 := NewPlayer(1, "Alice", Human, 200)
	player2 := NewPlayer(2, "Bob", AI, 200)

	if !table.AddPlayer(player1) {
		t.Error("Should be able to add first player")
	}

	if !table.AddPlayer(player2) {
		t.Error("Should be able to add second player")
	}

	if len(table.Players) != 2 {
		t.Errorf("Expected 2 players, got %d", len(table.Players))
	}

	// Check seat assignments
	if player1.SeatNumber != 1 {
		t.Errorf("Expected player1 in seat 1, got %d", player1.SeatNumber)
	}

	if player2.SeatNumber != 2 {
		t.Errorf("Expected player2 in seat 2, got %d", player2.SeatNumber)
	}
}

func TestTableFull(t *testing.T) {
	table := NewTable(2, 1, 2) // Small table for testing

	player1 := NewPlayer(1, "Alice", Human, 200)
	player2 := NewPlayer(2, "Bob", AI, 200)
	player3 := NewPlayer(3, "Charlie", AI, 200)

	table.AddPlayer(player1)
	table.AddPlayer(player2)

	// Third player should not be able to join
	if table.AddPlayer(player3) {
		t.Error("Should not be able to add player to full table")
	}
}

func TestStartNewHand(t *testing.T) {
	table := NewTable(6, 1, 2)

	// Add players
	player1 := NewPlayer(1, "Alice", Human, 200)
	player2 := NewPlayer(2, "Bob", AI, 200)
	table.AddPlayer(player1)
	table.AddPlayer(player2)

	// Start hand
	table.StartNewHand()

	if table.State != InProgress {
		t.Errorf("Expected InProgress state, got %s", table.State)
	}

	if table.HandNumber != 1 {
		t.Errorf("Expected hand number 1, got %d", table.HandNumber)
	}

	if table.CurrentRound != PreFlop {
		t.Errorf("Expected PreFlop round, got %s", table.CurrentRound)
	}

	// Check that players have hole cards
	for _, player := range table.ActivePlayers {
		if len(player.HoleCards) != 2 {
			t.Errorf("Player %s should have 2 hole cards, got %d", player.Name, len(player.HoleCards))
		}
	}

	// Check blinds were posted
	if table.Pot != 3 { // 1 + 2
		t.Errorf("Expected pot of 3 after blinds, got %d", table.Pot)
	}
}

func TestPositionsHeadsUp(t *testing.T) {
	table := NewTable(6, 1, 2)

	player1 := NewPlayer(1, "Alice", Human, 200)
	player2 := NewPlayer(2, "Bob", AI, 200)
	table.AddPlayer(player1)
	table.AddPlayer(player2)

	table.StartNewHand()

	// In heads-up, dealer is small blind
	var sbPlayer, bbPlayer *Player
	for _, player := range table.ActivePlayers {
		switch player.Position {
		case SmallBlind:
			sbPlayer = player
		case BigBlind:
			bbPlayer = player
		}
	}

	if sbPlayer == nil {
		t.Error("Should have a small blind player")
	}

	if bbPlayer == nil {
		t.Error("Should have a big blind player")
	}

	if sbPlayer == bbPlayer {
		t.Error("Small blind and big blind should be different players")
	}
}

func TestPositionsMultiWay(t *testing.T) {
	table := NewTable(6, 1, 2)

	// Add 4 players
	for i := 1; i <= 4; i++ {
		player := NewPlayer(i, fmt.Sprintf("Player%d", i), AI, 200)
		table.AddPlayer(player)
	}

	table.StartNewHand()

	// Check that we have button, small blind, big blind, and UTG
	positions := make(map[Position]bool)
	for _, player := range table.ActivePlayers {
		positions[player.Position] = true
	}

	expectedPositions := []Position{Button, SmallBlind, BigBlind, UnderTheGun}
	for _, pos := range expectedPositions {
		if !positions[pos] {
			t.Errorf("Missing position: %s", pos)
		}
	}
}

func TestPlayerActions(t *testing.T) {
	player := NewPlayer(1, "Alice", Human, 200)

	// Test fold
	player.Fold()
	if !player.IsFolded || player.IsActive {
		t.Error("Player should be folded and inactive after folding")
	}

	// Reset for next test
	player.ResetForNewHand()

	// Test call
	if !player.Call(20) {
		t.Error("Player should be able to call 20")
	}

	if player.Chips != 180 {
		t.Errorf("Expected 180 chips after calling 20, got %d", player.Chips)
	}

	if player.LastAction != Call {
		t.Errorf("Expected Call action, got %s", player.LastAction)
	}

	// Test call more than available chips (should trigger all-in)
	remaining := player.Chips
	result := player.Call(remaining + 100) // Try to call more than available
	if !result {
		t.Error("Call should succeed but trigger all-in when insufficient chips")
	}

	if player.Chips != 0 {
		t.Errorf("Expected 0 chips after calling more than available, got %d", player.Chips)
	}

	if !player.IsAllIn {
		t.Error("Player should be all-in after calling more than available chips")
	}

	// Reset player to test explicit all-in
	player.ResetForNewHand()
	player.Chips = 100

	// Test explicit all-in
	if !player.AllIn() {
		t.Error("Player should be able to go all-in")
	}

	if player.Chips != 0 {
		t.Errorf("Expected 0 chips after all-in, got %d", player.Chips)
	}

	if !player.IsAllIn {
		t.Error("Player should be marked as all-in")
	}
}

func TestBettingRounds(t *testing.T) {
	table := NewTable(6, 1, 2)

	player1 := NewPlayer(1, "Alice", Human, 200)
	player2 := NewPlayer(2, "Bob", AI, 200)
	table.AddPlayer(player1)
	table.AddPlayer(player2)

	table.StartNewHand()

	// Should start at preflop
	if table.CurrentRound != PreFlop {
		t.Errorf("Expected PreFlop, got %s", table.CurrentRound)
	}

	// Deal flop
	table.DealFlop()
	if table.CurrentRound != Flop {
		t.Errorf("Expected Flop, got %s", table.CurrentRound)
	}

	if len(table.CommunityCards) != 3 {
		t.Errorf("Expected 3 community cards after flop, got %d", len(table.CommunityCards))
	}

	// Deal turn
	table.DealTurn()
	if table.CurrentRound != Turn {
		t.Errorf("Expected Turn, got %s", table.CurrentRound)
	}

	if len(table.CommunityCards) != 4 {
		t.Errorf("Expected 4 community cards after turn, got %d", len(table.CommunityCards))
	}

	// Deal river
	table.DealRiver()
	if table.CurrentRound != River {
		t.Errorf("Expected River, got %s", table.CurrentRound)
	}

	if len(table.CommunityCards) != 5 {
		t.Errorf("Expected 5 community cards after river, got %d", len(table.CommunityCards))
	}
}
