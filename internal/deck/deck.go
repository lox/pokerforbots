package deck

import (
	"math/rand"
	"time"
)

// Deck represents a deck of playing cards
type Deck struct {
	cards []Card
	rng   *rand.Rand
}

// NewDeck creates a new standard 52-card deck
func NewDeck() *Deck {
	deck := &Deck{
		cards: make([]Card, 0, 52),
		rng:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	
	// Create all 52 cards
	for suit := Spades; suit <= Clubs; suit++ {
		for rank := Two; rank <= Ace; rank++ {
			deck.cards = append(deck.cards, NewCard(suit, rank))
		}
	}
	
	return deck
}

// Shuffle randomizes the order of cards in the deck
func (d *Deck) Shuffle() {
	for i := len(d.cards) - 1; i > 0; i-- {
		j := d.rng.Intn(i + 1)
		d.cards[i], d.cards[j] = d.cards[j], d.cards[i]
	}
}

// Deal removes and returns the top card from the deck
func (d *Deck) Deal() (Card, bool) {
	if len(d.cards) == 0 {
		return Card{}, false
	}
	
	card := d.cards[0]
	d.cards = d.cards[1:]
	return card, true
}

// DealN deals n cards from the deck
func (d *Deck) DealN(n int) []Card {
	if n > len(d.cards) {
		n = len(d.cards)
	}
	
	cards := make([]Card, n)
	for i := 0; i < n; i++ {
		if card, ok := d.Deal(); ok {
			cards[i] = card
		}
	}
	
	return cards
}

// CardsRemaining returns the number of cards left in the deck
func (d *Deck) CardsRemaining() int {
	return len(d.cards)
}

// IsEmpty returns true if the deck has no cards left
func (d *Deck) IsEmpty() bool {
	return len(d.cards) == 0
}

// Reset restores the deck to a full 52-card deck and shuffles it
func (d *Deck) Reset() {
	d.cards = d.cards[:0] // Clear the slice but keep capacity
	
	// Recreate all 52 cards
	for suit := Spades; suit <= Clubs; suit++ {
		for rank := Two; rank <= Ace; rank++ {
			d.cards = append(d.cards, NewCard(suit, rank))
		}
	}
	
	d.Shuffle()
}

// Peek returns the top card without removing it from the deck
func (d *Deck) Peek() (Card, bool) {
	if len(d.cards) == 0 {
		return Card{}, false
	}
	return d.cards[0], true
}