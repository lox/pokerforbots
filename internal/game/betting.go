package game

import "sort"

// IsBettingRoundComplete checks if the current betting round is complete
func (t *Table) IsBettingRoundComplete() bool {
	playersInHand := 0
	playersActed := 0
	playersAllIn := 0

	for _, player := range t.activePlayers {
		if player.IsInHand() {
			playersInHand++
			if player.IsAllIn {
				playersAllIn++
			}

			// A player has "acted properly" if:
			// 1. They're all-in (can't act in current round, automatically counts as acted), OR
			// 2. They have acted AND bet the current amount (normal case)
			if player.IsAllIn {
				playersActed++
			} else if t.playersActed[player.ID] && player.BetThisRound == t.currentBet {
				playersActed++
			}
		}
	}

	return playersActed == playersInHand || playersInHand <= 1 || playersInHand-playersAllIn <= 1
}

// startNewBettingRound initializes a new betting round
func (t *Table) startNewBettingRound() {
	t.currentBet = 0
	t.minRaise = t.bigBlind // Reset to big blind for new betting round
	t.playersActed = make(map[int]bool)

	// Reset all players for new round
	for _, player := range t.activePlayers {
		if player.IsInHand() {
			player.ResetForNewRound()
		}
	}

	// Find first active player after dealer
	t.actionOn = t.findNextActivePlayer(t.getDealerIndex())
}

// postBlinds posts small and big blinds
func (t *Table) postBlinds() {
	var smallBlindPlayer, bigBlindPlayer *Player

	// Find blind players (include sitting out players for blind posting)
	for _, player := range t.players {
		if player.Chips > 0 { // Only consider players with chips
			switch player.Position {
			case SmallBlind:
				smallBlindPlayer = player
			case BigBlind:
				bigBlindPlayer = player
			}
		}
	}

	// Post blinds
	if smallBlindPlayer != nil {
		amount := min(t.smallBlind, smallBlindPlayer.Chips)
		// Small blind posting
		smallBlindPlayer.Call(amount)
		t.pot += amount

		// Publish small blind event
		if t.eventBus != nil {
			event := NewPlayerActionEvent(smallBlindPlayer, Call, amount, PreFlop, "", t.pot)
			t.eventBus.Publish(event)
		}
	}

	if bigBlindPlayer != nil {
		amount := min(t.bigBlind, bigBlindPlayer.Chips)
		// Big blind posting
		bigBlindPlayer.Call(amount)
		t.pot += amount
		t.currentBet = amount

		// Publish big blind event
		if t.eventBus != nil {
			event := NewPlayerActionEvent(bigBlindPlayer, Call, amount, PreFlop, "", t.pot)
			t.eventBus.Publish(event)
		}
	}
}

// setFirstToAct determines who acts first preflop
func (t *Table) setFirstToAct() {
	numPlayers := len(t.activePlayers)
	if numPlayers < 2 {
		return
	}

	// In heads-up, big blind acts first preflop
	// In multi-way, first player after big blind acts first
	var firstToAct *Player

	if numPlayers == 2 {
		for _, player := range t.activePlayers {
			if player.Position == BigBlind {
				firstToAct = player
				break
			}
		}
	} else {
		// For 3+ players, find the first player after big blind to act
		// This could be UTG in larger games, or the button in 3-player games
		for _, player := range t.activePlayers {
			if player.Position == UnderTheGun {
				firstToAct = player
				break
			}
		}

		// If no UTG (e.g., 3-player game), find player after big blind
		if firstToAct == nil {
			// Find big blind player index
			bigBlindIndex := -1
			for i, player := range t.activePlayers {
				if player.Position == BigBlind {
					bigBlindIndex = i
					break
				}
			}

			// First player after big blind
			if bigBlindIndex != -1 {
				nextIndex := (bigBlindIndex + 1) % len(t.activePlayers)
				firstToAct = t.activePlayers[nextIndex]
			}
		}
	}

	// Find the index of first to act
	for i, player := range t.activePlayers {
		if player == firstToAct {
			t.actionOn = i
			break
		}
	}
}

