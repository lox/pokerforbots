package evaluator

import (
	"testing"

	"github.com/lox/holdem-cli/internal/deck"
)

func TestEvaluateRoyalFlush(t *testing.T) {
	cards := deck.MustParseCards("AsKsQsJsTs")

	hand := Evaluate5(cards)
	if hand.Rank != RoyalFlush {
		t.Errorf("Expected Royal Flush, got %s", hand.Rank)
	}
}

func TestEvaluateStraightFlush(t *testing.T) {
	cards := deck.MustParseCards("9h8h7h6h5h")

	hand := Evaluate5(cards)
	if hand.Rank != StraightFlush {
		t.Errorf("Expected Straight Flush, got %s", hand.Rank)
	}
	if hand.HighCard != deck.Nine {
		t.Errorf("Expected high card Nine, got %s", hand.HighCard)
	}
}

func TestEvaluateFourOfAKind(t *testing.T) {
	cards := deck.MustParseCards("AsAhAdAcKs")

	hand := Evaluate5(cards)
	if hand.Rank != FourOfAKind {
		t.Errorf("Expected Four of a Kind, got %s", hand.Rank)
	}
	if hand.Kickers[0] != deck.Ace {
		t.Errorf("Expected quad Aces, got %s", hand.Kickers[0])
	}
}

func TestEvaluateFullHouse(t *testing.T) {
	cards := deck.MustParseCards("KsKhKdQcQs")

	hand := Evaluate5(cards)
	if hand.Rank != FullHouse {
		t.Errorf("Expected Full House, got %s", hand.Rank)
	}
	if hand.Kickers[0] != deck.King || hand.Kickers[1] != deck.Queen {
		t.Errorf("Expected Kings full of Queens, got %s full of %s", hand.Kickers[0], hand.Kickers[1])
	}
}

func TestEvaluateFlush(t *testing.T) {
	cards := deck.MustParseCards("AcJc9c7c5c")

	hand := Evaluate5(cards)
	if hand.Rank != Flush {
		t.Errorf("Expected Flush, got %s", hand.Rank)
	}
	if hand.HighCard != deck.Ace {
		t.Errorf("Expected Ace high, got %s", hand.HighCard)
	}
}

func TestEvaluateStraight(t *testing.T) {
	cards := deck.MustParseCards("Ts9h8d7c6s")

	hand := Evaluate5(cards)
	if hand.Rank != Straight {
		t.Errorf("Expected Straight, got %s", hand.Rank)
	}
	if hand.HighCard != deck.Ten {
		t.Errorf("Expected Ten high straight, got %s", hand.HighCard)
	}
}

func TestEvaluateWheelStraight(t *testing.T) {
	// A-2-3-4-5 straight (wheel)
	cards := deck.MustParseCards("As5h4d3c2s")

	hand := Evaluate5(cards)
	if hand.Rank != Straight {
		t.Errorf("Expected Straight, got %s", hand.Rank)
	}
	if hand.HighCard != deck.Five {
		t.Errorf("Expected Five high wheel, got %s", hand.HighCard)
	}
}

func TestEvaluateThreeOfAKind(t *testing.T) {
	cards := deck.MustParseCards("JsJhJd9c7s")

	hand := Evaluate5(cards)
	if hand.Rank != ThreeOfAKind {
		t.Errorf("Expected Three of a Kind, got %s", hand.Rank)
	}
	if hand.Kickers[0] != deck.Jack {
		t.Errorf("Expected trip Jacks, got %s", hand.Kickers[0])
	}
}

func TestEvaluateTwoPair(t *testing.T) {
	cards := deck.MustParseCards("AsAh8d8c5s")

	hand := Evaluate5(cards)
	if hand.Rank != TwoPair {
		t.Errorf("Expected Two Pair, got %s", hand.Rank)
	}
	if hand.Kickers[0] != deck.Ace || hand.Kickers[1] != deck.Eight {
		t.Errorf("Expected Aces and Eights, got %s and %s", hand.Kickers[0], hand.Kickers[1])
	}
}

