package evaluator

import (
	"testing"

	"github.com/lox/holdem-cli/internal/deck"
)

func TestEvaluateRoyalFlush(t *testing.T) {
	cards := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Spades, Rank: deck.King},
		{Suit: deck.Spades, Rank: deck.Queen},
		{Suit: deck.Spades, Rank: deck.Jack},
		{Suit: deck.Spades, Rank: deck.Ten},
	}
	
	hand := EvaluateHand(cards)
	if hand.Rank != RoyalFlush {
		t.Errorf("Expected Royal Flush, got %s", hand.Rank)
	}
}

func TestEvaluateStraightFlush(t *testing.T) {
	cards := []deck.Card{
		{Suit: deck.Hearts, Rank: deck.Nine},
		{Suit: deck.Hearts, Rank: deck.Eight},
		{Suit: deck.Hearts, Rank: deck.Seven},
		{Suit: deck.Hearts, Rank: deck.Six},
		{Suit: deck.Hearts, Rank: deck.Five},
	}
	
	hand := EvaluateHand(cards)
	if hand.Rank != StraightFlush {
		t.Errorf("Expected Straight Flush, got %s", hand.Rank)
	}
	if hand.HighCard != deck.Nine {
		t.Errorf("Expected high card Nine, got %s", hand.HighCard)
	}
}

func TestEvaluateFourOfAKind(t *testing.T) {
	cards := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Hearts, Rank: deck.Ace},
		{Suit: deck.Diamonds, Rank: deck.Ace},
		{Suit: deck.Clubs, Rank: deck.Ace},
		{Suit: deck.Spades, Rank: deck.King},
	}
	
	hand := EvaluateHand(cards)
	if hand.Rank != FourOfAKind {
		t.Errorf("Expected Four of a Kind, got %s", hand.Rank)
	}
	if hand.Kickers[0] != deck.Ace {
		t.Errorf("Expected quad Aces, got %s", hand.Kickers[0])
	}
}

func TestEvaluateFullHouse(t *testing.T) {
	cards := []deck.Card{
		{Suit: deck.Spades, Rank: deck.King},
		{Suit: deck.Hearts, Rank: deck.King},
		{Suit: deck.Diamonds, Rank: deck.King},
		{Suit: deck.Clubs, Rank: deck.Queen},
		{Suit: deck.Spades, Rank: deck.Queen},
	}
	
	hand := EvaluateHand(cards)
	if hand.Rank != FullHouse {
		t.Errorf("Expected Full House, got %s", hand.Rank)
	}
	if hand.Kickers[0] != deck.King || hand.Kickers[1] != deck.Queen {
		t.Errorf("Expected Kings full of Queens, got %s full of %s", hand.Kickers[0], hand.Kickers[1])
	}
}

func TestEvaluateFlush(t *testing.T) {
	cards := []deck.Card{
		{Suit: deck.Clubs, Rank: deck.Ace},
		{Suit: deck.Clubs, Rank: deck.Jack},
		{Suit: deck.Clubs, Rank: deck.Nine},
		{Suit: deck.Clubs, Rank: deck.Seven},
		{Suit: deck.Clubs, Rank: deck.Five},
	}
	
	hand := EvaluateHand(cards)
	if hand.Rank != Flush {
		t.Errorf("Expected Flush, got %s", hand.Rank)
	}
	if hand.HighCard != deck.Ace {
		t.Errorf("Expected Ace high, got %s", hand.HighCard)
	}
}

func TestEvaluateStraight(t *testing.T) {
	cards := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ten},
		{Suit: deck.Hearts, Rank: deck.Nine},
		{Suit: deck.Diamonds, Rank: deck.Eight},
		{Suit: deck.Clubs, Rank: deck.Seven},
		{Suit: deck.Spades, Rank: deck.Six},
	}
	
	hand := EvaluateHand(cards)
	if hand.Rank != Straight {
		t.Errorf("Expected Straight, got %s", hand.Rank)
	}
	if hand.HighCard != deck.Ten {
		t.Errorf("Expected Ten high straight, got %s", hand.HighCard)
	}
}

func TestEvaluateWheelStraight(t *testing.T) {
	// A-2-3-4-5 straight (wheel)
	cards := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Hearts, Rank: deck.Five},
		{Suit: deck.Diamonds, Rank: deck.Four},
		{Suit: deck.Clubs, Rank: deck.Three},
		{Suit: deck.Spades, Rank: deck.Two},
	}
	
	hand := EvaluateHand(cards)
	if hand.Rank != Straight {
		t.Errorf("Expected Straight, got %s", hand.Rank)
	}
	if hand.HighCard != deck.Five {
		t.Errorf("Expected Five high wheel, got %s", hand.HighCard)
	}
}

