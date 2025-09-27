package solver

import (
	"math"

	"github.com/lox/pokerforbots/poker"
	"github.com/lox/pokerforbots/sdk/classification"
)

// BucketMapper converts raw poker states into coarse abstractions that CFR can
// operate on. The default implementation is intentionally simple yet deterministic
// so we can iterate quickly while refining the abstraction in later milestones.
type BucketMapper struct {
	config AbstractionConfig
}

// NewBucketMapper returns a mapper backed by the provided abstraction config.
func NewBucketMapper(cfg AbstractionConfig) (*BucketMapper, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &BucketMapper{config: cfg}, nil
}

// HoleBucket deterministically maps a two-card hand into a preflop bucket.
func (m *BucketMapper) HoleBucket(hand poker.Hand) int {
	if hand.CountCards() != 2 {
		return 0
	}

	c0 := hand.GetCard(0)
	c1 := hand.GetCard(1)

	r0 := int(c0.Rank())
	r1 := int(c1.Rank())
	if r0 < r1 {
		r0, r1 = r1, r0
	}
	pair := 0
	if r0 == r1 {
		pair = 1
	}
	suited := 0
	if c0.Suit() == c1.Suit() {
		suited = 1
	}

	// Map the 169 combos into a continuous space by combining rank strength,
	// pair bonus, and suitedness. The constants are chosen to keep values within
	// a comfortable range before bucketing.
	score := float64(r0*13 + r1)
	if pair == 1 {
		score += 200
	}
	if suited == 1 {
		score += 13
	}

	bucket := int(score / (312.0 / float64(m.config.PreflopBucketCount)))
	if bucket >= m.config.PreflopBucketCount {
		bucket = m.config.PreflopBucketCount - 1
	}
	if bucket < 0 {
		bucket = 0
	}
	return bucket
}

// BoardBucket maps a board texture (3-5 cards) into a coarse bucket.
func (m *BucketMapper) BoardBucket(board poker.Hand) int {
	if board == 0 {
		return 0
	}

	texture := classification.AnalyzeBoardTexture(board)
	paired := float64(countBoardPairs(board))
	highCards := float64(countHighCards(board))

	score := float64(texture)*2 + paired + highCards*0.5
	bucket := int(math.Round(score / (8.0 / float64(m.config.PostflopBucketCount))))
	if bucket >= m.config.PostflopBucketCount {
		bucket = m.config.PostflopBucketCount - 1
	}
	if bucket < 0 {
		bucket = 0
	}
	return bucket
}

// countBoardPairs is copied locally to avoid exporting from classification.
func countBoardPairs(board poker.Hand) int {
	counts := make(map[uint8]int, 5)
	for i := 0; i < board.CountCards(); i++ {
		c := board.GetCard(i)
		counts[c.Rank()]++
	}
	pairs := 0
	for _, c := range counts {
		if c >= 2 {
			pairs++
		}
	}
	return pairs
}

func countHighCards(board poker.Hand) int {
	high := 0
	for i := 0; i < board.CountCards(); i++ {
		if board.GetCard(i).Rank() >= poker.Ten {
			high++
		}
	}
	return high
}
