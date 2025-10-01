package server

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/lox/pokerforbots/v2/protocol"
)

const responseReservoirSize = 128

// ResponseOutcome categorizes how an action request completed from the server's perspective.
type ResponseOutcome int

const (
	ResponseOutcomeSuccess ResponseOutcome = iota
	ResponseOutcomeTimeout
	ResponseOutcomeDisconnect
)

// BotStatistics tracks statistics for a single bot
type BotStatistics struct {
	mu              sync.RWMutex
	bigBlind        int
	hands           int
	sumBB           float64
	sumBB2          float64   // Sum of squares for variance
	values          []float64 // All BB results for median/percentile
	winningHands    int
	showdownWins    int
	nonShowdownWins int
	showdownLosses  int
	showdownBB      float64
	nonShowdownBB   float64
	vpipHands       int // Number of hands where player voluntarily put money in pot
	pfrHands        int // Number of hands where player raised preflop
	preflopHands    int // Number of hands where player had opportunity to act preflop
	timeoutCount    int
	bustCount       int
	lastVPIPHand    string // Track last hand where VPIP was counted
	lastPFRHand     string // Track last hand where PFR was counted

	responseCount       int
	responseSum         float64
	responseSumSquares  float64
	responseMin         float64
	responseMax         float64
	responseSamples     []float64
	responseSampleIndex int
	responseTimeouts    int
	responseDisconnects int
}

// NewBotStatistics creates a new BotStatistics instance
func NewBotStatistics(bigBlind int) *BotStatistics {
	return &BotStatistics{
		bigBlind: bigBlind,
		values:   make([]float64, 0),
		// Initialise min so the first observation always wins the comparison.
		responseMin: math.MaxFloat64,
	}
}

// AddResult incorporates a new hand result
func (b *BotStatistics) AddResult(netBB float64, wentToShowdown, wonAtShowdown bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.hands++
	b.sumBB += netBB
	b.sumBB2 += netBB * netBB
	b.values = append(b.values, netBB)

	// Track wins/losses
	if netBB > 0 {
		b.winningHands++
		if wentToShowdown {
			b.showdownWins++
		} else {
			b.nonShowdownWins++
		}
	} else if netBB < 0 && wentToShowdown {
		b.showdownLosses++
	}

	// Track showdown vs non-showdown BB
	if wentToShowdown {
		b.showdownBB += netBB
	} else {
		b.nonShowdownBB += netBB
	}
}

// RecordHandStart increments the count of hands where the player could act preflop
// Must be called without holding the lock
func (b *BotStatistics) RecordHandStart() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.preflopHands++
}

// RecordPreflopActionWithContext updates VPIP/PFR counters based on the action taken.
// The isRaise parameter indicates whether this action actually raised the betting (important for all-ins).
func (b *BotStatistics) RecordPreflopActionWithContext(action string, handID string, isRaise bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch action {
	case "call", "raise", "allin", "bet":
		// Only count VPIP once per hand
		if b.lastVPIPHand != handID {
			b.vpipHands++
			b.lastVPIPHand = handID
		}
		// Only count PFR once per hand - use isRaise flag to determine if it's actually aggressive
		if isRaise && b.lastPFRHand != handID {
			b.pfrHands++
			b.lastPFRHand = handID
		}
	}
}

// RecordTimeout increments the timeout counter for the bot.
func (b *BotStatistics) RecordTimeout() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.timeoutCount++
}

// RecordBust increments the bust counter for the bot.
func (b *BotStatistics) RecordBust() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.bustCount++
}

