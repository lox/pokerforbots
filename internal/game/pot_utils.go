package game

import "sort"

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
