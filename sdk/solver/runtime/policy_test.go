package runtime

import (
	"testing"
	"time"

	"github.com/lox/pokerforbots/sdk/solver"
)

func TestPolicyActionWeightsErrors(t *testing.T) {
	var p *Policy
	if _, err := p.ActionWeights(solver.InfoSetKey{}, 1); err == nil {
		t.Fatalf("expected error for nil policy")
	}

	p = &Policy{}
	if _, err := p.ActionWeights(solver.InfoSetKey{}, 0); err == nil {
		t.Fatalf("expected error for non-positive action count")
	}
}

func TestPolicyActionWeightsPaddingAndUniformFallback(t *testing.T) {
	key := solver.InfoSetKey{Street: solver.StreetFlop, Player: 1, HoleBucket: 2}
	bp := &solver.Blueprint{
		Version:     1,
		GeneratedAt: time.Now().UTC(),
		Iterations:  10,
		Abstraction: solver.DefaultAbstraction(),
		Strategies: map[string][]float64{
			key.String(): {0.7},
		},
	}

	policy := &Policy{blueprint: bp}

	weights, err := policy.ActionWeights(key, 3)
	if err != nil {
		t.Fatalf("action weights: %v", err)
	}
	if len(weights) != 3 {
		t.Fatalf("expected 3 weights, got %d", len(weights))
	}

	if diff(weights[0], 0.7) > 1e-9 {
		t.Fatalf("expected first weight 0.7, got %v", weights[0])
	}
	for i := 1; i < len(weights); i++ {
		if diff(weights[i], 1.0/3.0) > 1e-9 {
			t.Fatalf("expected padded weight 1/3 at index %d, got %v", i, weights[i])
		}
	}

	missing, err := policy.ActionWeights(solver.InfoSetKey{Street: solver.StreetTurn}, 4)
	if err != nil {
		t.Fatalf("missing key fallback: %v", err)
	}
	for i, w := range missing {
		if diff(w, 0.25) > 1e-9 {
			t.Fatalf("expected uniform fallback 0.25 at index %d, got %v", i, w)
		}
	}
}

func diff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}
