package evaluator

import (
	"testing"
)

func TestRoyalFlush(t *testing.T) {
	e := NewEvaluator()

	// Royal flush in spades + 2 random cards
	royalFlush := Hand(MakeCard(Spades, 12) | MakeCard(Spades, 11) | MakeCard(Spades, 10) |
		MakeCard(Spades, 9) | MakeCard(Spades, 8) |
		MakeCard(Clubs, 0) | MakeCard(Diamonds, 1))

	rank := e.EvaluateHand(royalFlush)
	if rank != 0 {
		t.Errorf("Royal flush should have rank 0, got %d", rank)
	}

	category := e.GetHandCategory(royalFlush)
	if category != HandCategoryStraightFlush {
		t.Errorf("Royal flush should be categorized as straight flush, got %d", category)
	}
}

func TestRoyalFlushClubsOnly(t *testing.T) {
	e := NewEvaluator()

	// Royal flush in clubs (5 cards only)
	royalClubs := Hand(0x1F00) // A-K-Q-J-T clubs only

	rank := e.EvaluateHand(royalClubs)
	if rank != 0 {
		t.Errorf("Royal flush should have rank 0, got %d", rank)
	}
}

func TestStraightFlush(t *testing.T) {
	e := NewEvaluator()

	// 9-high straight flush in hearts + 2 random cards
	straightFlush := Hand(MakeCard(Hearts, 7) | MakeCard(Hearts, 6) | MakeCard(Hearts, 5) |
		MakeCard(Hearts, 4) | MakeCard(Hearts, 3) |
		MakeCard(Clubs, 0) | MakeCard(Diamonds, 1))

	rank := e.EvaluateHand(straightFlush)
	if rank < 1 || rank > 9 {
		t.Errorf("Straight flush should have rank 1-9, got %d", rank)
	}
}

func TestWheelStraightFlush(t *testing.T) {
	e := NewEvaluator()

	// Wheel straight flush: A,2,3,4,5 all clubs + 2 off-suit cards
	wheelSF := Hand(MakeCard(Clubs, 12) | MakeCard(Clubs, 0) | MakeCard(Clubs, 1) |
		MakeCard(Clubs, 2) | MakeCard(Clubs, 3) |
		MakeCard(Diamonds, 10) | MakeCard(Hearts, 8))

	rank := e.EvaluateHand(wheelSF)
	if rank != 9 {
		t.Errorf("Wheel straight flush should have rank 9, got %d", rank)
	}
}

func TestFourOfAKind(t *testing.T) {
	e := NewEvaluator()

	// Four aces + King kicker
	fourAces := Hand(MakeCard(Clubs, 12) | MakeCard(Diamonds, 12) | MakeCard(Hearts, 12) | MakeCard(Spades, 12) |
		MakeCard(Clubs, 11) | MakeCard(Diamonds, 10) | MakeCard(Hearts, 9))

	rank := e.EvaluateHand(fourAces)
	if rank < 10 || rank > 165 {
		t.Errorf("Four of a kind should have rank 10-165, got %d", rank)
	}

	category := e.GetHandCategory(fourAces)
	if category != HandCategoryFourOfAKind {
		t.Errorf("Four of a kind should be categorized correctly, got %d", category)
	}
}

func TestFullHouse(t *testing.T) {
	e := NewEvaluator()

	// Queens full of Jacks
	fullHouse := Hand(MakeCard(Clubs, 10) | MakeCard(Diamonds, 10) | MakeCard(Hearts, 10) |
		MakeCard(Clubs, 9) | MakeCard(Diamonds, 9) |
		MakeCard(Hearts, 7) | MakeCard(Spades, 6))

	rank := e.EvaluateHand(fullHouse)
	if rank < 166 || rank > 321 {
		t.Errorf("Full house should have rank 166-321, got %d", rank)
	}

	category := e.GetHandCategory(fullHouse)
	if category != HandCategoryFullHouse {
		t.Errorf("Full house should be categorized correctly, got %d", category)
	}
}

func TestTwoTripsFullHouse(t *testing.T) {
	e := NewEvaluator()

	// AAAKKK7 - two trips should be a full house
	hand := Hand(MakeCard(Clubs, 12) | MakeCard(Diamonds, 12) | MakeCard(Hearts, 12) |
		MakeCard(Clubs, 11) | MakeCard(Diamonds, 11) | MakeCard(Hearts, 11) |
		MakeCard(Clubs, 5))

	rank := e.EvaluateHand(hand)
	if rank < 166 || rank > 321 {
		t.Errorf("Two trips should make a full house, rank 166-321, got %d", rank)
	}

	// Should be AAAKK which is rank 167 (Aces over Kings)
	if rank != 167 {
		t.Errorf("AAAKK should have rank 167, got %d", rank)
	}
}

