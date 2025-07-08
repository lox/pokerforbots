package sdk

import (
	"encoding/json"
	"time"

	"github.com/lox/pokerforbots/sdk/deck"
)

// Message represents a WebSocket message between client and server
type Message struct {
	Type      MessageType     `json:"type"`
	Data      json.RawMessage `json:"data,omitempty"`
	Timestamp time.Time       `json:"timestamp,omitempty"`
}

// MessageType represents the type of a WebSocket message
type MessageType string

// Client to Server message types
const (
	MessageTypeAuth           MessageType = "auth"
	MessageTypeJoinTable      MessageType = "join_table"
	MessageTypeLeaveTable     MessageType = "leave_table"
	MessageTypePlayerDecision MessageType = "player_decision"
	MessageTypeAddBot         MessageType = "add_bot"
	MessageTypeKickBot        MessageType = "kick_bot"
	MessageTypeListTables     MessageType = "list_tables"
)

// Server to Client message types
const (
	MessageTypeAuthResponse   MessageType = "auth_response"
	MessageTypeError          MessageType = "error"
	MessageTypeTableList      MessageType = "table_list"
	MessageTypeTableJoined    MessageType = "table_joined"
	MessageTypeHandStart      MessageType = "hand_start"
	MessageTypePlayerAction   MessageType = "player_action"
	MessageTypeStreetChange   MessageType = "street_change"
	MessageTypeHandEnd        MessageType = "hand_end"
	MessageTypeActionRequired MessageType = "action_required"
	MessageTypeTableState     MessageType = "table_state"
	MessageTypeBotAdded       MessageType = "bot_added"
	MessageTypeBotKicked      MessageType = "bot_kicked"
	MessageTypePlayerTimeout  MessageType = "player_timeout"
	MessageTypeGamePause      MessageType = "game_pause"
)

// Client to Server message data structures

// AuthData is sent when authenticating
type AuthData struct {
	PlayerName string `json:"player_name"`
}

// JoinTableData is sent when joining a table
type JoinTableData struct {
	TableID string `json:"table_id"`
	BuyIn   int    `json:"buy_in"`
}

// LeaveTableData is sent when leaving a table
type LeaveTableData struct {
	TableID string `json:"table_id"`
}

// PlayerDecisionData is sent when making a game decision
type PlayerDecisionData struct {
	TableID   string `json:"table_id"`
	Action    string `json:"action"`
	Amount    int    `json:"amount,omitempty"`
	Reasoning string `json:"reasoning,omitempty"`
}

// AddBotData is sent when adding a bot to a table
type AddBotData struct {
	TableID string `json:"table_id"`
	Count   int    `json:"count"`
}

// KickBotData is sent when removing a bot from a table
type KickBotData struct {
	TableID string `json:"table_id"`
	BotName string `json:"bot_name"`
}

// Server to Client message data structures

// AuthResponseData is sent in response to authentication
type AuthResponseData struct {
	Success    bool   `json:"success"`
	PlayerID   string `json:"player_id"`
	PlayerName string `json:"player_name"`
}

// ErrorData is sent when an error occurs
type ErrorData struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// TableInfo represents a table's configuration and state
type TableInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	MaxSeats   int    `json:"max_seats"`
	SmallBlind int    `json:"small_blind"`
	BigBlind   int    `json:"big_blind"`
	Players    int    `json:"players"`
	InProgress bool   `json:"in_progress"`
}

// TableListData contains a list of available tables
type TableListData struct {
	Tables []TableInfo `json:"tables"`
}

// TableJoinedData is sent when successfully joining a table
type TableJoinedData struct {
	TableID string        `json:"table_id"`
	Seat    int           `json:"seat"`
	BuyIn   int           `json:"buy_in"`
	Players []PlayerState `json:"players"`
}

