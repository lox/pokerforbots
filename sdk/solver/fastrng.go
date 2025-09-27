package solver

import "math/rand"

// PCG32 is a fast, small, statistically good RNG
// Based on PCG-XSH-RR with 64-bit state and 32-bit output
type PCG32 struct {
	state uint64
}

// NewPCG32 creates a new PCG32 RNG with the given seed
func NewPCG32(seed int64) *PCG32 {
	return &PCG32{state: uint64(seed)*2 + 1}
}

// InitSeed reinitializes with a new seed (avoids allocation)
func (r *PCG32) InitSeed(seed int64) {
	r.state = uint64(seed)*2 + 1
}

// Uint32 generates a random uint32
func (r *PCG32) Uint32() uint32 {
	oldstate := r.state
	r.state = oldstate*6364136223846793005 + 1442695040888963407
	xorshifted := uint32(((oldstate >> 18) ^ oldstate) >> 27)
	rot := uint32(oldstate >> 59)
	return (xorshifted >> rot) | (xorshifted << ((-rot) & 31))
}

// Intn returns a random int in [0, n)
func (r *PCG32) Intn(n int) int {
	return int(r.Uint32() % uint32(n))
}

// wrapperRand wraps our fast RNG to implement rand.Source
type wrapperSource struct {
	rng *PCG32
}

func (w *wrapperSource) Int63() int64 {
	return int64(w.rng.Uint32())<<31 | int64(w.rng.Uint32())
}

func (w *wrapperSource) Seed(seed int64) {
	w.rng = NewPCG32(seed)
}

// NewFastRand creates a math/rand.Rand using our fast PCG32
func NewFastRand(seed int64) *rand.Rand {
	return rand.New(&wrapperSource{rng: NewPCG32(seed)})
}
