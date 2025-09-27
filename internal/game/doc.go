// Package game implements the core poker game logic for Texas Hold'em.
//
// The main type is HandState, which manages the state of a single poker hand
// including players, betting rounds, pot management, and winner determination.
//
// # Basic Usage
//
// Create and run a simple hand with required RNG:
//
//	rng := randutil.New(time.Now().UnixNano()) // Random
//	h := game.NewHandState(rng, []string{"Alice", "Bob", "Charlie"}, 0, 5, 10)
//	// Process actions...
//	h.ProcessAction(game.Call, 0)
//	// Check if hand is complete
//	if h.IsComplete() {
//	    winners := h.GetWinners()
//	}
//
// # Deterministic Testing
//
// For deterministic testing, provide a seeded RNG:
//
//	rng := randutil.New(42) // Fixed seed
//	h := game.NewHandState(rng, players, button, sb, bb)
//
// # Configuration Options
//
// Use options to customize hand creation:
//
//	// With individual chip counts
//	h := game.NewHandState(rng, players, button, sb, bb,
//	    game.WithChipsByPlayer([]int{1000, 800, 1200}))
//
//	// With uniform chip counts
//	h := game.NewHandState(rng, players, button, sb, bb,
//	    game.WithChips(500))
//
//	// With pre-shuffled deck
//	deck := poker.NewDeck(rng)
//	h := game.NewHandState(rng, players, button, sb, bb,
//	    game.WithDeck(deck))
//
// # Architecture
//
// HandState delegates responsibilities to specialized components:
//   - BettingRound: Manages betting logic and action validation
//   - PotManager: Handles pot collection and side pot calculations
//   - poker.Deck: Provides shuffled cards with optional RNG injection
//   - poker.Evaluate7Cards: Determines hand rankings and winners
//
// The design follows a stateless-per-hand approach where each hand is
// independent, supporting high-performance concurrent execution.
package game
