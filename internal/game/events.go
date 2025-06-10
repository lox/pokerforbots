package game

import (
	"time"

	"github.com/lox/pokerforbots/internal/deck"
)

// GameEvent represents any event that occurs during a poker game
type GameEvent interface {
	EventType() EventType
	Timestamp() time.Time
}

// PlayerActionEvent is published when a player takes an action
type PlayerActionEvent struct {
	Player    *Player
	Action    Action
	Amount    int
	Round     BettingRound
	Reasoning string
	PotAfter  int
	timestamp time.Time
}

func (e PlayerActionEvent) EventType() EventType { return EventTypePlayerAction }
func (e PlayerActionEvent) Timestamp() time.Time { return e.timestamp }

// NewPlayerActionEvent creates a new player action event
func NewPlayerActionEvent(player *Player, action Action, amount int, round BettingRound, reasoning string, potAfter int) PlayerActionEvent {
	return PlayerActionEvent{
		Player:    player,
		Action:    action,
		Amount:    amount,
		Round:     round,
		Reasoning: reasoning,
		PotAfter:  potAfter,
		timestamp: time.Now(),
	}
}

// StreetChangeEvent is published when the betting round changes
type StreetChangeEvent struct {
	Round          BettingRound
	CommunityCards []deck.Card
	CurrentBet     int
	timestamp      time.Time
}

func (e StreetChangeEvent) EventType() EventType { return EventTypeStreetChange }
func (e StreetChangeEvent) Timestamp() time.Time { return e.timestamp }

// NewStreetChangeEvent creates a new street change event
func NewStreetChangeEvent(round BettingRound, communityCards []deck.Card, currentBet int) StreetChangeEvent {
	cards := make([]deck.Card, len(communityCards))
	copy(cards, communityCards)
	return StreetChangeEvent{
		Round:          round,
		CommunityCards: cards,
		CurrentBet:     currentBet,
		timestamp:      time.Now(),
	}
}

// HandStartEvent is published when a new hand begins
type HandStartEvent struct {
	HandID        string
	Players       []*Player
	ActivePlayers []*Player
	SmallBlind    int
	BigBlind      int
	InitialPot    int
	timestamp     time.Time
}

func (e HandStartEvent) EventType() EventType { return EventTypeHandStart }
func (e HandStartEvent) Timestamp() time.Time { return e.timestamp }

// NewHandStartEvent creates a new hand start event
func NewHandStartEvent(handID string, players []*Player, activePlayers []*Player, smallBlind, bigBlind, initialPot int) HandStartEvent {
	return HandStartEvent{
		HandID:        handID,
		Players:       players,
		ActivePlayers: activePlayers,
		SmallBlind:    smallBlind,
		BigBlind:      bigBlind,
		InitialPot:    initialPot,
		timestamp:     time.Now(),
	}
}

// HandEndEvent is published when a hand completes
type HandEndEvent struct {
	HandID       string
	Winners      []WinnerInfo
	PotSize      int
	ShowdownType string
	FinalBoard   []deck.Card
	Summary      string // Rich formatted summary from HandHistory
	timestamp    time.Time
}

func (e HandEndEvent) EventType() EventType { return EventTypeHandEnd }
func (e HandEndEvent) Timestamp() time.Time { return e.timestamp }

// NewHandEndEvent creates a new hand end event
func NewHandEndEvent(handID string, winners []WinnerInfo, potSize int, showdownType string, finalBoard []deck.Card, summary string) HandEndEvent {
	return HandEndEvent{
		HandID:       handID,
		Winners:      winners,
		PotSize:      potSize,
		ShowdownType: showdownType,
		FinalBoard:   finalBoard,
		Summary:      summary,
		timestamp:    time.Now(),
	}
}

// GamePauseEvent is published when the game is paused due to no available players
type GamePauseEvent struct {
	Reason    string
	Message   string
	timestamp time.Time
}

func (e GamePauseEvent) EventType() EventType { return EventTypeGamePause }
func (e GamePauseEvent) Timestamp() time.Time { return e.timestamp }

// NewGamePauseEvent creates a new game pause event
func NewGamePauseEvent(reason string, message string) GamePauseEvent {
	return GamePauseEvent{
		Reason:    reason,
		Message:   message,
		timestamp: time.Now(),
	}
}

// EventSubscriber can subscribe to game events
type EventSubscriber interface {
	OnEvent(event GameEvent)
}

// EventBus manages event publishing and subscription
type EventBus interface {
	Subscribe(subscriber EventSubscriber)
	Unsubscribe(subscriber EventSubscriber)
	Publish(event GameEvent)
}

// SimpleEventBus is a basic in-memory event bus implementation
type SimpleEventBus struct {
	subscribers []EventSubscriber
}

// NewEventBus creates a new event bus
func NewEventBus() EventBus {
	return &SimpleEventBus{
		subscribers: make([]EventSubscriber, 0),
	}
}

// Subscribe adds a subscriber to receive events
func (bus *SimpleEventBus) Subscribe(subscriber EventSubscriber) {
	bus.subscribers = append(bus.subscribers, subscriber)
}

// Unsubscribe removes a subscriber from receiving events
func (bus *SimpleEventBus) Unsubscribe(subscriber EventSubscriber) {
	for i, sub := range bus.subscribers {
		if sub == subscriber {
			bus.subscribers = append(bus.subscribers[:i], bus.subscribers[i+1:]...)
			break
		}
	}
}

// Publish sends an event to all subscribers
func (bus *SimpleEventBus) Publish(event GameEvent) {
	for _, subscriber := range bus.subscribers {
		// TODO: Consider adding error handling and async delivery
		subscriber.OnEvent(event)
	}
}
