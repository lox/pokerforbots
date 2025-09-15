package protocol

//go:generate msgp

// MessageType identifies the type of message
type MessageType string

const (
	// Client -> Server
	TypeConnect = "connect"
	TypeAction  = "action"

	// Server -> Client
	TypeHandStart     = "hand_start"
	TypeActionRequest = "action_request"
	TypeGameUpdate    = "game_update"
	TypeStreetChange  = "street_change"
	TypeHandResult    = "hand_result"
	TypeError         = "error"
)

// Card representation as string (e.g., "As", "Kh")
type Card string

// Client -> Server Messages

// Connect is sent by client when connecting
type Connect struct {
	Type string `msg:"type"`
	Name string `msg:"name"`
}

// Action is sent by client in response to ActionRequest
type Action struct {
	Type   string `msg:"type"`
	Action string `msg:"action"` // fold, call, check, raise, allin
	Amount int    `msg:"amount"` // Only for raise
}

// Server -> Client Messages

// HandStart is sent when a new hand begins
type HandStart struct {
	Type       string   `msg:"type"`
	HandID     int      `msg:"hand_id"`
	HoleCards  []Card   `msg:"hole_cards"`
	Seat       int      `msg:"seat"`
	Button     int      `msg:"button"`
	Players    []Player `msg:"players"`
	SmallBlind int      `msg:"small_blind"`
	BigBlind   int      `msg:"big_blind"`
}

// Player info in a hand
type Player struct {
	Seat  int    `msg:"seat"`
	Name  string `msg:"name"`
	Chips int    `msg:"chips"`
}

// ActionRequest asks a bot to make a decision
type ActionRequest struct {
	Type         string   `msg:"type"`
	TimeoutMs    int      `msg:"timeout_ms"`
	ValidActions []string `msg:"valid_actions"`
	ToCall       int      `msg:"to_call"`
	MinRaise     int      `msg:"min_raise"`
	MaxRaise     int      `msg:"max_raise"`
	Pot          int      `msg:"pot"`
	Board        []Card   `msg:"board"`
	CurrentBet   int      `msg:"current_bet"`
}

// GameUpdate is broadcast when any player acts
type GameUpdate struct {
	Type   string `msg:"type"`
	Seat   int    `msg:"seat"`
	Action string `msg:"action"`
	Amount int    `msg:"amount"`
	Pot    int    `msg:"pot"`
	Street string `msg:"street"` // preflop, flop, turn, river
}

// StreetChange is sent when moving to next betting round
type StreetChange struct {
	Type   string `msg:"type"`
	Street string `msg:"street"` // flop, turn, river
	Board  []Card `msg:"board"`
	Pot    int    `msg:"pot"`
}

// HandResult is sent at hand completion
type HandResult struct {
	Type     string   `msg:"type"`
	Winners  []Winner `msg:"winners"`
	Board    []Card   `msg:"board"`
	Pot      int      `msg:"pot"`
	Showdown bool     `msg:"showdown"`
}

// Winner info
type Winner struct {
	Seat      int    `msg:"seat"`
	Amount    int    `msg:"amount"`
	HandRank  string `msg:"hand_rank,omitempty"`
	HoleCards []Card `msg:"hole_cards,omitempty"`
}

// Error message
type Error struct {
	Type    string `msg:"type"`
	Code    string `msg:"code"`
	Message string `msg:"message"`
}