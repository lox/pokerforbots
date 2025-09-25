package server

import (
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lox/pokerforbots/protocol"
	"github.com/rs/zerolog"
)

// BotPool manages available bots and matches them into hands
type BotPool struct {
	bots              map[string]*Bot
	available         chan *Bot
	register          chan *Bot
	unregister        chan *Bot
	mu                sync.RWMutex
	minPlayers        int
	maxPlayers        int
	handCounter       uint64
	handLimit         uint64      // 0 means unlimited
	handLimitLogged   atomic.Bool // Track if we've logged the hand limit message
	handLimitNotified atomic.Bool
	stopCh            chan struct{}
	stopOnce          sync.Once
	logger            zerolog.Logger
	rng               *rand.Rand
	rngMutex          sync.Mutex // Protect RNG access
	config            Config     // Server configuration
	gameID            string
	matchTrigger      chan struct{}
	matcherWG         sync.WaitGroup
	runOnce           sync.Once

	// Metrics
	timeoutCounter   uint64
	handStartTime    time.Time
	gameEndTime      time.Time
	metricsLock      sync.RWMutex
	completionReason atomic.Value

	progressMonitor HandMonitor
	statsMonitor    *StatsMonitor
}

// WithRNG executes fn with exclusive access to the pool's RNG.
func (p *BotPool) WithRNG(fn func(*rand.Rand)) {
	p.rngMutex.Lock()
	defer p.rngMutex.Unlock()
	fn(p.rng)
}

const reasonHandLimitReached = "hand_limit_reached"

// DefaultConfig returns a config with sensible defaults
func DefaultConfig(minPlayers, maxPlayers int) Config {
	return Config{
		SmallBlind: 5,
		BigBlind:   10,
		StartChips: 1000,
		Timeout:    100 * time.Millisecond,
		MinPlayers: minPlayers,
		MaxPlayers: maxPlayers,
		HandLimit:  0,
		Seed:       0,
	}
}

// NewBotPool creates a new bot pool with explicit random source and config
func NewBotPool(logger zerolog.Logger, rng *rand.Rand, config Config) *BotPool {
	maxHands := config.MaxStatsHands
	if maxHands <= 0 {
		maxHands = 10000 // Default retention window for statistics
	}

	statsMonitor := NewStatsMonitor(config.BigBlind, config.EnableStats, maxHands)
	if config.EnableStats {
		logger.Info().
			Int("max_hands", maxHands).
			Msg("Statistics collection enabled")
	}

	pool := &BotPool{
		bots:          make(map[string]*Bot),
		available:     make(chan *Bot, 100),
		register:      make(chan *Bot),
		unregister:    make(chan *Bot),
		minPlayers:    config.MinPlayers,
		maxPlayers:    config.MaxPlayers,
		handLimit:     config.HandLimit,
		stopCh:        make(chan struct{}),
		logger:        logger.With().Str("component", "pool").Logger(),
		rng:           rng,
		config:        config,
		handStartTime: time.Time{},
		matchTrigger:  make(chan struct{}, 1),
		statsMonitor:  statsMonitor,
	}
	pool.completionReason.Store("")

	statsMonitor.OnGameStart(config.HandLimit)

	return pool
}

// SetGameID stores the identifier of the game this pool manages.
func (p *BotPool) SetGameID(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.gameID = id
}

// SetHandMonitor sets or replaces the hand monitor
func (p *BotPool) SetHandMonitor(monitor HandMonitor) {
	p.progressMonitor = monitor
	// Notify monitor of game start if we're starting fresh
	if monitor != nil && atomic.LoadUint64(&p.handCounter) == 0 {
		monitor.OnGameStart(p.handLimit)
	}
}

func (p *BotPool) ensureMatchLoop() {
	p.runOnce.Do(func() {
		p.matcherWG.Add(1)
		go p.matchLoop()
	})
}