// RecordResponse records latency information for an action request.
func (b *BotStatistics) RecordResponse(duration time.Duration, outcome ResponseOutcome) {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch outcome {
	case ResponseOutcomeSuccess:
		ms := float64(duration) / float64(time.Millisecond)
		if ms < 0 {
			ms = 0
		}
		b.responseCount++
		b.responseSum += ms
		b.responseSumSquares += ms * ms
		if b.responseMin == math.MaxFloat64 || ms < b.responseMin {
			b.responseMin = ms
		}
		if ms > b.responseMax {
			b.responseMax = ms
		}
		if b.responseSamples == nil {
			b.responseSamples = make([]float64, 0, responseReservoirSize)
		}
		if len(b.responseSamples) < responseReservoirSize {
			b.responseSamples = append(b.responseSamples, ms)
		} else {
			b.responseSamples[b.responseSampleIndex] = ms
			b.responseSampleIndex = (b.responseSampleIndex + 1) % responseReservoirSize
		}
	case ResponseOutcomeTimeout:
		b.responseTimeouts++
	case ResponseOutcomeDisconnect:
		b.responseDisconnects++
	}
}

// ToProtocolStats converts to protocol.PlayerDetailedStats
func (b *BotStatistics) ToProtocolStats() *protocol.PlayerDetailedStats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.hands == 0 {
		return nil
	}

	mean := b.sumBB / float64(b.hands)
	bb100 := mean * 100

	result := &protocol.PlayerDetailedStats{
		Hands:           b.hands,
		NetBB:           b.sumBB,
		BB100:           bb100,
		Mean:            mean,
		Median:          b.median(),
		StdDev:          b.stdDev(),
		WinningHands:    b.winningHands,
		WinRate:         float64(b.winningHands) / float64(b.hands) * 100,
		ShowdownWins:    b.showdownWins,
		NonShowdownWins: b.nonShowdownWins,
		ShowdownBB:      b.showdownBB,
		NonShowdownBB:   b.nonShowdownBB,
	}

	// Calculate VPIP/PFR based on preflop opportunities, not total hands
	if b.preflopHands > 0 {
		result.VPIP = float64(b.vpipHands) / float64(b.preflopHands) * 100
		result.PFR = float64(b.pfrHands) / float64(b.preflopHands) * 100
	}

	result.Timeouts = b.timeoutCount
	result.Busts = b.bustCount

	// Calculate CI if we have enough hands
	if b.hands >= 30 {
		se := b.stdError()
		margin := 1.96 * se
		result.CI95Low = (mean - margin) * 100
		result.CI95High = (mean + margin) * 100
	}

	// Calculate showdown win rate
	showdownsTotal := b.showdownWins + b.showdownLosses
	if showdownsTotal > 0 {
		result.ShowdownWinRate = float64(b.showdownWins) / float64(showdownsTotal) * 100
	}

	result.ResponseTimeouts = b.responseTimeouts
	result.ResponseDisconnects = b.responseDisconnects
	result.ResponsesTracked = b.responseCount

	if b.responseCount > 0 {
		avg := b.responseSum / float64(b.responseCount)
		result.AvgResponseMs = avg
		if b.responseCount > 1 {
			variance := b.responseSumSquares/float64(b.responseCount) - (avg * avg)
			if variance < 0 {
				variance = 0
			}
			result.ResponseStdMs = math.Sqrt(variance)
		}
		if b.responseMax > 0 {
			result.MaxResponseMs = b.responseMax
		}
		if b.responseMin != math.MaxFloat64 {
			result.MinResponseMs = b.responseMin
		}
		if p95 := b.responsePercentileLocked(0.95); p95 > 0 {
			result.P95ResponseMs = p95
		}
	}

	return result
}

// Hands returns the total number of hands
func (b *BotStatistics) Hands() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.hands
}

// Mean returns the arithmetic mean in BB/hand
func (b *BotStatistics) Mean() float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.hands == 0 {
		return 0
	}
	return b.sumBB / float64(b.hands)
}

// BB100 returns big blinds per 100 hands
func (b *BotStatistics) BB100() float64 {
	return b.Mean() * 100
}

