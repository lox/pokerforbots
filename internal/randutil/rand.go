package randutil

import rand "math/rand/v2"

const (
	goldenRatio64 = 0x9e3779b97f4a7c15
)

// New returns a *rand.Rand seeded deterministically from the provided int64.
// The helper centralises how we derive the two 64-bit seeds required by rand/v2
// so that all call sites get reproducible sequences.
func New(seed int64) *rand.Rand {
	u := uint64(seed)
	return rand.New(rand.NewPCG(mix(u), mix(u+goldenRatio64)))
}

func mix(x uint64) uint64 {
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}
