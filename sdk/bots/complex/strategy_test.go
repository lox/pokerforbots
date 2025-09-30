package complex

import "testing"

func TestFoldThresholdValue(t *testing.T) {
	if got := defaultStrategy.FoldThresholdValue(StreetFlop, 0.25); got != 0.15 {
		t.Fatalf("expected flop threshold 0.15, got %0.2f", got)
	}

	if got := defaultStrategy.FoldThresholdValue(StreetTurn, 0.70); got != 0.50 {
		t.Fatalf("expected turn threshold 0.50, got %0.2f", got)
	}

	if got := defaultStrategy.FoldThresholdValue("unknown", 0.25); got != 0.50 {
		t.Fatalf("expected default threshold 0.50, got %0.2f", got)
	}
}

func TestPreflopRangeForHeadsUp(t *testing.T) {
	r := defaultStrategy.PreflopRangeFor(PositionButton, ActionOpenHeadsUp)
	if r == nil {
		t.Fatalf("expected heads-up button open range")
	}

	if !r.Contains("As", "2s") {
		t.Fatalf("expected A2s to be in heads-up button open range")
	}

	r = defaultStrategy.PreflopRangeFor(PositionCutoff, ActionDefendHeadsUp)
	if r == nil {
		t.Fatalf("expected heads-up defend range")
	}

	if !r.Contains("Kd", "2d") {
		t.Fatalf("expected K2s to be in heads-up defend range")
	}
}

func TestPreflopRangeFallbackToEarly(t *testing.T) {
	r := defaultStrategy.PreflopRangeFor(5, ActionOpen)
	if r == nil {
		t.Fatalf("expected fallback range for early position")
	}

	if !r.Contains("Ah", "Kd") {
		t.Fatalf("expected AK offsuit to be in early open range")
	}
}

func TestPostflopDecision(t *testing.T) {
	action, size := defaultStrategy.PostflopDecision("TripsPlus", true, 1.5, false)
	if action != "bet" || size != 0.50 {
		t.Fatalf("expected bet 50%%, got %s %0.2f", action, size)
	}

	action, size = defaultStrategy.PostflopDecision("TopPair", true, 10.0, true)
	if action != "check" || size != 0 {
		t.Fatalf("expected check multiway pot control, got %s %0.2f", action, size)
	}
}

func TestBetSizing(t *testing.T) {
	if size := defaultStrategy.BetSize(StreetFlop, BoardTextureDry, HandStrengthStrong); size != 0.33 {
		t.Fatalf("expected dry flop bet size 0.33, got %0.2f", size)
	}

	if size := defaultStrategy.BetSize(StreetRiver, BoardTextureAny, HandStrengthMedium); size != 0.50 {
		t.Fatalf("expected river medium strength bet size 0.50, got %0.2f", size)
	}
}
