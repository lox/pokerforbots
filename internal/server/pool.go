package server

import (
	"fmt"
	"sync"
	"time"
)

// BotPool manages available bots and matches them into hands
type BotPool struct {
	bots       map[string]*Bot
	available  chan *Bot
	register   chan *Bot
	unregister chan *Bot
	mu         sync.RWMutex
	minPlayers int
	maxPlayers int
}

// NewBotPool creates a new bot pool
func NewBotPool(minPlayers, maxPlayers int) *BotPool {
	return &BotPool{
		bots:       make(map[string]*Bot),
		available:  make(chan *Bot, 100),
		register:   make(chan *Bot),
		unregister: make(chan *Bot),
		minPlayers: minPlayers,
		maxPlayers: maxPlayers,
	}
}

// Run starts the bot pool manager
func (p *BotPool) Run() {
	matchTicker := time.NewTicker(100 * time.Millisecond)
	defer matchTicker.Stop()

	for {
		select {
		case bot := <-p.register:
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

		case bot := <-p.unregister:
			p.mu.Lock()
			if _, ok := p.bots[bot.ID]; ok {
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

	// Collect bots for the hand
	bots := make([]*Bot, 0, numPlayers)
	for i := 0; i < numPlayers; i++ {
		select {
		case bot := <-p.available:
			// Double-check bot is still connected and not in hand
			p.mu.RLock()
			_, connected := p.bots[bot.ID]
			p.mu.RUnlock()

			if connected && !bot.IsInHand() {
				bots = append(bots, bot)
			}
		default:
			// No more available bots
			break
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
			select {
			case p.available <- bot:
			default:
				// Queue full
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
	handID := fmt.Sprintf("hand-%d", time.Now().Unix())

	// Run the hand
	runner := NewHandRunner(bots, handID, 0)
	runner.Run()
}

// Register adds a bot to the pool
func (p *BotPool) Register(bot *Bot) {
	p.register <- bot
}

// Unregister removes a bot from the pool
func (p *BotPool) Unregister(bot *Bot) {
	p.unregister <- bot
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