// RecordActionLatency forwards latency metrics to the stats monitor when enabled.
func (p *BotPool) RecordActionLatency(botID string, duration time.Duration, outcome ResponseOutcome) {
	if p == nil || !p.config.EnableLatencyTracking || p.statsMonitor == nil {
		return
	}
	p.statsMonitor.RecordResponse(botID, duration, outcome)
}

// GetHandMonitor returns the combined monitor (both progress and stats)
func (p *BotPool) GetHandMonitor() HandMonitor {
	monitors := []HandMonitor{}
	if p.progressMonitor != nil {
		monitors = append(monitors, p.progressMonitor)
	}
	if p.statsMonitor != nil {
		monitors = append(monitors, p.statsMonitor)
	}
	return NewMultiHandMonitor(monitors...)
}

// GameID returns the identifier associated with this pool.
func (p *BotPool) GameID() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.gameID
}

// Run starts the bot pool manager
func (p *BotPool) Run() {
	p.ensureMatchLoop()

	for {
		select {
		case <-p.stopCh:
			return

		case bot, ok := <-p.register:
			if !ok || bot == nil {
				continue
			}
			p.mu.Lock()
			p.bots[bot.ID] = bot
			p.mu.Unlock()

			// Add to available queue if not in hand
			if !bot.IsInHand() {
				select {
				case p.available <- bot:
				default:
					// Queue full
				}
				p.triggerMatch()
			}

		case bot, ok := <-p.unregister:
			if !ok || bot == nil {
				continue
			}
			bot.close()
			p.mu.Lock()
			delete(p.bots, bot.ID)
			remainingBots := len(p.bots)
			p.mu.Unlock()

			if remainingBots < p.minPlayers {
				p.logger.Warn().
					Int("remaining_bots", remainingBots).
					Int("min_players", p.minPlayers).
					Msg("Insufficient bots remaining, ending game early")
				p.notifyGameCompleted("insufficient_players")
			}
		}
	}
}

func (p *BotPool) matchLoop() {
	defer p.matcherWG.Done()
	for {
		select {
		case <-p.stopCh:
			return
		case _, ok := <-p.matchTrigger:
			if !ok {
				return
			}
			p.tryMatch()
		}
	}
}

func (p *BotPool) triggerMatch() {
	select {
	case <-p.stopCh:
		return
	default:
	}
	select {
	case p.matchTrigger <- struct{}{}:
	default:
	}
}

