package deck

// handRankings maps each starting hand to its percentile rank (1.0 = best, 0.0 = worst)
// Data source: http://iholdemindicator.com/rank.html
var handRankings = map[string]float64{
	"AA": 1.000, "KK": 0.994, "QQ": 0.988, "AKs": 0.982, "JJ": 0.976,
	"AQs": 0.970, "KQs": 0.964, "AJs": 0.958, "KJs": 0.952, "TT": 0.946,
	"AKo": 0.940, "ATs": 0.934, "QJs": 0.928, "KTs": 0.922, "QTs": 0.916,
	"JTs": 0.910, "99": 0.904, "AQo": 0.898, "A9s": 0.892, "KQo": 0.886,
	"88": 0.880, "K9s": 0.874, "T9s": 0.868, "A8s": 0.862, "Q9s": 0.856,
	"J9s": 0.850, "AJo": 0.844, "A5s": 0.838, "77": 0.832, "A7s": 0.826,
	"KJo": 0.820, "A4s": 0.814, "A3s": 0.808, "A6s": 0.802, "QJo": 0.796,
	"66": 0.790, "K8s": 0.784, "T8s": 0.778, "A2s": 0.772, "98s": 0.766,
	"J8s": 0.760, "ATo": 0.754, "Q8s": 0.748, "K7s": 0.742, "KTo": 0.736,
	"55": 0.730, "JTo": 0.724, "87s": 0.718, "QTo": 0.712, "44": 0.706,
	"22": 0.700, "33": 0.694, "K6s": 0.688, "97s": 0.682, "K5s": 0.676,
	"76s": 0.670, "T7s": 0.664, "K4s": 0.658, "K2s": 0.652, "K3s": 0.646,
	"Q7s": 0.640, "86s": 0.634, "65s": 0.628, "J7s": 0.622, "54s": 0.616,
	"Q6s": 0.610, "75s": 0.604, "96s": 0.598, "Q5s": 0.592, "64s": 0.586,
	"Q4s": 0.580, "Q3s": 0.574, "T9o": 0.568, "T6s": 0.562, "Q2s": 0.556,
	"A9o": 0.550, "53s": 0.544, "85s": 0.538, "J6s": 0.532, "J9o": 0.526,
	"K9o": 0.520, "J5s": 0.514, "Q9o": 0.508, "43s": 0.502, "74s": 0.496,
	"J4s": 0.490, "J3s": 0.484, "95s": 0.478, "J2s": 0.472, "63s": 0.466,
	"A8o": 0.460, "52s": 0.454, "T5s": 0.448, "84s": 0.442, "T4s": 0.436,
	"T3s": 0.430, "42s": 0.424, "T2s": 0.418, "98o": 0.412, "T8o": 0.406,
	"A5o": 0.400, "A7o": 0.394, "73s": 0.388, "A4o": 0.382, "32s": 0.376,
	"94s": 0.370, "93s": 0.364, "J8o": 0.358, "A3o": 0.352, "62s": 0.346,
	"92s": 0.340, "K8o": 0.334, "A6o": 0.328, "87o": 0.322, "Q8o": 0.316,
	"83s": 0.310, "A2o": 0.304, "82s": 0.298, "97o": 0.292, "72s": 0.286,
	"76o": 0.280, "K7o": 0.274, "65o": 0.268, "T7o": 0.262, "K6o": 0.256,
	"86o": 0.250, "54o": 0.244, "K5o": 0.238, "J7o": 0.232, "75o": 0.226,
	"Q7o": 0.220, "K4o": 0.214, "K3o": 0.208, "96o": 0.202, "K2o": 0.196,
	"64o": 0.190, "Q6o": 0.184, "53o": 0.178, "85o": 0.172, "T6o": 0.166,
	"Q5o": 0.160, "43o": 0.154, "Q4o": 0.148, "Q3o": 0.142, "74o": 0.136,
	"Q2o": 0.130, "J6o": 0.124, "63o": 0.118, "J5o": 0.112, "95o": 0.106,
	"52o": 0.100, "J4o": 0.094, "J3o": 0.088, "42o": 0.082, "J2o": 0.076,
	"84o": 0.070, "T5o": 0.064, "T4o": 0.058, "32o": 0.052, "T3o": 0.046,
	"73o": 0.040, "T2o": 0.034, "62o": 0.028, "94o": 0.022, "93o": 0.016,
	"92o": 0.010, "83o": 0.006, "82o": 0.003, "72o": 0.000,
}

// GetHandPercentile returns the percentile ranking (0.0-1.0) for the given hole cards
func GetHandPercentile(holeCards []Card) float64 {
	handKey := formatHandKey(holeCards)
	percentile, exists := handRankings[handKey]

	if !exists {
		return 0.0 // Default to worst hand
	}

	return percentile
}

// formatHandKey converts hole cards to lookup key (e.g., "AKs", "72o")
func formatHandKey(holeCards []Card) string {
	if len(holeCards) != 2 {
		return "72o" // Default to worst hand
	}

	card1, card2 := holeCards[0], holeCards[1]
	rank1, rank2 := card1.Rank, card2.Rank

	// Ensure higher rank comes first
	if rank2 > rank1 {
		rank1, rank2 = rank2, rank1
	}

	// Convert ranks to string
	rankStr1 := rankToString(rank1)
	rankStr2 := rankToString(rank2)

	// Determine if suited
	suitChar := "o"
	if card1.Suit == card2.Suit {
		suitChar = "s"
	}

	// Handle pairs (no suit indicator)
	if rank1 == rank2 {
		return rankStr1 + rankStr2
	}

	return rankStr1 + rankStr2 + suitChar
}

// rankToString converts Rank to string
func rankToString(rank Rank) string {
	switch rank {
	case Two:
		return "2"
	case Three:
		return "3"
	case Four:
		return "4"
	case Five:
		return "5"
	case Six:
		return "6"
	case Seven:
		return "7"
	case Eight:
		return "8"
	case Nine:
		return "9"
	case Ten:
		return "T"
	case Jack:
		return "J"
	case Queen:
		return "Q"
	case King:
		return "K"
	case Ace:
		return "A"
	default:
		return "2"
	}
}
