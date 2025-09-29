package main

import (
	"context"
	"fmt"
	"os"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/lox/pokerforbots/sdk/solver"
)

var cli struct {
	Debug bool `help:"enable debug logging"`

	Train TrainCmd `cmd:"" help:"run MCCFR training and emit a blueprint"`
	Eval  EvalCmd  `cmd:"" help:"evaluate an existing blueprint"`
}

type TrainCmd struct {
	Out                 string `help:"path to write the blueprint pack" required:""`
	Iterations          int    `help:"number of MCCFR iterations" default:"100000"`
	Players             int    `help:"number of players in self-play" default:"2"`
	Parallel            int    `help:"number of concurrent tables" default:"1"`
	Seed                int64  `help:"random seed; 0 uses time seed" default:"0"`
	CheckpointMins      int    `help:"checkpoint interval in minutes" default:"10"`
	SmallBlind          int    `help:"small blind size" default:"5"`
	BigBlind            int    `help:"big blind size" default:"10"`
	Stack               int    `help:"starting stack size" default:"1000"`
	CheckpointPath      string `help:"path to write periodic checkpoints"`
	CheckpointEvery     int    `help:"checkpoint interval in iterations (0 disables)" default:"0"`
	ProgressEvery       int    `help:"log progress every N iterations (0 => iterations/100)" default:"0"`
	DisableRaises       bool   `help:"disable raise actions for minimal smoke testing"`
	MaxRaises           int    `help:"limit raises per node (0 keeps full abstraction)" default:"0"`
	AdaptiveRaiseVisits int    `help:"visits before expanding raises (0 disables adaptive expansion)" default:"500"`
	Smoke               bool   `help:"apply smoke preset (stack=50, small blind=1, big blind=2, max raises=2)"`
	ResumeFrom          string `help:"resume training from checkpoint file"`
	CPUProfile          string `help:"write CPU profile to file"`
	CFRPlus             bool   `help:"enable CFR+ (positive regret matching with linear averaging)"`
	Sampling            string `help:"sampling mode (external|full)" enum:"external,full" default:"external"`
}

type EvalCmd struct {
	Blueprint string `help:"path to blueprint pack" required:""`
	Hands     int    `help:"number of hands to simulate" default:"10000"`
	Mirror    bool   `help:"enable mirror mode to reduce variance"`
	Seed      int64  `help:"random seed; 0 uses time seed" default:"0"`
}

func main() {
	ctx := kong.Parse(&cli,
		kong.Name("solver"),
		kong.Description("PokerForBots solver tooling"),
		kong.UsageOnError(),
	)

	setupLogger(cli.Debug)

	switch ctx.Command() {
	case "train":
		if err := cli.Train.Run(context.Background()); err != nil {
			log.Fatal().Err(err).Msg("training failed")
		}
	case "eval":
		if err := cli.Eval.Run(context.Background()); err != nil {
			log.Fatal().Err(err).Msg("evaluation failed")
		}
	default:
		log.Fatal().Msgf("unknown command: %s", ctx.Command())
	}
}

func setupLogger(debug bool) {
	level := zerolog.InfoLevel
	if debug {
		level = zerolog.DebugLevel
	}
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).Level(level)
}

