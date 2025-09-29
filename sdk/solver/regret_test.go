package solver

import (
	"sync"
	"testing"
)

func TestRegretEntryStrategyNormalizesPositiveRegrets(t *testing.T) {
	var entry RegretEntry
	entry.ensureSize(3)
	entry.RegretSum[0] = 1
	entry.RegretSum[1] = 2
	entry.RegretSum[2] = -5

	strat := entry.Strategy()

	if got, want := strat[0], 1.0/3.0; abs(got-want) > 1e-9 {
		t.Fatalf("expected first action %v, got %v", want, got)
	}
	if got, want := strat[1], 2.0/3.0; abs(got-want) > 1e-9 {
		t.Fatalf("expected second action %v, got %v", want, got)
	}
	if strat[2] != 0 {
		t.Fatalf("expected negative regret action to drop to 0, got %v", strat[2])
	}
}

func TestRegretEntryStrategyUniformFallback(t *testing.T) {
	var entry RegretEntry
	entry.ensureSize(4)

	strat := entry.Strategy()
	for i, s := range strat {
		if abs(s-0.25) > 1e-9 {
			t.Fatalf("expected uniform fallback 0.25 at index %d, got %v", i, s)
		}
	}
}

func TestRegretEntryUpdateAndAverage(t *testing.T) {
	var entry RegretEntry
	entry.ensureSize(2)

	regrets := []float64{1, -1}
	strategy := []float64{0.6, 0.4}
	entry.Update(regrets, strategy, 2.0, RegretUpdateOptions{})

	if entry.RegretSum[0] != 1 || entry.RegretSum[1] != -1 {
		t.Fatalf("unexpected regret sums: %+v", entry.RegretSum)
	}
	if entry.StrategySum[0] != 1.2 || entry.StrategySum[1] != 0.8 {
		t.Fatalf("unexpected strategy sums: %+v", entry.StrategySum)
	}
	if entry.Normalising != 2.0 {
		t.Fatalf("expected normalising weight 2.0, got %v", entry.Normalising)
	}

	avg := entry.AverageStrategy()
	if abs(avg[0]-0.6) > 1e-9 || abs(avg[1]-0.4) > 1e-9 {
		t.Fatalf("expected average strategy [0.6,0.4], got %v", avg)
	}
}

func TestRegretEntryUpdateCFRPlus(t *testing.T) {
	var entry RegretEntry
	entry.ensureSize(2)

	opts := RegretUpdateOptions{ClampNegativeRegrets: true, LinearAveraging: true, Iteration: 3}

	regrets := []float64{-2, 5}
	strategy := []float64{0.5, 0.5}
	entry.Update(regrets, strategy, 1.0, opts)

	if entry.RegretSum[0] != 0 {
		t.Fatalf("negative regret should clamp to zero, got %v", entry.RegretSum[0])
	}
	if entry.RegretSum[1] != 5 {
		t.Fatalf("positive regret should accumulate, got %v", entry.RegretSum[1])
	}

	entry.Update([]float64{-1, -1}, strategy, 1.0, opts)

	if entry.RegretSum[0] != 0 {
		t.Fatalf("clamped regret should remain zero, got %v", entry.RegretSum[0])
	}
	if entry.RegretSum[1] != 4 {
		t.Fatalf("expected regret sum 4 after clamp, got %v", entry.RegretSum[1])
	}

	expectedStrategy := 3.0
	if abs(entry.StrategySum[0]-expectedStrategy) > 1e-9 {
		t.Fatalf("expected linear-weighted strategy sum %v, got %v", expectedStrategy, entry.StrategySum[0])
	}
	if abs(entry.StrategySum[1]-expectedStrategy) > 1e-9 {
		t.Fatalf("expected linear-weighted strategy sum %v, got %v", expectedStrategy, entry.StrategySum[1])
	}
	if abs(entry.Normalising-6) > 1e-9 {
		t.Fatalf("expected normalising weight 6, got %v", entry.Normalising)
	}
}

func TestRegretEntryUpdateDCFR(t *testing.T) {
	var entry RegretEntry
	entry.ensureSize(2)
	entry.RegretSum[0] = 10
	entry.RegretSum[1] = -5
	entry.StrategySum[0] = 4
	entry.StrategySum[1] = 6
	entry.Normalising = 10

	opts := RegretUpdateOptions{
		UseDCFR:   true,
		Iteration: 1,
		DCFRAlpha: 1.5,
		DCFRBeta:  0,
		DCFRGamma: 2,
	}

	entry.Update([]float64{1, -2}, []float64{0.5, 0.5}, 1.0, opts)

	posScale := dcfrScale(1, 1.5)
	negScale := dcfrScale(1, 0)
	avgScale := dcfrScale(1, 2)

	if diff := abs(entry.RegretSum[0] - (10*posScale + 1)); diff > 1e-9 {
		t.Fatalf("unexpected DCFR positive regret: diff=%v", diff)
	}
	if diff := abs(entry.RegretSum[1] - (-5*negScale - 2)); diff > 1e-9 {
		t.Fatalf("unexpected DCFR negative regret: diff=%v", diff)
	}
	if diff := abs(entry.StrategySum[0] - (4*avgScale + 0.5)); diff > 1e-9 {
		t.Fatalf("unexpected DCFR strategy sum0 diff=%v", diff)
	}
	if diff := abs(entry.StrategySum[1] - (6*avgScale + 0.5)); diff > 1e-9 {
		t.Fatalf("unexpected DCFR strategy sum1 diff=%v", diff)
	}
	if diff := abs(entry.Normalising - (10*avgScale + 1)); diff > 1e-9 {
		t.Fatalf("unexpected DCFR normalising diff=%v", diff)
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func TestRegretTableGetCachesEntries(t *testing.T) {
	table := NewRegretTable()
	key := InfoSetKey{Player: 1}

	entryA := table.Get(key, 2)
	if entryA == nil {
		t.Fatalf("expected entry, got nil")
	}

	entryB := table.Get(key, 3)
	if entryA != entryB {
		t.Fatalf("expected cached entry to be reused")
	}
	if len(entryB.Actions) != 3 {
		t.Fatalf("expected ensureSize to grow actions to 3, got %d", len(entryB.Actions))
	}
}

func TestRegretTableConcurrentAccess(t *testing.T) {
	table := NewRegretTable()
	key := InfoSetKey{Player: 2}

	regrets := []float64{1, -0.5, 0.25}
	strategy := []float64{0.4, 0.3, 0.3}

	const workers = 32
	const updates = 100

	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < updates; j++ {
				entry := table.Get(key, len(regrets))
				entry.Update(regrets, strategy, 1.0, RegretUpdateOptions{})
			}
		}()
	}

	wg.Wait()

	entry := table.Get(key, len(regrets))
	expected := float64(workers * updates)
	if abs(entry.Normalising-expected) > 1e-6 {
		t.Fatalf("expected normalising weight %v, got %v", expected, entry.Normalising)
	}
}