// AwardPot awards the pot to winner(s), handling both simple splits and side pots
func (t *Table) AwardPot() {
	if t.pot <= 0 {
		return
	}

	// Calculate side pots for multi-way all-in scenarios
	sidePots := CalculateSidePots(t.players, t.pot)

	if len(sidePots) == 0 {
		// Simple case: no side pots, just award to winners
		winners := t.FindWinners()
		t.splitPotWithButtonOrder(t.pot, winners)
	} else {
		// Complex case: award each side pot to eligible winners
		for _, sidePot := range sidePots {
			if len(sidePot.EligiblePlayers) == 0 || sidePot.Amount <= 0 {
				continue
			}

			// Find winners among eligible players
			allWinners := t.FindWinners()
			var winnersInSidePot []*Player

			eligibleSet := make(map[*Player]bool)
			for _, p := range sidePot.EligiblePlayers {
				eligibleSet[p] = true
			}

			for _, winner := range allWinners {
				if eligibleSet[winner] {
					winnersInSidePot = append(winnersInSidePot, winner)
				}
			}

			// If no winners in this side pot, award to first eligible player
			if len(winnersInSidePot) == 0 && len(sidePot.EligiblePlayers) > 0 {
				winnersInSidePot = []*Player{sidePot.EligiblePlayers[0]}
			}

			// Use table's button-order split function for proper remainder distribution
			t.splitPotWithButtonOrder(sidePot.Amount, winnersInSidePot)
		}
	}

	t.pot = 0 // Pot has been fully awarded
}

// splitPotWithButtonOrder splits pot giving remainder to player closest clockwise to button
func (t *Table) splitPotWithButtonOrder(potAmount int, winners []*Player) {
	if len(winners) == 0 || potAmount <= 0 {
		return
	}

	// Integer division for each player
	sharePerPlayer := potAmount / len(winners)
	remainder := potAmount % len(winners)

	// Give each player their share
	for _, winner := range winners {
		winner.Chips += sharePerPlayer
	}

	// Give remainder to player closest clockwise to button
	if remainder > 0 {
		closestToButton := t.findClosestToButton(winners)
		if closestToButton != nil {
			closestToButton.Chips += remainder
		} else {
			// Fallback: give to first winner
			winners[0].Chips += remainder
		}
	}
}

// findClosestToButton finds the player closest clockwise to the button among the given players
func (t *Table) findClosestToButton(players []*Player) *Player {
	if len(players) == 0 {
		return nil
	}

	// Find button position
	buttonSeat := t.dealerPosition
	if buttonSeat <= 0 {
		return players[0] // Fallback
	}

	// Find the player with the smallest clockwise distance from button
	closest := players[0]
	minDistance := t.clockwiseDistance(buttonSeat, closest.SeatNumber)

	for _, player := range players[1:] {
		distance := t.clockwiseDistance(buttonSeat, player.SeatNumber)
		if distance < minDistance {
			minDistance = distance
			closest = player
		}
	}

	return closest
}

// clockwiseDistance calculates clockwise distance from start to end seat
func (t *Table) clockwiseDistance(startSeat, endSeat int) int {
	if startSeat <= 0 || endSeat <= 0 {
		return 999 // Invalid seats get max distance
	}

	distance := endSeat - startSeat
	if distance <= 0 {
		distance += t.maxSeats // Wrap around
	}
	return distance
}

// getDealerIndex returns the index of the dealer in active players
func (t *Table) getDealerIndex() int {
	for i, player := range t.activePlayers {
		if player.Position == Button || (len(t.activePlayers) == 2 && player.Position == SmallBlind) {
			return i
		}
	}
	return 0
}

