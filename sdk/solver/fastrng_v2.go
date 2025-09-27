package solver

import (
	"math/rand"
	randv2 "math/rand/v2"
)

// NewFastRandV2 creates a math/rand.Rand using Go 1.22+ rand/v2 PCG
func NewFastRandV2(seed int64) *rand.Rand {
	// rand/v2.NewPCG is MUCH faster than rand.NewSource
	src := randv2.NewPCG(uint64(seed), uint64(seed))

	// Wrap it to work with old rand.Rand interface
	return rand.New(&v2Wrapper{src: src})
}

// v2Wrapper adapts rand/v2.Source to rand.Source interface
type v2Wrapper struct {
	src *randv2.PCG
}

func (w *v2Wrapper) Int63() int64 {
	return int64(w.src.Uint64() >> 1)
}

func (w *v2Wrapper) Seed(seed int64) {
	// PCG doesn't have a Seed method, would need to recreate
	// This is why our custom PCG32 is still slightly better
	*w.src = *randv2.NewPCG(uint64(seed), uint64(seed))
}
