package main

import (
	"flag"
	"math/rand"
	"os"

	"github.com/rs/zerolog"
)

// ComplexStrategy is a placeholder for future smarter logic.
type ComplexStrategy struct{}

func (s *ComplexStrategy) GetName() string { return "complex" }

func (s *ComplexStrategy) SelectAction(validActions []string, pot int, toCall int, minBet int, chips int) (string, int) {
	// Simple baseline: play like random for now.
	if len(validActions) == 0 {
		return "fold", 0
	}
	action := validActions[rand.Intn(len(validActions))]
	if action == "raise" {
		amount := minBet
		if chips > minBet {
			amount = minBet + rand.Intn(chips-minBet+1)
		}
		return "raise", amount
	}
	return action, 0
}

func runComplex() {
	serverURL := flag.String("server", "ws://localhost:8080/ws", "WebSocket server URL")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	s := &ComplexStrategy{}
	bot := NewBot(s)

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	level := zerolog.InfoLevel
	if *debug {
		level = zerolog.DebugLevel
	}
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).Level(level).With().Timestamp().Logger()

	if err := bot.Connect(*serverURL); err != nil {
		logger.Fatal().Err(err).Msg("Failed to connect complex bot")
	}

	if err := bot.Run(); err != nil {
		logger.Error().Err(err).Msg("Complex bot disconnected")
	}
}
