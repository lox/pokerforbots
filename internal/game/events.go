package game

import (
	"fmt"
	"strings"
	"time"

	"github.com/lox/pokerforbots/sdk/deck"
)

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

// FormattingOptions controls how events are formatted for different contexts
type FormattingOptions struct {
	ShowReasonings bool   // Include AI reasoning (for hand history analysis)
	ShowTimeouts   bool   // Include timeout information (for TUI)
	ShowHoleCards  bool   // Include hole cards in summaries (for hand history)
	Perspective    string // Player name for personalized formatting
}

// EventFormatter provides centralized formatting for all game events
type EventFormatter struct {
	opts FormattingOptions
}

// NewEventFormatter creates a new event formatter with the given options
func NewEventFormatter(opts FormattingOptions) *EventFormatter {
	return &EventFormatter{opts: opts}
}

// FormatPlayerAction formats a player action event into a human-readable string
func (ef *EventFormatter) FormatPlayerAction(event PlayerActionEvent) string {
	playerName := event.Player.Name
	action := event.Action
	amount := event.Amount
	potAfter := event.PotAfter
	reasoning := event.Reasoning

	// Check for timeout in reasoning
	isTimeout := ef.opts.ShowTimeouts && (strings.Contains(reasoning, "timeout") || strings.Contains(reasoning, "Decision timeout"))

	var actionText string
	switch action {
	case Fold:
		if isTimeout {
			actionText = fmt.Sprintf("%s: times out and folds", playerName)
		} else {
			actionText = fmt.Sprintf("%s: folds", playerName)
		}
	case Call:
		// Check if this is a blind posting based on reasoning or position
		if ef.isBlindPosting(event) {
			switch event.Player.Position {
			case SmallBlind:
				actionText = fmt.Sprintf("%s: posts small blind $%d", playerName, amount)
			case BigBlind:
				actionText = fmt.Sprintf("%s: posts big blind $%d", playerName, amount)
			default:
				actionText = fmt.Sprintf("%s: calls $%d (pot now: $%d)", playerName, amount, potAfter)
			}
		} else {
			actionText = fmt.Sprintf("%s: calls $%d (pot now: $%d)", playerName, amount, potAfter)
		}
	case Check:
		if isTimeout {
			actionText = fmt.Sprintf("%s: times out and checks", playerName)
		} else {
			actionText = fmt.Sprintf("%s: checks", playerName)
		}
	case Raise:
		actionText = fmt.Sprintf("%s: raises $%d (pot now: $%d)", playerName, amount, potAfter)
	case AllIn:
		actionText = fmt.Sprintf("%s: goes all-in for $%d (pot now: $%d)", playerName, amount, potAfter)
	case SitOut:
		actionText = fmt.Sprintf("%s: sits out", playerName)
	case SitIn:
		actionText = fmt.Sprintf("%s: sits back in", playerName)
	default:
		actionText = fmt.Sprintf("%s: %s $%d", playerName, action.String(), amount)
	}

	// Add reasoning if requested and available
	if ef.opts.ShowReasonings && reasoning != "" && !isTimeout {
		actionText += fmt.Sprintf(" (%s)", reasoning)
	}

	return actionText
}

// FormatStreetChange formats a street change event into a human-readable string
func (ef *EventFormatter) FormatStreetChange(event StreetChangeEvent) string {
	switch event.Round {
	case Flop:
		if len(event.CommunityCards) >= 3 {
			flop := event.CommunityCards[:3]
			return fmt.Sprintf("\n\033[1m*** FLOP ***\033[0m %s", ef.formatCardsWithColor(flop))
		}
		return "\n\033[1m*** FLOP ***\033[0m"
	case Turn:
		if len(event.CommunityCards) >= 4 {
			board := ef.formatCardsWithColor(event.CommunityCards[:3])
			turn := ef.formatCardWithColor(event.CommunityCards[3])
			return fmt.Sprintf("\n\033[1m*** TURN ***\033[0m %s %s", board, turn)
		}
		return "\n\033[1m*** TURN ***\033[0m"
	case River:
		if len(event.CommunityCards) >= 5 {
			board := ef.formatCardsWithColor(event.CommunityCards[:4])
			river := ef.formatCardWithColor(event.CommunityCards[4])
			return fmt.Sprintf("\n\033[1m*** RIVER ***\033[0m %s %s", board, river)
		}
		return "\n\033[1m*** RIVER ***\033[0m"
	case Showdown:
		if len(event.CommunityCards) > 0 {
			return fmt.Sprintf("\n\033[1m*** SHOWDOWN ***\033[0m %s", ef.formatCardsWithColor(event.CommunityCards))
		}
		return "\n\033[1m*** SHOWDOWN ***\033[0m"
	default:
		return fmt.Sprintf("\n\033[1m*** %s ***\033[0m", event.Round.String())
	}
}

