package analysis

//go:generate go run ../../cmd/gen-preflop/main.go -simulations=10000 -output=preflop_gen.go

import (
	"github.com/lox/pokerforbots/internal/randutil"

	"fmt"
	rand "math/rand/v2"
	"sort"
	"strings"
	"time"

	"github.com/lox/pokerforbots/poker"
)

// PreflopHand represents a starting hand category (e.g., AA, AKs, AKo)
type PreflopHand struct {
	Category   string  // e.g., "AA", "AKs", "AKo", "72o"
	HighRank   uint8   // Higher card rank (2-14)
	LowRank    uint8   // Lower card rank (2-14)
	Suited     bool    // Whether the hand is suited
	Equity     float64 // Average equity vs random hands
	Opponents1 float64 // Equity vs 1 opponent
	Opponents2 float64 // Equity vs 2 opponents
	Opponents3 float64 // Equity vs 3 opponents
	Opponents4 float64 // Equity vs 4 opponents
	Opponents5 float64 // Equity vs 5 opponents
	Opponents6 float64 // Equity vs 6 opponents
	Opponents7 float64 // Equity vs 7 opponents
	Opponents8 float64 // Equity vs 8 opponents
	Opponents9 float64 // Equity vs 9 opponents
}

// PreflopTable holds preflop equities for all starting hands
type PreflopTable struct {
	Hands      []PreflopHand
	handLookup map[string]*PreflopHand // For quick lookups
}

// GeneratePreflopTable creates a table of all starting hands with their equities
// Uses Monte Carlo simulation to calculate accurate win rates
func GeneratePreflopTable(simulations int) *PreflopTable {
	startTime := time.Now()

	table := &PreflopTable{
		Hands:      make([]PreflopHand, 0, 169), // 169 unique starting hands
		handLookup: make(map[string]*PreflopHand),
	}

	// Use fixed seed for reproducible results
	rng := randutil.New(42)

	// Generate all unique starting hands
	// Ranks go from 2 (deuce) to 14 (ace)
	for high := uint8(14); high >= 2; high-- {
		for low := high; low >= 2; low-- {
			if high == low {
				// Pocket pair (e.g., AA, KK)
				hand := generateHandEquity(high, low, false, simulations, rng)
				table.Hands = append(table.Hands, hand)
				table.handLookup[hand.Category] = &table.Hands[len(table.Hands)-1]
			} else {
				// Suited hand (e.g., AKs)
				handSuited := generateHandEquity(high, low, true, simulations, rng)
				table.Hands = append(table.Hands, handSuited)
				table.handLookup[handSuited.Category] = &table.Hands[len(table.Hands)-1]

				// Offsuit hand (e.g., AKo)
				handOffsuit := generateHandEquity(high, low, false, simulations, rng)
				table.Hands = append(table.Hands, handOffsuit)
				table.handLookup[handOffsuit.Category] = &table.Hands[len(table.Hands)-1]
			}
		}
	}

	// Sort by equity (best hands first)
	sort.Slice(table.Hands, func(i, j int) bool {
		return table.Hands[i].Equity > table.Hands[j].Equity
	})

	// Rebuild lookup map after sorting
	table.handLookup = make(map[string]*PreflopHand)
	for i := range table.Hands {
		table.handLookup[table.Hands[i].Category] = &table.Hands[i]
	}

	_ = time.Since(startTime)

	return table
}

// generateHandEquity calculates equity for a specific starting hand
func generateHandEquity(highRank, lowRank uint8, suited bool, simulations int, rng *rand.Rand) PreflopHand {
	// Convert ranks to card strings for display
	rankChars := "??23456789TJQKA"
	highChar := rankChars[highRank]
	lowChar := rankChars[lowRank]

	var category string
	switch {
	case highRank == lowRank:
		category = fmt.Sprintf("%c%c", highChar, lowChar)
	case suited:
		category = fmt.Sprintf("%c%cs", highChar, lowChar)
	default:
		category = fmt.Sprintf("%c%co", highChar, lowChar)
	}

	// Calculate equity for different numbers of opponents
	hand := PreflopHand{
		Category: category,
		HighRank: highRank,
		LowRank:  lowRank,
		Suited:   suited,
	}

	// Calculate equity for this hand category

	// Calculate equity vs different numbers of opponents
	for opponents := 1; opponents <= 9; opponents++ {
		// Ensure at least 100 simulations per opponent
		simsPerOpponent := max(simulations/10, 100)
		equity := calculatePreflopEquity(highRank, lowRank, suited, opponents, simsPerOpponent, rng)

		switch opponents {
		case 1:
			hand.Opponents1 = equity
		case 2:
			hand.Opponents2 = equity
		case 3:
			hand.Opponents3 = equity
		case 4:
			hand.Opponents4 = equity
		case 5:
			hand.Opponents5 = equity
		case 6:
			hand.Opponents6 = equity
		case 7:
			hand.Opponents7 = equity
		case 8:
			hand.Opponents8 = equity
		case 9:
			hand.Opponents9 = equity
		}
	}

	// Average equity (vs 1-3 opponents as most common)
	hand.Equity = (hand.Opponents1 + hand.Opponents2 + hand.Opponents3) / 3.0

	return hand
}