// tryMatch attempts to match available bots into a hand
func (p *BotPool) tryMatch() {
	select {
	case <-p.stopCh:
		return
	default:
	}

	// Check if we've reached the hand limit
	if p.handLimit > 0 && atomic.LoadUint64(&p.handCounter) >= p.handLimit {
		// Only log once to avoid spam
		if p.handLimitLogged.CompareAndSwap(false, true) {
			p.logger.Info().Uint64("hands_completed", atomic.LoadUint64(&p.handCounter)).
				Uint64("hand_limit", p.handLimit).
				Msg("Hand limit reached - stopping new hand creation")
		}
		// Record game end time once when we reach the limit
		p.metricsLock.Lock()
		if p.gameEndTime.IsZero() {
			p.gameEndTime = time.Now()
		}
		p.metricsLock.Unlock()
		p.notifyGameCompleted(reasonHandLimitReached)
		return
	}

	// Count available bots
	availableCount := len(p.available)
	if availableCount < p.minPlayers {
		return
	}

	// Determine number of players for this hand
	numPlayers := min(availableCount, p.maxPlayers)

	// Collect all available bots first for random selection
	allBots := make([]*Bot, 0, availableCount)
collectLoop:
	for range availableCount {
		select {
		case <-p.stopCh:
			return
		case bot := <-p.available:
			// Double-check bot is still connected and not in hand
			p.mu.RLock()
			_, connected := p.bots[bot.ID]
			p.mu.RUnlock()

			switch {
			case connected && !bot.IsInHand() && bot.HasChips():
				allBots = append(allBots, bot)
			case connected && !bot.HasChips():
				// Bot is out of chips, remove from pool
				p.logger.Warn().Str("bot_id", bot.ID).Msg("Bot out of chips, removing from pool")
				p.Unregister(bot)
			case connected:
				// Return bot to available queue if it's valid
				select {
				case p.available <- bot:
				default:
				}
			}
		default:
			// No more available bots
			break collectLoop
		}
	}

	// Sort bots by ID for deterministic ordering before shuffle
	// This ensures consistent behavior regardless of connection timing
	sort.Slice(allBots, func(i, j int) bool {
		return allBots[i].ID < allBots[j].ID
	})

	// Randomly shuffle and select bots for this hand with mutex protection
	p.rngMutex.Lock()
	p.rng.Shuffle(len(allBots), func(i, j int) {
		allBots[i], allBots[j] = allBots[j], allBots[i]
	})
	p.rngMutex.Unlock()

	// Take the first numPlayers after shuffle
	bots := make([]*Bot, 0, numPlayers)
	if numPlayers > len(allBots) {
		numPlayers = len(allBots)
	}
	for i := 0; i < numPlayers; i++ {
		bots = append(bots, allBots[i])
	}

	// Return unused bots to available queue
	for i := numPlayers; i < len(allBots); i++ {
		select {
		case p.available <- allBots[i]:
		default:
			// Queue full
		}
	}
	if len(p.available) >= p.minPlayers {
		p.triggerMatch()
	}

	// If we got enough bots, start a hand
	if len(bots) >= p.minPlayers {
		// On first actual hand start, record game start time
		if atomic.LoadUint64(&p.handCounter) == 0 {
			p.metricsLock.Lock()
			if p.handStartTime.IsZero() {
				p.handStartTime = time.Now()
			}
			p.metricsLock.Unlock()
		}

		for _, bot := range bots {
			bot.SetInHand(true)
		}

		// Start hand runner (will be implemented)
		go p.runHand(bots)
	} else {
		// Return bots to available queue
		for _, bot := range bots {
			select {
			case p.available <- bot:
			default:
				// Queue full
			}
		}
	}
}

// runHand runs a single hand with the given bots
func (p *BotPool) runHand(bots []*Bot) {
	defer func() {
		// Return bots to pool after hand completes
		for _, bot := range bots {
			bot.SetInHand(false)
			// Only return to pool if bot still has chips
			if bot.HasChips() {
				select {
				case p.available <- bot:
				default:
					// Queue full
				}
			} else {
				p.logger.Debug().Str("bot_id", bot.ID).Msg("Bot out of chips after hand, removing from pool")
				p.Unregister(bot)
			}
		}
		p.triggerMatch()
	}()

	// Skip if any bot doesn't have a connection (for testing)
	for _, bot := range bots {
		if bot.conn == nil {
			return
		}
	}

	// Generate hand ID
	handNum := atomic.AddUint64(&p.handCounter, 1)
	handID := fmt.Sprintf("hand-%d", handNum)

	// Generate per-hand RNG to avoid concurrent access to the pool RNG
	p.rngMutex.Lock()
	handRNGSeed := p.rng.Int63()
	p.rngMutex.Unlock()

	button := 0 // With freshly shuffled seats, seat 0 acts as the button every hand

	handRNG := rand.New(rand.NewSource(handRNGSeed))
	p.logger.Debug().
		Str("hand_id", handID).
		Int("button_position", button).
		Int("player_count", len(bots)).
		Msg("Hand starting with deterministic button assignment")

	// Run the hand with the cloned RNG and config
	runner := NewHandRunnerWithConfig(p.logger, bots, handID, button, handRNG, p.config)
	runner.SetPool(p) // Pass pool for metrics tracking
	runner.Run()

	p.logger.Debug().
		Str("hand_id", handID).
		Msg("Hand complete")
}

// Register adds a bot to the pool
func (p *BotPool) Register(bot *Bot) {
	select {
	case <-p.stopCh:
		return
	case p.register <- bot:
	}
}

// Unregister removes a bot from the pool
func (p *BotPool) Unregister(bot *Bot) {
	select {
	case <-p.stopCh:
		return
	case p.unregister <- bot:
	}
}

