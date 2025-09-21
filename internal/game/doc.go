// Package game implements the core poker game logic for Texas Hold'em.
//
// The main type is HandState, which manages the state of a single poker hand
// including players, betting rounds, pot management, and winner determination.
//
// # Basic Usage
//
// Create and run a simple hand:
//
//	h := game.NewHandState([]string{"Alice", "Bob", "Charlie"}, 0, 5, 10, 1000)
//	// Process actions...
//	h.ProcessAction(game.Call, 0)
//	// Check if hand is complete
//	if h.IsComplete() {
//	    winners := h.GetWinners()
//	}
//
// # Deterministic Testing
//
// For deterministic testing, use the RNG injection variants which accept
// a *rand.Rand parameter:
//
//	rng := rand.New(rand.NewSource(42)) // Fixed seed
//	h := game.NewHandStateWithRNG(players, button, sb, bb, chips, rng)
//
// Or with individual chip counts:
//
//	chipCounts := []int{1000, 800, 1200}
//	h := game.NewHandStateWithChipsAndRNG(players, chipCounts, button, sb, bb, rng)
//
// You can also provide a pre-shuffled deck for complete control:
//
//	deck := poker.NewDeck(rng)
//	h := game.NewHandStateWithDeck(players, button, sb, bb, chips, deck)
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
