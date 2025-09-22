package regression

import (
	"fmt"
	"math"
	"strings"

	"github.com/lox/pokerforbots/internal/server"
)

// BatchStrategy defines the interface for different test mode strategies
type BatchStrategy interface {
	// Name returns the name of this strategy for logging
	Name() string

	// ConfigureBatch prepares bot commands and NPC configuration for a batch
	ConfigureBatch(batchNum int, seed int64) BatchConfiguration

	// AggregateStats aggregates statistics for this specific mode
	AggregateStats(stats *server.GameStats) (map[string]float64, error)

	// ShouldStopEarly checks if early stopping criteria are met
	ShouldStopEarly(results map[string]float64, totalHands int) bool

	// GetHealthPolicy returns mode-specific health monitoring policy
	GetHealthPolicy() HealthPolicy
}

// BatchConfiguration contains the configuration for a single batch
type BatchConfiguration struct {
	BotCommands []string
	NPCConfig   string // For NPC mode
	Seed        int64
	Hands       int
}

// HealthPolicy defines health monitoring configuration
type HealthPolicy struct {
	MaxCrashesPerBot  int
	MaxTimeoutsPerBot int
	RestartDelayMs    int
}

// HeadsUpStrategy implements BatchStrategy for heads-up testing
type HeadsUpStrategy struct {
	Challenger string
	Baseline   string
	Config     *Config
}

func (s *HeadsUpStrategy) Name() string {
	return "heads-up"
}

func (s *HeadsUpStrategy) ConfigureBatch(batchNum int, seed int64) BatchConfiguration {
	return BatchConfiguration{
		BotCommands: []string{s.Challenger, s.Baseline},
		Seed:        seed,
		Hands:       s.Config.BatchSize,
	}
}

func (s *HeadsUpStrategy) AggregateStats(stats *server.GameStats) (map[string]float64, error) {
	return AggregateHeadsUpStats(stats)
}

func (s *HeadsUpStrategy) ShouldStopEarly(results map[string]float64, totalHands int) bool {
	if !s.Config.EarlyStopping {
		return false
	}
	if totalHands < s.Config.MinHands {
		return false
	}
	// TODO: Check if statistical significance reached
	return false
}

func (s *HeadsUpStrategy) GetHealthPolicy() HealthPolicy {
	return HealthPolicy{
		MaxCrashesPerBot:  s.Config.MaxCrashesPerBot,
		MaxTimeoutsPerBot: s.Config.MaxTimeoutsPerBot,
		RestartDelayMs:    s.Config.RestartDelayMs,
	}
}

// PopulationStrategy implements BatchStrategy for population testing
type PopulationStrategy struct {
	Challenger      string
	Baseline        string
	ChallengerSeats int
	BaselineSeats   int
	Config          *Config
}

func (s *PopulationStrategy) Name() string {
	return "population"
}

func (s *PopulationStrategy) ConfigureBatch(batchNum int, seed int64) BatchConfiguration {
	var botCmds []string
	// Add challenger bots
	for i := 0; i < s.ChallengerSeats; i++ {
		botCmds = append(botCmds, s.Challenger)
	}
	// Add baseline bots
	for i := 0; i < s.BaselineSeats; i++ {
		botCmds = append(botCmds, s.Baseline)
	}

	return BatchConfiguration{
		BotCommands: botCmds,
		Seed:        seed,
		Hands:       s.Config.BatchSize,
	}
}

func (s *PopulationStrategy) AggregateStats(stats *server.GameStats) (map[string]float64, error) {
	return AggregatePopulationStats(stats, s.ChallengerSeats, s.BaselineSeats), nil
}

func (s *PopulationStrategy) ShouldStopEarly(results map[string]float64, totalHands int) bool {
	if !s.Config.EarlyStopping {
		return false
	}
	if totalHands < s.Config.MinHands {
		return false
	}
	// TODO: Check if statistical significance reached
	return false
}

func (s *PopulationStrategy) GetHealthPolicy() HealthPolicy {
	return HealthPolicy{
		MaxCrashesPerBot:  s.Config.MaxCrashesPerBot,
		MaxTimeoutsPerBot: s.Config.MaxTimeoutsPerBot,
		RestartDelayMs:    s.Config.RestartDelayMs,
	}
}