// Stop signals the pool manager to halt and prevents new registrations.
func (p *BotPool) Stop() {
	p.ensureMatchLoop()
	p.stopOnce.Do(func() {
		// Record end time if not already set
		p.metricsLock.Lock()
		if p.gameEndTime.IsZero() {
			p.gameEndTime = time.Now()
		}
		p.metricsLock.Unlock()

		close(p.stopCh)
		p.matcherWG.Wait()
	})
}

// GetBot returns a bot by ID
func (p *BotPool) GetBot(id string) (*Bot, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	bot, ok := p.bots[id]
	return bot, ok
}

// BotCount returns the number of connected bots
func (p *BotPool) BotCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.bots)
}

// HandCount returns the number of hands completed
func (p *BotPool) HandCount() uint64 {
	return atomic.LoadUint64(&p.handCounter)
}

// HandLimit returns the hand limit (0 means unlimited)
func (p *BotPool) HandLimit() uint64 {
	return p.handLimit
}

// HandsRemaining returns the number of hands remaining (0 if unlimited)
func (p *BotPool) HandsRemaining() uint64 {
	if p.handLimit == 0 {
		return 0 // Unlimited
	}
	completed := atomic.LoadUint64(&p.handCounter)
	if completed >= p.handLimit {
		return 0
	}
	return p.handLimit - completed
}

// Done returns a channel that will be closed when the pool stops (hand limit reached or explicitly stopped)
func (p *BotPool) Done() <-chan struct{} {
	return p.stopCh
}

// IncrementTimeoutCounter increments the timeout counter
func (p *BotPool) IncrementTimeoutCounter() {
	atomic.AddUint64(&p.timeoutCounter, 1)
}

// TimeoutCount returns the number of timeouts that have occurred
func (p *BotPool) TimeoutCount() uint64 {
	return atomic.LoadUint64(&p.timeoutCounter)
}

// HandsPerSecond returns the current hands per second rate
func (p *BotPool) HandsPerSecond() float64 {
	p.metricsLock.RLock()
	start := p.handStartTime
	end := p.gameEndTime
	p.metricsLock.RUnlock()

	if start.IsZero() {
		return 0
	}
	var elapsed float64
	if !end.IsZero() {
		elapsed = end.Sub(start).Seconds()
	} else {
		elapsed = time.Since(start).Seconds()
	}
	handCount := float64(atomic.LoadUint64(&p.handCounter))
	if handCount == 0 || elapsed <= 0 {
		return 0
	}
	return handCount / elapsed
}

// CompletionReason returns the reason the game finished, if any.
func (p *BotPool) CompletionReason() string {
	if value := p.completionReason.Load(); value != nil {
		if reason, ok := value.(string); ok {
			return reason
		}
	}
	return ""
}

// NeedsDetailedData reports whether the pool requires detailed hand outcomes.
func (p *BotPool) NeedsDetailedData() bool {
	return p.statsMonitor != nil
}

// StartTime returns the timestamp when the first hand in the game began (min players met).
func (p *BotPool) StartTime() time.Time {
	p.metricsLock.RLock()
	defer p.metricsLock.RUnlock()
	return p.handStartTime
}

// EndTime returns the timestamp when the game stopped (hand limit reached or Stop called), if set.
func (p *BotPool) EndTime() time.Time {
	p.metricsLock.RLock()
	defer p.metricsLock.RUnlock()
	return p.gameEndTime
}

// RecordHandOutcome notifies monitors about the result of a completed hand.
func (p *BotPool) RecordHandOutcome(outcome HandOutcome) {
	if p.statsMonitor != nil {
		p.statsMonitor.OnHandComplete(outcome)
	}

	if p.progressMonitor != nil {
		p.progressMonitor.OnHandComplete(outcome)
	}

	p.maybeNotifyHandLimit()
}

// PlayerStats returns a snapshot of aggregate statistics for all bots in the pool.
func (p *BotPool) PlayerStats() []PlayerStats {
	if p.statsMonitor == nil {
		return nil
	}
	return p.statsMonitor.GetPlayerStats()
}

