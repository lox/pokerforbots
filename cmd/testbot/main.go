package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
)

// BotStrategy defines how a bot makes decisions
type BotStrategy interface {
	// SelectAction chooses an action given the current game state
	SelectAction(validActions []string, pot int, toCall int) (string, int)
	// GetName returns the strategy name
	GetName() string
}

// Bot represents a poker bot client
type Bot struct {
	conn     *websocket.Conn
	strategy BotStrategy
	botID    string
	handID   string
}

// NewBot creates a new bot with the given strategy
func NewBot(strategy BotStrategy) *Bot {
	return &Bot{
		strategy: strategy,
		botID:    fmt.Sprintf("%s-%d", strategy.GetName(), rand.Intn(10000)),
	}
}

// Connect establishes a websocket connection to the server
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

	// Send connect message
	connectMsg := &protocol.Connect{
		Type: "connect",
		Name: b.botID,
	}

	data, err := protocol.Marshal(connectMsg)
	if err != nil {
		return err
	}

	return conn.WriteMessage(websocket.BinaryMessage, data)
}

// Run starts the bot's main loop
func (b *Bot) Run() error {
	for {
		msgType, data, err := b.conn.ReadMessage()
		if err != nil {
			return err
		}

		if msgType != websocket.BinaryMessage {
			continue
		}

		// Determine message type and handle it
		if err := b.handleMessage(data); err != nil {
			log.Printf("[%s] Error handling message: %v", b.botID, err)
		}
	}
}

func (b *Bot) handleMessage(data []byte) error {
	// Try to decode as each message type
	// First try ActionRequest as it's the most common
	var actionReq protocol.ActionRequest
	if err := protocol.Unmarshal(data, &actionReq); err == nil && actionReq.HandID != "" {
		return b.handleActionRequest(&actionReq)
	}

	// Try HandStart
	var handStart protocol.HandStart
	if err := protocol.Unmarshal(data, &handStart); err == nil && handStart.HandID != "" {
		b.handID = handStart.HandID
		log.Printf("[%s] Hand %s started. Hole cards: %v",
			b.botID, handStart.HandID, handStart.HoleCards)
		return nil
	}

	// Try GameUpdate
	var gameUpdate protocol.GameUpdate
	if err := protocol.Unmarshal(data, &gameUpdate); err == nil && gameUpdate.HandID != "" {
		// Log game updates if needed
		return nil
	}

	// Try StreetChange
	var streetChange protocol.StreetChange
	if err := protocol.Unmarshal(data, &streetChange); err == nil && streetChange.HandID != "" {
		log.Printf("[%s] Street changed to %s. Board: %v",
			b.botID, streetChange.Street, streetChange.Board)
		return nil
	}

	// Try HandResult
	var handResult protocol.HandResult
	if err := protocol.Unmarshal(data, &handResult); err == nil && handResult.HandID != "" {
		log.Printf("[%s] Hand %s completed. Winners: %v",
			b.botID, handResult.HandID, handResult.Winners)
		return nil
	}

	// Try Error
	var errorMsg protocol.Error
	if err := protocol.Unmarshal(data, &errorMsg); err == nil && errorMsg.Message != "" {
		log.Printf("[%s] Server error: %s", b.botID, errorMsg.Message)
		return nil
	}

	return nil
}

func (b *Bot) handleActionRequest(req *protocol.ActionRequest) error {
	// Use strategy to select action
	action, amount := b.strategy.SelectAction(req.ValidActions, req.Pot, req.ToCall)

	// Send action response
	actionMsg := &protocol.Action{
		Type:   "action",
		Action: action,
		Amount: amount,
	}

	data, err := protocol.Marshal(actionMsg)
	if err != nil {
		return err
	}

	log.Printf("[%s] Action: %s %d (pot: %d, to call: %d)",
		b.botID, action, amount, req.Pot, req.ToCall)

	return b.conn.WriteMessage(websocket.BinaryMessage, data)
}

