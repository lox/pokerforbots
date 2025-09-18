package main

import (
	"flag"
	"os"

	"github.com/rs/zerolog"
)

func runRandom() {
	serverURL := flag.String("server", "ws://localhost:8080/ws", "WebSocket server URL")
	count := flag.Int("count", 1, "Number of bots to run")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	s := &RandomStrategy{}

	level := zerolog.InfoLevel
	if *debug {
		level = zerolog.DebugLevel
	}
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).Level(level).With().Timestamp().Logger()

	for i := 0; i < *count; i++ {
		bot := NewBot(s)
		if err := bot.Connect(*serverURL); err != nil {
			logger.Fatal().Err(err).Int("bot_number", i).Msg("Failed to connect bot")
		}

		go func(b *Bot) {
			if err := b.Run(); err != nil {
				b.logger.Error().Err(err).Msg("Bot disconnected")
			}
		}(bot)

		logger.Info().Int("bot_number", i+1).Str("bot_id", bot.botID).Msg("Random bot connected")
	}

	waitForInterrupt()
}

// NewRandomBot returns a quick single instance for other examples.
func NewRandomBot() *Bot {
	return NewBot(&RandomStrategy{})
}
