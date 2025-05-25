package evaluator

import (
	"testing"

	"github.com/lox/holdem-cli/internal/deck"
)

func TestEvaluateRoyalFlush(t *testing.T) {
	cards := MustParseCards("AsKsQsJsTs")

	hand := EvaluateHand(cards)
	if hand.Rank != RoyalFlush {
		t.Errorf("Expected Royal Flush, got %s", hand.Rank)
	}
}

func TestEvaluateStraightFlush(t *testing.T) {
	cards := MustParseCards("9h8h7h6h5h")

	hand := EvaluateHand(cards)
	if hand.Rank != StraightFlush {
		t.Errorf("Expected Straight Flush, got %s", hand.Rank)
	}
	if hand.HighCard != deck.Nine {
		t.Errorf("Expected high card Nine, got %s", hand.HighCard)
	}
}

func TestEvaluateFourOfAKind(t *testing.T) {
	cards := MustParseCards("AsAhAdAcKs")

	hand := EvaluateHand(cards)
	if hand.Rank != FourOfAKind {
		t.Errorf("Expected Four of a Kind, got %s", hand.Rank)
	}
	if hand.Kickers[0] != deck.Ace {
		t.Errorf("Expected quad Aces, got %s", hand.Kickers[0])
	}
}

func TestEvaluateFullHouse(t *testing.T) {
	cards := MustParseCards("KsKhKdQcQs")

	hand := EvaluateHand(cards)
	if hand.Rank != FullHouse {
		t.Errorf("Expected Full House, got %s", hand.Rank)
	}
	if hand.Kickers[0] != deck.King || hand.Kickers[1] != deck.Queen {
		t.Errorf("Expected Kings full of Queens, got %s full of %s", hand.Kickers[0], hand.Kickers[1])
	}
}

func TestEvaluateFlush(t *testing.T) {
	cards := MustParseCards("AcJc9c7c5c")

	hand := EvaluateHand(cards)
	if hand.Rank != Flush {
		t.Errorf("Expected Flush, got %s", hand.Rank)
	}
	if hand.HighCard != deck.Ace {
		t.Errorf("Expected Ace high, got %s", hand.HighCard)
	}
}

func TestEvaluateStraight(t *testing.T) {
	cards := MustParseCards("Ts9h8d7c6s")

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
	cards := MustParseCards("As5h4d3c2s")

	hand := EvaluateHand(cards)
	if hand.Rank != Straight {
		t.Errorf("Expected Straight, got %s", hand.Rank)
	}
	if hand.HighCard != deck.Five {
		t.Errorf("Expected Five high wheel, got %s", hand.HighCard)
	}
}

func TestEvaluateThreeOfAKind(t *testing.T) {
	cards := MustParseCards("JsJhJd9c7s")

	hand := EvaluateHand(cards)
	if hand.Rank != ThreeOfAKind {
		t.Errorf("Expected Three of a Kind, got %s", hand.Rank)
	}
	if hand.Kickers[0] != deck.Jack {
		t.Errorf("Expected trip Jacks, got %s", hand.Kickers[0])
	}
}

func TestEvaluateTwoPair(t *testing.T) {
	cards := MustParseCards("AsAh8d8c5s")

	hand := EvaluateHand(cards)
	if hand.Rank != TwoPair {
		t.Errorf("Expected Two Pair, got %s", hand.Rank)
	}
	if hand.Kickers[0] != deck.Ace || hand.Kickers[1] != deck.Eight {
		t.Errorf("Expected Aces and Eights, got %s and %s", hand.Kickers[0], hand.Kickers[1])
	}
}

func TestEvaluateOnePair(t *testing.T) {
	cards := MustParseCards("KsKhJd9c7s")

	hand := EvaluateHand(cards)
	if hand.Rank != OnePair {
		t.Errorf("Expected One Pair, got %s", hand.Rank)
	}
	if hand.Kickers[0] != deck.King {
		t.Errorf("Expected pair of Kings, got %s", hand.Kickers[0])
	}
}

func TestEvaluateHighCard(t *testing.T) {
	cards := MustParseCards("AsJh9d7c5s")

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
	royalFlush := EvaluateHand(MustParseCards("AsKsQsJsTs"))
	straightFlush := EvaluateHand(MustParseCards("9h8h7h6h5h"))

	if !royalFlush.IsStrongerThan(straightFlush) {
		t.Error("Royal flush should beat straight flush")
	}

	// Ace-high beats King-high
	aceHigh := EvaluateHand(MustParseCards("AsJh9d7c5s"))
	kingHigh := EvaluateHand(MustParseCards("KsJh9d7c5h"))

	if !aceHigh.IsStrongerThan(kingHigh) {
		t.Error("Ace high should beat King high")
	}
}

func TestFindBestHandFrom7Cards(t *testing.T) {
	// 7 cards: A♠ A♥ K♠ K♥ Q♠ J♠ T♠
	// Best hand should be royal flush (A♠ K♠ Q♠ J♠ T♠)
	cards := MustParseCards("AsAhKsKhQsJsTs")

	hand := FindBestHand(cards)
	if hand.Rank != RoyalFlush {
		t.Errorf("Expected Royal Flush from 7 cards, got %s", hand.Rank)
	}
}