func TestFlush(t *testing.T) {
	e := NewEvaluator()

	// Ace-high flush in hearts
	flush := Hand(MakeCard(Hearts, 12) | MakeCard(Hearts, 10) | MakeCard(Hearts, 8) |
		MakeCard(Hearts, 6) | MakeCard(Hearts, 4) |
		MakeCard(Clubs, 11) | MakeCard(Diamonds, 9))

	rank := e.EvaluateHand(flush)
	if rank < 322 || rank > 1598 {
		t.Errorf("Flush should have rank 322-1598, got %d", rank)
	}

	category := e.GetHandCategory(flush)
	if category != HandCategoryFlush {
		t.Errorf("Flush should be categorized correctly, got %d", category)
	}
}

func TestStraight(t *testing.T) {
	e := NewEvaluator()

	// 9-high straight
	straight := Hand(MakeCard(Clubs, 7) | MakeCard(Diamonds, 6) | MakeCard(Hearts, 5) |
		MakeCard(Spades, 4) | MakeCard(Clubs, 3) |
		MakeCard(Diamonds, 1) | MakeCard(Hearts, 0))

	rank := e.EvaluateHand(straight)
	if rank < 1599 || rank > 1608 {
		t.Errorf("Straight should have rank 1599-1608, got %d", rank)
	}

	category := e.GetHandCategory(straight)
	if category != HandCategoryStraight {
		t.Errorf("Straight should be categorized correctly, got %d", category)
	}
}

func TestWheelStraight(t *testing.T) {
	e := NewEvaluator()

	// A-2-3-4-5 wheel straight
	wheel := Hand(MakeCard(Clubs, 12) | MakeCard(Diamonds, 3) | MakeCard(Hearts, 2) |
		MakeCard(Spades, 1) | MakeCard(Clubs, 0) |
		MakeCard(Diamonds, 11) | MakeCard(Hearts, 10))

	rank := e.EvaluateHand(wheel)
	if rank != 1608 {
		t.Errorf("Wheel straight should have rank 1608, got %d", rank)
	}
}

func TestThreeOfAKind(t *testing.T) {
	e := NewEvaluator()

	// Three Kings
	threeKings := Hand(MakeCard(Clubs, 11) | MakeCard(Diamonds, 11) | MakeCard(Hearts, 11) |
		MakeCard(Clubs, 9) | MakeCard(Diamonds, 7) | MakeCard(Hearts, 5) | MakeCard(Spades, 2))

	rank := e.EvaluateHand(threeKings)
	if rank < 1609 || rank > 2466 {
		t.Errorf("Three of a kind should have rank 1609-2466, got %d", rank)
	}

	category := e.GetHandCategory(threeKings)
	if category != HandCategoryThreeOfAKind {
		t.Errorf("Three of a kind should be categorized correctly, got %d", category)
	}
}

func TestTwoPair(t *testing.T) {
	e := NewEvaluator()

	// Queens and Tens
	twoPair := Hand(MakeCard(Clubs, 10) | MakeCard(Diamonds, 10) |
		MakeCard(Hearts, 8) | MakeCard(Spades, 8) |
		MakeCard(Clubs, 6) | MakeCard(Diamonds, 4) | MakeCard(Hearts, 2))

	rank := e.EvaluateHand(twoPair)
	if rank < 2467 || rank > 3324 {
		t.Errorf("Two pair should have rank 2467-3324, got %d", rank)
	}

	category := e.GetHandCategory(twoPair)
	if category != HandCategoryTwoPair {
		t.Errorf("Two pair should be categorized correctly, got %d", category)
	}
}

func TestOnePair(t *testing.T) {
	e := NewEvaluator()

	// Pair of Queens
	onePair := Hand(MakeCard(Clubs, 10) | MakeCard(Diamonds, 10) |
		MakeCard(Hearts, 8) | MakeCard(Spades, 6) |
		MakeCard(Clubs, 4) | MakeCard(Diamonds, 2) | MakeCard(Hearts, 0))

	rank := e.EvaluateHand(onePair)
	if rank < 3325 || rank > 6184 {
		t.Errorf("One pair should have rank 3325-6184, got %d", rank)
	}

	category := e.GetHandCategory(onePair)
	if category != HandCategoryPair {
		t.Errorf("One pair should be categorized correctly, got %d", category)
	}
}

func TestHighCard(t *testing.T) {
	e := NewEvaluator()

	// Ace high
	highCard := Hand(MakeCard(Clubs, 12) | MakeCard(Diamonds, 10) | MakeCard(Hearts, 8) |
		MakeCard(Spades, 6) | MakeCard(Clubs, 4) | MakeCard(Diamonds, 2) | MakeCard(Hearts, 0))

	rank := e.EvaluateHand(highCard)
	if rank < 6185 || rank > 7461 {
		t.Errorf("High card should have rank 6185-7461, got %d", rank)
	}

	category := e.GetHandCategory(highCard)
	if category != HandCategoryHighCard {
		t.Errorf("High card should be categorized correctly, got %d", category)
	}
}

