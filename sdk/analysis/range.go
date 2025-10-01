// Package analysis provides poker hand analysis tools including ranges.
package analysis

import (
	"fmt"
	"slices"
	"strings"

	"github.com/lox/pokerforbots/v2/poker"
)

// Range represents a collection of poker hands with associated weights.
// Uses the optimized poker.Hand type for efficient representation.
type Range struct {
	// Map from hand (as bit-packed poker.Hand) to weight (0-1)
	// We store the two hole cards combined as a single Hand
	hands map[poker.Hand]float64
}

// NewRange creates a new empty range.
func NewRange() *Range {
	return &Range{
		hands: make(map[poker.Hand]float64),
	}
}

// ParseRange creates a range from standard poker notation.
// Examples: "AA,KK", "AKs,AKo", "TT+", "A5s-A2s", "KTs+", "22-66"
func ParseRange(notation string) (*Range, error) {
	r := NewRange()

	// Split by commas to get individual range parts
	parts := strings.SplitSeq(notation, ",")
	for part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if err := r.addRangePart(part); err != nil {
			return nil, fmt.Errorf("invalid range part %q: %w", part, err)
		}
	}

	return r, nil
}

// addRangePart adds a single range notation part to the range.
func (r *Range) addRangePart(part string) error {
	// Check for range patterns like "TT+" or "A5s-A2s" or "22-66"
	if strings.Contains(part, "+") {
		return r.addPlusRange(part)
	}
	if strings.Contains(part, "-") {
		return r.addDashRange(part)
	}

	// Single hand notation
	return r.addSingleHand(part, 1.0)
}

// addSingleHand adds all combinations of a single hand notation.
func (r *Range) addSingleHand(notation string, weight float64) error {
	if len(notation) < 2 || len(notation) > 3 {
		return fmt.Errorf("invalid notation length: %s", notation)
	}

	rank1 := parseRank(notation[0])
	rank2 := parseRank(notation[1])
	if rank1 == 0 || rank2 == 0 {
		return fmt.Errorf("invalid rank in: %s", notation)
	}

	// Pocket pair
	if rank1 == rank2 {
		if len(notation) == 3 {
			return fmt.Errorf("pocket pairs cannot have suited/offsuit modifier: %s", notation)
		}
		return r.addPocketPair(rank1, weight)
	}

	// Unpaired hand
	if len(notation) == 2 {
		// Add all combinations (suited + offsuit)
		if err := r.addSuitedCombos(rank1, rank2, weight); err != nil {
			return err
		}
		return r.addOffsuitCombos(rank1, rank2, weight)
	}

	// Suited or offsuit specifically
	switch notation[2] {
	case 's':
		return r.addSuitedCombos(rank1, rank2, weight)
	case 'o':
		return r.addOffsuitCombos(rank1, rank2, weight)
	default:
		return fmt.Errorf("invalid modifier: %c", notation[2])
	}
}

// addPlusRange handles notations like "TT+" (all pairs TT and higher)
func (r *Range) addPlusRange(notation string) error {
	plusIdx := strings.Index(notation, "+")
	if plusIdx == -1 {
		return fmt.Errorf("no + found")
	}

	base := notation[:plusIdx]
	if len(base) < 2 || len(base) > 3 {
		return fmt.Errorf("invalid base notation: %s", base)
	}

	rank1 := parseRank(base[0])
	rank2 := parseRank(base[1])
	if rank1 == 0 || rank2 == 0 {
		return fmt.Errorf("invalid rank")
	}

	// Handle pocket pairs like "TT+"
	if rank1 == rank2 {
		for rank := rank1; rank <= 14; rank++ {
			if err := r.addPocketPair(rank, 1.0); err != nil {
				return err
			}
		}
		return nil
	}

	// Handle unpaired like "ATs+" or "KJo+"
	suited := false
	offsuit := false
	switch {
	case len(base) == 2:
		suited = true
		offsuit = true
	case base[2] == 's':
		suited = true
	case base[2] == 'o':
		offsuit = true
	default:
		return fmt.Errorf("invalid modifier")
	}

	// For hands like "KTs+", increment the lower card up to one below the higher
	for rank := rank2; rank < rank1; rank++ {
		if suited {
			if err := r.addSuitedCombos(rank1, rank, 1.0); err != nil {
				return err
			}
		}
		if offsuit {
			if err := r.addOffsuitCombos(rank1, rank, 1.0); err != nil {
				return err
			}
		}
	}

	return nil
}

