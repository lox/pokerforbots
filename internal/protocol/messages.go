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
	TypePlayerAction  = "player_action"
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
	MinRaise      int      `msg:"min_raise"`
	Pot           int      `msg:"pot"`
}

// GameUpdate is broadcast when any player acts
type GameUpdate struct {
	Type    string   `msg:"type"`
	HandID  string   `msg:"hand_id"`
	Pot     int      `msg:"pot"`
	Players []Player `msg:"players"`
}

// PlayerAction is broadcast after each player action (including blinds, timeouts)
type PlayerAction struct {
	Type        string `msg:"type"`
	HandID      string `msg:"hand_id"`
	Street      string `msg:"street"`
	Seat        int    `msg:"seat"`
	PlayerName  string `msg:"player_name"`
	Action      string `msg:"action"`       // fold, check, call, raise, allin, post_small_blind, post_big_blind, timeout_fold
	AmountPaid  int    `msg:"amount_paid"`  // Incremental amount paid with this action
	PlayerBet   int    `msg:"player_bet"`   // Player's total bet after action
	PlayerChips int    `msg:"player_chips"` // Player's chips after action
	Pot         int    `msg:"pot"`          // Total pot after action
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
	Type     string         `msg:"type"`
	HandID   string         `msg:"hand_id"`
	Winners  []Winner       `msg:"winners"`
	Board    []string       `msg:"board"`
	Showdown []ShowdownHand `msg:"showdown,omitempty"` // All hands shown at showdown
}

// Winner info
type Winner struct {
	Name      string   `msg:"name"`
	Amount    int      `msg:"amount"`
	HoleCards []string `msg:"hole_cards,omitempty"` // Winner's hole cards
	HandRank  string   `msg:"hand_rank,omitempty"`  // e.g., "Two Pair, Aces and Kings"
}

// ShowdownHand represents a player's hand shown at showdown (losers who show)
type ShowdownHand struct {
	Name      string   `msg:"name"`
	HoleCards []string `msg:"hole_cards"`
	HandRank  string   `msg:"hand_rank"` // e.g., "Pair of Queens"
}

// Error message
type Error struct {
	Type    string `msg:"type"`
	Code    string `msg:"code"`
	Message string `msg:"message"`
}
