package phh_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/lox/pokerforbots/v2/internal/phh"
)

func TestNormalizeCard(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"10h", "Th"},
		{"10H", "Th"},
		{"ah", "Ah"},
		{"As", "As"},
		{"??", "??"},
		{"", ""},
	}

	for _, tt := range tests {
		if got := phh.NormalizeCard(tt.in); got != tt.want {
			t.Fatalf("NormalizeCard(%q)=%q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatAction(t *testing.T) {
	tests := []struct {
		name      string
		seat      int
		action    string
		totalBet  int
		want      string
		shouldUse bool
	}{
		{"fold", 0, "fold", 0, "p1 f", true},
		{"timeout", 2, "timeout_fold", 0, "p3 f", true},
		{"check", 1, "check", 0, "p2 cc", true},
		{"call", 3, "call", 50, "p4 cc", true},
		{"raise", 0, "raise", 120, "p1 cbr 120", true},
		{"bet", 1, "bet", 40, "p2 cbr 40", true},
		{"zero bet", 2, "raise", 0, "", false},
		{"allin", 0, "allin", 350, "p1 cbr 350", true},
		{"post sb", 0, "post_small_blind", 5, "", false},
		{"post bb", 1, "post_big_blind", 10, "", false},
		{"unknown", 2, "weird", 10, "# p3 weird 10", true},
	}

	for _, tt := range tests {
		got, ok := phh.FormatAction(tt.seat, tt.action, tt.totalBet)
		if ok != tt.shouldUse {
			t.Fatalf("%s: ok=%v want %v", tt.name, ok, tt.shouldUse)
		}
		if got != tt.want {
			t.Fatalf("%s: got %q want %q", tt.name, got, tt.want)
		}
	}
}

func TestEncodeHandHistory(t *testing.T) {
	hand := &phh.HandHistory{
		Variant:           "NT",
		Table:             "default",
		SeatCount:         3,
		Seats:             []int{1, 2, 3},
		Antes:             []int{0, 0, 0},
		BlindsOrStraddles: []int{1, 2, 0},
		MinBet:            2,
		StartingStacks:    []int{200, 200, 200},
		FinishingStacks:   []int{200, 200, 200},
		Winnings:          []int{0, 0, 0},
		Actions: []string{
			"d dh p1 AhKh",
			"d dh p2 7c2d",
			"d dh p3 QsJs",
			"p1 cbr 6",
			"p2 f",
			"p3 cc",
		},
		Players:   []string{"alice-bot", "bob-bot", "charlie-bot"},
		HandID:    "hand-00042",
		Time:      "15:22:00",
		TimeZone:  "UTC",
		Day:       14,
		Month:     11,
		Year:      2025,
		Timestamp: time.Date(2025, time.November, 14, 15, 22, 0, 0, time.UTC),
	}

	var buf bytes.Buffer
	if err := phh.Encode(&buf, hand); err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}

	got := buf.String()
	want := "" +
		"variant = \"NT\"\n" +
		"table = \"default\"\n" +
		"seat_count = 3\n" +
		"seats = [1, 2, 3]\n" +
		"antes = [0, 0, 0]\n" +
		"blinds_or_straddles = [1, 2, 0]\n" +
		"min_bet = 2\n" +
		"starting_stacks = [200, 200, 200]\n" +
		"finishing_stacks = [200, 200, 200]\n" +
		"winnings = [0, 0, 0]\n" +
		"actions = [\"d dh p1 AhKh\", \"d dh p2 7c2d\", \"d dh p3 QsJs\", \"p1 cbr 6\", \"p2 f\", \"p3 cc\"]\n" +
		"players = [\"alice-bot\", \"bob-bot\", \"charlie-bot\"]\n" +
		"hand = \"hand-00042\"\n" +
		"time = \"15:22:00\"\n" +
		"time_zone = \"UTC\"\n" +
		"day = 14\n" +
		"month = 11\n" +
		"year = 2025\n"

	if got != want {
		t.Fatalf("Encode output mismatch.\nGot:\n%s\nWant:\n%s", got, want)
	}
}
