package server

import (
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lox/pokerforbots/internal/protocol"
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

	// Metrics
	timeoutCounter uint64
	handStartTime  time.Time
	metricsLock    sync.RWMutex

	statsMu   sync.RWMutex
	botStats  map[string]*botStats
	lastHand  string
	lastStamp time.Time

	// Statistics collector (optional)
	statsCollector StatsCollector
}

// WithRNG executes fn with exclusive access to the pool's RNG.
func (p *BotPool) WithRNG(fn func(*rand.Rand)) {
	p.rngMutex.Lock()
	defer p.rngMutex.Unlock()
	fn(p.rng)
}

type botStats struct {
	BotID       string
	DisplayName string
	Role        BotRole
	Hands       int
	NetChips    int64
	TotalWon    int64
	TotalLost   int64
	LastDelta   int
	LastUpdated time.Time
}

const reasonHandLimitReached = "hand_limit_reached"

// DefaultConfig returns a config with sensible defaults
func DefaultConfig(minPlayers, maxPlayers int) Config {
	return Config{
		SmallBlind:    5,
		BigBlind:      10,
		StartChips:    1000,
		Timeout:       100 * time.Millisecond,
		MinPlayers:    minPlayers,
		MaxPlayers:    maxPlayers,
		RequirePlayer: true,
		HandLimit:     0,
		Seed:          0,
	}
}

// NewBotPool creates a new bot pool with explicit random source and config
func NewBotPool(logger zerolog.Logger, rng *rand.Rand, config Config) *BotPool {
	// Create appropriate stats collector based on config
	var collector StatsCollector
	if config.EnableStats {
		maxHands := config.MaxStatsHands
		if maxHands <= 0 {
			maxHands = 10000 // Default limit
		}
		depth := config.StatsDepth
		if depth == "" {
			depth = StatsDepthBasic // Default to basic
		}
		collector = NewDetailedStatsCollector(depth, maxHands, config.BigBlind)
		logger.Info().
			Str("depth", string(depth)).
			Int("max_hands", maxHands).
			Msg("Statistics collection enabled")
	} else {
		collector = &NullStatsCollector{}
	}

	return &BotPool{
		bots:           make(map[string]*Bot),
		available:      make(chan *Bot, 100),
		register:       make(chan *Bot),
		unregister:     make(chan *Bot),
		minPlayers:     config.MinPlayers,
		maxPlayers:     config.MaxPlayers,
		handLimit:      config.HandLimit,
		stopCh:         make(chan struct{}),
		logger:         logger.With().Str("component", "pool").Logger(),
		rng:            rng,
		config:         config,
		handStartTime:  time.Now(),
		botStats:       make(map[string]*botStats),
		lastStamp:      time.Now(),
		matchTrigger:   make(chan struct{}, 1),
		statsCollector: collector,
	}
}

// SetGameID stores the identifier of the game this pool manages.
func (p *BotPool) SetGameID(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.gameID = id
}

// GameID returns the identifier associated with this pool.
func (p *BotPool) GameID() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.gameID
}

// Run starts the bot pool manager

func (p *BotPool) Run() {
	p.matcherWG.Add(1)
	go p.matchLoop()

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
			p.mu.Unlock()
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
		p.notifyGameCompleted(reasonHandLimitReached)
		return
	}

	// Count available bots
	availableCount := len(p.available)
	if availableCount < p.minPlayers {
		return
	}

	// Determine number of players for this hand
	numPlayers := availableCount
	if numPlayers > p.maxPlayers {
		numPlayers = p.maxPlayers
	}

	// Collect all available bots first for random selection
	allBots := make([]*Bot, 0, availableCount)
