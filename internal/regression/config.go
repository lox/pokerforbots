package regression

import (
	"time"

	"github.com/rs/zerolog"
)

// TestMode represents the type of regression test
type TestMode string

const (
	ModeHeadsUp      TestMode = "heads-up"
	ModePopulation   TestMode = "population"
	ModeNPCBenchmark TestMode = "npc-benchmark"
	ModeSelfPlay     TestMode = "self-play"
	ModeAll          TestMode = "all"
)

// Config holds all configuration for a regression test
type Config struct {
	// Core settings
	Mode       TestMode
	HandsTotal int
	BatchSize  int
	Seeds      []int64

	// Bot binaries - unified across all modes
	Challenger string // Primary bot being tested (all modes)
	Baseline   string // Reference bot for comparison (all modes)

	// Table configuration
	ChallengerSeats int
	BaselineSeats   int
	NPCs            map[string]int // NPC type -> count

	// Bankroll
	StartingChips    int
	InfiniteBankroll bool

	// Statistical
	SignificanceLevel      float64
	EffectSizeThreshold    float64
	MultipleTestCorrection bool
	EarlyStopping          bool
	MinHands               int
	MaxHands               int
	CheckInterval          int
	StdDevClampMin         float64
	StdDevClampFallback    float64
	WarnOnStdDevClamp      bool

	// Performance
	TimeoutMs           int
	MaxConcurrentTables int
	ServerAddr          string
	ServerCmd           string // Command to run the server
	MinPlayers          int
	MaxPlayers          int

	// Health
	MaxCrashesPerBot       int
	MaxTimeoutsPerBot      int
	RestartDelayMs         int
	StopOnInsufficientBots bool // Stop when not enough bots remain (for simulations)

	// Output
	OutputFormat string // "json", "summary", "both"
	OutputFile   string
	Verbose      bool
	ValidateOnly bool

	// Logging
	Logger zerolog.Logger
}

// BotStatus tracks the health of a running bot
type BotStatus struct {
	ID           string
	Binary       string
	DisplayName  string
	Crashes      int
	Timeouts     int
	LastError    time.Time
	IsHealthy    bool
	RestartCount int
	Cmd          any // Will be *exec.Cmd when running
}

// HealthMonitor manages bot health and restarts
type HealthMonitor struct {
	MaxCrashes   int
	MaxTimeouts  int
	RestartDelay time.Duration
	bots         map[string]*BotStatus
	logger       zerolog.Logger
}

// NewHealthMonitor creates a new health monitor
func NewHealthMonitor(maxCrashes, maxTimeouts int, restartDelay time.Duration, logger zerolog.Logger) *HealthMonitor {
	return &HealthMonitor{
		MaxCrashes:   maxCrashes,
		MaxTimeouts:  maxTimeouts,
		RestartDelay: restartDelay,
		bots:         make(map[string]*BotStatus),
		logger:       logger,
	}
}

// RegisterBot registers a bot with the health monitor
func (h *HealthMonitor) RegisterBot(id, binary, displayName string) {
	h.bots[id] = &BotStatus{
		ID:          id,
		Binary:      binary,
		DisplayName: displayName,
		IsHealthy:   true,
	}
}

// RecordCrash records a bot crash
func (h *HealthMonitor) RecordCrash(id string) bool {
	bot, exists := h.bots[id]
	if !exists {
		return false
	}

	bot.Crashes++
	bot.LastError = time.Now()

	if bot.Crashes >= h.MaxCrashes {
		bot.IsHealthy = false
		h.logger.Error().
			Str("bot_id", id).
			Str("binary", bot.Binary).
			Int("crashes", bot.Crashes).
			Msg("Bot exceeded crash limit, marking unhealthy")
		return false
	}

	h.logger.Warn().
		Str("bot_id", id).
		Str("binary", bot.Binary).
		Int("crashes", bot.Crashes).
		Msg("Bot crashed, will restart")
	return true
}

// RecordTimeout records a bot timeout
func (h *HealthMonitor) RecordTimeout(id string) bool {
	bot, exists := h.bots[id]
	if !exists {
		return false
	}

	bot.Timeouts++
	bot.LastError = time.Now()

	if bot.Timeouts >= h.MaxTimeouts {
		bot.IsHealthy = false
		h.logger.Error().
			Str("bot_id", id).
			Str("binary", bot.Binary).
			Int("timeouts", bot.Timeouts).
			Msg("Bot exceeded timeout limit, marking unhealthy")
		return false
	}

	return true
}

// IsHealthy returns true if the bot is healthy
func (h *HealthMonitor) IsHealthy(id string) bool {
	bot, exists := h.bots[id]
	if !exists {
		return false
	}
	return bot.IsHealthy
}

// GetStatus returns the status of a bot
func (h *HealthMonitor) GetStatus(id string) *BotStatus {
	return h.bots[id]
}

// GetAllStatuses returns all bot statuses
func (h *HealthMonitor) GetAllStatuses() map[string]*BotStatus {
	return h.bots
}

// GetErrorSummary returns crash and timeout counts
func (h *HealthMonitor) GetErrorSummary() (crashes, timeouts, recovered int) {
	for _, bot := range h.bots {
		crashes += bot.Crashes
		timeouts += bot.Timeouts
		recovered += bot.RestartCount
	}
	return
}
