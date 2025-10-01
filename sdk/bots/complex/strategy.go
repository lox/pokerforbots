package complex

import "github.com/lox/pokerforbots/v2/sdk/analysis"

// FoldThreshold defines minimum equity needed to continue at different bet sizes.
type FoldThreshold struct {
	Street    string
	MaxBetPct float64
	MinEquity float64
}

// Position constants for preflop ranges.
const (
	PositionButton = 0
	PositionCutoff = 1
	PositionMiddle = 2
	PositionEarly  = 3
	PositionAny    = -1
)

// Action constants for preflop ranges.
const (
	ActionOpen          = "open"
	Action3BetValue     = "3bet_value"
	Action3BetBluff     = "3bet_bluff"
	ActionDefend        = "defend"
	Action4Bet          = "4bet"
	ActionOpenHeadsUp   = "open_headsup"
	ActionDefendHeadsUp = "defend_headsup"
)

// PostflopAction defines actions based on hand classification and situation.
type PostflopAction struct {
	HandClass string
	CanCheck  bool
	MaxSPR    float64
	Multiway  bool
	Action    string
	SizePct   float64
}

// Street constants for bet sizing.
const (
	StreetFlop  = "flop"
	StreetTurn  = "turn"
	StreetRiver = "river"
)

// HandStrength categories for bet sizing.
const (
	HandStrengthStrong = "strong"
	HandStrengthMedium = "medium"
	HandStrengthDraw   = "draw"
	HandStrengthAny    = "*"
)

// BoardTextureString constants matching classification.BoardTexture.String().
const (
	BoardTextureDry     = "dry"
	BoardTextureSemiWet = "semi-wet"
	BoardTextureWet     = "wet"
	BoardTextureVeryWet = "very wet"
	BoardTextureAny     = "*"
)

// BetSizing defines bet sizes for different situations.
type BetSizing struct {
	Street       string
	BoardTexture string
	HandStrength string
	SizePct      float64
}

type preflopKey struct {
	Position int
	Action   string
}

// StrategyConfig stores parsed strategy tables so we only compute them once.
type StrategyConfig struct {
	FoldThresholds []FoldThreshold
	PreflopRanges  map[preflopKey]*analysis.Range
	PostflopMatrix []PostflopAction
	BetSizingTable []BetSizing
	FlatTrapRange  *analysis.Range
}

var defaultStrategy = buildDefaultStrategy()

