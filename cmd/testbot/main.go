package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"os/signal"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// BotStrategy defines how a bot makes decisions
type BotStrategy interface {
	// SelectAction chooses an action given the current game state
	SelectAction(validActions []string, pot int, toCall int, chips int) (string, int)
	// GetName returns the strategy name
	GetName() string
}

// Bot represents a poker bot client
type Bot struct {
	conn     *websocket.Conn
	strategy BotStrategy
	botID    string
	handID   string
	logger   zerolog.Logger
	chips    int // Current chip count
	seat     int // Our seat number
}

// NewBot creates a new bot with the given strategy
func NewBot(strategy BotStrategy) *Bot {
	botID := fmt.Sprintf("%s-%d", strategy.GetName(), rand.Intn(10000))
	return &Bot{
		strategy: strategy,
		botID:    botID,
		logger: log.With().
			Str("bot_id", botID).
			Str("strategy", strategy.GetName()).
			Logger(),
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
			b.logger.Error().Err(err).Msg("Error handling message")
		}
	}
}

func (b *Bot) handleMessage(data []byte) error {
	// Try to decode as each message type
	// First try ActionRequest as it's the most common
	var actionReq protocol.ActionRequest
	if err := protocol.Unmarshal(data, &actionReq); err == nil && actionReq.Type == "action_request" {
		return b.handleActionRequest(&actionReq)
	}

	// Try HandStart
	var handStart protocol.HandStart
	if err := protocol.Unmarshal(data, &handStart); err == nil && handStart.HandID != "" {
		b.handID = handStart.HandID
		b.seat = handStart.YourSeat

		// Find our chips
		for _, p := range handStart.Players {
			if p.Seat == b.seat {
				b.chips = p.Chips
				break
			}
		}

		b.logger.Info().
			Str("hand_id", handStart.HandID).
			Strs("hole_cards", handStart.HoleCards).
			Int("seat", handStart.YourSeat).
			Int("chips", b.chips).
			Int("button", handStart.Button).
			Int("small_blind", handStart.SmallBlind).
			Int("big_blind", handStart.BigBlind).
			Int("num_players", len(handStart.Players)).
			Msg("Hand started")
		return nil
	}

	// Try GameUpdate
	var gameUpdate protocol.GameUpdate
	if err := protocol.Unmarshal(data, &gameUpdate); err == nil && gameUpdate.HandID != "" {
		// Update our chip count
		for _, p := range gameUpdate.Players {
			if p.Seat == b.seat {
				b.chips = p.Chips
				break
			}
		}

		b.logger.Debug().
			Str("hand_id", gameUpdate.HandID).
			Int("pot", gameUpdate.Pot).
			Int("chips", b.chips).
			Int("num_players", len(gameUpdate.Players)).
			Msg("Game update")
		return nil
	}

	// Try StreetChange
	var streetChange protocol.StreetChange
	if err := protocol.Unmarshal(data, &streetChange); err == nil && streetChange.HandID != "" {
		b.logger.Info().
			Str("hand_id", streetChange.HandID).
			Str("street", streetChange.Street).
			Strs("board", streetChange.Board).
			Msg("Street changed")
		return nil
	}

	// Try HandResult
	var handResult protocol.HandResult
	if err := protocol.Unmarshal(data, &handResult); err == nil && handResult.HandID != "" {
		b.logger.Info().
			Str("hand_id", handResult.HandID).
			Interface("winners", handResult.Winners).
			Strs("board", handResult.Board).
			Msg("Hand completed")
		return nil
	}

	// Try Error
	var errorMsg protocol.Error
	if err := protocol.Unmarshal(data, &errorMsg); err == nil && errorMsg.Message != "" {
		b.logger.Error().
			Str("error_message", errorMsg.Message).
			Msg("Server error")
		return nil
	}

	return nil
}

