package solver

import (
	"encoding/json"
	"errors"
	"os"
	"time"
)

const blueprintFileVersion = 1

// Blueprint captures the averaged strategies produced by a solver run so that
// runtime bots can sample actions without rerunning CFR.
type Blueprint struct {
	Version     int                  `json:"version"`
	GeneratedAt time.Time            `json:"generated_at"`
	Iterations  int                  `json:"iterations"`
	Abstraction AbstractionConfig    `json:"abstraction"`
	Strategies  map[string][]float64 `json:"strategies"`
}

// Save writes the blueprint to disk in JSON format.
func (b *Blueprint) Save(path string) error {
	if b == nil {
		return errors.New("nil blueprint")
	}
	if path == "" {
		return errors.New("destination path is required")
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(b)
}

// LoadBlueprint reads a blueprint from disk and ensures the abstraction metadata
// is present for runtime compatibility checks.
func LoadBlueprint(path string) (*Blueprint, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var bp Blueprint
	if err := json.NewDecoder(f).Decode(&bp); err != nil {
		return nil, err
	}
	if err := bp.Abstraction.Validate(); err != nil {
		return nil, err
	}
	if bp.Version != blueprintFileVersion {
		return nil, errors.New("unsupported blueprint version")
	}
	return &bp, nil
}

// Strategy returns the stored average strategy for the provided info-set key.
func (b *Blueprint) Strategy(key InfoSetKey) ([]float64, bool) {
	if b == nil {
		return nil, false
	}
	strat, ok := b.Strategies[key.String()]
	return strat, ok
}
