package server

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/lox/pokerforbots/internal/game"
	"github.com/lox/pokerforbots/poker"
	"github.com/rs/zerolog"
)

// BenchmarkHandRunner is disabled because it requires real bot connections
// Use BenchmarkGameEngine or BenchmarkNPCStrategies instead

// BenchmarkBotPool is disabled because it requires real bot connections
// Use BenchmarkNPCStrategies to test pool behavior with NPCs instead

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

			for i := 0; i < b.N; i++ {
				// Create players
				players := make([]string, cfg.numPlayers)
				chipCounts := make([]int, cfg.numPlayers)
				for j := 0; j < cfg.numPlayers; j++ {
					players[j] = fmt.Sprintf("Player%d", j)
					chipCounts[j] = 1000
				}

				// Create hand state with deck
				rng := rand.New(rand.NewSource(getSeed(int64(i))))
				deck := poker.NewDeck(rng)
				hand := game.NewHandState(
					rng,
					players,
					0,  // button
					5,  // small blind
					10, // big blind
					game.WithChips(chipCounts),
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
							for _, a := range validActions {
								if a == game.Check {
									action = game.Check
									break
								}
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

// BenchmarkNPCStrategies benchmarks different NPC strategies
func BenchmarkNPCStrategies(b *testing.B) {
	strategies := []struct {
		name     string
		strategy npcStrategy
	}{
		{"CallingStation", &callingStationStrategy{}},
		{"Random", &randomStrategy{}},
		{"Aggressive", &aggressiveStrategy{}},
	}

	for _, s := range strategies {
		b.Run(s.name, func(b *testing.B) {
			logger := zerolog.Nop()
			rng := rand.New(rand.NewSource(12345))

			config := Config{
				MinPlayers:       6,
				MaxPlayers:       6,
				SmallBlind:       5,
				BigBlind:         10,
				StartChips:       1000,
				Timeout:          5 * time.Millisecond,
				InfiniteBankroll: true,
				HandLimit:        uint64(b.N),
			}

			config.HandLimit = uint64(b.N)
			pool := NewBotPool(logger, rng, config)

			// Start pool
			go pool.Run()

			// Create 6 NPCs with the same strategy
			npcs := make([]*npcBot, 6)
			for i := 0; i < 6; i++ {
				npc := newNPCBot(logger, pool, "bench", s.strategy)
				npcs[i] = npc
				npc.start()
			}

			// Wait for hands to complete
			b.ResetTimer()
			b.ReportAllocs()

			startTime := time.Now()
			targetHands := b.N

			// Wait for target number of hands
			for pool.HandCount() < uint64(targetHands) {
				time.Sleep(10 * time.Millisecond)
				if time.Since(startTime) > 30*time.Second {
					b.Fatalf("Timeout waiting for %d hands (completed: %d)", targetHands, pool.HandCount())
				}
			}

			// Stop all NPCs
			for _, npc := range npcs {
				npc.stop()
			}

			elapsed := time.Since(startTime)
			handsPerSec := float64(targetHands) / elapsed.Seconds()
			b.ReportMetric(handsPerSec, "hands/sec")
		})
	}
}

func getSeed(i int64) int64 {
	return 12345 + i // Deterministic seeds for reproducibility
}
