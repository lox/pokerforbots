package server

import (
	"fmt"

	"github.com/lox/pokerforbots/v2/internal/randutil"

	"slices"
	"testing"

	"github.com/lox/pokerforbots/v2/internal/game"
	"github.com/lox/pokerforbots/v2/poker"
)

// BenchmarkGameEngine benchmarks just the core game logic
func BenchmarkGameEngine(b *testing.B) {
	configs := []struct {
		name       string
		numPlayers int
		streets    int // how many streets to play
	}{
		{"2Players_Preflop", 2, 1},
		{"6Players_Preflop", 6, 1},
		{"6Players_Flop", 6, 2},
		{"6Players_River", 6, 4},
		{"9Players_River", 9, 4},
	}

	for _, cfg := range configs {
		b.Run(cfg.name, func(b *testing.B) {
			b.ReportAllocs()

			for i := 0; b.Loop(); i++ {
				// Create players
				players := make([]string, cfg.numPlayers)
				chipCounts := make([]int, cfg.numPlayers)
				for j := 0; j < cfg.numPlayers; j++ {
					players[j] = fmt.Sprintf("Player%d", j)
					chipCounts[j] = 1000
				}

				// Create hand state with deck
				rng := randutil.New(getSeed(int64(i)))
				deck := poker.NewDeck(rng)
				hand := game.NewHandState(
					rng,
					players,
					0,  // button
					5,  // small blind
					10, // big blind
					game.WithChipsByPlayer(chipCounts),
					game.WithDeck(deck),
				)

				// Play through streets
				streetsToPlay := cfg.streets
				currentStreet := 0

				for currentStreet < streetsToPlay && !hand.IsComplete() {
					for !hand.IsComplete() && hand.Street == game.Street(currentStreet) {
						validActions := hand.GetValidActions()
						if len(validActions) > 0 {
							// Simple strategy: mostly call/check
							action := game.Call
							if slices.Contains(validActions, game.Check) {
								action = game.Check
							}
							hand.ProcessAction(action, 0)
						}
					}
					currentStreet++
				}
			}

			handsPerSec := float64(b.N) / b.Elapsed().Seconds()
			b.ReportMetric(handsPerSec, "hands/sec")
		})
	}
}

func getSeed(i int64) int64 {
	return 12345 + i // Deterministic seeds for reproducibility
}