// PlayerState represents a player's current state
type PlayerState struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Type         string      `json:"type"`
	Chips        int         `json:"chips"`
	Bet          int         `json:"bet"`
	TotalBet     int         `json:"total_bet"`
	HoleCards    []deck.Card `json:"hole_cards,omitempty"`
	IsDealer     bool        `json:"is_dealer"`
	IsFolded     bool        `json:"is_folded"`
	IsAllIn      bool        `json:"is_all_in"`
	IsSittingOut bool        `json:"is_sitting_out"`
	Position     Position    `json:"position"`
}

// HandStartData is sent when a new hand begins
type HandStartData struct {
	TableID    string        `json:"table_id"`
	HandNumber int           `json:"hand_number"`
	Players    []PlayerState `json:"players"`
	Button     int           `json:"button"`
	SmallBlind int           `json:"small_blind"`
	BigBlind   int           `json:"big_blind"`
	InitialPot int           `json:"initial_pot"`
}

// PlayerActionData is sent when a player makes an action
type PlayerActionData struct {
	Player    string `json:"player"`
	Action    string `json:"action"`
	Amount    int    `json:"amount,omitempty"`
	PotAfter  int    `json:"pot_after"`
	Round     string `json:"round"`
	Reasoning string `json:"reasoning,omitempty"`
}

// StreetChangeData is sent when the betting round changes
type StreetChangeData struct {
	TableID        string      `json:"table_id"`
	Round          string      `json:"round"`
	CommunityCards []deck.Card `json:"community_cards"`
	CurrentBet     int         `json:"current_bet"`
}

// HandEndData is sent when a hand completes
type HandEndData struct {
	TableID   string       `json:"table_id"`
	Winners   []WinnerInfo `json:"winners"`
	ShowCards bool         `json:"show_cards"`
	FinalPot  int          `json:"final_pot"`
}

// WinnerInfo represents information about a hand winner
type WinnerInfo struct {
	Player       string      `json:"player"`
	Amount       int         `json:"amount"`
	HoleCards    []deck.Card `json:"hole_cards,omitempty"`
	HandStrength string      `json:"hand_strength,omitempty"`
}

// ValidActionInfo represents a valid action the player can take
type ValidActionInfo struct {
	Action    string `json:"action"`
	MinAmount int    `json:"min_amount,omitempty"`
	MaxAmount int    `json:"max_amount,omitempty"`
}

// TableStateData represents the current state of the table
type TableStateData struct {
	TableID        string        `json:"table_id"`
	Round          string        `json:"round"`
	CommunityCards []deck.Card   `json:"community_cards"`
	Pot            int           `json:"pot"`
	CurrentBet     int           `json:"current_bet"`
	Players        []PlayerState `json:"players"`
	ActingPlayer   int           `json:"acting_player"`
}

// ActionRequiredData is sent when the client needs to make a decision
type ActionRequiredData struct {
	TableID      string            `json:"table_id"`
	Round        string            `json:"round"`
	ValidActions []ValidActionInfo `json:"valid_actions"`
	TimeLimit    int               `json:"time_limit"`
	CurrentBet   int               `json:"current_bet"`
	Pot          int               `json:"pot"`
}

// BotAddedData is sent when a bot is added to the table
type BotAddedData struct {
	TableID string `json:"table_id"`
	BotName string `json:"bot_name"`
	BotType string `json:"bot_type"`
}

// BotKickedData is sent when a bot is removed from the table
type BotKickedData struct {
	TableID string `json:"table_id"`
	BotName string `json:"bot_name"`
	Reason  string `json:"reason,omitempty"`
}

// PlayerTimeoutData is sent when a player times out
type PlayerTimeoutData struct {
	TableID    string `json:"table_id"`
	PlayerName string `json:"player_name"`
	Action     string `json:"action"`
	Round      string `json:"round"`
}

// GamePauseData is sent when the game is paused
type GamePauseData struct {
	TableID string `json:"table_id"`
	Reason  string `json:"reason"`
	Paused  bool   `json:"paused"`
}

// NewMessage creates a new message with the given type and data
func NewMessage(msgType MessageType, data interface{}) (*Message, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return &Message{
		Type:      msgType,
		Data:      jsonData,
		Timestamp: time.Now().UTC(),
	}, nil
}
