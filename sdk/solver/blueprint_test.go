package solver

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadBlueprintRejectsVersionMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "version-mismatch.json")

	bp := &Blueprint{
		Version:     blueprintFileVersion + 1,
		GeneratedAt: time.Now().UTC(),
		Iterations:  5,
		Abstraction: DefaultAbstraction(),
		Strategies:  map[string][]float64{},
	}

	if err := bp.Save(path); err != nil {
		t.Fatalf("save blueprint: %v", err)
	}

	if _, err := LoadBlueprint(path); err == nil {
		t.Fatalf("expected version mismatch to fail")
	}
}

func TestLoadBlueprintRejectsInvalidAbstraction(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid-abstraction.json")

	bp := &Blueprint{
		Version:     blueprintFileVersion,
		GeneratedAt: time.Now().UTC(),
		Iterations:  1,
		Abstraction: AbstractionConfig{
			PreflopBucketCount:  0,
			PostflopBucketCount: 10,
			BetSizing:           []float64{0.5},
			MaxActionsPerNode:   3,
		},
		Strategies: map[string][]float64{},
	}

	if err := bp.Save(path); err != nil {
		t.Fatalf("save blueprint: %v", err)
	}

	if _, err := LoadBlueprint(path); err == nil {
		t.Fatalf("expected abstraction validation to fail")
	}
}

func TestLoadBlueprintRejectsCorruptedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupted.json")

	if err := os.WriteFile(path, []byte("{not-json"), 0o644); err != nil {
		t.Fatalf("write corrupted file: %v", err)
	}

	if _, err := LoadBlueprint(path); err == nil {
		t.Fatalf("expected corrupted blueprint to fail")
	}
}
