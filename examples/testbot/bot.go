package main

import (
	"fmt"
	"math/rand"
	"net/url"
	"os"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
	"github.com/rs/zerolog"
)

// BotStrategy defines how a bot makes decisions.
type BotStrategy interface {
	SelectAction(validActions []string, pot int, toCall int, minBet int, chips int) (string, int)
	GetName() string
}

// Bot represents a poker bot client.
type Bot struct {
	conn     *websocket.Conn
	strategy BotStrategy
	botID    string
	handID   string
	logger   zerolog.Logger
	chips    int
	seat     int
}

// NewBot creates a new bot with the given strategy.
func NewBot(strategy BotStrategy) *Bot {
	botID := fmt.Sprintf("%s-%d", strategy.GetName(), rand.Intn(10000))
	return &Bot{
		strategy: strategy,
		botID:    botID,
		logger: zerolog.New(os.Stderr).With().
			Str("bot_id", botID).
			Str("strategy", strategy.GetName()).
			Logger(),
	}
}

// Connect establishes a websocket connection to the server.
func (b *Bot) Connect(serverURL string) error {
	u, err := url.Parse(serverURL)
	if err != nil {
		return err
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}

	b.conn = conn
	connectMsg := &protocol.Connect{Type: protocol.TypeConnect, Name: b.botID, Role: "player"}
	data, err := protocol.Marshal(connectMsg)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, data)
}

// Run starts the bot's main loop.
func (b *Bot) Run() error {
	for {
		msgType, data, err := b.conn.ReadMessage()
		if err != nil {
			return err
		}
		if msgType != websocket.BinaryMessage {
			continue
		}
		if err := b.handleMessage(data); err != nil {
			b.logger.Error().Err(err).Msg("error handling message")
		}
	}
}

func (b *Bot) handleMessage(data []byte) error {
	var actionReq protocol.ActionRequest
	if err := protocol.Unmarshal(data, &actionReq); err == nil && actionReq.Type == protocol.TypeActionRequest {
		return b.handleActionRequest(&actionReq)
	}

	var handStart protocol.HandStart
	if err := protocol.Unmarshal(data, &handStart); err == nil && handStart.Type == protocol.TypeHandStart {
		b.handID = handStart.HandID
		b.seat = handStart.YourSeat
		for _, p := range handStart.Players {
			if p.Seat == b.seat {
				b.chips = p.Chips
				break
			}
		}
		return nil
	}

	var gameUpdate protocol.GameUpdate
	if err := protocol.Unmarshal(data, &gameUpdate); err == nil && gameUpdate.Type == protocol.TypeGameUpdate {
		for _, p := range gameUpdate.Players {
			if p.Seat == b.seat {
				b.chips = p.Chips
				break
			}
		}
		return nil
	}

	return nil
}

func (b *Bot) handleActionRequest(req *protocol.ActionRequest) error {
	action, amount := b.strategy.SelectAction(req.ValidActions, req.Pot, req.ToCall, req.MinBet, b.chips)
	actionMsg := &protocol.Action{Type: protocol.TypeAction, Action: action, Amount: amount}
	data, err := protocol.Marshal(actionMsg)
	if err != nil {
		return err
	}
	return b.conn.WriteMessage(websocket.BinaryMessage, data)
}