// Close closes the connection
func (b *Bot) Close() {
	if b.conn != nil {
		_ = b.conn.Close()
	}
}

// CallingStationStrategy always calls or checks
type CallingStationStrategy struct{}

func (s *CallingStationStrategy) GetName() string {
	return "calling-station"
}

func (s *CallingStationStrategy) SelectAction(validActions []string, pot int, toCall int) (string, int) {
	// Prefer check over call
	for _, action := range validActions {
		if action == "check" {
			return "check", 0
		}
	}
	// Otherwise call
	for _, action := range validActions {
		if action == "call" {
			return "call", 0
		}
	}
	// If can't call or check, must fold
	return "fold", 0
}

// RandomStrategy selects random valid actions
type RandomStrategy struct{}

func (s *RandomStrategy) GetName() string {
	return "random"
}

func (s *RandomStrategy) SelectAction(validActions []string, pot int, toCall int) (string, int) {
	if len(validActions) == 0 {
		return "fold", 0
	}

	// Pick a random valid action
	action := validActions[rand.Intn(len(validActions))]

	// If raising, pick a random amount between min and 3x pot
	if action == "raise" {
		minRaise := toCall * 2
		maxRaise := pot * 3
		if maxRaise < minRaise {
			maxRaise = minRaise * 2
		}
		amount := minRaise + rand.Intn(maxRaise-minRaise+1)
		return action, amount
	}

	return action, 0
}

// AggressiveStrategy raises frequently
type AggressiveStrategy struct{}

func (s *AggressiveStrategy) GetName() string {
	return "aggressive"
}

func (s *AggressiveStrategy) SelectAction(validActions []string, pot int, toCall int) (string, int) {
	// Check if we can raise
	canRaise := false
	for _, action := range validActions {
		if action == "raise" || action == "allin" {
			canRaise = true
			break
		}
	}

	// 70% chance to raise if possible
	if canRaise && rand.Float32() < 0.7 {
		for _, action := range validActions {
			if action == "allin" {
				return "allin", 0
			}
			if action == "raise" {
				// Raise between 2x and 4x the pot
				amount := pot*2 + rand.Intn(pot*2+1)
				if amount < toCall*2 {
					amount = toCall * 2
				}
				return "raise", amount
			}
		}
	}

	// Otherwise call if we can
	for _, action := range validActions {
		if action == "call" {
			return "call", 0
		}
	}

	// Check if possible
	for _, action := range validActions {
		if action == "check" {
			return "check", 0
		}
	}

	return "fold", 0
}

func main() {
	var (
		serverURL = flag.String("server", "ws://localhost:8080/ws", "WebSocket server URL")
		strategy  = flag.String("strategy", "random", "Bot strategy: calling-station, random, or aggressive")
		count     = flag.Int("count", 1, "Number of bots to run")
	)
	flag.Parse()

	// Seed random number generator
	rand.Seed(time.Now().UnixNano())

	// Create bots with selected strategy
	var bots []*Bot
	for i := 0; i < *count; i++ {
		var strat BotStrategy
		switch *strategy {
		case "calling-station":
			strat = &CallingStationStrategy{}
		case "aggressive":
			strat = &AggressiveStrategy{}
		default:
			strat = &RandomStrategy{}
		}

		bot := NewBot(strat)
		if err := bot.Connect(*serverURL); err != nil {
			log.Fatalf("Failed to connect bot %d: %v", i, err)
		}
		bots = append(bots, bot)

		// Start bot in goroutine
		go func(b *Bot) {
			if err := b.Run(); err != nil {
				log.Printf("Bot %s disconnected: %v", b.botID, err)
			}
		}(bot)

		log.Printf("Bot %d connected: %s", i+1, bot.botID)
	}

	// Wait for interrupt
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	<-interrupt

	log.Println("Shutting down bots...")
	for _, bot := range bots {
		bot.Close()
	}
}
