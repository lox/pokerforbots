package solver_test

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lox/pokerforbots/sdk/solver"
	solverRuntime "github.com/lox/pokerforbots/sdk/solver/runtime"
)

func TestTrainerBlueprintRoundTrip(t *testing.T) {
	abs := solver.DefaultAbstraction()
	abs.MaxActionsPerNode = 3

	key := solver.InfoSetKey{}
	bp := &solver.Blueprint{
		Version:     1,
		GeneratedAt: time.Now().UTC(),
		Iterations:  1,
		Abstraction: abs,
		Strategies: map[string][]float64{
			key.String(): {0.2, 0.5, 0.3},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "blueprint.json")
	if err := bp.Save(path); err != nil {
		t.Fatalf("save blueprint: %v", err)
	}

	policy, err := solverRuntime.Load(path)
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}

	weights, err := policy.ActionWeights(key, 3)
	if err != nil {
		t.Fatalf("action weights: %v", err)
	}
	if len(weights) != 3 {
		t.Fatalf("expected 3 weights, got %d", len(weights))
	}

	for i, want := range []float64{0.2, 0.5, 0.3} {
		if math.Abs(weights[i]-want) > 1e-9 {
			t.Fatalf("weight[%d] = %v, want %v", i, weights[i], want)
		}
	}
}

func TestTrainerTraversalStatsDeterministic(t *testing.T) {
	abs := solver.DefaultAbstraction()
	abs.MaxActionsPerNode = 3
	cfg := solver.DefaultTrainingConfig()
	cfg.Iterations = 1
	cfg.Seed = 123
	cfg.SmallBlind = 1
	cfg.BigBlind = 2
	cfg.StartingStack = 2

	trainerA, err := solver.NewTrainer(abs, cfg)
	if err != nil {
		t.Fatalf("new trainer A: %v", err)
	}
	if err := trainerA.Run(context.Background(), nil); err != nil {
		t.Fatalf("trainer A run: %v", err)
	}
	statsA := trainerA.Stats()
	if statsA.NodesVisited == 0 || statsA.TerminalNodes == 0 {
		t.Fatalf("expected non-zero stats, got %+v", statsA)
	}
	if statsA.MaxDepth <= 0 {
		t.Fatalf("expected positive max depth, got %d", statsA.MaxDepth)
	}

	trainerB, err := solver.NewTrainer(abs, cfg)
	if err != nil {
		t.Fatalf("new trainer B: %v", err)
	}
	if err := trainerB.Run(context.Background(), nil); err != nil {
		t.Fatalf("trainer B run: %v", err)
	}
	statsB := trainerB.Stats()
	statsA.IterationTime = 0
	statsB.IterationTime = 0
	if statsA != statsB {
		t.Fatalf("expected deterministic stats, got %+v vs %+v", statsA, statsB)
	}
}

func TestTrainerCheckpointRoundTrip(t *testing.T) {
	abs := solver.DefaultAbstraction()
	cfg := solver.DefaultTrainingConfig()
	cfg.Iterations = 2
	cfg.Seed = 77
	cfg.Players = 2
	cfg.SmallBlind = 1
	cfg.BigBlind = 2
	cfg.StartingStack = 6

	trainer, err := solver.NewTrainer(abs, cfg)
	if err != nil {
		t.Fatalf("new trainer: %v", err)
	}

	dir := t.TempDir()
	ckpt := filepath.Join(dir, "trainer.ckpt.json")
	trainer.EnableCheckpoints(ckpt, 1)

	if err := trainer.Run(context.Background(), nil); err != nil {
		t.Fatalf("trainer run: %v", err)
	}

	if _, err := os.Stat(ckpt); err != nil {
		t.Fatalf("checkpoint not written: %v", err)
	}

	resumed, err := solver.LoadTrainerFromCheckpoint(ckpt)
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if resumed.Iteration() != trainer.Iteration() {
		t.Fatalf("iteration mismatch resume=%d original=%d", resumed.Iteration(), trainer.Iteration())
	}

	if err := resumed.SetTotalIterations(int(resumed.Iteration()) + 1); err != nil {
		t.Fatalf("set total iterations: %v", err)
	}
	resumed.EnableCheckpoints(filepath.Join(dir, "trainer-resume.ckpt.json"), 1)
	if err := resumed.Run(context.Background(), nil); err != nil {
		t.Fatalf("resumed run: %v", err)
	}
}

func TestTrainerSamplingModesAffectTraversal(t *testing.T) {
	abs := solver.DefaultAbstraction()
	abs.MaxActionsPerNode = 3

	base := solver.DefaultTrainingConfig()
	base.Iterations = 1
	base.Seed = 123
	base.SmallBlind = 1
	base.BigBlind = 2
	base.StartingStack = 10
	base.ParallelTables = 1
	base.AdaptiveRaiseVisits = 0

	fullCfg := base
	fullCfg.Sampling = solver.SamplingModeFullTraversal

	fullTrainer, err := solver.NewTrainer(abs, fullCfg)
	if err != nil {
		t.Fatalf("new trainer (full): %v", err)
	}
	if err := fullTrainer.Run(context.Background(), nil); err != nil {
		t.Fatalf("run trainer (full): %v", err)
	}
	fullStats := fullTrainer.Stats()
	if fullStats.NodesVisited == 0 {
		t.Fatalf("expected nodes visited > 0, got %+v", fullStats)
	}

	externalCfg := base
	externalCfg.Sampling = solver.SamplingModeExternal

	externalTrainer, err := solver.NewTrainer(abs, externalCfg)
	if err != nil {
		t.Fatalf("new trainer (external): %v", err)
	}
	if err := externalTrainer.Run(context.Background(), nil); err != nil {
		t.Fatalf("run trainer (external): %v", err)
	}
	externalStats := externalTrainer.Stats()
	if externalStats.NodesVisited == 0 {
		t.Fatalf("expected nodes visited > 0, got %+v", externalStats)
	}

	if fullStats.NodesVisited <= externalStats.NodesVisited {
		t.Fatalf("expected full traversal to visit more nodes (full=%d, external=%d)", fullStats.NodesVisited, externalStats.NodesVisited)
	}
}
