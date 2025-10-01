package server

import (
	"fmt"

	"github.com/lox/pokerforbots/v2/internal/randutil"

	"testing"
	"time"

	"github.com/lox/pokerforbots/v2/protocol"
)

func newBenchBot(id string) *Bot {
	return &Bot{
		ID:       id,
		send:     make(chan []byte, 64),
		bankroll: defaultStartChips,
		done:     make(chan struct{}),
	}
}

func BenchmarkHandRunnerHeadsUpFold(b *testing.B) {
	b.ReportAllocs()
	logger := testLogger()

	masterRNG := randutil.New(2024)

	for i := 0; i < b.N; i++ {
		b.StopTimer()

		bots := []*Bot{
			newBenchBot(fmt.Sprintf("bench-botA-%d", i)),
			newBenchBot(fmt.Sprintf("bench-botB-%d", i)),
		}

		cfg := Config{
			SmallBlind: defaultSmallBlind,
			BigBlind:   defaultBigBlind,
			StartChips: defaultStartChips,
			Timeout:    5 * time.Millisecond,
		}

		runner := NewHandRunnerWithConfig(
			logger,
			bots,
			fmt.Sprintf("bench-hand-%d", i),
			0,
			masterRNG,
			cfg,
		)

		runner.botActionChan <- ActionEnvelope{
			BotID: bots[0].ID,
			Action: protocol.Action{
				Type:   protocol.TypeAction,
				Action: "fold",
			},
		}

		b.StartTimer()
		runner.Run()
	}
}