// calculatePreflopEquity runs Monte Carlo simulation for a specific starting hand
func calculatePreflopEquity(highRank, lowRank uint8, suited bool, opponents int, simulations int, rng *rand.Rand) float64 {
	// Create a specific hand based on ranks and suited flag
	var heroHand poker.Hand

	// Choose specific cards based on suited flag
	if suited {
		// Both same suit (use spades)
		heroHand = poker.NewHand(
			poker.NewCard(highRank-2, poker.Spades),
			poker.NewCard(lowRank-2, poker.Spades),
		)
	} else {
		// Different suits
		heroHand = poker.NewHand(
			poker.NewCard(highRank-2, poker.Spades),
			poker.NewCard(lowRank-2, poker.Hearts),
		)
	}

	// Run all simulations at once for better performance
	result := CalculateEquity(heroHand, 0, opponents, simulations, rng)
	return result.Equity()
}

// GetEquity returns the equity for a given hand category and opponent count
func (t *PreflopTable) GetEquity(category string, opponents int) float64 {
	hand, ok := t.handLookup[category]
	if !ok {
		return 0.0
	}

	switch opponents {
	case 1:
		return hand.Opponents1
	case 2:
		return hand.Opponents2
	case 3:
		return hand.Opponents3
	case 4:
		return hand.Opponents4
	case 5:
		return hand.Opponents5
	case 6:
		return hand.Opponents6
	case 7:
		return hand.Opponents7
	case 8:
		return hand.Opponents8
	case 9:
		return hand.Opponents9
	default:
		// Default to heads-up if invalid
		return hand.Opponents1
	}
}

// GetHandCategory returns the category string for given hole cards
func GetHandCategory(card1, card2 string) string {
	// Parse cards to get ranks and suits
	c1, err1 := poker.ParseCard(card1)
	c2, err2 := poker.ParseCard(card2)
	if err1 != nil || err2 != nil {
		return ""
	}

	r1 := c1.Rank() + 2 // Convert from 0-12 to 2-14
	r2 := c2.Rank() + 2

	// Ensure high rank comes first
	if r1 < r2 {
		r1, r2 = r2, r1
	}

	rankChars := "??23456789TJQKA"
	highChar := rankChars[r1]
	lowChar := rankChars[r2]

	if r1 == r2 {
		return fmt.Sprintf("%c%c", highChar, lowChar)
	}

	suited := c1.Suit() == c2.Suit()
	if suited {
		return fmt.Sprintf("%c%cs", highChar, lowChar)
	}
	return fmt.Sprintf("%c%co", highChar, lowChar)
}

// GenerateGoCode outputs Go code for embedding the preflop table
func (t *PreflopTable) GenerateGoCode() string {
	var sb strings.Builder

	sb.WriteString("// Code generated by preflop.go; DO NOT EDIT.\n\n")
	sb.WriteString("package analysis\n\n")
	sb.WriteString("// PreflopEquityData contains pre-calculated equities for all starting hands\n")
	sb.WriteString("// Key format: \"AA\", \"AKs\", \"AKo\" etc.\n")
	sb.WriteString("var PreflopEquityData = map[string][]float64{\n")

	for _, hand := range t.Hands {
		sb.WriteString(fmt.Sprintf("\t\"%s\": {%.4f, %.4f, %.4f, %.4f, %.4f, %.4f, %.4f, %.4f, %.4f},\n",
			hand.Category,
			hand.Opponents1,
			hand.Opponents2,
			hand.Opponents3,
			hand.Opponents4,
			hand.Opponents5,
			hand.Opponents6,
			hand.Opponents7,
			hand.Opponents8,
			hand.Opponents9,
		))
	}

	sb.WriteString("}\n\n")
	sb.WriteString("// GetPreflopEquity returns the equity for a hand category and opponent count\n")
	sb.WriteString("func GetPreflopEquity(category string, opponents int) float64 {\n")
	sb.WriteString("\tequities, ok := PreflopEquityData[category]\n")
	sb.WriteString("\tif !ok || opponents < 1 || opponents > 9 {\n")
	sb.WriteString("\t\treturn 0.0\n")
	sb.WriteString("\t}\n")
	sb.WriteString("\treturn equities[opponents-1]\n")
	sb.WriteString("}\n")

	return sb.String()
}