// median returns the median BB/hand (internal, called with lock held)
func (b *BotStatistics) median() float64 {
	if len(b.values) == 0 {
		return 0
	}
	sorted := make([]float64, len(b.values))
	copy(sorted, b.values)
	sort.Float64s(sorted)

	n := len(sorted)
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

// responsePercentileLocked returns the percentile of recorded response samples. Caller must hold the lock.
func (b *BotStatistics) responsePercentileLocked(p float64) float64 {
	if len(b.responseSamples) == 0 {
		return 0
	}
	samples := make([]float64, len(b.responseSamples))
	copy(samples, b.responseSamples)
	sort.Float64s(samples)
	idx := int(math.Ceil(p*float64(len(samples)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(samples) {
		idx = len(samples) - 1
	}
	return samples[idx]
}

// variance returns the sample variance (internal, called with lock held)
func (b *BotStatistics) variance() float64 {
	if b.hands < 2 {
		return 0
	}
	mean := b.sumBB / float64(b.hands)
	return (b.sumBB2 - float64(b.hands)*mean*mean) / float64(b.hands-1)
}

// stdDev returns the sample standard deviation (internal, called with lock held)
func (b *BotStatistics) stdDev() float64 {
	return math.Sqrt(b.variance())
}

// stdError returns the standard error of the mean (internal, called with lock held)
func (b *BotStatistics) stdError() float64 {
	if b.hands == 0 {
		return 0
	}
	return b.stdDev() / math.Sqrt(float64(b.hands))
}

// GetPositionName returns friendly name for button distance
func GetPositionName(distance int) string {
	switch distance {
	case 0:
		return "Button"
	case 1:
		return "Cutoff"
	case 2:
		return "Hijack"
	case 3:
		return "MP"
	case 4:
		return "EP2"
	case 5:
		return "EP1"
	default:
		return fmt.Sprintf("Pos%d", distance)
	}
}

// BasicBotStats tracks lightweight per-bot aggregates.
type BasicBotStats struct {
	BotID          string
	DisplayName    string
	BotCommand     string
	ConnectOrder   int
	Hands          int
	NetChips       int64
	TotalWon       int64
	TotalLost      int64
	Timeouts       int
	InvalidActions int
	Disconnects    int
	Busts          int
	LastDelta      int
	LastUpdated    time.Time
}

// StatsMonitor collects both basic and detailed statistics and satisfies HandMonitor and StatsProvider.
type StatsMonitor struct {
	mu             sync.RWMutex
	basicStats     map[string]*BasicBotStats
	detailedStats  map[string]*BotStatistics
	enableDetailed bool
	bigBlind       int
	maxHands       int
	currentHands   int
	currentStreet  string          // Track current street for VPIP/PFR
	seatToBotID    map[int]string  // Map seat to bot ID for current hand
	handStarted    map[string]bool // Track if bot has started the hand (for VPIP/PFR denominator)
	seatBets       map[int]int     // Track current bet per seat to distinguish raise vs call all-ins
	highestBet     int             // Track highest bet in current betting round
}

// NewStatsMonitor creates a new statistics monitor.
func NewStatsMonitor(bigBlind int, enableDetailed bool, maxHands int) *StatsMonitor {
	monitor := &StatsMonitor{
		basicStats:     make(map[string]*BasicBotStats),
		enableDetailed: enableDetailed,
		bigBlind:       bigBlind,
		maxHands:       maxHands,
		seatToBotID:    make(map[int]string),
		handStarted:    make(map[string]bool),
		seatBets:       make(map[int]int),
	}
	if enableDetailed {
		monitor.detailedStats = make(map[string]*BotStatistics)
	}
	return monitor
}

// OnGameStart resets per-game counters if necessary.
func (s *StatsMonitor) OnGameStart(uint64) {}

// OnGameComplete currently performs no cleanup; stats remain available for querying.
func (s *StatsMonitor) OnGameComplete(uint64, string) {}

// OnHandStart implements HandMonitor.
func (s *StatsMonitor) OnHandStart(handID string, players []HandPlayer, button int, blinds Blinds) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Reset hand-specific tracking
	s.currentStreet = "preflop"
	s.seatToBotID = make(map[int]string)
	s.handStarted = make(map[string]bool)
	s.seatBets = make(map[int]int)
	// Initialize highestBet to big blind since blinds are already posted
	s.highestBet = blinds.Big

	// Map seats to bot IDs and track hand starts for VPIP/PFR calculation
	for _, player := range players {
		botID := player.Name
		s.seatToBotID[player.Seat] = botID
		s.handStarted[botID] = true

		// Track that this bot has started a hand (for VPIP/PFR denominator)
		if s.enableDetailed {
			if s.detailedStats[botID] == nil {
				s.detailedStats[botID] = NewBotStatistics(s.bigBlind)
			}
			s.detailedStats[botID].RecordHandStart()
		}
	}
}

// OnPlayerAction implements HandMonitor.
func (s *StatsMonitor) OnPlayerAction(handID string, seat int, action string, amount int, stack int) {
	if !s.enableDetailed {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Only track preflop actions for VPIP/PFR
	if s.currentStreet != "preflop" {
		return
	}

	// Get bot ID from seat
	botID, ok := s.seatToBotID[seat]
	if !ok {
		return
	}

	// Get or create detailed stats for this bot
	detailed := s.detailedStats[botID]
	if detailed == nil {
		detailed = NewBotStatistics(s.bigBlind)
		s.detailedStats[botID] = detailed
	}

	// Handle blind posting to track initial bets
	if action == "post_small_blind" || action == "post_big_blind" {
		// Track the blind amounts
		s.seatBets[seat] = amount
		if action == "post_big_blind" {
			s.highestBet = amount
		}
		return
	}

	// Track preflop action (excludes posting blinds)
	// amount is the delta (additional chips put in), not the total bet
	// We need to track cumulative contributions
	currentBet := s.seatBets[seat]
	newTotalBet := currentBet + amount
	s.seatBets[seat] = newTotalBet

	// Determine if this action increases the bet (for PFR tracking)
	isRaise := false

	switch action {
	case "bet":
		// Bet is always an aggressive action (opening the betting)
		isRaise = true
		s.highestBet = newTotalBet
	case "raise":
		// Raise is always aggressive
		isRaise = true
		s.highestBet = newTotalBet
	case "call":
		// Call matches the current bet - no need to update highestBet
		// The newTotalBet should equal highestBet for a true call
	case "allin":
		// All-in could be a call or raise depending on total amount
		if newTotalBet > s.highestBet {
			isRaise = true
			s.highestBet = newTotalBet
		}
	}

	detailed.RecordPreflopActionWithContext(action, handID, isRaise)
}

// OnStreetChange implements HandMonitor.
func (s *StatsMonitor) OnStreetChange(handID string, street string, cards []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentStreet = street
	// Reset bet tracking for new betting round
	s.seatBets = make(map[int]int)
	s.highestBet = 0
}

// OnHandComplete records the provided outcome and updates aggregates.
func (s *StatsMonitor) OnHandComplete(outcome HandOutcome) {
	if outcome.Detail == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.maxHands > 0 && s.currentHands >= s.maxHands {
		s.resetLocked()
	}

	now := time.Now()

	for _, botOutcome := range outcome.Detail.BotOutcomes {
		if botOutcome.Bot == nil {
			continue
		}

		botID := botOutcome.Bot.ID
		stats := s.basicStats[botID]
		if stats == nil {
			stats = &BasicBotStats{
				BotID:        botID,
				ConnectOrder: len(s.basicStats) + 1,
			}
			s.basicStats[botID] = stats
		}

		displayName := botOutcome.Bot.DisplayName()
		if displayName == "" {
			displayName = botID
		}
		stats.DisplayName = displayName
		stats.BotCommand = botOutcome.Bot.BotCommand()
		stats.Hands++
		stats.NetChips += int64(botOutcome.NetChips)
		if botOutcome.NetChips >= 0 {
			stats.TotalWon += int64(botOutcome.NetChips)
		} else {
			stats.TotalLost += int64(-botOutcome.NetChips)
		}
		stats.LastDelta = botOutcome.NetChips
		stats.LastUpdated = now
		if botOutcome.TimedOut {
			stats.Timeouts++
		}
		stats.InvalidActions += botOutcome.InvalidActions
		if botOutcome.Disconnected {
			stats.Disconnects++
		}
		if botOutcome.WentBroke {
			stats.Busts++
		}
	}

	if s.enableDetailed && s.bigBlind > 0 {
		for _, botOutcome := range outcome.Detail.BotOutcomes {
			if botOutcome.Bot == nil {
				continue
			}
			botID := botOutcome.Bot.ID
			detailed := s.detailedStats[botID]
			if detailed == nil {
				detailed = NewBotStatistics(s.bigBlind)
				s.detailedStats[botID] = detailed
			}
			netBB := float64(botOutcome.NetChips) / float64(s.bigBlind)
			detailed.AddResult(netBB, botOutcome.WentToShowdown, botOutcome.WonAtShowdown)
		}
	}

	s.currentHands++
}

// GetPlayerStats returns a deterministic snapshot of player statistics.
func (s *StatsMonitor) GetPlayerStats() []PlayerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.basicStats) == 0 {
		return nil
	}

	ordered := make([]*BasicBotStats, 0, len(s.basicStats))
	for _, stats := range s.basicStats {
		ordered = append(ordered, stats)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].ConnectOrder == ordered[j].ConnectOrder {
			return ordered[i].BotID < ordered[j].BotID
		}
		return ordered[i].ConnectOrder < ordered[j].ConnectOrder
	})

	players := make([]PlayerStats, 0, len(ordered))
	for _, stats := range ordered {
		avg := 0.0
		if stats.Hands > 0 {
			avg = float64(stats.NetChips) / float64(stats.Hands)
		}

		ps := PlayerStats{
			GameCompletedPlayer: protocol.GameCompletedPlayer{
				BotID:          stats.BotID,
				DisplayName:    stats.DisplayName,
				Hands:          stats.Hands,
				NetChips:       stats.NetChips,
				AvgPerHand:     avg,
				TotalWon:       stats.TotalWon,
				TotalLost:      stats.TotalLost,
				LastDelta:      stats.LastDelta,
				Timeouts:       stats.Timeouts,
				InvalidActions: stats.InvalidActions,
				Disconnects:    stats.Disconnects,
				Busts:          stats.Busts,
			},
			LastUpdated: stats.LastUpdated,
		}

		if s.enableDetailed {
			if detail := s.detailedStats[stats.BotID]; detail != nil {
				if protoStats := detail.ToProtocolStats(); protoStats != nil {
					protoStats.Timeouts = stats.Timeouts
					protoStats.Busts = stats.Busts
					ps.DetailedStats = protoStats
				}
			}
		}

		players = append(players, ps)
	}

	return players
}