// FormatHandStart formats a hand start event into a human-readable string
func (ef *EventFormatter) FormatHandStart(event HandStartEvent) string {
	line1 := fmt.Sprintf("\033[1mHand %s\033[0m", event.HandID)
	line2 := fmt.Sprintf("%d players • $%d/$%d", len(event.Players), event.SmallBlind, event.BigBlind)

	return fmt.Sprintf("%s\n%s", line1, line2)
}

// FormatHandEnd formats a hand end event into a human-readable string
func (ef *EventFormatter) FormatHandEnd(event HandEndEvent) string {
	var result strings.Builder

	result.WriteString(fmt.Sprintf("=== Hand %s Complete ===\n", event.HandID))
	result.WriteString(fmt.Sprintf("Pot: $%d\n", event.PotSize))

	for _, winner := range event.Winners {
		winnerText := fmt.Sprintf("Winner: %s ($%d)", winner.PlayerName, winner.Amount)
		if winner.HandRank != "" {
			winnerText += fmt.Sprintf(" - %s", winner.HandRank)
		}

		// Add hole cards if showing cards and available
		if ef.opts.ShowHoleCards && len(winner.HoleCards) > 0 {
			winnerText += fmt.Sprintf(" [%s]", ef.formatCards(winner.HoleCards))
		}

		result.WriteString(winnerText + "\n")
	}

	return result.String()
}

// FormatHoleCards formats hole cards for a specific player
func (ef *EventFormatter) FormatHoleCards(playerName string, cards []deck.Card) string {
	if len(cards) == 0 {
		return ""
	}

	// Only show hole cards if it's the perspective player or if showing all cards
	if ef.opts.Perspective != "" && playerName != ef.opts.Perspective && !ef.opts.ShowHoleCards {
		return "" // Hidden
	}

	return fmt.Sprintf("Dealt to %s: %s", playerName, ef.formatCardsWithColor(cards))
}

// formatCards formats a slice of cards with appropriate styling
func (ef *EventFormatter) formatCards(cards []deck.Card) string {
	if len(cards) == 0 {
		return ""
	}

	var formatted []string
	for _, card := range cards {
		formatted = append(formatted, card.String())
	}

	return strings.Join(formatted, " ")
}

// formatCardsWithColor formats a slice of cards with color styling
func (ef *EventFormatter) formatCardsWithColor(cards []deck.Card) string {
	if len(cards) == 0 {
		return ""
	}

	var formatted []string
	for _, card := range cards {
		formatted = append(formatted, ef.formatCardWithColor(card))
	}

	return "[" + strings.Join(formatted, " ") + "]"
}

// formatCardWithColor formats a single card with color styling
func (ef *EventFormatter) formatCardWithColor(card deck.Card) string {
	if card.IsRed() {
		return fmt.Sprintf("\033[31m%s\033[0m", card.String())
	} else {
		return fmt.Sprintf("\033[30m%s\033[0m", card.String())
	}
}

// isBlindPosting determines if a call action is actually a blind posting
func (ef *EventFormatter) isBlindPosting(event PlayerActionEvent) bool {
	// Check if it's preflop and the player is in a blind position
	if event.Round != PreFlop || event.Action != Call {
		return false
	}

	// Check if the reasoning indicates it's a blind posting
	if strings.Contains(event.Reasoning, "blind") || strings.Contains(event.Reasoning, "Blind") {
		return true
	}

	// For now, don't assume position alone determines blind posting
	// since we need more context about timing and game state
	return false
}