func TestEvaluateThreeOfAKind(t *testing.T) {
	cards := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Jack},
		{Suit: deck.Hearts, Rank: deck.Jack},
		{Suit: deck.Diamonds, Rank: deck.Jack},
		{Suit: deck.Clubs, Rank: deck.Nine},
		{Suit: deck.Spades, Rank: deck.Seven},
	}
	
	hand := EvaluateHand(cards)
	if hand.Rank != ThreeOfAKind {
		t.Errorf("Expected Three of a Kind, got %s", hand.Rank)
	}
	if hand.Kickers[0] != deck.Jack {
		t.Errorf("Expected trip Jacks, got %s", hand.Kickers[0])
	}
}

func TestEvaluateTwoPair(t *testing.T) {
	cards := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Hearts, Rank: deck.Ace},
		{Suit: deck.Diamonds, Rank: deck.Eight},
		{Suit: deck.Clubs, Rank: deck.Eight},
		{Suit: deck.Spades, Rank: deck.Five},
	}
	
	hand := EvaluateHand(cards)
	if hand.Rank != TwoPair {
		t.Errorf("Expected Two Pair, got %s", hand.Rank)
	}
	if hand.Kickers[0] != deck.Ace || hand.Kickers[1] != deck.Eight {
		t.Errorf("Expected Aces and Eights, got %s and %s", hand.Kickers[0], hand.Kickers[1])
	}
}

func TestEvaluateOnePair(t *testing.T) {
	cards := []deck.Card{
		{Suit: deck.Spades, Rank: deck.King},
		{Suit: deck.Hearts, Rank: deck.King},
		{Suit: deck.Diamonds, Rank: deck.Jack},
		{Suit: deck.Clubs, Rank: deck.Nine},
		{Suit: deck.Spades, Rank: deck.Seven},
	}
	
	hand := EvaluateHand(cards)
	if hand.Rank != OnePair {
		t.Errorf("Expected One Pair, got %s", hand.Rank)
	}
	if hand.Kickers[0] != deck.King {
		t.Errorf("Expected pair of Kings, got %s", hand.Kickers[0])
	}
}

func TestEvaluateHighCard(t *testing.T) {
	cards := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Hearts, Rank: deck.Jack},
		{Suit: deck.Diamonds, Rank: deck.Nine},
		{Suit: deck.Clubs, Rank: deck.Seven},
		{Suit: deck.Spades, Rank: deck.Five},
	}
	
	hand := EvaluateHand(cards)
	if hand.Rank != HighCard {
		t.Errorf("Expected High Card, got %s", hand.Rank)
	}
	if hand.HighCard != deck.Ace {
		t.Errorf("Expected Ace high, got %s", hand.HighCard)
	}
}

func TestHandComparison(t *testing.T) {
	// Royal flush beats straight flush
	royalFlush := EvaluateHand([]deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Spades, Rank: deck.King},
		{Suit: deck.Spades, Rank: deck.Queen},
		{Suit: deck.Spades, Rank: deck.Jack},
		{Suit: deck.Spades, Rank: deck.Ten},
	})
	
	straightFlush := EvaluateHand([]deck.Card{
		{Suit: deck.Hearts, Rank: deck.Nine},
		{Suit: deck.Hearts, Rank: deck.Eight},
		{Suit: deck.Hearts, Rank: deck.Seven},
		{Suit: deck.Hearts, Rank: deck.Six},
		{Suit: deck.Hearts, Rank: deck.Five},
	})
	
	if !royalFlush.IsStrongerThan(straightFlush) {
		t.Error("Royal flush should beat straight flush")
	}
	
	// Ace-high beats King-high
	aceHigh := EvaluateHand([]deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Hearts, Rank: deck.Jack},
		{Suit: deck.Diamonds, Rank: deck.Nine},
		{Suit: deck.Clubs, Rank: deck.Seven},
		{Suit: deck.Spades, Rank: deck.Five},
	})
	
	kingHigh := EvaluateHand([]deck.Card{
		{Suit: deck.Spades, Rank: deck.King},
		{Suit: deck.Hearts, Rank: deck.Jack},
		{Suit: deck.Diamonds, Rank: deck.Nine},
		{Suit: deck.Clubs, Rank: deck.Seven},
		{Suit: deck.Hearts, Rank: deck.Five},
	})
	
	if !aceHigh.IsStrongerThan(kingHigh) {
		t.Error("Ace high should beat King high")
	}
}

func TestFindBestHandFrom7Cards(t *testing.T) {
	// 7 cards: A♠ A♥ K♠ K♥ Q♠ J♠ T♠
	// Best hand should be royal flush (A♠ K♠ Q♠ J♠ T♠)
	cards := []deck.Card{
		{Suit: deck.Spades, Rank: deck.Ace},
		{Suit: deck.Hearts, Rank: deck.Ace},
		{Suit: deck.Spades, Rank: deck.King},
		{Suit: deck.Hearts, Rank: deck.King},
		{Suit: deck.Spades, Rank: deck.Queen},
		{Suit: deck.Spades, Rank: deck.Jack},
		{Suit: deck.Spades, Rank: deck.Ten},
	}
	
	hand := FindBestHand(cards)
	if hand.Rank != RoyalFlush {
		t.Errorf("Expected Royal Flush from 7 cards, got %s", hand.Rank)
	}
}