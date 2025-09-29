package solver

import (
	"github.com/lox/pokerforbots/internal/randutil"

	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const checkpointFileVersion = 1

type checkpointSnapshot struct {
	Version     int                       `json:"version"`
	Iteration   int64                     `json:"iteration"`
	RNGSeed     int64                     `json:"rng_seed"`
	RNGInt64    int64                     `json:"rng_int63_calls"`
	RNGIntN     int64                     `json:"rng_intn_calls"`
	Training    TrainingConfig            `json:"training"`
	Abstraction AbstractionConfig         `json:"abstraction"`
	Regrets     map[string]regretSnapshot `json:"regrets"`
	Stats       TraversalStats            `json:"stats"`
}

type regretSnapshot struct {
	Actions     []float64 `json:"actions"`
	RegretSum   []float64 `json:"regret_sum"`
	StrategySum []float64 `json:"strategy_sum"`
	Normalising float64   `json:"normalising"`
}

// EnableCheckpoints configures the trainer to write checkpoints every n iterations.
func (t *Trainer) EnableCheckpoints(path string, every int) {
	t.checkpointPath = path
	t.checkpointEvery = every
}

// SaveCheckpoint writes a snapshot of the trainer state to the provided path.
func (t *Trainer) SaveCheckpoint(path string) error {
	snap, err := t.buildCheckpoint()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create checkpoint dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create checkpoint temp: %w", err)
	}
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(snap); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return fmt.Errorf("encode checkpoint: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("close checkpoint temp: %w", err)
	}

	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("persist checkpoint: %w", err)
	}
	return nil
}

// LoadTrainerFromCheckpoint restores a trainer from a previously saved checkpoint.
func LoadTrainerFromCheckpoint(path string) (*Trainer, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	snap, err := decodeCheckpoint(f)
	if err != nil {
		return nil, err
	}

	trainer, err := NewTrainer(snap.Abstraction, snap.Training)
	if err != nil {
		return nil, err
	}

	trainer.iteration.Store(snap.Iteration)
	trainer.stats = snap.Stats
	trainer.rngSeed = snap.RNGSeed
	trainer.rng = randutil.New(snap.RNGSeed)
	trainer.rngInt64 = snap.RNGInt64
	trainer.rngIntN = snap.RNGIntN

	// Advance RNG to stored position.
	for i := int64(0); i < snap.RNGInt64; i++ {
		trainer.rng.Int64()
	}
	for i := int64(0); i < snap.RNGIntN; i++ {
		trainer.rng.IntN(trainer.trainCfg.Players)
	}

	trainer.regrets = restoreRegretTable(snap.Regrets)
	return trainer, nil
}

func (t *Trainer) buildCheckpoint() (*checkpointSnapshot, error) {
	stats := t.Stats()
	snap := &checkpointSnapshot{
		Version:     checkpointFileVersion,
		Iteration:   t.iteration.Load(),
		RNGSeed:     t.rngSeed,
		RNGInt64:    t.rngInt64,
		RNGIntN:     t.rngIntN,
		Training:    t.trainCfg,
		Abstraction: t.absCfg,
		Regrets:     make(map[string]regretSnapshot),
		Stats:       stats,
	}

	entries := t.regrets.Entries()
	for key, entry := range entries {
		snap.Regrets[key] = entry.snapshot()
	}
	return snap, nil
}

func decodeCheckpoint(r io.Reader) (*checkpointSnapshot, error) {
	var snap checkpointSnapshot
	if err := json.NewDecoder(r).Decode(&snap); err != nil {
		return nil, err
	}
	if snap.Version != checkpointFileVersion {
		return nil, errors.New("unsupported checkpoint version")
	}
	if err := snap.Abstraction.Validate(); err != nil {
		return nil, fmt.Errorf("checkpoint abstraction invalid: %w", err)
	}
	if err := snap.Training.Validate(); err != nil {
		return nil, fmt.Errorf("checkpoint training invalid: %w", err)
	}
	return &snap, nil
}

func restoreRegretTable(snaps map[string]regretSnapshot) *RegretTable {
	table := NewRegretTable()
	for key, snap := range snaps {
		shard := table.shardFor(key)
		shard.mu.Lock()
		shard.entries[key] = newRegretEntryFromSnapshot(snap)
		shard.mu.Unlock()
	}
	return table
}
