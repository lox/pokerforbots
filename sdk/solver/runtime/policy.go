package runtime

import (
	"errors"

	"github.com/lox/pokerforbots/sdk/solver"
)

// Policy exposes read-only access to a solver blueprint for sampling actions
// during live play.
type Policy struct {
	blueprint *solver.Blueprint
}

// Load constructs a runtime policy from a stored blueprint file.
func Load(path string) (*Policy, error) {
	bp, err := solver.LoadBlueprint(path)
	if err != nil {
		return nil, err
	}
	return &Policy{blueprint: bp}, nil
}

// Blueprint returns the underlying blueprint metadata (read-only).
func (p *Policy) Blueprint() *solver.Blueprint {
	if p == nil {
		return nil
	}
	return p.blueprint
}

// ActionWeights returns the stored probability distribution for the provided
// info-set key and action count. When the key is missing, a uniform policy is
// returned to guarantee a valid distribution.
func (p *Policy) ActionWeights(key solver.InfoSetKey, actionCount int) ([]float64, error) {
	if p == nil || p.blueprint == nil {
		return nil, errors.New("nil policy")
	}
	if actionCount <= 0 {
		return nil, errors.New("action count must be positive")
	}

	if strat, ok := p.blueprint.Strategy(key); ok {
		out := make([]float64, actionCount)
		copy(out, strat)
		if len(strat) >= actionCount {
			return out, nil
		}
		// Pad missing entries uniformly for remaining actions.
		uniform := 1.0 / float64(actionCount)
		for i := len(strat); i < actionCount; i++ {
			out[i] = uniform
		}
		return out, nil
	}

	// Uniform fallback.
	out := make([]float64, actionCount)
	v := 1.0 / float64(actionCount)
	for i := range out {
		out[i] = v
	}
	return out, nil
}