func buildDefaultStrategy() *StrategyConfig {
	cfg := &StrategyConfig{
		FoldThresholds: []FoldThreshold{
			{StreetFlop, 0.33, 0.15},
			{StreetFlop, 0.66, 0.35},
			{StreetFlop, 999, 0.50},
			{StreetTurn, 0.50, 0.30},
			{StreetTurn, 1.00, 0.50},
			{StreetTurn, 999, 0.60},
			{StreetRiver, 0.25, 0.30},
			{StreetRiver, 0.50, 0.45},
			{StreetRiver, 999, 0.60},
		},
		PreflopRanges: map[preflopKey]*analysis.Range{},
		PostflopMatrix: []PostflopAction{
			{"TripsPlus", true, 999, false, "bet", 0.50},
			{"TripsPlus", true, 999, true, "bet", 0.75},
			{"TripsPlus", false, 999, false, "raise", 0.50},
			{"TripsPlus", false, 999, true, "call", 0},
			{"TwoPair", true, 999, false, "bet", 0.50},
			{"TwoPair", true, 999, true, "check", 0},
			{"TwoPair", false, 999, false, "call", 0},
			{"TwoPair", false, 999, true, "call", 0},
			{"TPTK", true, 8.0, false, "bet", 0.33},
			{"TPTK", true, 999, false, "check", 0},
			{"TPTK", true, 999, true, "check", 0},
			{"TPTK", false, 999, false, "call", 0},
			{"TPTK", false, 999, true, "fold", 0},
			{"TopPair", true, 5.0, false, "bet", 0.25},
			{"TopPair", true, 999, false, "check", 0},
			{"TopPair", true, 999, true, "check", 0},
			{"TopPair", false, 999, false, "call", 0},
			{"TopPair", false, 999, true, "fold", 0},
			{"ComboDraw", true, 8.0, false, "bet", 0.33},
			{"ComboDraw", true, 999, false, "check", 0},
			{"ComboDraw", false, 999, false, "call", 0},
			{"ComboDraw", false, 999, true, "call", 0},
			{"StrongDraw", true, 5.0, false, "bet", 0.25},
			{"StrongDraw", true, 999, false, "check", 0},
			{"StrongDraw", false, 999, false, "call", 0},
			{"StrongDraw", false, 999, true, "fold", 0},
			{"WeakDraw", true, 999, false, "check", 0},
			{"WeakDraw", false, 999, false, "fold", 0},
			{"WeakDraw", false, 999, true, "fold", 0},
			{"Air", true, 999, false, "check", 0},
			{"Air", false, 999, false, "fold", 0},
			{"Air", false, 999, true, "fold", 0},
		},
		BetSizingTable: []BetSizing{
			{StreetFlop, BoardTextureDry, HandStrengthAny, 0.33},
			{StreetFlop, BoardTextureSemiWet, HandStrengthAny, 0.50},
			{StreetFlop, BoardTextureWet, HandStrengthAny, 0.66},
			{StreetFlop, BoardTextureVeryWet, HandStrengthAny, 0.75},
			{StreetTurn, BoardTextureAny, HandStrengthStrong, 0.66},
			{StreetTurn, BoardTextureAny, HandStrengthMedium, 0.50},
			{StreetTurn, BoardTextureAny, HandStrengthDraw, 0.50},
			{StreetRiver, BoardTextureAny, HandStrengthStrong, 1.00},
			{StreetRiver, BoardTextureAny, HandStrengthMedium, 0.50},
			{StreetRiver, BoardTextureAny, HandStrengthDraw, 0.75},
		},
	}

	addRange := func(pos int, action, spec string) {
		if spec == "" {
			cfg.PreflopRanges[preflopKey{Position: pos, Action: action}] = nil
			return
		}
		r, err := analysis.ParseRange(spec)
		if err != nil {
			cfg.PreflopRanges[preflopKey{Position: pos, Action: action}] = nil
			return
		}
		cfg.PreflopRanges[preflopKey{Position: pos, Action: action}] = r
	}

	addRange(PositionButton, ActionOpenHeadsUp, "22+,A2+,K2+,Q2+,J4o+,T6o+,96o+,86o+,76o+,65o+,54o,J2s+,T2s+,92s+,82s+,72s+,62s+,52s+,42s+,32s")
	addRange(PositionCutoff, ActionDefendHeadsUp, "22+,A2+,K2+,Q5o+,J7o+,T7o+,97o+,87o,Q2s+,J2s+,T4s+,95s+,85s+,74s+,64s+,53s+,43s")
	addRange(PositionEarly, ActionOpen, "77+,AJo+,KQo,A5s+,KTs+,QTs+,JTs,T9s")
	addRange(PositionMiddle, ActionOpen, "55+,ATo+,KJo+,A2s+,K9s+,Q9s+,J9s+,T9s,98s,87s,76s")
	addRange(PositionCutoff, ActionOpen, "22+,A2+,K8o+,Q9o+,J9o+,T9o,K2s+,Q4s+,J7s+,T7s+,97s+,86s+,75s+,65s,54s")
	addRange(PositionButton, ActionOpen, "22+,A2+,K5o+,Q8o+,J8o+,T8o+,98o,K2s+,Q2s+,J4s+,T6s+,96s+,85s+,74s+,64s+,53s+,43s")
	addRange(PositionAny, Action3BetValue, "TT+,AQs+,AKo")
	addRange(PositionButton, Action3BetBluff, "A5s-A2s,K9s,K8s,QTs,JTs,T9s,98s,87s,76s,65s")
	addRange(PositionCutoff, Action3BetBluff, "A5s-A2s,KTs,K9s,QTs,JTs")
	addRange(PositionAny, ActionDefend, "99-22,AJs,KQs,QJs,JTs,T9s,98s,87s,76s")
	addRange(PositionAny, Action4Bet, "QQ+,AK")

	if trap, err := analysis.ParseRange("TT,JJ"); err == nil {
		cfg.FlatTrapRange = trap
	}

	return cfg
}

func (s *StrategyConfig) FoldThresholdValue(street string, betPct float64) float64 {
	for _, threshold := range s.FoldThresholds {
		if threshold.Street == street && betPct <= threshold.MaxBetPct {
			return threshold.MinEquity
		}
	}
	return 0.50
}

func (s *StrategyConfig) PreflopRangeFor(position int, action string) *analysis.Range {
	key := preflopKey{Position: position, Action: action}
	if position >= PositionEarly {
		if _, ok := s.PreflopRanges[preflopKey{Position: PositionEarly, Action: action}]; ok {
			key = preflopKey{Position: PositionEarly, Action: action}
		}
	}
	if r, ok := s.PreflopRanges[key]; ok && r != nil {
		return r
	}
	if r, ok := s.PreflopRanges[preflopKey{Position: PositionAny, Action: action}]; ok {
		return r
	}
	return nil
}

func (s *StrategyConfig) PostflopDecision(handClass string, canCheck bool, spr float64, multiway bool) (string, float64) {
	for _, action := range s.PostflopMatrix {
		if action.HandClass != handClass {
			continue
		}
		if action.CanCheck != canCheck {
			continue
		}
		if spr > action.MaxSPR {
			continue
		}
		if action.Multiway != multiway {
			continue
		}
		return action.Action, action.SizePct
	}
	if canCheck {
		return "check", 0
	}
	return "fold", 0
}

func (s *StrategyConfig) BetSize(street, boardTexture, handStrength string) float64 {
	for _, sizing := range s.BetSizingTable {
		if sizing.Street != street {
			continue
		}
		if sizing.BoardTexture != BoardTextureAny && sizing.BoardTexture != boardTexture {
			continue
		}
		if sizing.HandStrength != HandStrengthAny && sizing.HandStrength != handStrength {
			continue
		}
		return sizing.SizePct
	}
	return 0.50
}
