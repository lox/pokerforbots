package game

import (
	"math/rand"
	"testing"

	"github.com/lox/pokerforbots/poker"
)

func TestNewHand(t *testing.T) {
	t.Parallel()

	t.Run("basic construction", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		h := NewHand(rng, []string{"Alice", "Bob", "Charlie"}, 0, 5, 10)

		if len(h.Players) != 3 {
			t.Errorf("Expected 3 players, got %d", len(h.Players))
		}

		// Check default chip counts
		for i, p := range h.Players {
			if p.Chips != 990 && p.Chips != 995 { // After blinds
				if i == 0 { // Button, no blind
					if p.Chips != 1000 {
						t.Errorf("Player %d should have 1000 chips, got %d", i, p.Chips)
					}
				}
			}
		}

		if h.Button != 0 {
			t.Errorf("Button should be 0, got %d", h.Button)
		}

		if h.Street != Preflop {
			t.Errorf("Should start at Preflop, got %v", h.Street)
		}
	})

	t.Run("requires RNG", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for nil RNG")
			}
		}()
		NewHand(nil, []string{"Alice", "Bob"}, 0, 5, 10)
	})

	t.Run("requires at least 2 players", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for < 2 players")
			}
		}()
		rng := rand.New(rand.NewSource(42))
		NewHand(rng, []string{"Alice"}, 0, 5, 10)
	})

	t.Run("validates button position", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for invalid button")
			}
		}()
		rng := rand.New(rand.NewSource(42))
		NewHand(rng, []string{"Alice", "Bob"}, 5, 5, 10) // button out of range
	})
}

func TestHandOptions(t *testing.T) {
	t.Parallel()

	t.Run("WithUniformChips", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		h := NewHand(rng, []string{"Alice", "Bob", "Charlie"}, 0, 5, 10, WithUniformChips(500))

		// Check chips after blinds
		if h.Players[0].Chips != 500 { // Button, no blind
			t.Errorf("Button should have 500 chips, got %d", h.Players[0].Chips)
		}
		if h.Players[1].Chips != 495 { // Small blind
			t.Errorf("SB should have 495 chips after blind, got %d", h.Players[1].Chips)
		}
		if h.Players[2].Chips != 490 { // Big blind
			t.Errorf("BB should have 490 chips after blind, got %d", h.Players[2].Chips)
		}
	})

	t.Run("WithChips individual counts", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		chips := []int{1000, 800, 1200}
		h := NewHand(rng, []string{"Alice", "Bob", "Charlie"}, 0, 5, 10, WithChips(chips))

		if h.Players[0].Chips != 1000 { // Button, no blind
			t.Errorf("Button should have 1000 chips, got %d", h.Players[0].Chips)
		}
		if h.Players[1].Chips != 795 { // Small blind (800-5)
			t.Errorf("SB should have 795 chips after blind, got %d", h.Players[1].Chips)
		}
		if h.Players[2].Chips != 1190 { // Big blind (1200-10)
			t.Errorf("BB should have 1190 chips after blind, got %d", h.Players[2].Chips)
		}
	})

	t.Run("WithChips validates count", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for mismatched chip counts")
			}
		}()
		rng := rand.New(rand.NewSource(42))
		chips := []int{1000, 800} // Only 2, but 3 players
		NewHand(rng, []string{"Alice", "Bob", "Charlie"}, 0, 5, 10, WithChips(chips))
	})

	t.Run("WithDeck uses provided deck", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		deck := poker.NewDeck(rand.New(rand.NewSource(99))) // Different seed
		h := NewHand(rng, []string{"Alice", "Bob"}, 0, 5, 10, WithDeck(deck))

		if h.Deck != deck {
			t.Error("Should use provided deck")
		}
	})

	t.Run("multiple options compose", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		deck := poker.NewDeck(rand.New(rand.NewSource(99)))
		chips := []int{500, 600}

		h := NewHand(rng, []string{"Alice", "Bob"}, 0, 5, 10,
			WithChips(chips),
			WithDeck(deck))

		if h.Deck != deck {
			t.Error("Should use provided deck")
		}
		if h.Players[0].Chips != 495 { // Button posts SB in heads-up (500-5)
			t.Errorf("Button should have 495 chips after SB, got %d", h.Players[0].Chips)
		}
		if h.Players[1].Chips != 590 { // BB (600-10)
			t.Errorf("BB should have 590 chips after blind, got %d", h.Players[1].Chips)
		}
	})

	t.Run("WithUniformChips overrides WithChips", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		chips := []int{500, 600, 700}

		h := NewHand(rng, []string{"Alice", "Bob", "Charlie"}, 0, 5, 10,
			WithChips(chips),
			WithUniformChips(1500)) // This should win

		// All should have 1500 minus blinds
		if h.Players[0].Chips != 1500 { // Button, no blind
			t.Errorf("Button should have 1500 chips, got %d", h.Players[0].Chips)
		}
		if h.Players[1].Chips != 1495 { // Small blind
			t.Errorf("SB should have 1495 chips, got %d", h.Players[1].Chips)
		}
		if h.Players[2].Chips != 1490 { // Big blind
			t.Errorf("BB should have 1490 chips, got %d", h.Players[2].Chips)
		}
	})
}

func TestNewHandDeterministic(t *testing.T) {
	t.Parallel()

	// Two hands with same seed should be identical
	seed := int64(12345)
	players := []string{"Alice", "Bob", "Charlie"}

	rng1 := rand.New(rand.NewSource(seed))
	h1 := NewHand(rng1, players, 0, 5, 10)

	rng2 := rand.New(rand.NewSource(seed))
	h2 := NewHand(rng2, players, 0, 5, 10)

	// Check that hole cards are the same
	for i := range players {
		if h1.Players[i].HoleCards != h2.Players[i].HoleCards {
			t.Errorf("Player %d hole cards differ with same seed", i)
		}
	}
}

func TestBackwardCompatibility(t *testing.T) {
	t.Parallel()

	// Test that deprecated constructors still work
	t.Run("NewHandState", func(t *testing.T) {
		h := NewHandState([]string{"Alice", "Bob"}, 0, 5, 10, 1000)
		if h == nil {
			t.Error("NewHandState should still work")
		}
	})

	t.Run("NewHandStateWithRNG", func(t *testing.T) {
		rng := rand.New(rand.NewSource(42))
		h := NewHandStateWithRNG([]string{"Alice", "Bob"}, 0, 5, 10, 1000, rng)
		if h == nil {
			t.Error("NewHandStateWithRNG should still work")
		}
	})

	t.Run("NewHandStateWithChips", func(t *testing.T) {
		h := NewHandStateWithChips([]string{"Alice", "Bob"}, []int{1000, 800}, 0, 5, 10)
		if h == nil {
			t.Error("NewHandStateWithChips should still work")
		}
	})

	// Verify they produce similar results to new constructor
	t.Run("equivalence", func(t *testing.T) {
		seed := int64(42)
		players := []string{"Alice", "Bob"}

		// Old way
		rng1 := rand.New(rand.NewSource(seed))
		h1 := NewHandStateWithRNG(players, 0, 5, 10, 1000, rng1)

		// New way
		rng2 := rand.New(rand.NewSource(seed))
		h2 := NewHand(rng2, players, 0, 5, 10, WithUniformChips(1000))

		// Should have same chip counts after blinds
		for i := range players {
			if h1.Players[i].Chips != h2.Players[i].Chips {
				t.Errorf("Player %d chips differ: old=%d new=%d",
					i, h1.Players[i].Chips, h2.Players[i].Chips)
			}
		}
	})
}