func TestEvaluateOnePair(t *testing.T) {
	cards := deck.MustParseCards("KsKhJd9c7s")

	hand := Evaluate5(cards)
	if hand.Rank != OnePair {
		t.Errorf("Expected One Pair, got %s", hand.Rank)
	}
	if hand.Kickers[0] != deck.King {
		t.Errorf("Expected pair of Kings, got %s", hand.Kickers[0])
	}
}

func TestEvaluateHighCard(t *testing.T) {
	cards := deck.MustParseCards("AsJh9d7c5s")

	hand := Evaluate5(cards)
	if hand.Rank != HighCard {
		t.Errorf("Expected High Card, got %s", hand.Rank)
	}
	if hand.HighCard != deck.Ace {
		t.Errorf("Expected Ace high, got %s", hand.HighCard)
	}
}

func TestHandComparison(t *testing.T) {
	// Royal flush beats straight flush
	royalFlush := Evaluate5(deck.MustParseCards("AsKsQsJsTs"))
	straightFlush := Evaluate5(deck.MustParseCards("9h8h7h6h5h"))

	if !royalFlush.IsStrongerThan(straightFlush) {
		t.Error("Royal flush should beat straight flush")
	}

	// Ace-high beats King-high
	aceHigh := Evaluate5(deck.MustParseCards("AsJh9d7c5s"))
	kingHigh := Evaluate5(deck.MustParseCards("KsJh9d7c5h"))

	if !aceHigh.IsStrongerThan(kingHigh) {
		t.Error("Ace high should beat King high")
	}
}

func TestEvaluate7From7Cards(t *testing.T) {
	// 7 cards: A♠ A♥ K♠ K♥ Q♠ J♠ T♠
	// Best hand should be royal flush (A♠ K♠ Q♠ J♠ T♠)
	cards := deck.MustParseCards("AsAhKsKhQsJsTs")

	hand := Evaluate7(cards)
	if hand.Category != RoyalFlush {
		t.Errorf("Expected Royal Flush from 7 cards, got %s", hand.String())
	}
}

// Edge case tests
func TestWheelStraightFlush(t *testing.T) {
	// A-2-3-4-5 straight flush (wheel straight flush)
	cards := deck.MustParseCards("As5s4s3s2s")

	hand := Evaluate5(cards)
	if hand.Rank != StraightFlush {
		t.Errorf("Expected Straight Flush for wheel, got %s", hand.Rank)
	}
	if hand.HighCard != deck.Five {
		t.Errorf("Expected Five high wheel straight flush, got %s high", hand.HighCard)
	}
}

func TestBroadwayStraight(t *testing.T) {
	// A-K-Q-J-T straight (broadway)
	cards := deck.MustParseCards("AsKhQdJcTs")

	hand := Evaluate5(cards)
	if hand.Rank != Straight {
		t.Errorf("Expected Straight for broadway, got %s", hand.Rank)
	}
	if hand.HighCard != deck.Ace {
		t.Errorf("Expected Ace high broadway straight, got %s high", hand.HighCard)
	}
}

func TestFullHouseWithMultipleTrips(t *testing.T) {
	// AAAKKKQ in 7 cards - should be Aces full of Kings
	cards := deck.MustParseCards("AsAhAdKsKhKdQc")

	hand := Evaluate7(cards)
	if hand.Category != FullHouse {
		t.Errorf("Expected Full House, got %s", hand.String())
	}
	// Should pick Aces full of Kings (higher trips)
	if len(hand.Tiebreak) < 2 || deck.Rank(hand.Tiebreak[0]) != deck.Ace || deck.Rank(hand.Tiebreak[1]) != deck.King {
		t.Errorf("Expected Aces full of Kings, got tiebreak %v", hand.Tiebreak)
	}
}

