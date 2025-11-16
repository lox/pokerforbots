package phh

import "strings"

var rankMap = map[string]string{
	"a":  "A",
	"k":  "K",
	"q":  "Q",
	"j":  "J",
	"10": "T",
	"t":  "T",
	"9":  "9",
	"8":  "8",
	"7":  "7",
	"6":  "6",
	"5":  "5",
	"4":  "4",
	"3":  "3",
	"2":  "2",
}

// NormalizeCard converts internal notation (e.g. 10h) to PHH notation (Th).
func NormalizeCard(card string) string {
	card = strings.TrimSpace(card)
	if card == "" {
		return ""
	}
	lowered := strings.ToLower(card)
	if lowered == "??" {
		return "??"
	}
	if len(lowered) < 2 {
		return strings.ToUpper(lowered)
	}

	suit := lowered[len(lowered)-1:]
	rankPart := lowered[:len(lowered)-1]
	rank, ok := rankMap[rankPart]
	if !ok {
		// Fall back to first rune upper-cased
		rank = strings.ToUpper(rankPart[:1])
	}

	return rank + suit
}

// NormalizeCards normalizes a slice of card strings.
func NormalizeCards(cards []string) []string {
	if len(cards) == 0 {
		return nil
	}
	out := make([]string, len(cards))
	for i, c := range cards {
		out[i] = NormalizeCard(c)
	}
	return out
}