func TestCardUtilities(t *testing.T) {
	// Test MakeCard
	aceSpades := MakeCard(Spades, 12)
	expectedAS := Card(1) << (3*13 + 12)
	if aceSpades != expectedAS {
		t.Errorf("Ace of spades incorrect: got %064b, want %064b", aceSpades, expectedAS)
	}

	// Test getRankMask
	hand := MakeCard(Clubs, 12) | MakeCard(Diamonds, 10) | MakeCard(Hearts, 8)
	rankMask := getRankMask(Hand(hand))

	if (rankMask & (1 << 12)) == 0 {
		t.Error("Rank mask should have Ace bit set")
	}
	if (rankMask & (1 << 10)) == 0 {
		t.Error("Rank mask should have Queen bit set")
	}
	if (rankMask & (1 << 8)) == 0 {
		t.Error("Rank mask should have Ten bit set")
	}
}

func TestFlushDetection(t *testing.T) {
	// Flush hand
	flushHand := Hand(MakeCard(Hearts, 12) | MakeCard(Hearts, 10) | MakeCard(Hearts, 8) |
		MakeCard(Hearts, 6) | MakeCard(Hearts, 4))

	if !hasFlush(flushHand) {
		t.Error("Should detect flush")
	}

	// Non-flush hand
	noFlush := Hand(MakeCard(Clubs, 12) | MakeCard(Diamonds, 10) | MakeCard(Hearts, 8) | MakeCard(Spades, 6))

	if hasFlush(noFlush) {
		t.Error("Should not detect flush")
	}
}

func TestOverlappingStraights(t *testing.T) {
	e := NewEvaluator()

	// Hand with two possible straights: 7-6-5-4-3 AND 6-5-4-3-2
	// Should return the higher straight: 7-6-5-4-3
	hand := Hand(0x1F8000000008) // spades 2,3,4,5,6,7 + clubs 5

	rank := e.EvaluateHand(hand)

	// Should be a straight flush (spades)
	if rank < 1 || rank > 9 {
		t.Errorf("Should be a straight flush, got rank %d", rank)
	}

	// Verify it detected the flush
	if !hasFlush(hand) {
		t.Error("Should detect flush in spades")
	}
}

func TestGetHandClass(t *testing.T) {
	e := NewEvaluator()

	tests := []struct {
		rank     int
		expected string
	}{
		{0, "Royal Flush"},
		{5, "Straight Flush"},
		{50, "Four of a Kind"},
		{200, "Full House"},
		{500, "Flush"},
		{1600, "Straight"},
		{2000, "Three of a Kind"},
		{3000, "Two Pair"},
		{5000, "One Pair"},
		{7000, "High Card"},
	}

	for _, tt := range tests {
		result := e.GetHandClass(tt.rank)
		if result != tt.expected {
			t.Errorf("GetHandClass(%d) = %s, want %s", tt.rank, result, tt.expected)
		}
	}
}

func TestEvaluateCards(t *testing.T) {
	e := NewEvaluator()

	// Test with a simple pair
	cards := []Card{
		MakeCard(Clubs, 10),
		MakeCard(Diamonds, 10),
		MakeCard(Hearts, 8),
		MakeCard(Spades, 6),
		MakeCard(Clubs, 4),
		MakeCard(Diamonds, 2),
		MakeCard(Hearts, 0),
	}

	rank := e.EvaluateCards(cards)
	if rank < 3325 || rank > 6184 {
		t.Errorf("Pair should have rank 3325-6184, got %d", rank)
	}

	description := e.DescribeHand(cards)
	if description != "One Pair" {
		t.Errorf("DescribeHand should return 'One Pair', got %s", description)
	}
}

func TestGetPercentile(t *testing.T) {
	e := NewEvaluator()

	// Royal flush should be 100th percentile
	p1 := e.GetPercentile(0)
	if p1 != 100.0 {
		t.Errorf("Royal flush should be 100th percentile, got %.2f", p1)
	}

	// Worst hand should be 0th percentile
	p2 := e.GetPercentile(7461)
	if p2 != 0.0 {
		t.Errorf("Worst hand should be 0th percentile, got %.2f", p2)
	}

	// Middle rank should be around 50th percentile
	p3 := e.GetPercentile(3730)
	if p3 < 49 || p3 > 51 {
		t.Errorf("Middle rank should be around 50th percentile, got %.2f", p3)
	}
}
