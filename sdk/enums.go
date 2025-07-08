package sdk

// Action represents the type of action a player can take
type Action int

const (
	// ActionFold discards hand and forfeit interest in pot
	ActionFold Action = iota
	// ActionCheck passes action with no bet (when no bet to call)
	ActionCheck
	// ActionCall matches the current bet
	ActionCall
	// ActionRaise increases the current bet
	ActionRaise
	// ActionAllIn bets all remaining chips
	ActionAllIn
)

// String returns the string representation of an action
func (a Action) String() string {
	switch a {
	case ActionFold:
		return "fold"
	case ActionCheck:
		return "check"
	case ActionCall:
		return "call"
	case ActionRaise:
		return "raise"
	case ActionAllIn:
		return "all-in"
	default:
		return "unknown"
	}
}

// ActionFromString converts a string to an Action
func ActionFromString(s string) Action {
	switch s {
	case "fold":
		return ActionFold
	case "check":
		return ActionCheck
	case "call":
		return ActionCall
	case "raise":
		return ActionRaise
	case "all-in", "allin":
		return ActionAllIn
	default:
		return ActionFold // Default to fold for unknown actions
	}
}

// Position represents a player's position at the table
type Position int

const (
	// UnknownPosition when position is not determined
	UnknownPosition Position = iota
	// SmallBlind position
	SmallBlind
	// BigBlind position
	BigBlind
	// UnderTheGun (first to act pre-flop after blinds)
	UnderTheGun
	// MiddlePosition
	MiddlePosition
	// Cutoff (one before button)
	Cutoff
	// Button (dealer position, acts last)
	Button
)

// String returns the string representation of a position
func (p Position) String() string {
	switch p {
	case SmallBlind:
		return "Small Blind"
	case BigBlind:
		return "Big Blind"
	case UnderTheGun:
		return "UTG"
	case MiddlePosition:
		return "MP"
	case Cutoff:
		return "Cutoff"
	case Button:
		return "Button"
	default:
		return "Unknown"
	}
}

// PositionFromString converts a string to a Position
func PositionFromString(s string) Position {
	switch s {
	case "Small Blind", "SB":
		return SmallBlind
	case "Big Blind", "BB":
		return BigBlind
	case "UTG", "Under the Gun":
		return UnderTheGun
	case "MP", "Middle Position":
		return MiddlePosition
	case "Cutoff", "CO":
		return Cutoff
	case "Button", "BTN", "Dealer":
		return Button
	default:
		return UnknownPosition
	}
}

// BettingRound represents the current betting round
type BettingRound int

const (
	// PreFlop before any community cards
	PreFlop BettingRound = iota
	// Flop after first 3 community cards
	Flop
	// Turn after 4th community card
	Turn
	// River after 5th community card
	River
	// Showdown when cards are revealed
	Showdown
)

// String returns the string representation of a betting round
func (r BettingRound) String() string {
	switch r {
	case PreFlop:
		return "preflop"
	case Flop:
		return "flop"
	case Turn:
		return "turn"
	case River:
		return "river"
	case Showdown:
		return "showdown"
	default:
		return "unknown"
	}
}

// BettingRoundFromString converts a string to a BettingRound
func BettingRoundFromString(s string) BettingRound {
	switch s {
	case "preflop", "pre-flop":
		return PreFlop
	case "flop":
		return Flop
	case "turn":
		return Turn
	case "river":
		return River
	case "showdown":
		return Showdown
	default:
		return PreFlop
	}
}

// GameState represents the overall state of the game
type GameState int

const (
	// WaitingForPlayers - need more players to start
	WaitingForPlayers GameState = iota
	// HandInProgress - hand is being played
	HandInProgress
	// HandComplete - hand finished, waiting to start next
	HandComplete
	// Paused - game is paused
	Paused
)

// String returns the string representation of a game state
func (g GameState) String() string {
	switch g {
	case WaitingForPlayers:
		return "waiting_for_players"
	case HandInProgress:
		return "hand_in_progress"
	case HandComplete:
		return "hand_complete"
	case Paused:
		return "paused"
	default:
		return "unknown"
	}
}

// PlayerType represents whether a player is human or AI
type PlayerType int

const (
	// Human player connected via client
	Human PlayerType = iota
	// AI player (bot)
	AI
)

// String returns the string representation of a player type
func (t PlayerType) String() string {
	switch t {
	case Human:
		return "human"
	case AI:
		return "ai"
	default:
		return "unknown"
	}
}

// Decision represents a player's decision with reasoning
type Decision struct {
	Action    Action
	Amount    int
	Reasoning string
}

// ValidAction represents a legal action a player can take
type ValidAction struct {
	Action    Action
	MinAmount int
	MaxAmount int
}