// HandLimitNotified returns true if the pool has already broadcast the completion notification.
func (p *BotPool) HandLimitNotified() bool {
	return p.handLimitNotified.Load()
}

func (p *BotPool) maybeNotifyHandLimit() {
	if p.handLimit == 0 {
		return
	}
	if atomic.LoadUint64(&p.handCounter) < p.handLimit {
		return
	}
	p.notifyGameCompleted(reasonHandLimitReached)
}

func (p *BotPool) notifyGameCompleted(reason string) {
	if reason == "" {
		reason = reasonHandLimitReached
	}
	if !p.handLimitNotified.CompareAndSwap(false, true) {
		return
	}
	p.completionReason.Store(reason)

	// Signal that the game is complete
	p.logger.Info().
		Str("reason", reason).
		Msg("Game completion triggered, signaling stop")

	completed := atomic.LoadUint64(&p.handCounter)

	// Notify monitors if present
	if p.progressMonitor != nil {
		p.progressMonitor.OnGameComplete(completed, reason)
	}
	if p.statsMonitor != nil {
		p.statsMonitor.OnGameComplete(completed, reason)
	}

	// Trigger shutdown asynchronously to avoid deadlock
	// (We're in the match loop, and Stop() waits for it to complete)
	go func() {
		p.Stop()
	}()

	playerStats := p.PlayerStats()
	players := make([]protocol.GameCompletedPlayer, len(playerStats))
	for i, ps := range playerStats {
		players[i] = ps.GameCompletedPlayer
	}

	msg := &protocol.GameCompleted{
		Type:           protocol.TypeGameCompleted,
		GameID:         p.GameID(),
		HandsCompleted: completed,
		HandLimit:      p.handLimit,
		Reason:         reason,
		Seed:           p.config.Seed,
		Players:        players,
	}

	// Snapshot bots to avoid holding locks during network sends
	p.mu.RLock()
	bots := make([]*Bot, 0, len(p.bots))
	for _, bot := range p.bots {
		bots = append(bots, bot)
	}
	p.mu.RUnlock()

	for _, bot := range bots {
		if bot == nil {
			continue
		}
		if err := bot.SendMessage(msg); err != nil {
			p.logger.Debug().Str("bot_id", bot.ID).Err(err).Msg("failed to send game_completed message")
		}
	}

	p.logger.Info().
		Str("game_id", msg.GameID).
		Uint64("hands_completed", msg.HandsCompleted).
		Uint64("hand_limit", msg.HandLimit).
		Str("reason", msg.Reason).
		Msg("Broadcasted game_completed message")
}

// PlayerStats captures aggregate performance metrics for a single bot within a game.
type PlayerStats struct {
	protocol.GameCompletedPlayer
	LastUpdated time.Time `json:"last_updated"`
}

// GameStats provides an aggregated snapshot for a game instance.
type GameStats struct {
	ID               string                         `json:"id"`
	SmallBlind       int                            `json:"small_blind"`
	BigBlind         int                            `json:"big_blind"`
	StartChips       int                            `json:"start_chips"`
	TimeoutMs        int                            `json:"timeout_ms"`
	MinPlayers       int                            `json:"min_players"`
	MaxPlayers       int                            `json:"max_players"`
	InfiniteBankroll bool                           `json:"infinite_bankroll"`
	HandsCompleted   uint64                         `json:"hands_completed"`
	HandLimit        uint64                         `json:"hand_limit"`
	HandsRemaining   uint64                         `json:"hands_remaining"`
	Timeouts         uint64                         `json:"timeouts"`
	HandsPerSecond   float64                        `json:"hands_per_second"`
	StartTime        time.Time                      `json:"start_time"`
	EndTime          time.Time                      `json:"end_time"`
	DurationSeconds  float64                        `json:"duration_seconds"`
	Seed             int64                          `json:"seed"`
	Players          []protocol.GameCompletedPlayer `json:"players"`
	CompletionReason string                         `json:"completion_reason"`
}
