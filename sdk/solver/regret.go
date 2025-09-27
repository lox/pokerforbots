package solver

import (
	"fmt"
	"sync"
)

// Street enumerates the betting round within a Texas Hold'em hand.
type Street uint8

const (
	StreetPreflop Street = iota
	StreetFlop
	StreetTurn
	StreetRiver
)

func (s Street) String() string {
	switch s {
	case StreetPreflop:
		return "preflop"
	case StreetFlop:
		return "flop"
	case StreetTurn:
		return "turn"
	case StreetRiver:
		return "river"
	default:
		return "unknown"
	}
}

// InfoSetKey uniquely identifies the situation a player experiences. It must
// correspond to the abstraction used while training; otherwise averaging becomes
// meaningless.
type InfoSetKey struct {
	Street       Street
	Player       int
	HoleBucket   int
	BoardBucket  int
	PotBucket    int
	ToCallBucket int
}

func (k InfoSetKey) String() string {
	return fmt.Sprintf("%d/%d/%d/%d/%d/%d", k.Street, k.Player, k.HoleBucket, k.BoardBucket, k.PotBucket, k.ToCallBucket)
}

// RegretEntry accumulates regrets and strategy sums for a node. Values are kept
// in slices to avoid map churn during CFR traversals.
type RegretEntry struct {
	Actions     []float64
	RegretSum   []float64
	StrategySum []float64
	Normalising float64
	mutex       sync.Mutex
}

// ensureSize grows the regret entry to accommodate n actions.
func (e *RegretEntry) ensureSize(n int) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	if len(e.Actions) >= n {
		return
	}
	missing := n - len(e.Actions)
	e.Actions = append(e.Actions, make([]float64, missing)...)
	e.RegretSum = append(e.RegretSum, make([]float64, missing)...)
	e.StrategySum = append(e.StrategySum, make([]float64, missing)...)
}

// Strategy returns the current regret-matching distribution for the node.
func (e *RegretEntry) Strategy() []float64 {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	total := 0.0
	strat := make([]float64, len(e.RegretSum))
	for i, r := range e.RegretSum {
		if r > 0 {
			strat[i] = r
			total += r
		}
	}
	if total <= 0 {
		// Uniform fallback
		v := 1.0 / float64(len(strat))
		for i := range strat {
			strat[i] = v
		}
		return strat
	}
	for i := range strat {
		strat[i] /= total
	}
	return strat
}

// Update accumulates regrets and strategy sums for the node.
func (e *RegretEntry) Update(regret []float64, strategy []float64, reachWeight float64) {
	e.mutex.Lock()
	for i := range regret {
		e.RegretSum[i] += regret[i]
		e.StrategySum[i] += reachWeight * strategy[i]
	}
	e.Normalising += reachWeight
	e.mutex.Unlock()
}

// AverageStrategy returns the normalised average strategy for the node.
func (e *RegretEntry) AverageStrategy() []float64 {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	strat := make([]float64, len(e.StrategySum))
	if e.Normalising <= 0 {
		v := 1.0 / float64(len(strat))
		for i := range strat {
			strat[i] = v
		}
		return strat
	}
	for i := range strat {
		strat[i] = e.StrategySum[i] / e.Normalising
	}
	return strat
}

// RegretTable maintains thread-safe entries keyed by info set.
type RegretTable struct {
	entries map[string]*RegretEntry
	mu      sync.RWMutex
}

// NewRegretTable returns an empty regret table ready for use.
func NewRegretTable() *RegretTable {
	return &RegretTable{entries: make(map[string]*RegretEntry)}
}

// Get returns the entry for the given key, creating it if missing.
func (t *RegretTable) Get(key InfoSetKey, actionCount int) *RegretEntry {
	k := key.String()

	t.mu.RLock()
	entry, ok := t.entries[k]
	t.mu.RUnlock()
	if ok {
		entry.ensureSize(actionCount)
		return entry
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if entry, ok = t.entries[k]; ok {
		entry.ensureSize(actionCount)
		return entry
	}

	entry = &RegretEntry{}
	entry.ensureSize(actionCount)
	t.entries[k] = entry
	return entry
}

// Entries exposes a snapshot of the underlying table for serialisation.
func (t *RegretTable) Entries() map[string]*RegretEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make(map[string]*RegretEntry, len(t.entries))
	for k, v := range t.entries {
		out[k] = v
	}
	return out
}

// Size returns the number of info sets tracked.
func (t *RegretTable) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.entries)
}

func (e *RegretEntry) snapshot() regretSnapshot {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	snap := regretSnapshot{
		Actions:     append([]float64(nil), e.Actions...),
		RegretSum:   append([]float64(nil), e.RegretSum...),
		StrategySum: append([]float64(nil), e.StrategySum...),
		Normalising: e.Normalising,
	}
	return snap
}

func newRegretEntryFromSnapshot(snap regretSnapshot) *RegretEntry {
	entry := &RegretEntry{
		Actions:     append([]float64(nil), snap.Actions...),
		RegretSum:   append([]float64(nil), snap.RegretSum...),
		StrategySum: append([]float64(nil), snap.StrategySum...),
		Normalising: snap.Normalising,
	}
	return entry
}