func (cmd *TrainCmd) Run(ctx context.Context) error {
	mode, err := parseSamplingMode(cmd.Sampling)
	if err != nil {
		return err
	}

	// Set up CPU profiling if requested
	if cmd.CPUProfile != "" {
		f, err := os.Create(cmd.CPUProfile)
		if err != nil {
			return fmt.Errorf("create cpu profile: %w", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			return fmt.Errorf("start cpu profile: %w", err)
		}
		defer pprof.StopCPUProfile()
		log.Info().Str("path", cmd.CPUProfile).Msg("CPU profiling enabled")
	}

	var trainer *solver.Trainer

	if cmd.ResumeFrom != "" {
		trainer, err = solver.LoadTrainerFromCheckpoint(cmd.ResumeFrom)
		if err != nil {
			return fmt.Errorf("load checkpoint: %w", err)
		}
		if cmd.Iterations > 0 {
			if err := trainer.SetTotalIterations(cmd.Iterations); err != nil {
				return err
			}
		}
		if cmd.CheckpointPath != "" && cmd.CheckpointEvery > 0 {
			trainer.EnableCheckpoints(cmd.CheckpointPath, cmd.CheckpointEvery)
		}
		if cmd.ProgressEvery > 0 {
			trainer.SetProgressEvery(cmd.ProgressEvery)
		}
		if cmd.DisableRaises {
			log.Warn().Msg("cannot disable raises when resuming from checkpoint; keeping original abstraction")
		}
		if cmd.MaxRaises > 0 {
			log.Warn().Int("max_raises", cmd.MaxRaises).Msg("cannot change raise limit when resuming from checkpoint; keeping original abstraction")
		}
		if cmd.Smoke {
			log.Warn().Msg("cannot apply smoke preset when resuming from checkpoint; keeping original abstraction")
		}
		trainCfg := trainer.TrainingConfig()
		if mode != trainCfg.Sampling {
			log.Warn().Str("requested", mode.String()).Str("checkpoint", trainCfg.Sampling.String()).Msg("cannot change sampling mode when resuming from checkpoint; keeping original")
		}
		if cmd.CFRPlus && !trainCfg.UseCFRPlus {
			log.Warn().Msg("cannot enable CFR+ when resuming from checkpoint; keeping original regret mode")
		} else if !cmd.CFRPlus && trainCfg.UseCFRPlus {
			log.Warn().Msg("checkpoint was trained with CFR+; continuing with CFR+ mode")
		}
		log.Info().Int("iterations", trainCfg.Iterations).Int("resume_iteration", int(trainer.Iteration())).Int("max_raises", trainCfg.MaxRaisesPerBucket).Int("parallel", trainCfg.ParallelTables).Str("sampling", trainCfg.Sampling.String()).Str("checkpoint", cmd.ResumeFrom).Msg("resuming training run")
	} else {
		abs := solver.DefaultAbstraction()
		train := solver.DefaultTrainingConfig()

		if cmd.Smoke {
			train.SmallBlind = 1
			train.BigBlind = 2
			train.StartingStack = 50
			abs.MaxRaisesPerBucket = 2
			train.MaxRaisesPerBucket = 2
			train.AdaptiveRaiseVisits = 0 // Disable adaptive for smoke runs
			log.Info().Msg("applying smoke preset (stack=50, small_blind=1, big_blind=2, max_raises=2, adaptive_disabled)")
		}

		if cmd.Iterations > 0 {
			train.Iterations = cmd.Iterations
		}
		if cmd.Players > 0 {
			train.Players = cmd.Players
		}
		if cmd.Parallel > 0 {
			train.ParallelTables = cmd.Parallel
		}
		if cmd.Seed != 0 {
			train.Seed = cmd.Seed
		}
		if cmd.CheckpointMins > 0 {
			train.CheckpointEvery = time.Duration(cmd.CheckpointMins) * time.Minute
		}
		if cmd.SmallBlind > 0 {
			train.SmallBlind = cmd.SmallBlind
		}
		if cmd.BigBlind > 0 {
			train.BigBlind = cmd.BigBlind
		}
		if cmd.Stack > 0 {
			train.StartingStack = cmd.Stack
		}
		if cmd.ProgressEvery > 0 {
			train.ProgressEvery = cmd.ProgressEvery
		}
		if cmd.AdaptiveRaiseVisits >= 0 {
			train.AdaptiveRaiseVisits = cmd.AdaptiveRaiseVisits
		}
		if cmd.DisableRaises {
			train.EnableRaises = false
			abs.EnableRaises = false
			abs.BetSizing = nil
			if abs.MaxActionsPerNode < 2 {
				abs.MaxActionsPerNode = 2
			}
			abs.MaxRaisesPerBucket = 0
			train.MaxRaisesPerBucket = 0
		} else if cmd.MaxRaises > 0 {
			abs.MaxRaisesPerBucket = cmd.MaxRaises
			train.MaxRaisesPerBucket = cmd.MaxRaises
		}

		train.UseCFRPlus = cmd.CFRPlus
		train.Sampling = mode

		trainer, err = solver.NewTrainer(abs, train)
		if err != nil {
			return err
		}
		if cmd.CheckpointPath != "" && cmd.CheckpointEvery > 0 {
			trainer.EnableCheckpoints(cmd.CheckpointPath, cmd.CheckpointEvery)
		}
		if cmd.ProgressEvery > 0 {
			trainer.SetProgressEvery(cmd.ProgressEvery)
		}
		if cmd.DisableRaises {
			trainer.SetRaisesEnabled(false)
		}
		log.Info().Int("iterations", train.Iterations).Int("players", train.Players).Int("max_raises", abs.MaxRaisesPerBucket).Int("parallel", train.ParallelTables).Bool("cfr_plus", train.UseCFRPlus).Str("sampling", train.Sampling.String()).Msg("starting training run")
	}

	start := time.Now()
	progress := func(p solver.Progress) {
		log.Info().Int("iteration", p.Iteration).Int("infosets", p.RegretTableSize).Int64("nodes", p.Stats.NodesVisited).Int64("terminals", p.Stats.TerminalNodes).Int("max_depth", p.Stats.MaxDepth).Dur("iter_time", p.Stats.IterationTime).Msg("progress")
	}

	if err := trainer.Run(ctx, progress); err != nil {
		return err
	}

	if trainer.TrainingConfig().AdaptiveRaiseVisits > 0 {
		expanded, tracked := trainer.AdaptiveStats()
		log.Info().Int("adaptive_tracked", tracked).Int("adaptive_expanded", expanded).Msg("adaptive raise summary")
	}

	bp := trainer.Blueprint()
	bp.Version = 1
	duration := time.Since(start)
	log.Info().Dur("duration", duration).Int("infosets", len(bp.Strategies)).Msg("training completed")

	if err := bp.Save(cmd.Out); err != nil {
		return fmt.Errorf("save blueprint: %w", err)
	}
	log.Info().Str("path", cmd.Out).Msg("blueprint saved")
	return nil
}

func parseSamplingMode(input string) (solver.SamplingMode, error) {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "", "external":
		return solver.SamplingModeExternal, nil
	case "full":
		return solver.SamplingModeFullTraversal, nil
	default:
		return solver.SamplingModeExternal, fmt.Errorf("unknown sampling mode %q", input)
	}
}

