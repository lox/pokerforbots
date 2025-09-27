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
	entry.Update(regrets, strategy, 2.0)

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
				entry.Update(regrets, strategy, 1.0)
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