collectLoop:
	for i := 0; i < availableCount; i++ {
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
				p.logger.Debug().Str("bot_id", bot.ID).Msg("Bot out of chips, removing from pool")
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

	if p.config.RequirePlayer {
		hasPlayer := false
		for _, bot := range bots {
			if bot.Role() == BotRolePlayer {
				hasPlayer = true
				break
			}
		}
		if !hasPlayer {
			// No player present; return bots to available queue and wait
			for _, bot := range bots {
				select {
				case p.available <- bot:
				default:
				}
			}
			return
		}
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
			if bot.Role() == BotRoleNPC {
				continue
			}
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
	p.stopOnce.Do(func() {
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
	defer p.metricsLock.RUnlock()

	elapsed := time.Since(p.handStartTime).Seconds()
	if elapsed < 1.0 {
		return 0
	}

	handCount := float64(atomic.LoadUint64(&p.handCounter))
	return handCount / elapsed
}

// RecordHandOutcome updates aggregate statistics for the bots that participated in a hand.
func (p *BotPool) RecordHandOutcome(handID string, bots []*Bot, deltas []int) {
	if len(bots) != len(deltas) {
		return
	}

	now := time.Now()

	p.statsMu.Lock()

	if p.botStats == nil {
		p.botStats = make(map[string]*botStats)
	}

	p.lastHand = handID
	p.lastStamp = now

	for i, bot := range bots {
		if bot == nil {
			continue
		}

		key := bot.ID
		stats, exists := p.botStats[key]
		if !exists {
			stats = &botStats{BotID: key}
			p.botStats[key] = stats
		}

		displayName := bot.DisplayName()
		if displayName == "" {
			displayName = bot.ID
		}

		stats.DisplayName = displayName
		stats.Role = bot.Role()
		stats.Hands++

		delta := deltas[i]
		stats.NetChips += int64(delta)
		if delta >= 0 {
			stats.TotalWon += int64(delta)
		} else {
			stats.TotalLost += int64(-delta)
		}
		stats.LastDelta = delta
		stats.LastUpdated = now
	}

	p.statsMu.Unlock()

	p.maybeNotifyHandLimit()
}

// RecordHandOutcomeDetailed updates both basic and detailed statistics
func (p *BotPool) RecordHandOutcomeDetailed(detail HandOutcomeDetail) {
	// First update basic stats (for backward compatibility)
	if len(detail.BotOutcomes) > 0 {
		bots := make([]*Bot, len(detail.BotOutcomes))
		deltas := make([]int, len(detail.BotOutcomes))
		for i, outcome := range detail.BotOutcomes {
			bots[i] = outcome.Bot
			deltas[i] = outcome.NetChips
		}
		p.RecordHandOutcome(detail.HandID, bots, deltas)
	}

	// Then update detailed stats if collector is enabled
	if p.statsCollector != nil && p.statsCollector.IsEnabled() {
		if err := p.statsCollector.RecordHandOutcome(detail); err != nil {
			// Log error but don't fail - statistics should never break the game
			p.logger.Error().Err(err).Msg("Failed to record detailed hand statistics")
		}
	}
}

// PlayerStats returns a snapshot of aggregate statistics for all bots in the pool.
func (p *BotPool) PlayerStats() []PlayerStats {
	p.statsMu.RLock()
	defer p.statsMu.RUnlock()

	if len(p.botStats) == 0 {
		return []PlayerStats{}
	}

	players := make([]PlayerStats, 0, len(p.botStats))
	for _, stats := range p.botStats {
		role := string(stats.Role)
		avg := 0.0
		if stats.Hands > 0 {
			avg = float64(stats.NetChips) / float64(stats.Hands)
		}

		players = append(players, PlayerStats{
			BotID:       stats.BotID,
			DisplayName: stats.DisplayName,
			Role:        role,
			Hands:       stats.Hands,
			NetChips:    stats.NetChips,
			AvgPerHand:  avg,
			TotalWon:    stats.TotalWon,
			TotalLost:   stats.TotalLost,
			LastDelta:   stats.LastDelta,
			LastUpdated: stats.LastUpdated,
		})
	}

	sort.Slice(players, func(i, j int) bool {
		if players[i].Role == players[j].Role {
			if players[i].DisplayName == players[j].DisplayName {
				return players[i].BotID < players[j].BotID
			}
			return players[i].DisplayName < players[j].DisplayName
		}
		return players[i].Role < players[j].Role
	})

	return players
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

	playerStats := p.PlayerStats()
	players := make([]protocol.GameCompletedPlayer, len(playerStats))
	for i, ps := range playerStats {
		player := protocol.GameCompletedPlayer{
			BotID:       ps.BotID,
			DisplayName: ps.DisplayName,
			Role:        ps.Role,
			Hands:       ps.Hands,
			NetChips:    ps.NetChips,
			AvgPerHand:  ps.AvgPerHand,
			TotalWon:    ps.TotalWon,
			TotalLost:   ps.TotalLost,
			LastDelta:   ps.LastDelta,
		}

		// Add detailed stats if available
		if p.statsCollector != nil && p.statsCollector.IsEnabled() {
			if detailedStats := p.statsCollector.GetDetailedStats(ps.BotID); detailedStats != nil {
				player.DetailedStats = convertToProtocolStats(detailedStats)
			}
		}

		players[i] = player
	}

	completed := atomic.LoadUint64(&p.handCounter)

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

// convertToProtocolStats converts server DetailedStats to protocol PlayerDetailedStats
func convertToProtocolStats(stats *DetailedStats) *protocol.PlayerDetailedStats {
	if stats == nil {
		return nil
	}

	result := &protocol.PlayerDetailedStats{
		BB100:           stats.BB100,
		Mean:            stats.Mean,
		StdDev:          stats.StdDev,
		WinRate:         stats.WinRate,
		ShowdownWinRate: stats.ShowdownWinRate,
	}

	// Convert position stats
	if len(stats.PositionStats) > 0 {
		result.PositionStats = make(map[string]protocol.PositionStatSummary)
		for pos, stat := range stats.PositionStats {
			result.PositionStats[pos] = protocol.PositionStatSummary{
				Hands:     stat.Hands,
				NetBB:     stat.NetBB,
				BBPerHand: stat.BBPerHand,
			}
		}
	}

	// Convert street stats
	if len(stats.StreetStats) > 0 {
		result.StreetStats = make(map[string]protocol.StreetStatSummary)
		for street, stat := range stats.StreetStats {
			result.StreetStats[street] = protocol.StreetStatSummary{
				HandsEnded: stat.HandsEnded,
				NetBB:      stat.NetBB,
				BBPerHand:  stat.BBPerHand,
			}
		}
	}

	// Convert hand category stats
	if len(stats.HandCategoryStats) > 0 {
		result.HandCategoryStats = make(map[string]protocol.CategoryStatSummary)
		for cat, stat := range stats.HandCategoryStats {
			result.HandCategoryStats[cat] = protocol.CategoryStatSummary{
				Hands:     stat.Hands,
				NetBB:     stat.NetBB,
				BBPerHand: stat.BBPerHand,
			}
		}
	}

	return result
}

// PlayerStats captures aggregate performance metrics for a single bot within a game.
type PlayerStats struct {
	BotID       string    `json:"bot_id"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	Hands       int       `json:"hands"`
	NetChips    int64     `json:"net_chips"`
	AvgPerHand  float64   `json:"avg_per_hand"`
	TotalWon    int64     `json:"total_won"`
	TotalLost   int64     `json:"total_lost"`
	LastDelta   int       `json:"last_delta"`
	LastUpdated time.Time `json:"last_updated"`
}

// GameStats provides an aggregated snapshot for a game instance.
type GameStats struct {
	ID               string        `json:"id"`
	SmallBlind       int           `json:"small_blind"`
	BigBlind         int           `json:"big_blind"`
	StartChips       int           `json:"start_chips"`
	TimeoutMs        int           `json:"timeout_ms"`
	MinPlayers       int           `json:"min_players"`
	MaxPlayers       int           `json:"max_players"`
	RequirePlayer    bool          `json:"require_player"`
	InfiniteBankroll bool          `json:"infinite_bankroll"`
	HandsCompleted   uint64        `json:"hands_completed"`
	HandLimit        uint64        `json:"hand_limit"`
	HandsRemaining   uint64        `json:"hands_remaining"`
	Timeouts         uint64        `json:"timeouts"`
	HandsPerSecond   float64       `json:"hands_per_second"`
	Seed             int64         `json:"seed"`
	Players          []PlayerStats `json:"players"`
}
