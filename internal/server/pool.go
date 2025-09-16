package server

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

// BotPool manages available bots and matches them into hands
type BotPool struct {
	bots            map[string]*Bot
	available       chan *Bot
	register        chan *Bot
	unregister      chan *Bot
	mu              sync.RWMutex
	minPlayers      int
	maxPlayers      int
	handCounter     uint64
	handLimit       uint64 // 0 means unlimited
	handLimitLogged bool   // Track if we've logged the hand limit message
	stopCh          chan struct{}
	stopOnce        sync.Once
	logger          zerolog.Logger
	rng             *rand.Rand
	rngMutex        sync.Mutex // Protect RNG access
}

// NewBotPool creates a new bot pool with explicit random source
func NewBotPool(logger zerolog.Logger, minPlayers, maxPlayers int, rng *rand.Rand) *BotPool {
	return NewBotPoolWithLimit(logger, minPlayers, maxPlayers, rng, 0) // 0 = unlimited
}

// NewBotPoolWithLimit creates a new bot pool with explicit random source and hand limit
func NewBotPoolWithLimit(logger zerolog.Logger, minPlayers, maxPlayers int, rng *rand.Rand, handLimit uint64) *BotPool {
	return &BotPool{
		bots:       make(map[string]*Bot),
		available:  make(chan *Bot, 100),
		register:   make(chan *Bot),
		unregister: make(chan *Bot),
		minPlayers: minPlayers,
		maxPlayers: maxPlayers,
		handLimit:  handLimit,
		stopCh:     make(chan struct{}),
		logger:     logger.With().Str("component", "pool").Logger(),
		rng:        rng,
	}
}

// Run starts the bot pool manager
func (p *BotPool) Run() {
	matchTicker := time.NewTicker(100 * time.Millisecond)
	defer matchTicker.Stop()

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
			}

		case bot, ok := <-p.unregister:
			if !ok || bot == nil {
				continue
			}
			p.mu.Lock()
			if _, exists := p.bots[bot.ID]; exists {
				delete(p.bots, bot.ID)
				close(bot.send)
			}
			p.mu.Unlock()

		case <-matchTicker.C:
			p.tryMatch()
		}
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
		if !p.handLimitLogged {
			p.handLimitLogged = true
			p.logger.Info().Uint64("hands_completed", atomic.LoadUint64(&p.handCounter)).
				Uint64("hand_limit", p.handLimit).
				Msg("Hand limit reached - stopping new hand creation")
		}
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
	for i := 0; i < availableCount; i++ {
		select {
		case <-p.stopCh:
			return
		case bot := <-p.available:
			// Double-check bot is still connected and not in hand
			p.mu.RLock()
			_, connected := p.bots[bot.ID]
			p.mu.RUnlock()

			if connected && !bot.IsInHand() && bot.HasChips() {
				allBots = append(allBots, bot)
			} else if connected && !bot.HasChips() {
				// Bot is out of chips, remove from pool
				p.logger.Info().Str("bot_id", bot.ID).Msg("Bot out of chips, removing from pool")
				p.Unregister(bot)
			} else {
				// Return disconnected bot to available queue if it's valid
				if connected {
					select {
					case p.available <- bot:
					default:
					}
				}
			}
		default:
			// No more available bots
			break
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

	// Return unused bots to available queue
	for i := numPlayers; i < len(allBots); i++ {
		select {
		case p.available <- allBots[i]:
		default:
			// Queue full
		}
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
				p.logger.Info().Str("bot_id", bot.ID).Msg("Bot out of chips after hand, removing from pool")
				p.Unregister(bot)
			}
		}
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

	// Random button position for stateless hands with mutex protection
	p.rngMutex.Lock()
	button := p.rng.Intn(len(bots))
	// Generate a separate RNG for the hand runner to avoid continued concurrent access
	handRNGSeed := p.rng.Int63()
	p.rngMutex.Unlock()

	handRNG := rand.New(rand.NewSource(handRNGSeed))
	p.logger.Info().
		Str("hand_id", handID).
		Int("button_position", button).
		Int("player_count", len(bots)).
		Msg("Hand starting with random button position")

	// Run the hand with the cloned RNG
	runner := NewHandRunner(p.logger, bots, handID, button, handRNG)
	runner.Run()

	p.logger.Info().
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