func TestTwoPairKickers(t *testing.T) {
	// Test that two pair selects correct kicker
	cards := deck.MustParseCards("AsAhKsKhQc")

	hand := Evaluate5(cards)
	if hand.Rank != TwoPair {
		t.Errorf("Expected Two Pair, got %s", hand.Rank)
	}
	// Should be Aces and Kings with Queen kicker
	if len(hand.Kickers) < 3 || hand.Kickers[0] != deck.Ace || hand.Kickers[1] != deck.King || hand.Kickers[2] != deck.Queen {
		t.Errorf("Expected Aces and Kings with Queen kicker, got %v", hand.Kickers)
	}
}

func TestFlushKickerOrdering(t *testing.T) {
	// Test that flush kickers are properly ordered (descending)
	cards := deck.MustParseCards("Ac9c7c5c3c")

	hand := Evaluate5(cards)
	if hand.Rank != Flush {
		t.Errorf("Expected Flush, got %s", hand.Rank)
	}
	// Kickers should be in descending order: A, 9, 7, 5, 3
	expected := []deck.Rank{deck.Ace, deck.Nine, deck.Seven, deck.Five, deck.Three}
	if len(hand.Kickers) != len(expected) {
		t.Errorf("Expected %d kickers, got %d", len(expected), len(hand.Kickers))
	}
	for i, exp := range expected {
		if i >= len(hand.Kickers) || hand.Kickers[i] != exp {
			t.Errorf("Expected kicker %d to be %s, got %s", i, exp, hand.Kickers[i])
		}
	}
}

func TestHandTies(t *testing.T) {
	// Test identical hands tie
	hand1 := Evaluate5(deck.MustParseCards("AsKhQdJcTs"))
	hand2 := Evaluate5(deck.MustParseCards("AdKsQhJsTc"))

	if !hand1.Equals(hand2) {
		t.Error("Identical straights should tie")
	}
	if hand1.Compare(hand2) != 0 {
		t.Error("Identical hands should have Compare result of 0")
	}
}

func TestKickerComparisons(t *testing.T) {
	// Test same hand type, different kickers
	pairAcesKingKicker := Evaluate5(deck.MustParseCards("AsAhKdQcJs"))
	pairAcesQueenKicker := Evaluate5(deck.MustParseCards("AdAcQhJsTs"))

	if !pairAcesKingKicker.IsStrongerThan(pairAcesQueenKicker) {
		t.Error("Pair of Aces with King kicker should beat pair of Aces with Queen kicker")
	}
}

func TestEvaluate5PanicOnWrongCards(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Evaluate5 should panic with wrong number of cards")
		}
	}()

	// Should panic with 4 cards
	_ = Evaluate5(deck.MustParseCards("AsKhQdJc"))
}

func TestEvaluate7PanicOnWrongCards(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Evaluate7 should panic with wrong number of cards")
		}
	}()

	// Should panic with 6 cards
	_ = Evaluate7(deck.MustParseCards("AsKhQdJcTsJs"))
}

func TestHandStrengthVsHandConsistency(t *testing.T) {
	// Test that Evaluate7 and Evaluate5+conversion give equivalent results for 5 cards
	cards := deck.MustParseCards("AsKsQsJsTs")
	
	// Extend to 7 cards for Evaluate7
	cards7 := append(cards, deck.MustParseCards("2h3d")...)
	
	hand5 := Evaluate5(cards)
	strength7 := Evaluate7(cards7)
	strengthFromHand := HandToHandStrength(hand5)

	if strengthFromHand.Category != strength7.Category {
		t.Errorf("Category mismatch: Evaluate5 conversion got %s, Evaluate7 got %s", 
			strengthFromHand.Category, strength7.Category)
	}
}

func TestSuitIndependence(t *testing.T) {
	// Same hand different suits should be equal
	spadeFlush := Evaluate5(deck.MustParseCards("AsKsQsJsTs"))
	heartFlush := Evaluate5(deck.MustParseCards("AhKhQhJhTh"))

	if !spadeFlush.Equals(heartFlush) {
		t.Error("Royal flushes of different suits should be equal")
	}
}
