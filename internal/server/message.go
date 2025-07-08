package server

import (
	"encoding/json"
	"time"

	"github.com/lox/pokerforbots/internal/game"
	"github.com/lox/pokerforbots/sdk/deck"
)

// Message represents the base WebSocket message structure
type Message struct {
	Type      MessageType     `json:"type"`
	Data      json.RawMessage `json:"data"`
	Timestamp time.Time       `json:"timestamp"`
	RequestID string          `json:"requestId,omitempty"`
}

// NewMessage creates a new message with the current timestamp
func NewMessage(messageType MessageType, data interface{}) (*Message, error) {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return &Message{
		Type:      messageType,
		Data:      dataBytes,
		Timestamp: time.Now(),
	}, nil
}

// Client → Server Messages

type AuthData struct {
	PlayerName string `json:"playerName"`
	Token      string `json:"token,omitempty"`
}

type JoinTableData struct {
	TableID    string `json:"tableId"`
	SeatNumber *int   `json:"seatNumber,omitempty"`
	BuyIn      int    `json:"buyIn"`
}

type LeaveTableData struct {
	TableID string `json:"tableId"`
}

type PlayerDecisionData struct {
	TableID   string `json:"tableId"`
	Action    string `json:"action"`
	Amount    int    `json:"amount,omitempty"`
	Reasoning string `json:"reasoning,omitempty"`
}

type AddBotData struct {
	TableID string `json:"tableId"`
	Count   int    `json:"count,omitempty"` // Number of bots to add, default 1
}

type KickBotData struct {
	TableID string `json:"tableId"`
	BotName string `json:"botName"`
}

// Server → Client Messages

type AuthResponseData struct {
	Success  bool   `json:"success"`
	PlayerID string `json:"playerId,omitempty"`
	Error    string `json:"error,omitempty"`
}

type ErrorData struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type TableInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	PlayerCount int    `json:"playerCount"`
	MaxPlayers  int    `json:"maxPlayers"`
	Stakes      string `json:"stakes"`
	Status      string `json:"status"`
}

type TableListData struct {
	Tables []TableInfo `json:"tables"`
}

type TableJoinedData struct {
	TableID    string        `json:"tableId"`
	SeatNumber int           `json:"seatNumber"`
	Players    []PlayerState `json:"players"`
}

type PlayerState struct {
	Name         string      `json:"name"`
	Chips        int         `json:"chips"`
	Position     string      `json:"position"`
	SeatNumber   int         `json:"seatNumber"`
	HoleCards    []deck.Card `json:"holeCards,omitempty"` // Only for acting player
	BetThisRound int         `json:"betThisRound"`
	TotalBet     int         `json:"totalBet"`
	IsActive     bool        `json:"isActive"`
	IsFolded     bool        `json:"isFolded"`
	IsAllIn      bool        `json:"isAllIn"`
	LastAction   string      `json:"lastAction"`
}

type HandStartData struct {
	HandID     string        `json:"handId"`
	Players    []PlayerState `json:"players"`
	SmallBlind int           `json:"smallBlind"`
	BigBlind   int           `json:"bigBlind"`
	InitialPot int           `json:"initialPot"`
	DealerSeat int           `json:"dealerSeat"`
}

type PlayerActionData struct {
	Player    string `json:"player"`
	Action    string `json:"action"`
	Amount    int    `json:"amount"`
	PotAfter  int    `json:"potAfter"`
	Round     string `json:"round"`
	Reasoning string `json:"reasoning"`
}

type StreetChangeData struct {
	Round          string      `json:"round"`
	CommunityCards []deck.Card `json:"communityCards"`
	CurrentBet     int         `json:"currentBet"`
}

type HandEndData struct {
	HandID       string            `json:"handId"`
	Winners      []game.WinnerInfo `json:"winners"`
	PotSize      int               `json:"potSize"`
	ShowdownType string            `json:"showdownType"`
	FinalBoard   []deck.Card       `json:"finalBoard"`
	Summary      string            `json:"summary"`
}

type ValidActionInfo struct {
	Action    string `json:"action"`
	MinAmount int    `json:"minAmount"`
	MaxAmount int    `json:"maxAmount"`
}

type TableStateData struct {
	CurrentBet      int           `json:"currentBet"`
	Pot             int           `json:"pot"`
	CurrentRound    string        `json:"currentRound"`
	CommunityCards  []deck.Card   `json:"communityCards"`
	Players         []PlayerState `json:"players"`
	ActingPlayerIdx int           `json:"actingPlayerIdx"`
}

type ActionRequiredData struct {
	TableID        string            `json:"tableId"`
	PlayerName     string            `json:"playerName"`
	ValidActions   []ValidActionInfo `json:"validActions"`
	TableState     TableStateData    `json:"tableState"`
	TimeoutSeconds int               `json:"timeoutSeconds"`
}

type BotAddedData struct {
	TableID  string   `json:"tableId"`
	BotNames []string `json:"botNames"`
	Message  string   `json:"message"`
}

type BotKickedData struct {
	TableID string `json:"tableId"`
	BotName string `json:"botName"`
	Message string `json:"message"`
}

type PlayerTimeoutData struct {
	TableID        string `json:"tableId"`
	PlayerName     string `json:"playerName"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
	Action         string `json:"action"` // The action taken due to timeout (fold/check)
}

type GamePauseData struct {
	TableID string `json:"tableId"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// Helper functions to convert between internal types and message types

func PlayerStateFromGame(p *game.Player, includeHoleCards bool) PlayerState {
	var holeCards []deck.Card
	if includeHoleCards {
		holeCards = p.HoleCards
	}

	return PlayerState{
		Name:         p.Name,
		Chips:        p.Chips,
		Position:     p.Position.String(),
		SeatNumber:   p.SeatNumber,
		HoleCards:    holeCards,
		BetThisRound: p.BetThisRound,
		TotalBet:     p.TotalBet,
		IsActive:     p.IsActive,
		IsFolded:     p.IsFolded,
		IsAllIn:      p.IsAllIn,
		LastAction:   p.LastAction.String(),
	}
}

func ValidActionInfoFromGame(va game.ValidAction) ValidActionInfo {
	return ValidActionInfo{
		Action:    va.Action.String(),
		MinAmount: va.MinAmount,
		MaxAmount: va.MaxAmount,
	}
}

func TableStateFromGame(ts game.TableState) TableStateData {
	players := make([]PlayerState, len(ts.Players))
	for i, p := range ts.Players {
		// Include hole cards only for the acting player
		includeHoleCards := (i == ts.ActingPlayerIdx)
		players[i] = PlayerState{
			Name:       p.Name,
			Chips:      p.Chips,
			Position:   p.Position.String(),
			SeatNumber: 0, // Not available in PlayerState
			HoleCards: func() []deck.Card {
				if includeHoleCards {
					return p.HoleCards
				}
				return nil
			}(),
			BetThisRound: p.BetThisRound,
			TotalBet:     p.TotalBet,
			IsActive:     p.IsActive,
			IsFolded:     p.IsFolded,
			IsAllIn:      p.IsAllIn,
			LastAction:   p.LastAction.String(),
		}
	}

	return TableStateData{
		CurrentBet:      ts.CurrentBet,
		Pot:             ts.Pot,
		CurrentRound:    ts.CurrentRound.String(),
		CommunityCards:  ts.CommunityCards,
		Players:         players,
		ActingPlayerIdx: ts.ActingPlayerIdx,
	}
}
