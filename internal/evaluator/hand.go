package evaluator

import (
	"fmt"
	"strings"

	"github.com/lox/holdem-cli/internal/deck"
)

// HandRank represents the ranking of a poker hand
type HandRank int

const (
	HighCard HandRank = iota
	OnePair
	TwoPair
	ThreeOfAKind
	Straight
	Flush
	FullHouse
	FourOfAKind
	StraightFlush
	RoyalFlush
)

// String returns the string representation of a hand rank
func (hr HandRank) String() string {
	switch hr {
	case HighCard:
		return "High Card"
	case OnePair:
		return "One Pair"
	case TwoPair:
		return "Two Pair"
	case ThreeOfAKind:
		return "Three of a Kind"
	case Straight:
		return "Straight"
	case Flush:
		return "Flush"
	case FullHouse:
		return "Full House"
	case FourOfAKind:
		return "Four of a Kind"
	case StraightFlush:
		return "Straight Flush"
	case RoyalFlush:
		return "Royal Flush"
	default:
		return "Unknown"
	}
}

// Hand represents a poker hand with its ranking and key cards
type Hand struct {
	Rank     HandRank
	Cards    []deck.Card // The 5 cards that make up the hand
	Kickers  []deck.Rank // High cards for tie-breaking, in descending order
	HighCard deck.Rank   // Highest card in the hand
}

// String returns a string representation of the hand
func (h Hand) String() string {
	var cardStrs []string
	for _, card := range h.Cards {
		cardStrs = append(cardStrs, card.String())
	}
	return fmt.Sprintf("%s [%s]", h.Rank, strings.Join(cardStrs, " "))
}

// Compare compares two hands and returns:
// -1 if h1 is weaker than h2
//  0 if h1 equals h2
//  1 if h1 is stronger than h2
func (h1 Hand) Compare(h2 Hand) int {
	// First compare by hand rank
	if h1.Rank < h2.Rank {
		return -1
	}
	if h1.Rank > h2.Rank {
		return 1
	}
	
	// Same rank, compare kickers
	for i := 0; i < len(h1.Kickers) && i < len(h2.Kickers); i++ {
		if h1.Kickers[i] < h2.Kickers[i] {
			return -1
		}
		if h1.Kickers[i] > h2.Kickers[i] {
			return 1
		}
	}
	
	// If all kickers are equal, hands are tied
	return 0
}

