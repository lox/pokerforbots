package main

import (
	"testing"
	"time"
)

func TestBuildEvalSpecsSwapOrder(t *testing.T) {
	opts := evaluationOptions{BlueprintPath: "/tmp/foo"}
	specs := buildEvalSpecs(opts, false)
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if specs[0].Args[1] != "./sdk/examples/calling-station" {
		t.Fatalf("expected baseline first, got %v", specs[0].Args)
	}
	if specs[1].Env["POKERFORBOTS_BLUEPRINT"] != "/tmp/foo" {
		t.Fatalf("missing blueprint env, got %v", specs[1].Env)
	}

	swapped := buildEvalSpecs(opts, true)
	if swapped[0].Args[1] != "./sdk/examples/complex" {
		t.Fatalf("expected blueprint first when swap=true, got %v", swapped[0].Args)
	}
}

func TestCombineEvalResultsAggregatesPlayers(t *testing.T) {
	res1 := &evalResult{
		HandsCompleted: 10,
		Duration:       time.Second,
		Players: []evalPlayer{
			{Name: "bot-a", NetChips: 20, Hands: 10},
			{Name: "bot-b", NetChips: -20, Hands: 10},
		},
	}
	res2 := &evalResult{
		HandsCompleted: 5,
		Duration:       2 * time.Second,
		Players: []evalPlayer{
			{Name: "bot-a", NetChips: -10}, // Hands defaults via HandsCompleted fallback
			{Name: "bot-b", NetChips: 10},
		},
	}

	combined := combineEvalResults(10, res1, res2)
	if combined.HandsCompleted != 15 {
		t.Fatalf("expected 15 hands, got %d", combined.HandsCompleted)
	}
	if combined.Duration != 3*time.Second {
		t.Fatalf("expected duration 3s, got %v", combined.Duration)
	}
	if len(combined.Players) != 2 {
		t.Fatalf("expected two players, got %d", len(combined.Players))
	}
	a := combined.Players[0]
	if a.Name != "bot-a" {
		t.Fatalf("expected bot-a first, got %s", a.Name)
	}
	if a.NetChips != 10 {
		t.Fatalf("expected net chips 10, got %d", a.NetChips)
	}
	if a.Hands != 15 {
		t.Fatalf("expected 15 hands, got %d", a.Hands)
	}
	expectedBBPerHand := (float64(10) / float64(10)) / float64(15)
	if diff := abs(a.BBPerHand - expectedBBPerHand); diff > 1e-9 {
		t.Fatalf("unexpected BB/hand %.10f (diff %.10f)", a.BBPerHand, diff)
	}
	if diff := abs(a.BBPer100 - expectedBBPerHand*100); diff > 1e-6 {
		t.Fatalf("unexpected BB/100 %.10f (diff %.10f)", a.BBPer100, diff)
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
