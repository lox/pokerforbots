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
	TypeGameCompleted = "game_completed"
)

// Card representation as string (e.g., "As", "Kh")
type Card string

// Client -> Server Messages

// Connect is sent by client when connecting
type Connect struct {
	Type      string `msg:"type"`
	Name      string `msg:"name"`
	Game      string `msg:"game,omitempty"`
	AuthToken string `msg:"auth_token,omitempty"`
	Role      string `msg:"role,omitempty"`
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

// GameCompletedPlayer summarizes a bot's performance during the game run.
type GameCompletedPlayer struct {
	BotID       string  `msg:"bot_id" json:"bot_id"`
	DisplayName string  `msg:"display_name" json:"display_name"`
	Role        string  `msg:"role" json:"role"`
	Hands       int     `msg:"hands" json:"hands"`
	NetChips    int64   `msg:"net_chips" json:"net_chips"`
	AvgPerHand  float64 `msg:"avg_per_hand" json:"avg_per_hand"`
	TotalWon    int64   `msg:"total_won" json:"total_won"`
	TotalLost   int64   `msg:"total_lost" json:"total_lost"`
	LastDelta   int     `msg:"last_delta" json:"last_delta"`

	// Optional detailed statistics (only when server has statistics enabled)
	DetailedStats *PlayerDetailedStats `msg:"detailed_stats,omitempty" json:"detailed_stats,omitempty"`
}

// GameCompleted is sent when a game instance stops spawning new hands (e.g. hand limit reached).
type GameCompleted struct {
	Type           string                `msg:"type" json:"type"`
	GameID         string                `msg:"game_id" json:"game_id"`
	HandsCompleted uint64                `msg:"hands_completed" json:"hands_completed"`
	HandLimit      uint64                `msg:"hand_limit" json:"hand_limit"`
	Reason         string                `msg:"reason" json:"reason"`
	Seed           int64                 `msg:"seed" json:"seed"`
	Players        []GameCompletedPlayer `msg:"players" json:"players"`
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

// PlayerDetailedStats contains comprehensive statistics for a bot (when enabled)
type PlayerDetailedStats struct {
	BB100             float64                        `msg:"bb_100" json:"bb_100"`
	Mean              float64                        `msg:"mean" json:"mean"`
	StdDev            float64                        `msg:"std_dev" json:"std_dev"`
	WinRate           float64                        `msg:"win_rate" json:"win_rate"`
	ShowdownWinRate   float64                        `msg:"showdown_win_rate" json:"showdown_win_rate"`
	PositionStats     map[string]PositionStatSummary `msg:"position_stats,omitempty" json:"position_stats,omitempty"`
	StreetStats       map[string]StreetStatSummary   `msg:"street_stats,omitempty" json:"street_stats,omitempty"`
	HandCategoryStats map[string]CategoryStatSummary `msg:"hand_category_stats,omitempty" json:"hand_category_stats,omitempty"`
}

// PositionStatSummary contains position-specific statistics
type PositionStatSummary struct {
	Hands     int     `msg:"hands" json:"hands"`
	NetBB     float64 `msg:"net_bb" json:"net_bb"`
	BBPerHand float64 `msg:"bb_per_hand" json:"bb_per_hand"`
}

// StreetStatSummary contains street-specific statistics
type StreetStatSummary struct {
	HandsEnded int     `msg:"hands_ended" json:"hands_ended"`
	NetBB      float64 `msg:"net_bb" json:"net_bb"`
	BBPerHand  float64 `msg:"bb_per_hand" json:"bb_per_hand"`
}

// CategoryStatSummary contains hand category statistics
type CategoryStatSummary struct {
	Hands     int     `msg:"hands" json:"hands"`
	NetBB     float64 `msg:"net_bb" json:"net_bb"`
	BBPerHand float64 `msg:"bb_per_hand" json:"bb_per_hand"`
}
