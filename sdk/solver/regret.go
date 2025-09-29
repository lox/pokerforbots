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

// RegretUpdateOptions configures how regrets and strategy sums are accumulated.
type RegretUpdateOptions struct {
	ClampNegativeRegrets bool
	LinearAveraging      bool
	Iteration            int
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
func (e *RegretEntry) Update(regret []float64, strategy []float64, reachWeight float64, opts RegretUpdateOptions) {
	e.mutex.Lock()
	iterWeight := 1.0
	if opts.LinearAveraging {
		iter := opts.Iteration
		if iter <= 0 {
			iter = 1
		}
		iterWeight = float64(iter)
	}
	weight := reachWeight * iterWeight
	for i := range regret {
		if opts.ClampNegativeRegrets {
			e.RegretSum[i] += regret[i]
			if e.RegretSum[i] < 0 {
				e.RegretSum[i] = 0
			}
		} else {
			e.RegretSum[i] += regret[i]
		}
		e.StrategySum[i] += weight * strategy[i]
	}
	e.Normalising += weight
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
const regretTableShardCount = 64
const regretTableShardMask = regretTableShardCount - 1

type regretShard struct {
	mu      sync.RWMutex
	entries map[string]*RegretEntry
}

// RegretTable maintains thread-safe entries keyed by info set using sharded maps.
type RegretTable struct {
	shards [regretTableShardCount]regretShard
}

// NewRegretTable returns an empty regret table ready for use.
func NewRegretTable() *RegretTable {
	table := &RegretTable{}
	for i := 0; i < regretTableShardCount; i++ {
		table.shards[i].entries = make(map[string]*RegretEntry)
	}
	return table
}

// Get returns the entry for the given key, creating it if missing.
func (t *RegretTable) Get(key InfoSetKey, actionCount int) *RegretEntry {
	k := key.String()
	shard := t.shardFor(k)

	shard.mu.RLock()
	entry, ok := shard.entries[k]
	shard.mu.RUnlock()
	if ok {
		entry.ensureSize(actionCount)
		return entry
	}

	shard.mu.Lock()
	defer shard.mu.Unlock()
	if entry, ok = shard.entries[k]; ok {
		entry.ensureSize(actionCount)
		return entry
	}

	entry = &RegretEntry{}
	entry.ensureSize(actionCount)
	shard.entries[k] = entry
	return entry
}

// Entries exposes a snapshot of the underlying table for serialisation.
func (t *RegretTable) Entries() map[string]*RegretEntry {
	out := make(map[string]*RegretEntry)
	for i := 0; i < regretTableShardCount; i++ {
		shard := &t.shards[i]
		shard.mu.RLock()
		for k, v := range shard.entries {
			out[k] = v
		}
		shard.mu.RUnlock()
	}
	return out
}

// Size returns the number of info sets tracked.
func (t *RegretTable) Size() int {
	total := 0
	for i := 0; i < regretTableShardCount; i++ {
		shard := &t.shards[i]
		shard.mu.RLock()
		total += len(shard.entries)
		shard.mu.RUnlock()
	}
	return total
}

func (t *RegretTable) shardFor(key string) *regretShard {
	h := hashKey(key)
	return &t.shards[h&regretTableShardMask]
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

func hashKey(key string) uint32 {
	const offset32 = 2166136261
	const prime32 = 16777619
	var hash uint32 = offset32
	for i := 0; i < len(key); i++ {
		hash ^= uint32(key[i])
		hash *= prime32
	}
	return hash
}
