package game

// GetCard returns the nth card from a hand (0-indexed)
func (h Hand) GetCard(n int) Card {
	count := 0
	for i := uint8(0); i < 52; i++ {
		card := Card(1) << i
		if h&Hand(card) != 0 {
			if count == n {
				return card
			}
			count++
		}
	}
	return 0
}

// CardString returns the string representation of a single card
func CardString(c Card) string {
	if c == 0 {
		return ""
	}

	// Find which bit is set
	for i := uint8(0); i < 52; i++ {
		if c == Card(1)<<i {
			rank := i % 13
			suit := i / 13

			rankStr := "23456789TJQKA"[rank : rank+1]
			suitStr := "cdhs"[suit : suit+1]

			return rankStr + suitStr
		}
	}
	return ""
}
