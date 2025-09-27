package solver

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// TraversalStats captures instrumentation metrics for a single MCCFR iteration.
type TraversalStats struct {
	NodesVisited  int64
	TerminalNodes int64
	MaxDepth      int
	IterationTime time.Duration
}

// Progress contains metadata emitted during long-running solver operations.
type Progress struct {
	Iteration       int
	RegretTableSize int
	Stats           TraversalStats
}

// Trainer orchestrates Monte Carlo CFR iterations over the PokerForBots engine.
type Trainer struct {
	absCfg          AbstractionConfig
	trainCfg        TrainingConfig
	bucket          *BucketMapper
	regrets         *RegretTable
	iteration       atomic.Int64
	rng             *rand.Rand
	playerNames     []string
	statsMu         sync.Mutex
	stats           TraversalStats
	rngSeed         int64
	rngInt63        int64
	rngIntn         int64
	checkpointPath  string
	checkpointEvery int
	adaptiveMu      sync.Mutex
	adaptiveState   map[string]*adaptiveInfo
}

type adaptiveInfo struct {
	visits   int64
	expanded bool
}

// NewTrainer constructs a solver trainer given abstraction and training configs.
func NewTrainer(absCfg AbstractionConfig, trainCfg TrainingConfig) (*Trainer, error) {
	if err := absCfg.Validate(); err != nil {
		return nil, err
	}
	if err := trainCfg.Validate(); err != nil {
		return nil, err
	}

	mapper, err := NewBucketMapper(absCfg)
	if err != nil {
		return nil, err
	}

	seed := trainCfg.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	names := make([]string, trainCfg.Players)
	for i := range names {
		names[i] = fmt.Sprintf("P%d", i)
	}

	trainer := &Trainer{
		absCfg:      absCfg,
		trainCfg:    trainCfg,
		bucket:      mapper,
		regrets:     NewRegretTable(),
		rng:         rand.New(rand.NewSource(seed)),
		playerNames: names,
		rngSeed:     seed,
	}
	if trainCfg.AdaptiveRaiseVisits > 0 {
		trainer.adaptiveState = make(map[string]*adaptiveInfo)
	}
	return trainer, nil
}

// Run executes the requested number of CFR iterations. The current implementation
// wires up all surrounding infrastructure but leaves the heavy domain logic as
// follow-up work; it still produces deterministic placeholder regret updates so
// the CLI, serialization, and telemetry can be exercised end-to-end.
func (t *Trainer) Run(ctx context.Context, progress func(Progress)) error {
	pLog := t.trainCfg.Iterations / 100
	if pLog == 0 {
		pLog = 1
	}
	batch := pLog
	if cfg := t.trainCfg.ProgressEvery; cfg > 0 {
		batch = cfg
	}

	for i := int(t.iteration.Load()); i < t.trainCfg.Iterations; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		startIter := time.Now()
		stats, err := t.singleIteration()
		if err != nil {
			return err
		}
		stats.IterationTime = time.Since(startIter)
		t.setStats(stats)
		iter := int(t.iteration.Add(1))

		if t.checkpointPath != "" && t.checkpointEvery > 0 && iter%t.checkpointEvery == 0 {
			if err := t.SaveCheckpoint(t.checkpointPath); err != nil {
				return err
			}
		}

		if progress != nil && iter%batch == 0 {
			progress(Progress{Iteration: iter, RegretTableSize: t.regrets.Size(), Stats: stats})
		}
	}

	if progress != nil {
		iter := int(t.iteration.Load())
		progress(Progress{Iteration: iter, RegretTableSize: t.regrets.Size(), Stats: t.Stats()})
	}

	if t.checkpointPath != "" && t.checkpointEvery > 0 {
		if err := t.SaveCheckpoint(t.checkpointPath); err != nil {
			return err
		}
	}
	return nil
}

// Blueprint materialises the averaged strategy produced so far. Even though the
// simulation currently performs placeholder updates, this plumbing keeps the data
// model aligned with future real solver logic.
func (t *Trainer) Blueprint() *Blueprint {
	entries := t.regrets.Entries()
	strategies := make(map[string][]float64, len(entries))
	for key, entry := range entries {
		strategies[key] = entry.AverageStrategy()
	}
	return &Blueprint{
		Version:     blueprintFileVersion,
		GeneratedAt: time.Now().UTC(),
		Iterations:  int(t.iteration.Load()),
		Abstraction: t.absCfg,
		Strategies:  strategies,
	}
}