// GetDetailedStats returns comprehensive statistics for a specific bot.
func (s *StatsMonitor) GetDetailedStats(botID string) *protocol.PlayerDetailedStats {
	if !s.enableDetailed {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	detailed := s.detailedStats[botID]
	if detailed == nil {
		return nil
	}

	protoStats := detailed.ToProtocolStats()
	if protoStats == nil {
		return nil
	}

	if basic := s.basicStats[botID]; basic != nil {
		protoStats.Timeouts = basic.Timeouts
		protoStats.Busts = basic.Busts
	}

	return protoStats
}

// Reset clears all collected statistics.
func (s *StatsMonitor) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetLocked()
}

func (s *StatsMonitor) resetLocked() {
	s.basicStats = make(map[string]*BasicBotStats)
	s.currentHands = 0
	if s.enableDetailed {
		s.detailedStats = make(map[string]*BotStatistics)
	}
}

// RecordResponse captures latency metrics for a bot response.
func (s *StatsMonitor) RecordResponse(botID string, duration time.Duration, outcome ResponseOutcome) {
	if !s.enableDetailed {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	detailed := s.detailedStats[botID]
	if detailed == nil {
		detailed = NewBotStatistics(s.bigBlind)
		s.detailedStats[botID] = detailed
	}
	detailed.RecordResponse(duration, outcome)
}
