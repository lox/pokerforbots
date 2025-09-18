package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
	"github.com/rs/zerolog"
)

func main() {
	serverURL := flag.String("server", "ws://localhost:8080/ws", "WebSocket server URL")
	flag.Parse()

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()

	conn, _, err := websocket.DefaultDialer.Dial(*serverURL, nil)
	if err != nil {
		logger.Fatal().Err(err).Msg("connect error")
	}
	defer conn.Close()

	id := fmt.Sprintf("random-%04d", rand.Intn(10000))
	connect := &protocol.Connect{Type: protocol.TypeConnect, Name: id, Role: "player"}
	payload, err := protocol.Marshal(connect)
	if err != nil {
		logger.Fatal().Err(err).Msg("marshal connect")
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, payload); err != nil {
		conn.Close()
		logger.Fatal().Err(err).Msg("send connect")
	}

	logger.Info().Str("bot_id", id).Msg("connected")

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-interrupt:
			logger.Info().Msg("shutting down")
			return
		default:
		}

		msgType, data, err := conn.ReadMessage()
		if err != nil {
			logger.Error().Err(err).Msg("read error")
			return
		}
		if msgType != websocket.BinaryMessage {
			continue
		}

		var req protocol.ActionRequest
		if err := protocol.Unmarshal(data, &req); err != nil || req.Type != protocol.TypeActionRequest {
			continue
		}

		action := pickRandomAction(req)
		resp, err := protocol.Marshal(&action)
		if err != nil {
			logger.Error().Err(err).Msg("marshal action")
			return
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, resp); err != nil {
			logger.Error().Err(err).Msg("send action")
			return
		}
	}
}

func pickRandomAction(req protocol.ActionRequest) protocol.Action {
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