func (b *Bot) handleActionRequest(req *protocol.ActionRequest) error {
	// Log the decision context
	b.logger.Info().
		Str("hand_id", req.HandID).
		Strs("valid_actions", req.ValidActions).
		Int("pot", req.Pot).
		Int("to_call", req.ToCall).
		Int("min_raise", req.MinRaise).
		Int("time_remaining", req.TimeRemaining).
		Msg("Action requested")

	// Use strategy to select action
	action, amount := b.strategy.SelectAction(req.ValidActions, req.Pot, req.ToCall, b.chips)

	// Log the decision
	b.logger.Info().
		Str("hand_id", req.HandID).
		Str("action", action).
		Int("amount", amount).
		Int("pot_before", req.Pot).
		Int("to_call", req.ToCall).
		Msg("Action decided")

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

func (s *CallingStationStrategy) SelectAction(validActions []string, pot int, toCall int, chips int) (string, int) {
	log.Debug().
		Strs("valid_actions", validActions).
		Str("strategy", "calling-station").
		Msg("Selecting action: prefer check > call > fold")

	// Prefer check over call
	for _, action := range validActions {
		if action == "check" {
			log.Debug().Str("reason", "can check for free").Msg("Decision made")
			return "check", 0
		}
	}
	// Otherwise call
	for _, action := range validActions {
		if action == "call" {
			log.Debug().
				Int("amount_to_call", toCall).
				Str("reason", "calling to stay in hand").
				Msg("Decision made")
			return "call", 0
		}
	}
	// If can't call or check, must fold
	log.Debug().Str("reason", "no check or call available").Msg("Decision made")
	return "fold", 0
}

// RandomStrategy selects random valid actions
type RandomStrategy struct{}

func (s *RandomStrategy) GetName() string {
	return "random"
}

func (s *RandomStrategy) SelectAction(validActions []string, pot int, toCall int, chips int) (string, int) {
	if len(validActions) == 0 {
		log.Debug().Msg("No valid actions available, folding")
		return "fold", 0
	}

	// Pick a random valid action
	actionIndex := rand.Intn(len(validActions))
	action := validActions[actionIndex]

	log.Debug().
		Strs("valid_actions", validActions).
		Str("selected_action", action).
		Int("action_index", actionIndex).
		Str("strategy", "random").
		Msg("Randomly selecting action")

	// If raising, pick a random amount between min and 3x pot (capped by chips)
	if action == "raise" {
		minRaise := toCall * 2
		maxRaise := pot * 3
		if maxRaise < minRaise {
			maxRaise = minRaise * 2
		}
		// Cap at our chip count
		if maxRaise > chips {
			maxRaise = chips
		}
		if minRaise > chips {
			// Can't raise, switch to call or fold
			for _, fallback := range validActions {
				if fallback == "call" {
					return "call", 0
				}
			}
			return "fold", 0
		}
		amount := minRaise + rand.Intn(maxRaise-minRaise+1)
		log.Debug().
			Int("min_raise", minRaise).
			Int("max_raise", maxRaise).
			Int("chips", chips).
			Int("selected_amount", amount).
			Str("reason", "random raise amount").
			Msg("Decision made")
		return action, amount
	}

	log.Debug().Str("reason", "random action selected").Msg("Decision made")
	return action, 0
}

// AggressiveStrategy raises frequently
type AggressiveStrategy struct{}

func (s *AggressiveStrategy) GetName() string {
	return "aggressive"
}

func (s *AggressiveStrategy) SelectAction(validActions []string, pot int, toCall int, chips int) (string, int) {
	// Check if we can raise
	canRaise := false
	canAllIn := false
	for _, action := range validActions {
		if action == "raise" {
			canRaise = true
		}
		if action == "allin" {
			canAllIn = true
		}
	}

	log.Debug().
		Strs("valid_actions", validActions).
		Bool("can_raise", canRaise).
		Bool("can_allin", canAllIn).
		Str("strategy", "aggressive").
		Msg("Evaluating aggressive options")

	// 70% chance to raise if possible
	raiseRoll := rand.Float32()
	if (canRaise || canAllIn) && raiseRoll < 0.7 {
		log.Debug().
			Float32("roll", raiseRoll).
			Float32("threshold", 0.7).
			Str("decision", "will raise/allin").
			Msg("Aggression check passed")

		if canAllIn {
			log.Debug().Str("reason", "going all-in aggressively").Msg("Decision made")
			return "allin", 0
		}
		if canRaise {
			// Raise between 2x and 4x the pot (capped by chips)
			amount := pot*2 + rand.Intn(pot*2+1)
			if amount < toCall*2 {
				amount = toCall * 2
			}
			// Cap at our chip count
			if amount > chips {
				amount = chips
			}
			log.Debug().
				Int("raise_amount", amount).
				Int("pot_size", pot).
				Int("chips", chips).
				Str("reason", "aggressive raise 2-4x pot").
				Msg("Decision made")
			return "raise", amount
		}
	} else if canRaise || canAllIn {
		log.Debug().
			Float32("roll", raiseRoll).
			Float32("threshold", 0.7).
			Str("decision", "will not raise").
			Msg("Aggression check failed")
	}

	// Otherwise call if we can
	for _, action := range validActions {
		if action == "call" {
			log.Debug().
				Int("to_call", toCall).
				Str("reason", "calling instead of raising").
				Msg("Decision made")
			return "call", 0
		}
	}

	// Check if possible
	for _, action := range validActions {
		if action == "check" {
			log.Debug().Str("reason", "checking - no raise or call available").Msg("Decision made")
			return "check", 0
		}
	}

	log.Debug().Str("reason", "forced to fold - no other options").Msg("Decision made")
	return "fold", 0
}

func main() {
	var (
		serverURL = flag.String("server", "ws://localhost:8080/ws", "WebSocket server URL")
		strategy  = flag.String("strategy", "random", "Bot strategy: calling-station, random, or aggressive")
		count     = flag.Int("count", 1, "Number of bots to run")
		debug     = flag.Bool("debug", false, "Enable debug logging")
	)
	flag.Parse()

	// Configure zerolog
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	if *debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Random number generator is automatically seeded in Go 1.20+

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
			log.Fatal().Err(err).Int("bot_number", i).Msg("Failed to connect bot")
		}
		bots = append(bots, bot)

		// Start bot in goroutine
		go func(b *Bot) {
			if err := b.Run(); err != nil {
				b.logger.Error().Err(err).Msg("Bot disconnected")
			}
		}(bot)

		log.Info().
			Int("bot_number", i+1).
			Str("bot_id", bot.botID).
			Str("strategy", strat.GetName()).
			Msg("Bot connected")
	}

	// Wait for interrupt
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	<-interrupt

	log.Info().Msg("Shutting down bots...")
	for _, bot := range bots {
		bot.Close()
	}
	log.Info().Msg("All bots disconnected")
}
