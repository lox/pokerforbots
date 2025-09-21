package poker

import (
	"math/rand"
)

// Deck represents a standard 52-card deck
type Deck struct {
	cards [52]Card // Fixed size array
	next  int
	rng   *rand.Rand // Random source for deterministic shuffling
}

// NewDeck creates a new shuffled deck with explicit RNG
func NewDeck(rng *rand.Rand) *Deck {
	d := &Deck{
		next: 0,
		rng:  rng,
	}

	// Create all 52 cards
	i := 0
	for suit := range uint8(4) {
		for rank := range uint8(13) {
			d.cards[i] = NewCard(rank, suit)
			i++
		}
	}

	// Shuffle
	d.Shuffle()
	return d
}

// Shuffle shuffles the deck using Fisher-Yates
func (d *Deck) Shuffle() {
	d.next = 0
	for i := len(d.cards) - 1; i > 0; i-- {
		var j int
		if d.rng != nil {
			j = d.rng.Intn(i + 1)
		} else {
			j = rand.Intn(i + 1)
		}
		d.cards[i], d.cards[j] = d.cards[j], d.cards[i]
	}
}

// Deal deals n cards from the deck
func (d *Deck) Deal(n int) []Card {
	if d.next+n > len(d.cards) {
		return nil
	}
	cards := d.cards[d.next : d.next+n]
	d.next += n
	return cards
}

// DealOne deals a single card from the deck
func (d *Deck) DealOne() Card {
	if d.next >= len(d.cards) {
		return 0
	}
	card := d.cards[d.next]
	d.next++
	return card
}

// Reset resets and reshuffles the deck
func (d *Deck) Reset() {
	d.Shuffle()
}

// CardsRemaining returns the number of cards left in the deck
func (d *Deck) CardsRemaining() int {
	return len(d.cards) - d.next
}
