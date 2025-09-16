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
	HandID     string   `msg:"hand_id"`
	HoleCards  []string `msg:"hole_cards"`
	YourSeat   int      `msg:"your_seat"`
	Button     int      `msg:"button"`
	Players    []Player `msg:"players"`
	SmallBlind int      `msg:"small_blind"`
	BigBlind   int      `msg:"big_blind"`
}

// Player info in a hand
type Player struct {
	Seat   int    `msg:"seat"`
	Name   string `msg:"name"`
	Chips  int    `msg:"chips"`
	Bet    int    `msg:"bet,omitempty"`
	Folded bool   `msg:"folded,omitempty"`
	AllIn  bool   `msg:"all_in,omitempty"`
}

// ActionRequest asks a bot to make a decision
type ActionRequest struct {
	Type          string   `msg:"type"`
	HandID        string   `msg:"hand_id"`
	TimeRemaining int      `msg:"time_remaining"`
	ValidActions  []string `msg:"valid_actions"`
	ToCall        int      `msg:"to_call"`
	MinBet        int      `msg:"min_bet"`
	Pot           int      `msg:"pot"`
}

// GameUpdate is broadcast when any player acts
type GameUpdate struct {
	Type    string   `msg:"type"`
	HandID  string   `msg:"hand_id"`
	Pot     int      `msg:"pot"`
	Players []Player `msg:"players"`
}

// StreetChange is sent when moving to next betting round
type StreetChange struct {
	Type   string   `msg:"type"`
	HandID string   `msg:"hand_id"`
	Street string   `msg:"street"`
	Board  []string `msg:"board"`
}

// HandResult is sent at hand completion
type HandResult struct {
	Type    string   `msg:"type"`
	HandID  string   `msg:"hand_id"`
	Winners []Winner `msg:"winners"`
	Board   []string `msg:"board"`
}

// Winner info
type Winner struct {
	Name   string `msg:"name"`
	Amount int    `msg:"amount"`
}

// Error message
type Error struct {
	Type    string `msg:"type"`
	Code    string `msg:"code"`
	Message string `msg:"message"`
}
