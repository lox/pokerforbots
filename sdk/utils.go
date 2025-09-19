package sdk

// CardRank returns the numeric rank of a card (2-14, where 14 is Ace)
func CardRank(card string) int {
	if len(card) < 1 {
		return 0
	}
	switch card[0] {
	case '2':
		return 2
	case '3':
		return 3
	case '4':
		return 4
	case '5':
		return 5
	case '6':
		return 6
	case '7':
		return 7
	case '8':
		return 8
	case '9':
		return 9
	case 'T':
		return 10
	case 'J':
		return 11
	case 'Q':
		return 12
	case 'K':
		return 13
	case 'A':
		return 14
	default:
		return 0
	}
}

// CardSuit returns the suit character of a card
func CardSuit(card string) byte {
	if len(card) < 2 {
		return 0
	}
	return card[1]
}

// IsSuited returns true if both cards have the same suit
func IsSuited(card1, card2 string) bool {
	return len(card1) >= 2 && len(card2) >= 2 && card1[1] == card2[1]
}

// CalculatePosition returns position relative to button (0=button, 1=cutoff, etc.)
func CalculatePosition(seat, button int, players []any) int {
	activePlayers := []int{}

	// This is a generic function, so we can't assume the player struct type
	// The caller should pass active player seats
	for i := range players {
		activePlayers = append(activePlayers, i)
	}

	if len(activePlayers) <= 2 {
		return 0 // Heads up
	}

	// Find our position relative to button
	buttonIdx := -1
	ourIdx := -1
	for i, s := range activePlayers {
		if s == button {
			buttonIdx = i
		}
		if s == seat {
			ourIdx = i
		}
	}

	if buttonIdx < 0 || ourIdx < 0 {
		return 2
	}

	distance := (ourIdx - buttonIdx + len(activePlayers)) % len(activePlayers)
	return distance
}
