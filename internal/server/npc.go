package server

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lox/pokerforbots/protocol"
	"github.com/rs/zerolog"
)

// NPCSpec describes the NPC bots to attach to a game instance via the admin API.
type NPCSpec struct {
	Strategy string `json:"strategy"`
	Count    int    `json:"count"`
}

type npcStrategy interface {
	Name() string
	Decide(req *protocol.ActionRequest, st npcState, rng *rand.Rand) (action string, amount int)
}

type npcState struct {
	Chips int
}

type npcBot struct {
	bot      *Bot
	strategy npcStrategy
	rng      *rand.Rand
	stopCh   chan struct{}
	logger   zerolog.Logger
	chips    int
	seat     int
}

func newNPCBot(logger zerolog.Logger, pool *BotPool, gameID string, strategy npcStrategy) *npcBot {
	id := fmt.Sprintf("npc-%s-%s", strategy.Name(), uuid.NewString()[:8])
	bot := NewBot(logger, id, nil, pool)
	bot.SetDisplayName(id)
	bot.SetGameID(gameID)
	bot.SetRole(BotRoleNPC)

	return &npcBot{
		bot:      bot,
		strategy: strategy,
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
		stopCh:   make(chan struct{}),
		logger:   logger.With().Str("component", "npc").Str("npc_id", id).Logger(),
		chips:    pool.config.StartChips,
		seat:     -1,
	}
}

func (n *npcBot) start() {
	n.pool().Register(n.bot)
	go n.loop()
}

func (n *npcBot) stop() {
	close(n.stopCh)
	n.pool().Unregister(n.bot)
}

func (n *npcBot) pool() *BotPool {
	return n.bot.pool
}

func (n *npcBot) loop() {
	for {
		select {
		case data, ok := <-n.bot.send:
			if !ok {
				return
			}
			n.handleMessage(data)
		case <-n.bot.Done():
			return
		case <-n.stopCh:
			return
		}
	}
}

func (n *npcBot) handleMessage(data []byte) {
	var handStart protocol.HandStart
	if err := protocol.Unmarshal(data, &handStart); err == nil && handStart.Type == protocol.TypeHandStart {
		if handStart.YourSeat >= 0 && handStart.YourSeat < len(handStart.Players) {
			n.seat = handStart.YourSeat
			n.chips = handStart.Players[handStart.YourSeat].Chips
		}
		return
	}

	var gameUpdate protocol.GameUpdate
	if err := protocol.Unmarshal(data, &gameUpdate); err == nil && gameUpdate.Type == protocol.TypeGameUpdate {
		if n.seat >= 0 && n.seat < len(gameUpdate.Players) {
			n.chips = gameUpdate.Players[n.seat].Chips
		}
		return
	}

	var actionReq protocol.ActionRequest
	if err := protocol.Unmarshal(data, &actionReq); err == nil && actionReq.Type == protocol.TypeActionRequest {
		n.handleActionRequest(&actionReq)
		return
	}
}

func (n *npcBot) handleActionRequest(req *protocol.ActionRequest) {
	state := npcState{Chips: n.chips}
	action, amount := n.strategy.Decide(req, state, n.rng)

	if amount > n.chips {
		amount = n.chips
	}

	envelope := ActionEnvelope{
		BotID: n.bot.ID,
		Action: protocol.Action{
			Type:   protocol.TypeAction,
			Action: action,
			Amount: amount,
		},
	}

	n.bot.handRunnerMu.RLock()
	ch := n.bot.actionChan
	n.bot.handRunnerMu.RUnlock()
	if ch == nil {
		return
	}

	select {
	case ch <- envelope:
	default:
		n.logger.Warn().Str("action", action).Msg("failed to deliver NPC action (channel full)")
	}
}

// Strategies

type callingStationStrategy struct{}

type aggressiveStrategy struct{}

type randomStrategy struct{}

func (callingStationStrategy) Name() string { return "calling" }

func (callingStationStrategy) Decide(req *protocol.ActionRequest, st npcState, rng *rand.Rand) (string, int) {
	for _, action := range req.ValidActions {
		if action == "check" {
			return "check", 0
		}
	}
	for _, action := range req.ValidActions {
		if action == "call" {
			return "call", 0
		}
	}
	return "fold", 0
}

func (aggressiveStrategy) Name() string { return "aggressive" }

func (aggressiveStrategy) Decide(req *protocol.ActionRequest, st npcState, rng *rand.Rand) (string, int) {
	if rng.Float32() < 0.7 {
		for _, action := range req.ValidActions {
			if action == "raise" {
				minRequired := req.MinBet
				if req.MinRaise > minRequired {
					minRequired = req.MinRaise
				}
				amount := minRequired
				if req.Pot > 0 {
					amount = req.Pot * (2 + rng.Intn(2))
				}
				if amount < minRequired {
					amount = minRequired
				}
				if amount > st.Chips {
					amount = st.Chips
				}
				if amount <= 0 {
					amount = st.Chips
				}
				if amount <= 0 {
					return "fold", 0
				}
				// If stack is below min required, prefer all-in (if allowed) or fallback to call/check
				if st.Chips < minRequired {
					for _, a := range req.ValidActions {
						if a == "allin" {
							return "allin", 0
						}
					}
					for _, a := range req.ValidActions {
						if a == "call" {
							return "call", 0
						}
					}
					for _, a := range req.ValidActions {
						if a == "check" {
							return "check", 0
						}
					}
					return "fold", 0
				}
				return "raise", amount
			}
		}
		for _, action := range req.ValidActions {
			if action == "allin" {
				return "allin", 0
			}
		}
	}
	for _, action := range req.ValidActions {
		if action == "call" {
			return "call", 0
		}
	}
	for _, action := range req.ValidActions {
		if action == "check" {
			return "check", 0
		}
	}
	return "fold", 0
}

func (randomStrategy) Name() string { return "random" }

func (randomStrategy) Decide(req *protocol.ActionRequest, st npcState, rng *rand.Rand) (string, int) {
	if len(req.ValidActions) == 0 {
		return "fold", 0
	}
	action := req.ValidActions[rng.Intn(len(req.ValidActions))]
	if action == "raise" {
		min := req.MinBet
		if req.MinRaise > min {
			min = req.MinRaise
		}
		max := st.Chips
		if max < min {
			return "allin", 0
		}
		amount := min
		if max > min {
			amount = min + rng.Intn(max-min+1)
		}
		return "raise", amount
	}
	return action, 0
}

func resolveStrategy(name string) npcStrategy {
	switch strings.ToLower(name) {
	case "calling", "calling-station", "station", "call":
		return callingStationStrategy{}
	case "aggressive", "aggro":
		return aggressiveStrategy{}
	case "random", "rand":
		return randomStrategy{}
	default:
		return randomStrategy{}
	}
}
