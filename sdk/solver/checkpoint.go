package solver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
)

const checkpointFileVersion = 1

type checkpointSnapshot struct {
	Version     int                       `json:"version"`
	Iteration   int64                     `json:"iteration"`
	RNGSeed     int64                     `json:"rng_seed"`
	RNGInt63    int64                     `json:"rng_int63_calls"`
	RNGIntn     int64                     `json:"rng_intn_calls"`
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
	trainer.rng = rand.New(rand.NewSource(snap.RNGSeed))
	trainer.rngInt63 = snap.RNGInt63
	trainer.rngIntn = snap.RNGIntn

	// Advance RNG to stored position.
	for i := int64(0); i < snap.RNGInt63; i++ {
		trainer.rng.Int63()
	}
	for i := int64(0); i < snap.RNGIntn; i++ {
		trainer.rng.Intn(trainer.trainCfg.Players)
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
		RNGInt63:    t.rngInt63,
		RNGIntn:     t.rngIntn,
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
	table.mu.Lock()
	defer table.mu.Unlock()
	table.entries = make(map[string]*RegretEntry, len(snaps))
	for key, snap := range snaps {
		table.entries[key] = newRegretEntryFromSnapshot(snap)
	}
	return table
}
