package game

// Pot represents a pot (main or side)
type Pot struct {
	Amount       int
	Eligible     []int // Seat numbers eligible for this pot
	MaxPerPlayer int   // Maximum contribution per player
}

// PotManager manages main and side pots
type PotManager struct {
	pots []Pot
}

// NewPotManager creates a new pot manager
func NewPotManager(players []*Player) *PotManager {
	return &PotManager{
		pots: []Pot{{
			Amount:   0,
			Eligible: makeEligible(players),
		}},
	}
}

// makeEligible creates a list of eligible seats
func makeEligible(players []*Player) []int {
	eligible := make([]int, 0, len(players))
	for _, p := range players {
		if !p.Folded {
			eligible = append(eligible, p.Seat)
		}
	}
	return eligible
}

// Total returns the total amount in all pots
func (pm *PotManager) Total() int {
	total := 0
	for _, pot := range pm.pots {
		total += pot.Amount
	}
	return total
}

// CollectBets collects bets from players and adds to the main pot
func (pm *PotManager) CollectBets(players []*Player) {
	for _, player := range players {
		if player.Bet > 0 {
			pm.pots[0].Amount += player.Bet
			player.Bet = 0
		}
	}
}

// CalculateSidePots calculates side pots based on player all-ins
func (pm *PotManager) CalculateSidePots(players []*Player) {
	// First, identify all unique all-in amounts
	allInAmounts := make(map[int]bool)
	for _, p := range players {
		if p.AllInFlag && p.TotalBet > 0 {
			allInAmounts[p.TotalBet] = true
		}
	}

	if len(allInAmounts) == 0 {
		return // No all-ins, just main pot
	}

	// Convert to sorted slice
	amounts := make([]int, 0, len(allInAmounts))
	for amount := range allInAmounts {
		amounts = append(amounts, amount)
	}

	// Sort amounts
	for i := 0; i < len(amounts); i++ {
		for j := i + 1; j < len(amounts); j++ {
			if amounts[i] > amounts[j] {
				amounts[i], amounts[j] = amounts[j], amounts[i]
			}
		}
	}

	// Reset pots
	pm.pots = []Pot{}

	// Create side pots for each all-in level
	previousMax := 0
	for _, maxBet := range amounts {
		pot := Pot{
			MaxPerPlayer: maxBet,
		}

		// Determine who is eligible for this pot
		for _, p := range players {
			if !p.Folded && p.TotalBet > previousMax {
				pot.Eligible = append(pot.Eligible, p.Seat)
			}
		}

		// Calculate pot amount
		for _, p := range players {
			contribution := p.TotalBet - previousMax
			if contribution > maxBet-previousMax {
				contribution = maxBet - previousMax
			}
			if contribution > 0 {
				pot.Amount += contribution
			}
		}

		if pot.Amount > 0 && len(pot.Eligible) > 0 {
			pm.pots = append(pm.pots, pot)
		}
		previousMax = maxBet
	}

	// Create main pot for any remaining chips
	mainPot := Pot{}
	for _, p := range players {
		if !p.Folded && p.TotalBet > previousMax {
			mainPot.Eligible = append(mainPot.Eligible, p.Seat)
			mainPot.Amount += p.TotalBet - previousMax
		}
	}

	if mainPot.Amount > 0 && len(mainPot.Eligible) > 0 {
		pm.pots = append(pm.pots, mainPot)
	}
}

// GetPots returns the current pots
func (pm *PotManager) GetPots() []Pot {
	return pm.pots
}

// GetPotsWithUncollected returns pots with uncollected bets added to the appropriate pot
func (pm *PotManager) GetPotsWithUncollected(players []*Player) []Pot {
	// Calculate uncollected bets
	uncollected := 0
	for _, p := range players {
		uncollected += p.Bet
	}

	if uncollected == 0 {
		return pm.pots
	}

	// Return a copy with uncollected added to the last pot
	// (where active players are still betting)
	result := make([]Pot, len(pm.pots))
	copy(result, pm.pots)
	if len(result) > 0 {
		// Add to the last pot, which is where current betting goes
		result[len(result)-1].Amount += uncollected
	}
	return result
}
