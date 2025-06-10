package server

// Note: Game events (hand_start, hand_end, etc.) are defined in
// internal/game/event_types.go and are also sent as WebSocket messages

// MessageType represents a WebSocket message type with type safety
type MessageType string

// WebSocket message type constants
// These are used for client-server communication protocol
const (
	// Client to server messages
	MessageTypeAuth           MessageType = "auth"
	MessageTypeJoinTable      MessageType = "join_table"
	MessageTypeLeaveTable     MessageType = "leave_table"
	MessageTypeListTables     MessageType = "list_tables"
	MessageTypePlayerDecision MessageType = "player_decision"
	MessageTypeAddBot         MessageType = "add_bot"
	MessageTypeKickBot        MessageType = "kick_bot"

	// Server to client messages
	MessageTypeActionRequired MessageType = "action_required"
	MessageTypePlayerTimeout  MessageType = "player_timeout"
	MessageTypeError          MessageType = "error"
	MessageTypeAuthResponse   MessageType = "auth_response"
	MessageTypeTableJoined    MessageType = "table_joined"
	MessageTypeTableLeft      MessageType = "table_left"
	MessageTypeTableList      MessageType = "table_list"
	MessageTypeBotAdded       MessageType = "bot_added"
	MessageTypeBotKicked      MessageType = "bot_kicked"
	MessageTypeHandStart      MessageType = "hand_start"
	MessageTypePlayerAction   MessageType = "player_action"
	MessageTypeStreetChange   MessageType = "street_change"
	MessageTypeHandEnd        MessageType = "hand_end"
	MessageTypeGamePause      MessageType = "game_pause"
)

// String returns the string representation of the message type
func (mt MessageType) String() string {
	return string(mt)
}