// addDashRange handles notations like "22-66" or "A5s-A2s"
func (r *Range) addDashRange(notation string) error {
	parts := strings.Split(notation, "-")
	if len(parts) != 2 {
		return fmt.Errorf("invalid dash range format")
	}

	start := strings.TrimSpace(parts[0])
	end := strings.TrimSpace(parts[1])

	if len(start) < 2 || len(end) < 2 {
		return fmt.Errorf("invalid notation in range")
	}

	// Parse start and end
	startRank1 := parseRank(start[0])
	startRank2 := parseRank(start[1])
	endRank1 := parseRank(end[0])
	endRank2 := parseRank(end[1])

	if startRank1 == 0 || startRank2 == 0 || endRank1 == 0 || endRank2 == 0 {
		return fmt.Errorf("invalid ranks in range")
	}

	// Handle pocket pair ranges like "22-66"
	if startRank1 == startRank2 && endRank1 == endRank2 {
		lower := min(startRank1, endRank1)
		upper := max(startRank1, endRank1)
		for rank := lower; rank <= upper; rank++ {
			if err := r.addPocketPair(rank, 1.0); err != nil {
				return err
			}
		}
		return nil
	}

	// Handle suited/offsuit ranges like "A5s-A2s"
	if startRank1 == endRank1 {
		// Same high card, different kickers
		suited := len(start) == 3 && start[2] == 's'
		offsuit := len(start) == 3 && start[2] == 'o'
		if len(start) == 2 {
			suited = true
			offsuit = true
		}

		lower := min(startRank2, endRank2)
		upper := max(startRank2, endRank2)
		for rank := lower; rank <= upper; rank++ {
			if suited {
				if err := r.addSuitedCombos(startRank1, rank, 1.0); err != nil {
					return err
				}
			}
			if offsuit {
				if err := r.addOffsuitCombos(startRank1, rank, 1.0); err != nil {
					return err
				}
			}
		}
		return nil
	}

	return fmt.Errorf("unsupported range format: %s", notation)
}

// addPocketPair adds all 6 combinations of a pocket pair using optimized poker.Card types
func (r *Range) addPocketPair(rank int, weight float64) error {
	// Convert rank to poker rank value (0-based: 0=2, 12=A)
	pRank := uint8(rank - 2)

	// Generate all 6 combinations of pocket pairs
	for suit1 := range uint8(4) {
		for suit2 := suit1 + 1; suit2 < 4; suit2++ {
			card1 := poker.NewCard(pRank, suit1)
			card2 := poker.NewCard(pRank, suit2)

			// Combine into a single Hand
			hand := poker.Hand(card1) | poker.Hand(card2)
			r.hands[hand] = weight
		}
	}

	return nil
}

// addSuitedCombos adds all 4 suited combinations using optimized types
func (r *Range) addSuitedCombos(rank1, rank2 int, weight float64) error {
	if rank1 == rank2 {
		return fmt.Errorf("cannot have suited pocket pair")
	}

	// Convert ranks to poker rank values
	pRank1 := uint8(rank1 - 2)
	pRank2 := uint8(rank2 - 2)

	// All 4 suited combinations
	for suit := range uint8(4) {
		card1 := poker.NewCard(pRank1, suit)
		card2 := poker.NewCard(pRank2, suit)

		// Combine into a single Hand
		hand := poker.Hand(card1) | poker.Hand(card2)
		r.hands[hand] = weight
	}

	return nil
}

// addOffsuitCombos adds all 12 offsuit combinations using optimized types
func (r *Range) addOffsuitCombos(rank1, rank2 int, weight float64) error {
	if rank1 == rank2 {
		return fmt.Errorf("cannot have offsuit pocket pair")
	}

	// Convert ranks to poker rank values
	pRank1 := uint8(rank1 - 2)
	pRank2 := uint8(rank2 - 2)

	// All 12 offsuit combinations
	for suit1 := range uint8(4) {
		for suit2 := range uint8(4) {
			if suit1 != suit2 {
				card1 := poker.NewCard(pRank1, suit1)
				card2 := poker.NewCard(pRank2, suit2)

				// Combine into a single Hand
				hand := poker.Hand(card1) | poker.Hand(card2)
				r.hands[hand] = weight
			}
		}
	}

	return nil
}

// Contains checks if a specific hand is in the range using string cards
func (r *Range) Contains(card1, card2 string) bool {
	// Parse cards
	c1, err1 := poker.ParseCard(card1)
	c2, err2 := poker.ParseCard(card2)
	if err1 != nil || err2 != nil {
		return false
	}

	// Combine into Hand
	hand := poker.Hand(c1) | poker.Hand(c2)
	_, ok := r.hands[hand]
	return ok
}

// ContainsHand checks if a hand (as poker.Hand) is in the range
func (r *Range) ContainsHand(hand poker.Hand) bool {
	_, ok := r.hands[hand]
	return ok
}

// ContainsCards checks if hole cards are in the range
func (r *Range) ContainsCards(c1, c2 poker.Card) bool {
	hand := poker.Hand(c1) | poker.Hand(c2)
	_, ok := r.hands[hand]
	return ok
}

// Size returns the number of hand combinations in the range
func (r *Range) Size() int {
	return len(r.hands)
}

// Hands returns all hands in the range as a sorted list of poker.Hand
func (r *Range) Hands() []poker.Hand {
	hands := make([]poker.Hand, 0, len(r.hands))
	for hand := range r.hands {
		hands = append(hands, hand)
	}

	// Sort by numeric value for consistency
	slices.Sort(hands)

	return hands
}

// Weight returns the weight of a specific hand in the range
func (r *Range) Weight(hand poker.Hand) float64 {
	return r.hands[hand]
}

// parseRank converts a rank character to its numeric value
func parseRank(c byte) int {
	switch c {
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

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
