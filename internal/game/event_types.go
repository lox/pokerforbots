package game

// EventType represents a game event type with type safety
type EventType string

// EventType constants for game domain events
// These represent events that occur within the poker game logic
const (
	EventTypeHandStart    EventType = "hand_start"
	EventTypeHandEnd      EventType = "hand_end"
	EventTypeStreetChange EventType = "street_change"
	EventTypePlayerAction EventType = "player_action"
	EventTypeGamePause    EventType = "game_pause"
)

// String returns the string representation of the event type
func (et EventType) String() string {
	return string(et)
}
