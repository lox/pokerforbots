package solver

import (
	"errors"
	"fmt"
	"time"
)

// SamplingMode controls how opponent actions are handled during traversal.
type SamplingMode uint8

const (
	SamplingModeExternal SamplingMode = iota
	SamplingModeFullTraversal
)

func (m SamplingMode) String() string {
	switch m {
	case SamplingModeExternal:
		return "external"
	case SamplingModeFullTraversal:
		return "full"
	default:
		return "unknown"
	}
}

// AbstractionConfig captures the coarse representation used by the solver when
// clustering hands and actions. Values here should align with the abstraction
// used during blueprint generation and runtime consumption.
type AbstractionConfig struct {
	// PreflopBucketCount controls how many distinct holes-card classes the solver
	// will maintain before shared cards are exposed.
	PreflopBucketCount int

	// PostflopBucketCount controls how many buckets community-card textures map into.
	PostflopBucketCount int

	// BetSizing lists bet size fractions relative to the current pot that will be
	// exposed in the action abstraction. Values should be monotonic increasing.
	BetSizing []float64

	// MaxActionsPerNode caps the number of actions the solver will expand for any
	// decision node (fold/call counted separately from raises).
	MaxActionsPerNode int

	// EnableRaises toggles whether the abstraction exposes raise actions.
	EnableRaises bool

	// MaxRaisesPerBucket limits how many distinct raise sizes survive pruning for a
	// single decision. Zero disables pruning.
	MaxRaisesPerBucket int
}

// Validate ensures the abstraction is well-formed before training begins.
func (c AbstractionConfig) Validate() error {
	if c.PreflopBucketCount <= 0 {
		return errors.New("preflop bucket count must be > 0")
	}
	if c.PostflopBucketCount <= 0 {
		return errors.New("postflop bucket count must be > 0")
	}
	if c.EnableRaises {
		if len(c.BetSizing) == 0 {
			return errors.New("at least one bet sizing fraction is required")
		}
		last := 0.0
		for i, v := range c.BetSizing {
			if v <= 0 {
				return fmt.Errorf("bet sizing[%d] must be > 0", i)
			}
			if v <= last {
				return fmt.Errorf("bet sizing[%d] must be strictly increasing", i)
			}
			last = v
		}
		if c.MaxActionsPerNode < 3 {
			return errors.New("max actions per node must allow at least fold/call/raise")
		}
		if c.MaxRaisesPerBucket < 0 {
			return errors.New("max raises per bucket cannot be negative")
		}
	} else {
		if len(c.BetSizing) > 0 {
			return errors.New("bet sizing must be empty when raises are disabled")
		}
		if c.MaxActionsPerNode < 2 {
			return errors.New("max actions per node must allow at least fold/call when raises disabled")
		}
	}
	return nil
}

// TrainingConfig aggregates parameters that control MCCFR execution.
type TrainingConfig struct {
	Iterations          int
	Players             int
	Seed                int64
	ParallelTables      int
	CheckpointEvery     time.Duration
	ProgressEvery       int
	SmallBlind          int
	BigBlind            int
	StartingStack       int
	EnableRaises        bool
	MaxRaisesPerBucket  int
	AdaptiveRaiseVisits int
	UseCFRPlus          bool
	Sampling            SamplingMode
	UseDCFR             bool
}

// Validate ensures the training parameters are safe to use.
func (c TrainingConfig) Validate() error {
	if c.Iterations <= 0 {
		return errors.New("iterations must be > 0")
	}
	if c.Players < 2 {
		return errors.New("players must be >= 2")
	}
	if c.ParallelTables <= 0 {
		return errors.New("parallel tables must be > 0")
	}
	if c.CheckpointEvery < 0 {
		return errors.New("checkpoint interval cannot be negative")
	}
	if c.ProgressEvery < 0 {
		return errors.New("progress interval cannot be negative")
	}
	if c.SmallBlind <= 0 {
		return errors.New("small blind must be > 0")
	}
	if c.BigBlind <= c.SmallBlind {
		return errors.New("big blind must be greater than small blind")
	}
	if c.StartingStack <= 0 {
		return errors.New("starting stack must be > 0")
	}
	if c.EnableRaises && c.MaxRaisesPerBucket < 0 {
		return errors.New("max raises per bucket cannot be negative")
	}
	if c.AdaptiveRaiseVisits < 0 {
		return errors.New("adaptive raise visits cannot be negative")
	}
	if c.Sampling > SamplingModeFullTraversal {
		return errors.New("invalid sampling mode")
	}
	return nil
}

// DefaultAbstraction returns a conservative abstraction suitable for smoke tests.
func DefaultAbstraction() AbstractionConfig {
	return AbstractionConfig{
		PreflopBucketCount:  10,
		PostflopBucketCount: 20,
		BetSizing:           []float64{0.33, 0.5, 0.75, 1.0, 1.5},
		MaxActionsPerNode:   8,
		EnableRaises:        true,
		MaxRaisesPerBucket:  3,
	}
}

// DefaultTrainingConfig returns a minimal configuration for local experimentation.
func DefaultTrainingConfig() TrainingConfig {
	return TrainingConfig{
		Iterations:          1000,
		Players:             2,
		Seed:                1,
		ParallelTables:      1,
		CheckpointEvery:     5 * time.Minute,
		ProgressEvery:       0,
		SmallBlind:          5,
		BigBlind:            10,
		StartingStack:       1000,
		EnableRaises:        true,
		MaxRaisesPerBucket:  3,
		AdaptiveRaiseVisits: 500,
		UseCFRPlus:          false,
		Sampling:            SamplingModeExternal,
		UseDCFR:             true,
	}
}