// NPCBenchmarkStrategy implements BatchStrategy for NPC benchmark testing
type NPCBenchmarkStrategy struct {
	Challenger      string
	Baseline        string
	ChallengerSeats int
	BaselineSeats   int
	NPCs            map[string]int
	Config          *Config
}

func (s *NPCBenchmarkStrategy) Name() string {
	return "npc-benchmark"
}

func (s *NPCBenchmarkStrategy) ConfigureBatch(batchNum int, seed int64) BatchConfiguration {
	var botCmds []string

	// For NPC benchmark, only add challenger bots OR baseline bots, never both
	// The runner creates separate strategies for challenger vs NPCs and baseline vs NPCs
	if s.ChallengerSeats > 0 {
		// This is a challenger vs NPCs run
		for i := 0; i < s.ChallengerSeats; i++ {
			botCmds = append(botCmds, s.Challenger)
		}
	} else if s.BaselineSeats > 0 {
		// This is a baseline vs NPCs run
		for i := 0; i < s.BaselineSeats; i++ {
			botCmds = append(botCmds, s.Baseline)
		}
	}

	// Build NPC configuration string
	var npcParts []string
	for strategy, count := range s.NPCs {
		if count > 0 {
			npcParts = append(npcParts, fmt.Sprintf("%s:%d", strategy, count))
		}
	}
	npcConfig := strings.Join(npcParts, ",")

	return BatchConfiguration{
		BotCommands: botCmds,
		NPCConfig:   npcConfig,
		Seed:        seed,
		Hands:       s.Config.BatchSize,
	}
}

func (s *NPCBenchmarkStrategy) AggregateStats(stats *server.GameStats) (map[string]float64, error) {
	// Determine if this is a challenger run (has ChallengerSeats) or baseline run
	isChallenger := s.ChallengerSeats > 0
	return AggregateNPCStats(stats, isChallenger), nil
}

func (s *NPCBenchmarkStrategy) ShouldStopEarly(results map[string]float64, totalHands int) bool {
	if !s.Config.EarlyStopping {
		return false
	}
	if totalHands < s.Config.MinHands {
		return false
	}
	// TODO: Check if performance against NPCs is conclusive
	return false
}

func (s *NPCBenchmarkStrategy) GetHealthPolicy() HealthPolicy {
	return HealthPolicy{
		MaxCrashesPerBot:  s.Config.MaxCrashesPerBot,
		MaxTimeoutsPerBot: s.Config.MaxTimeoutsPerBot,
		RestartDelayMs:    s.Config.RestartDelayMs,
	}
}

// SelfPlayStrategy implements BatchStrategy for self-play testing
type SelfPlayStrategy struct {
	Challenger string // Bot playing against itself
	Baseline   string // Same as Challenger for self-play
	BotSeats   int
	Config     *Config
}

func (s *SelfPlayStrategy) Name() string {
	return "self-play"
}

func (s *SelfPlayStrategy) ConfigureBatch(batchNum int, seed int64) BatchConfiguration {
	var botCmds []string
	// In self-play, all bots are the same (using Challenger)
	for i := 0; i < s.BotSeats; i++ {
		botCmds = append(botCmds, s.Challenger)
	}

	return BatchConfiguration{
		BotCommands: botCmds,
		Seed:        seed,
		Hands:       s.Config.BatchSize,
	}
}

func (s *SelfPlayStrategy) AggregateStats(stats *server.GameStats) (map[string]float64, error) {
	return AggregateSelfPlayStats(stats), nil
}

func (s *SelfPlayStrategy) ShouldStopEarly(results map[string]float64, totalHands int) bool {
	if !s.Config.EarlyStopping {
		return false
	}
	if totalHands < s.Config.MinHands {
		return false
	}
	// In self-play, check if we've established variance baseline
	// (i.e., results are consistently near zero)
	avgBB100 := results["avg_bb_per_100"]
	if math.Abs(avgBB100) > 5.0 && totalHands > s.Config.MinHands*2 {
		// Something might be wrong if avg is not near zero
		return true
	}
	return false
}

func (s *SelfPlayStrategy) GetHealthPolicy() HealthPolicy {
	return HealthPolicy{
		MaxCrashesPerBot:  s.Config.MaxCrashesPerBot,
		MaxTimeoutsPerBot: s.Config.MaxTimeoutsPerBot,
		RestartDelayMs:    s.Config.RestartDelayMs,
	}
}
