package solver

import (
	"testing"

	"github.com/lox/pokerforbots/poker"
)

func mustHand(t *testing.T, cards ...string) poker.Hand {
	t.Helper()
	hand, err := poker.ParseHand(cards...)
	if err != nil {
		t.Fatalf("parse hand %v: %v", cards, err)
	}
	return hand
}

func TestBucketMapperHoleBucketOrdering(t *testing.T) {
	cfg := DefaultAbstraction()
	mapper, err := NewBucketMapper(cfg)
	if err != nil {
		t.Fatalf("new mapper: %v", err)
	}

	aa := mapper.HoleBucket(mustHand(t, "As", "Ad"))
	if aa != cfg.PreflopBucketCount-1 {
		t.Fatalf("expected AA to occupy top bucket %d, got %d", cfg.PreflopBucketCount-1, aa)
	}

	suited := mapper.HoleBucket(mustHand(t, "Ks", "Qs"))
	offsuit := mapper.HoleBucket(mustHand(t, "Ks", "Qd"))
	if suited <= offsuit {
		t.Fatalf("expected suited bucket > offsuit: got %d <= %d", suited, offsuit)
	}

	j9s := mapper.HoleBucket(mustHand(t, "Js", "9s"))
	sevenTwo := mapper.HoleBucket(mustHand(t, "7c", "2d"))
	if j9s <= sevenTwo {
		t.Fatalf("expected J9s bucket > 72o: got %d <= %d", j9s, sevenTwo)
	}

	reverse := mapper.HoleBucket(mustHand(t, "Qd", "Ks"))
	if reverse != offsuit {
		t.Fatalf("hole bucket should ignore card order: expected %d, got %d", offsuit, reverse)
	}

	empty := mapper.HoleBucket(0)
	if empty != 0 {
		t.Fatalf("expected empty hand bucket 0, got %d", empty)
	}
}

func TestBucketMapperBoardBucketTexture(t *testing.T) {
	cfg := DefaultAbstraction()
	mapper, err := NewBucketMapper(cfg)
	if err != nil {
		t.Fatalf("new mapper: %v", err)
	}

	dry := mapper.BoardBucket(mustHand(t, "2c", "7d", "Kc"))
	wet := mapper.BoardBucket(mustHand(t, "Th", "Jh", "Qh"))
	if wet <= dry {
		t.Fatalf("expected wet board bucket > dry board bucket: got wet=%d dry=%d", wet, dry)
	}

	incomplete := mapper.BoardBucket(mustHand(t, "As", "Kd"))
	if incomplete < 0 {
		t.Fatalf("expected non-negative bucket for partial board, got %d", incomplete)
	}

	monotone := mapper.BoardBucket(mustHand(t, "Ah", "Kh", "Qh", "Jh", "Th"))
	if monotone < wet {
		t.Fatalf("expected monotone straight flush board bucket >= wet bucket: got %d < %d", monotone, wet)
	}

	if zero := mapper.BoardBucket(0); zero != 0 {
		t.Fatalf("expected empty board to map to 0, got %d", zero)
	}
}
