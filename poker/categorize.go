package poker

// HoleCardCategory represents the strength category of hole cards
type HoleCardCategory string

const (
	CategoryPremium HoleCardCategory = "Premium"
	CategoryStrong  HoleCardCategory = "Strong"
	CategoryMedium  HoleCardCategory = "Medium"
	CategoryWeak    HoleCardCategory = "Weak"
	CategoryTrash   HoleCardCategory = "Trash"
	CategoryUnknown HoleCardCategory = "Unknown"
)

// CategorizeHoleCards provides a simple preflop hand categorization.
// Categories: Premium (JJ+, AK), Strong (TT+, AQ/AJ), Medium (77+, suited broadway),
// Weak (small pairs, suited connectors), Trash (everything else).
func CategorizeHoleCards(card1, card2 Card) HoleCardCategory {
	r1 := card1.Rank()
	r2 := card2.Rank()
	s1 := card1.Suit()
	s2 := card2.Suit()

	// Invalid cards
	if r1 > 12 || r2 > 12 {
		return CategoryUnknown
	}

	// Convert to 2-14 range for easier comparison (matching old logic)
	rank1 := rankToValue(r1)
	rank2 := rankToValue(r2)
	suited := s1 == s2

	// Order ranks (smaller first)
	small, big := rank1, rank2
	if small > big {
		small, big = big, small
	}

	// Premium: JJ+, AK (any suit)
	isPair := small == big
	if isPair && small >= 11 { // JJ, QQ, KK, AA
		return CategoryPremium
	}
	if small == 13 && big == 14 { // AK
		return CategoryPremium
	}

	// Strong: TT, AQ, AJ
	if isPair && small == 10 { // TT
		return CategoryStrong
	}
	if big == 14 && (small == 12 || small == 11) { // AQ, AJ
		return CategoryStrong
	}

	// Medium: 77-99, suited broadway cards (KQ, KJ, QJ suited)
	if isPair && small >= 7 && small <= 9 { // 77, 88, 99
		return CategoryMedium
	}
	if suited && small >= 10 && big >= 10 { // Suited broadway
		return CategoryMedium
	}

	// Weak: small pairs (22-66) or suited connectors
	if isPair && small >= 2 && small <= 6 { // 22-66
		return CategoryWeak
	}
	if suited && absDiff(small, big) <= 2 { // Suited connectors
		return CategoryWeak
	}

	return CategoryTrash
}

// CategorizeHoleCardsFromStrings categorizes hole cards from string representations.
// This is a convenience wrapper for the old API.
func CategorizeHoleCardsFromStrings(cards []string) string {
	if len(cards) != 2 {
		return string(CategoryUnknown)
	}

	card1, err1 := ParseCard(cards[0])
	card2, err2 := ParseCard(cards[1])
	if err1 != nil || err2 != nil {
		return string(CategoryUnknown)
	}

	return string(CategorizeHoleCards(card1, card2))
}

// rankToValue converts our 0-12 rank system to 2-14 for categorization
func rankToValue(rank uint8) int {
	if rank == Ace {
		return 14
	}
	return int(rank) + 2 // Two=0 becomes 2, Three=1 becomes 3, etc.
}

// absDiff returns the absolute difference between two integers
func absDiff(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}