// CompareWithExplanation compares two hands and returns the result with an explanation
func (h1 Hand) CompareWithExplanation(h2 Hand) (int, string) {
	result := h1.Compare(h2)
	
	if result == 0 {
		return result, "hands tie"
	}
	
	var winner, loser Hand
	var explanation string
	
	if result > 0 {
		winner, loser = h1, h2
		explanation = fmt.Sprintf("%s beats %s", winner.String(), loser.String())
	} else {
		winner, loser = h2, h1
		explanation = fmt.Sprintf("%s beats %s", winner.String(), loser.String())
	}
	
	// Add specific reasoning based on hand types
	if winner.Rank != loser.Rank {
		explanation += fmt.Sprintf(" (%s beats %s)", winner.Rank.String(), loser.Rank.String())
	} else {
		// Same hand type, explain the kicker difference
		explanation += " with "
		switch winner.Rank {
		case OnePair:
			if len(winner.Kickers) >= 1 && len(loser.Kickers) >= 1 {
				if winner.Kickers[0] != loser.Kickers[0] {
					explanation += fmt.Sprintf("higher pair (%s vs %s)", 
						deck.Rank(winner.Kickers[0]).String(), 
						deck.Rank(loser.Kickers[0]).String())
				} else if len(winner.Kickers) >= 2 && len(loser.Kickers) >= 2 {
					explanation += fmt.Sprintf("higher kicker (%s vs %s)", 
						deck.Rank(winner.Kickers[1]).String(), 
						deck.Rank(loser.Kickers[1]).String())
				}
			}
		case TwoPair:
			if len(winner.Kickers) >= 2 && len(loser.Kickers) >= 2 {
				if winner.Kickers[0] != loser.Kickers[0] {
					explanation += fmt.Sprintf("higher top pair (%s vs %s)", 
						deck.Rank(winner.Kickers[0]).String(), 
						deck.Rank(loser.Kickers[0]).String())
				} else if winner.Kickers[1] != loser.Kickers[1] {
					explanation += fmt.Sprintf("higher bottom pair (%s vs %s)", 
						deck.Rank(winner.Kickers[1]).String(), 
						deck.Rank(loser.Kickers[1]).String())
				} else if len(winner.Kickers) >= 3 && len(loser.Kickers) >= 3 {
					explanation += fmt.Sprintf("higher kicker (%s vs %s)", 
						deck.Rank(winner.Kickers[2]).String(), 
						deck.Rank(loser.Kickers[2]).String())
				}
			}
		case ThreeOfAKind, FourOfAKind:
			if len(winner.Kickers) >= 1 && len(loser.Kickers) >= 1 {
				explanation += fmt.Sprintf("higher trips/quads (%s vs %s)", 
					deck.Rank(winner.Kickers[0]).String(), 
					deck.Rank(loser.Kickers[0]).String())
			}
		case FullHouse:
			if len(winner.Kickers) >= 2 && len(loser.Kickers) >= 2 {
				if winner.Kickers[0] != loser.Kickers[0] {
					explanation += fmt.Sprintf("higher trips (%s vs %s)", 
						deck.Rank(winner.Kickers[0]).String(), 
						deck.Rank(loser.Kickers[0]).String())
				} else {
					explanation += fmt.Sprintf("higher pair (%s vs %s)", 
						deck.Rank(winner.Kickers[1]).String(), 
						deck.Rank(loser.Kickers[1]).String())
				}
			}
		case Straight, StraightFlush:
			if len(winner.Kickers) >= 1 && len(loser.Kickers) >= 1 {
				explanation += fmt.Sprintf("higher straight (%s-high vs %s-high)", 
					deck.Rank(winner.Kickers[0]).String(), 
					deck.Rank(loser.Kickers[0]).String())
			}
		case Flush:
			if len(winner.Kickers) >= 1 && len(loser.Kickers) >= 1 {
				explanation += fmt.Sprintf("higher flush (%s-high vs %s-high)", 
					deck.Rank(winner.Kickers[0]).String(), 
					deck.Rank(loser.Kickers[0]).String())
			}
		default:
			if len(winner.Kickers) >= 1 && len(loser.Kickers) >= 1 {
				explanation += fmt.Sprintf("higher kicker (%s vs %s)", 
					deck.Rank(winner.Kickers[0]).String(), 
					deck.Rank(loser.Kickers[0]).String())
			}
		}
	}
	
	return result, explanation
}

// IsStrongerThan returns true if this hand beats the other hand
func (h1 Hand) IsStrongerThan(h2 Hand) bool {
	return h1.Compare(h2) > 0
}

// IsWeakerThan returns true if this hand loses to the other hand
func (h1 Hand) IsWeakerThan(h2 Hand) bool {
	return h1.Compare(h2) < 0
}

// Equals returns true if both hands are equal in strength
func (h1 Hand) Equals(h2 Hand) bool {
	return h1.Compare(h2) == 0
}

// cardsByRank is a helper type for sorting cards by rank
type cardsByRank []deck.Card

func (c cardsByRank) Len() int           { return len(c) }
func (c cardsByRank) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c cardsByRank) Less(i, j int) bool { return c[i].Rank < c[j].Rank }

// cardsByRankDesc sorts cards by rank in descending order
type cardsByRankDesc []deck.Card

func (c cardsByRankDesc) Len() int           { return len(c) }
func (c cardsByRankDesc) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c cardsByRankDesc) Less(i, j int) bool { return c[i].Rank > c[j].Rank }

// ranksByDesc sorts ranks in descending order
type ranksByDesc []deck.Rank

func (r ranksByDesc) Len() int           { return len(r) }
func (r ranksByDesc) Swap(i, j int)      { r[i], r[j] = r[j], r[i] }
func (r ranksByDesc) Less(i, j int) bool { return r[i] > r[j] }