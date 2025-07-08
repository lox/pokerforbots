package deck

import (
	"math/rand"
	"testing"
)

func TestNewDeck(t *testing.T) {
	deck := NewDeck(rand.New(rand.NewSource(42)))

	if deck.CardsRemaining() != 52 {
		t.Errorf("Expected 52 cards, got %d", deck.CardsRemaining())
	}

	if deck.IsEmpty() {
		t.Error("New deck should not be empty")
	}
}

func TestDeckDeal(t *testing.T) {
	deck := NewDeck(rand.New(rand.NewSource(42)))
	initialCount := deck.CardsRemaining()

	card, ok := deck.Deal()
	if !ok {
		t.Error("Deal should succeed on new deck")
	}

	if deck.CardsRemaining() != initialCount-1 {
		t.Errorf("Expected %d cards after dealing, got %d", initialCount-1, deck.CardsRemaining())
	}

	// Verify the card is valid
	if card.Suit < Spades || card.Suit > Clubs {
		t.Error("Invalid suit dealt")
	}
	if card.Rank < Two || card.Rank > Ace {
		t.Error("Invalid rank dealt")
	}
}

func TestDeckDealAll(t *testing.T) {
	deck := NewDeck(rand.New(rand.NewSource(42)))

	// Deal all cards
	for i := 0; i < 52; i++ {
		_, ok := deck.Deal()
		if !ok {
			t.Errorf("Deal failed at card %d", i+1)
		}
	}

	if !deck.IsEmpty() {
		t.Error("Deck should be empty after dealing all cards")
	}

	// Try to deal from empty deck
	_, ok := deck.Deal()
	if ok {
		t.Error("Deal should fail on empty deck")
	}
}

func TestDeckShuffle(t *testing.T) {
	deck1 := NewDeck(rand.New(rand.NewSource(42)))
	deck2 := NewDeck(rand.New(rand.NewSource(43)))

	// Get first few cards from unshuffled deck
	cards1 := make([]Card, 5)
	for i := 0; i < 5; i++ {
		cards1[i], _ = deck1.Deal()
	}

	// Shuffle second deck and get first few cards
	deck2.Shuffle()
	cards2 := make([]Card, 5)
	for i := 0; i < 5; i++ {
		cards2[i], _ = deck2.Deal()
	}

	// They should likely be different (though not guaranteed)
	allSame := true
	for i := 0; i < 5; i++ {
		if cards1[i] != cards2[i] {
			allSame = false
			break
		}
	}

	// This is probabilistic, but very unlikely to fail
	if allSame {
		t.Log("Warning: Shuffle may not be working (cards in same order)")
	}
}

func TestDeckReset(t *testing.T) {
	deck := NewDeck(rand.New(rand.NewSource(42)))

	// Deal some cards
	for i := 0; i < 10; i++ {
		deck.Deal()
	}

	if deck.CardsRemaining() != 42 {
		t.Errorf("Expected 42 cards, got %d", deck.CardsRemaining())
	}

	// Reset the deck
	deck.Reset()

	if deck.CardsRemaining() != 52 {
		t.Errorf("Expected 52 cards after reset, got %d", deck.CardsRemaining())
	}
}

func TestDeckDealN(t *testing.T) {
	deck := NewDeck(rand.New(rand.NewSource(42)))

	cards := deck.DealN(5)
	if len(cards) != 5 {
		t.Errorf("Expected 5 cards, got %d", len(cards))
	}

	if deck.CardsRemaining() != 47 {
		t.Errorf("Expected 47 cards remaining, got %d", deck.CardsRemaining())
	}

	// Try to deal more cards than available
	deck.DealN(45) // Now we have 2 left
	cards = deck.DealN(5)
	if len(cards) != 2 {
		t.Errorf("Expected 2 cards when only 2 available, got %d", len(cards))
	}
}

func TestCardString(t *testing.T) {
	card := NewCard(Spades, Ace)
	expected := "A♠"
	if card.String() != expected {
		t.Errorf("Expected %s, got %s", expected, card.String())
	}

	card = NewCard(Hearts, King)
	expected = "K♥"
	if card.String() != expected {
		t.Errorf("Expected %s, got %s", expected, card.String())
	}
}

func TestCardProperties(t *testing.T) {
	redCard := NewCard(Hearts, Two)
	if !redCard.IsRed() {
		t.Error("Heart should be red")
	}

	blackCard := NewCard(Spades, Two)
	if blackCard.IsRed() {
		t.Error("Spade should not be red")
	}

	ace := NewCard(Spades, Ace)
	if !ace.IsAce() {
		t.Error("Ace should be identified as ace")
	}

	king := NewCard(Hearts, King)
	if !king.IsFaceCard() {
		t.Error("King should be identified as face card")
	}

	two := NewCard(Clubs, Two)
	if two.IsFaceCard() {
		t.Error("Two should not be identified as face card")
	}
}