func (t *Trainer) singleIteration() (TraversalStats, error) {
	parallel := t.trainCfg.ParallelTables
	if parallel <= 0 {
		parallel = 1
	}

	statsSlice := make([]TraversalStats, parallel)

	type tableSeeds struct {
		deck   int64
		sample int64
		button int
	}

	seeds := make([]tableSeeds, parallel)
	for i := 0; i < parallel; i++ {
		seeds[i].deck = t.rng.Int63()
		t.rngInt63++
		seeds[i].sample = t.rng.Int63()
		t.rngInt63++
		seeds[i].button = t.rng.Intn(t.trainCfg.Players)
		t.rngIntn++
	}

	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error

	for i := 0; i < parallel; i++ {
		idx := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := &iterationContext{
				trainer:     t,
				deckSeed:    seeds[idx].deck,
				button:      seeds[idx].button,
				playerNames: t.playerNames,
				stats:       &statsSlice[idx],
				sampler:     rand.New(rand.NewSource(seeds[idx].sample)),
				fastRNG:     PCG32{state: uint64(seeds[idx].deck)*2 + 1}, // Initialize embedded RNG
			}

			for player := 0; player < t.trainCfg.Players; player++ {
				errMu.Lock()
				if firstErr != nil {
					errMu.Unlock()
					return
				}
				errMu.Unlock()

				if _, err := t.traverse(ctx, nil, player, 0, 1.0, 1.0); err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
					return
				}
			}
		}()
	}

	wg.Wait()
	if firstErr != nil {
		return TraversalStats{}, firstErr
	}

	aggregated := TraversalStats{}
	for i := 0; i < parallel; i++ {
		aggregated.NodesVisited += statsSlice[i].NodesVisited
		aggregated.TerminalNodes += statsSlice[i].TerminalNodes
		if statsSlice[i].MaxDepth > aggregated.MaxDepth {
			aggregated.MaxDepth = statsSlice[i].MaxDepth
		}
	}

	return aggregated, nil
}

func (t *Trainer) setStats(stats TraversalStats) {
	t.statsMu.Lock()
	defer t.statsMu.Unlock()
	t.stats = stats
}

// Stats returns the most recent traversal statistics recorded by the trainer.
func (t *Trainer) Stats() TraversalStats {
	t.statsMu.Lock()
	defer t.statsMu.Unlock()
	return t.stats
}

func (t *Trainer) AdaptiveStats() (int, int) {
	if t.adaptiveState == nil {
		return 0, 0
	}
	t.adaptiveMu.Lock()
	defer t.adaptiveMu.Unlock()
	expanded := 0
	tracked := 0
	for _, info := range t.adaptiveState {
		tracked++
		if info.expanded {
			expanded++
		}
	}
	return expanded, tracked
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (t *Trainer) TrainingConfig() TrainingConfig {
	return t.trainCfg
}

func (t *Trainer) Iteration() int64 {
	return t.iteration.Load()
}

func (t *Trainer) SetTotalIterations(n int) error {
	current := int(t.iteration.Load())
	if n < current {
		return fmt.Errorf("total iterations %d less than completed %d", n, current)
	}
	t.trainCfg.Iterations = n
	return nil
}

func (t *Trainer) raisesEnabled() bool {
	if !t.trainCfg.EnableRaises || !t.absCfg.EnableRaises {
		return false
	}
	return len(t.absCfg.BetSizing) > 0
}

func (t *Trainer) SetRaisesEnabled(enabled bool) {
	t.trainCfg.EnableRaises = enabled
}

func (t *Trainer) SetProgressEvery(n int) {
	if n < 0 {
		n = 0
	}
	t.trainCfg.ProgressEvery = n
}

func (t *Trainer) shouldExpandRaises(key InfoSetKey) bool {
	if t.trainCfg.AdaptiveRaiseVisits <= 0 {
		return false
	}
	if t.adaptiveState == nil {
		return false
	}
	ks := key.String()
	t.adaptiveMu.Lock()
	info, ok := t.adaptiveState[ks]
	t.adaptiveMu.Unlock()
	return ok && info.expanded
}

func (t *Trainer) recordVisit(key InfoSetKey) {
	if t.trainCfg.AdaptiveRaiseVisits <= 0 {
		return
	}
	ks := key.String()
	t.adaptiveMu.Lock()
	defer t.adaptiveMu.Unlock()
	if t.adaptiveState == nil {
		t.adaptiveState = make(map[string]*adaptiveInfo)
	}
	info := t.adaptiveState[ks]
	if info == nil {
		info = &adaptiveInfo{}
		t.adaptiveState[ks] = info
	}
	info.visits++
	if !info.expanded && info.visits >= int64(t.trainCfg.AdaptiveRaiseVisits) {
		info.expanded = true
	}
}