// findNextActivePlayer finds the next player who can act
func (t *Table) findNextActivePlayer(startIndex int) int {
	for i := 1; i <= len(t.activePlayers); i++ {
		index := (startIndex + i) % len(t.activePlayers)
		player := t.activePlayers[index]

		// Player can act if they're active, not folded, not all-in, AND
		// either they haven't acted yet OR they haven't matched the current bet
		if player.CanAct() && (!t.playersActed[player.ID] || player.BetThisRound < t.currentBet) {
			return index
		}
	}
	return -1 // No active players
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SidePot represents a side pot in a multi-way all-in scenario
type SidePot struct {
	Amount          int       // Amount in this side pot
	EligiblePlayers []*Player // Players eligible to win this side pot
}

// CalculateSidePots calculates side pots for multi-way all-in scenarios
// This is the correct way to handle side pots in Texas Hold'em
func CalculateSidePots(players []*Player, mainPot int) []SidePot {
	// Collect all players who contributed to the pot
	type contribution struct {
		player *Player
		amount int
	}

	var contributions []contribution
	hasAllIn := false
	for _, p := range players {
		if p.TotalBet > 0 {
			contributions = append(contributions, contribution{
				player: p,
				amount: p.TotalBet,
			})
			if p.IsAllIn {
				hasAllIn = true
			}
		}
	}

	if len(contributions) == 0 {
		return nil
	}

	// Only create side pots if there's an all-in situation
	// Otherwise, use simple pot distribution
	if !hasAllIn {
		return nil
	}

	// Sort by contribution amount (ascending)
	sort.Slice(contributions, func(i, j int) bool {
		return contributions[i].amount < contributions[j].amount
	})

	var sidePots []SidePot
	remainingPot := mainPot
	prevLevel := 0

	for i := 0; i < len(contributions); i++ {
		currentLevel := contributions[i].amount

		// Count all players who contributed at this level or higher (for pot calculation)
		contributorsAtLevel := len(contributions) - i

		// Only players still in hand are eligible to win (separate from pot calculation)
		eligible := make([]*Player, 0, len(contributions)-i)
		for j := i; j < len(contributions); j++ {
			// Only include players who are still in the hand (not folded)
			if contributions[j].player.IsInHand() {
				eligible = append(eligible, contributions[j].player)
			}
		}

		if len(eligible) == 0 {
			continue
		}

		// Calculate the amount for this side pot
		// It's (currentLevel - prevLevel) * number of ALL contributors at this level
		levelDiff := currentLevel - prevLevel
		sidePotAmount := levelDiff * contributorsAtLevel

		// Don't exceed remaining pot
		if sidePotAmount > remainingPot {
			sidePotAmount = remainingPot
		}

		if sidePotAmount > 0 {
			sidePots = append(sidePots, SidePot{
				Amount:          sidePotAmount,
				EligiblePlayers: eligible,
			})

			remainingPot -= sidePotAmount
			if remainingPot <= 0 {
				break
			}
		}

		prevLevel = currentLevel

		// Skip to next unique contribution level
		for i+1 < len(contributions) && contributions[i+1].amount == currentLevel {
			i++
		}
	}

	return sidePots
}

// AwardSidePots awards multiple side pots to their respective winners
// Note: This uses the basic splitPot function. For button-order remainder distribution,
// the caller should use Table.splitPotWithButtonOrder() directly.
func AwardSidePots(sidePots []SidePot, handEvaluator func([]*Player) []*Player) {
	for _, sidePot := range sidePots {
		if len(sidePot.EligiblePlayers) == 0 || sidePot.Amount <= 0 {
			continue
		}

		// Find winners among eligible players
		winners := handEvaluator(sidePot.EligiblePlayers)
		if len(winners) == 0 {
			// Fallback: award to first eligible player
			winners = []*Player{sidePot.EligiblePlayers[0]}
		}

		// Split this side pot among winners
		// TODO: Use button-order remainder distribution for side pots too
		splitPot(sidePot.Amount, winners)
	}
}

// splitPot splits a pot amount among multiple winners, with remainder going to player closest clockwise to button
func splitPot(potAmount int, winners []*Player) {
	if len(winners) == 0 || potAmount <= 0 {
		return
	}

	// Integer division for each player
	sharePerPlayer := potAmount / len(winners)
	remainder := potAmount % len(winners)

	// Give each player their share
	for _, winner := range winners {
		winner.Chips += sharePerPlayer
	}

	// Give remainder to first winner (closest clockwise to button)
	// TODO: Implement proper button-relative ordering when table context is available
	// For now, give remainder to first winner as a reasonable approximation
	if remainder > 0 {
		winners[0].Chips += remainder
	}
}
