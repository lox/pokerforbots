package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
	"github.com/rs/zerolog"
)

// tableState holds the latest state the bot knows about.
type tableState struct {
	HandID     string
	Seat       int
	Pot        int
	Chips      int
	Players    []protocol.Player
	LastAction protocol.PlayerAction
}

// complexBot is a placeholder for future advanced strategy.
type complexBot struct {
	id     string
	conn   *websocket.Conn
	logger zerolog.Logger
	state  tableState
}

func newComplexBot(logger zerolog.Logger) *complexBot {
	id := fmt.Sprintf("complex-%04d", rand.Intn(10000))
	return &complexBot{id: id, logger: logger.With().Str("bot_id", id).Logger()}
}

func (b *complexBot) connect(serverURL string) error {
	u, err := url.Parse(serverURL)
	if err != nil {
		return err
	}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	b.conn = conn

	connect := &protocol.Connect{Type: protocol.TypeConnect, Name: b.id, Role: "player"}
	payload, err := protocol.Marshal(connect)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, payload)
}

func (b *complexBot) run() error {
	for {
		msgType, data, err := b.conn.ReadMessage()
		if err != nil {
			return err
		}
		if msgType != websocket.BinaryMessage {
			continue
		}
		if err := b.handle(data); err != nil {
			b.logger.Error().Err(err).Msg("handler error")
		}
	}
}

func (b *complexBot) handle(data []byte) error {
	if b.tryHandStart(data) || b.tryGameUpdate(data) || b.tryPlayerAction(data) {
		return nil
	}

	var req protocol.ActionRequest
	if err := protocol.Unmarshal(data, &req); err == nil && req.Type == protocol.TypeActionRequest {
		return b.respond(req)
	}
	return nil
}

func (b *complexBot) tryHandStart(data []byte) bool {
	var start protocol.HandStart
	if err := protocol.Unmarshal(data, &start); err != nil || start.Type != protocol.TypeHandStart {
		return false
	}
	b.state.HandID = start.HandID
	b.state.Seat = start.YourSeat
	b.state.Players = start.Players
	b.state.Chips = start.Players[start.YourSeat].Chips
	b.logger.Info().Strs("holes", start.HoleCards).Msg("hand start")
	return true
}

func (b *complexBot) tryGameUpdate(data []byte) bool {
	var update protocol.GameUpdate
	if err := protocol.Unmarshal(data, &update); err != nil || update.Type != protocol.TypeGameUpdate {
		return false
	}
	b.state.Pot = update.Pot
	b.state.Players = update.Players
	if b.state.Seat >= 0 && b.state.Seat < len(update.Players) {
		b.state.Chips = update.Players[b.state.Seat].Chips
	}
	return true
}

func (b *complexBot) tryPlayerAction(data []byte) bool {
	var action protocol.PlayerAction
	if err := protocol.Unmarshal(data, &action); err != nil || action.Type != protocol.TypePlayerAction {
		return false
	}
	b.state.LastAction = action
	return true
}

func (b *complexBot) respond(req protocol.ActionRequest) error {
	// Basic placeholder: default to random choice but log context for future heuristics.
	b.logState(req)
	act := randomFallback(req)
	payload, err := protocol.Marshal(&act)
	if err != nil {
		return err
	}
	return b.conn.WriteMessage(websocket.BinaryMessage, payload)
}

func (b *complexBot) logState(req protocol.ActionRequest) {
	snapshot := struct {
		Time       time.Time
		HandID     string
		Seat       int
		Request    protocol.ActionRequest
		LastAction protocol.PlayerAction
	}{
		Time:       time.Now(),
		HandID:     b.state.HandID,
		Seat:       b.state.Seat,
		Request:    req,
		LastAction: b.state.LastAction,
	}
	buf, _ := json.Marshal(snapshot)
	b.logger.Debug().RawJSON("state", buf).Msg("decision")
}

func main() {
	serverURL := flag.String("server", "ws://localhost:8080/ws", "WebSocket server URL")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	level := zerolog.InfoLevel
	if *debug {
		level = zerolog.DebugLevel
	}
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).Level(level).With().Timestamp().Logger()

	bot := newComplexBot(logger)
	if err := bot.connect(*serverURL); err != nil {
		logger.Fatal().Err(err).Msg("connect failed")
	}
	logger.Info().Msg("complex bot connected")

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	runErr := make(chan error, 1)
	go func() { runErr <- bot.run() }()

	select {
	case <-interrupt:
		logger.Info().Msg("shutting down")
	case err := <-runErr:
		if err != nil {
			logger.Error().Err(err).Msg("run error")
		}
	}
}

func randomFallback(req protocol.ActionRequest) protocol.Action {
	if len(req.ValidActions) == 0 {
		return protocol.Action{Type: protocol.TypeAction, Action: "fold"}
	}
	choice := req.ValidActions[rand.Intn(len(req.ValidActions))]
	act := protocol.Action{Type: protocol.TypeAction, Action: choice}
	if choice == "raise" {
		amount := req.MinBet
		if req.ToCall > 0 {
			amount = req.ToCall + req.MinRaise
		}
		if amount < req.MinBet {
			amount = req.MinBet
		}
		act.Amount = amount
	}
	return act
}
