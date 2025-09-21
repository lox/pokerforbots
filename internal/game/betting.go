package game

// Street represents the betting round
type Street int

const (
	Preflop Street = iota
	Flop
	Turn
	River
	Showdown
)

func (s Street) String() string {
	return [...]string{"preflop", "flop", "turn", "river", "showdown"}[s]
}

// Action represents a player action
type Action int

const (
	Fold Action = iota
	Check
	Call
	Raise
	AllIn
)

func (a Action) String() string {
	return [...]string{"fold", "check", "call", "raise", "allin"}[a]
}

// BettingRound encapsulates the state for a betting round
type BettingRound struct {
	CurrentBet     int
	MinRaise       int
	LastRaiser     int
	BBActed        bool
	ActedThisRound []bool
	BigBlind       int // Store for resetting min raise on new streets
}

// NewBettingRound creates a new betting round
func NewBettingRound(numPlayers int, bigBlind int) *BettingRound {
	return &BettingRound{
		CurrentBet:     0,
		MinRaise:       bigBlind,
		LastRaiser:     -1,
		ActedThisRound: make([]bool, numPlayers),
		BigBlind:       bigBlind,
	}
}

// GetValidActions returns valid actions for a player
func (br *BettingRound) GetValidActions(player *Player) []Action {
	actions := []Action{Fold}
	toCall := br.CurrentBet - player.Bet

	if toCall == 0 {
		// No amount to call - can check
		actions = append(actions, Check)
		// Can also raise if we have enough chips
		if player.Chips > br.MinRaise {
			actions = append(actions, Raise)
		} else if player.Chips > 0 {
			actions = append(actions, AllIn)
		}
	} else {
		// Need to call or raise
		if toCall >= player.Chips {
			// Can only go all-in
			actions = append(actions, AllIn)
		} else {
			// Can call
			actions = append(actions, Call)
			// Can raise if we have enough chips
			if player.Chips > toCall+br.MinRaise {
				actions = append(actions, Raise)
			} else if player.Chips > toCall {
				// Not enough to min-raise but have chips left after calling
				actions = append(actions, AllIn)
			}
		}
	}

	return actions
}

// ResetForNewRound resets the betting round for a new street
func (br *BettingRound) ResetForNewRound(numPlayers int) {
	br.CurrentBet = 0
	br.MinRaise = br.BigBlind // Reset to big blind for new street
	br.LastRaiser = -1
	br.ActedThisRound = make([]bool, numPlayers)
	// Note: BBActed is not reset as it only matters preflop
}

// MarkPlayerActed marks a player as having acted
func (br *BettingRound) MarkPlayerActed(seat int) {
	if seat >= 0 && seat < len(br.ActedThisRound) {
		br.ActedThisRound[seat] = true
	}
}

// IsBettingComplete checks if betting is complete for this round
func (br *BettingRound) IsBettingComplete(players []*Player, street Street, button int) bool {
	// Count active players (not folded, not all-in)
	activePlayers := 0
	for _, p := range players {
		if !p.Folded && !p.AllInFlag {
			activePlayers++
		}
	}

	// If only one active player, check if they've matched the current bet
	if activePlayers == 1 {
		for _, p := range players {
			if !p.Folded && !p.AllInFlag {
				// This is the only active player - have they matched the bet?
				if p.Bet != br.CurrentBet {
					return false // They still need to act
				}
				break
			}
		}
		return true
	}

	if activePlayers == 0 {
		return true // Everyone is folded or all-in
	}

	// Check if all active players have matched the current bet
	allMatched := true
	for _, p := range players {
		if !p.Folded && !p.AllInFlag && p.Bet != br.CurrentBet {
			allMatched = false
			break
		}
	}

	// If not all matched, betting is not complete
	if !allMatched {
		return false
	}

	// Check if all active players have acted in this round
	allActed := true
	for i, p := range players {
		if !p.Folded && !p.AllInFlag && !br.ActedThisRound[i] {
			allActed = false
			break
		}
	}

	// Special case for preflop: BB gets option even if all bets match
	if street == Preflop && allMatched && allActed {
		var bbPos int
		if len(players) == 2 {
			// Heads-up: button+1 is BB
			bbPos = (button + 1) % len(players)
		} else {
			// Regular: button+2 is BB
			bbPos = (button + 2) % len(players)
		}
		bb := players[bbPos]

		// If no raises and BB hasn't acted yet
		if br.LastRaiser == -1 && !bb.Folded && !bb.AllInFlag && !br.BBActed {
			return false // BB still gets option
		}
	}

	return allMatched && allActed
}