func (cmd *EvalCmd) Run(ctx context.Context) error {
	if cmd.Hands <= 0 {
		return fmt.Errorf("hands must be positive (got %d)", cmd.Hands)
	}
	bp, err := solver.LoadBlueprint(cmd.Blueprint)
	if err != nil {
		return fmt.Errorf("load blueprint: %w", err)
	}

	log.Info().
		Str("generated", bp.GeneratedAt.Format(time.RFC3339)).
		Int("iterations", bp.Iterations).
		Int("infosets", len(bp.Strategies)).
		Msg("blueprint loaded")

	opts := evaluationOptions{
		BlueprintPath: cmd.Blueprint,
		Hands:         cmd.Hands,
		Seed:          cmd.Seed,
		SmallBlind:    5,
		BigBlind:      10,
		StartChips:    1000,
		TimeoutMs:     100,
		Mirror:        cmd.Mirror,
	}

	res, err := runEvaluation(ctx, log.Logger, opts)
	if err != nil {
		return fmt.Errorf("run evaluation: %w", err)
	}

	log.Info().
		Uint64("hands_completed", res.HandsCompleted).
		Dur("duration", res.Duration).
		Msg("evaluation complete")

	for _, p := range res.Players {
		log.Info().
			Str("player", p.Name).
			Float64("bb_per_100", p.BBPer100).
			Float64("bb_per_hand", p.BBPerHand).
			Int("net_chips", p.NetChips).
			Int("hands", p.Hands).
			Msg("player summary")
	}
	return nil
}
