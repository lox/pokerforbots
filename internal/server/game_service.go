package server

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/lox/holdem-cli/internal/bot"
	"github.com/lox/holdem-cli/internal/game"
)

// GameTable represents a poker table managed by the server
type GameTable struct {
	ID            string
	Name          string
	MaxPlayers    int
	SmallBlind    int
	BigBlind      int
	engine        *game.GameEngine
	players       map[string]*game.Player  // playerName -> Player
	networkAgents map[string]*NetworkAgent // playerName -> NetworkAgent for remote players
	botAgents     map[string]game.Agent    // playerName -> Bot agent for AI players
	status        string                   // "waiting", "active", "finished"
	logger        *log.Logger
	eventSub      *TableEventSubscriber
}

// TableEventSubscriber handles game events and forwards them to clients
type TableEventSubscriber struct {
	table  *GameTable
	server *Server
	logger *log.Logger
}

// OnEvent implements the EventSubscriber interface
func (tes *TableEventSubscriber) OnEvent(event game.GameEvent) {
	tes.logger.Debug("Processing game event", "type", event.EventType(), "table", tes.table.ID)

	switch e := event.(type) {
	case game.HandStartEvent:
		tes.handleHandStart(e)
	case game.PlayerActionEvent:
		tes.handlePlayerAction(e)
	case game.StreetChangeEvent:
		tes.handleStreetChange(e)
	case game.HandEndEvent:
		tes.handleHandEnd(e)
	}
}

func (tes *TableEventSubscriber) handleHandStart(event game.HandStartEvent) {
	// Convert players to message format
	players := make([]PlayerState, len(event.Players))
	for i, p := range event.Players {
		// Only include hole cards for the player themselves
		players[i] = PlayerStateFromGame(p, false) // Server will send hole cards separately
	}

	data := HandStartData{
		HandID:     event.HandID,
		Players:    players,
		SmallBlind: event.SmallBlind,
		BigBlind:   event.BigBlind,
		InitialPot: event.InitialPot,
		DealerSeat: tes.table.getDealerSeat(),
	}

	msg, err := NewMessage("hand_start", data)
	if err != nil {
		tes.logger.Error("Failed to create hand start message", "error", err)
		return
	}

	tes.server.BroadcastToTable(tes.table.ID, msg)

	// Send hole cards to each player individually
	for _, p := range event.Players {
		if len(p.HoleCards) > 0 {
			// Create player-specific hand start with hole cards
			playerData := data
			for j, ps := range playerData.Players {
				if ps.Name == p.Name {
					playerData.Players[j].HoleCards = p.HoleCards
					break
				}
			}

			playerMsg, err := NewMessage("hand_start", playerData)
			if err != nil {
				tes.logger.Error("Failed to create player-specific hand start message", "error", err)
				continue
			}

			tes.server.SendToPlayer(p.Name, playerMsg)
		}
	}
}

func (tes *TableEventSubscriber) handlePlayerAction(event game.PlayerActionEvent) {
	data := PlayerActionData{
		Player:    event.Player.Name,
		Action:    event.Action.String(),
		Amount:    event.Amount,
		PotAfter:  event.PotAfter,
		Round:     event.Round.String(),
		Reasoning: event.Reasoning,
	}

	msg, err := NewMessage("player_action", data)
	if err != nil {
		tes.logger.Error("Failed to create player action message", "error", err)
		return
	}

	tes.server.BroadcastToTable(tes.table.ID, msg)
}

func (tes *TableEventSubscriber) handleStreetChange(event game.StreetChangeEvent) {
	data := StreetChangeData{
		Round:          event.Round.String(),
		CommunityCards: event.CommunityCards,
		CurrentBet:     event.CurrentBet,
	}

	msg, err := NewMessage("street_change", data)
	if err != nil {
		tes.logger.Error("Failed to create street change message", "error", err)
		return
	}

	tes.server.BroadcastToTable(tes.table.ID, msg)
}

func (tes *TableEventSubscriber) handleHandEnd(event game.HandEndEvent) {
	winners := make([]WinnerInfo, len(event.Winners))
	for i, w := range event.Winners {
		winners[i] = WinnerInfo{
			PlayerName: w.PlayerName,
			Amount:     w.Amount,
			HandRank:   w.HandRank,
			HoleCards:  w.HoleCards,
		}
	}

	data := HandEndData{
		HandID:       event.HandID,
		Winners:      winners,
		PotSize:      event.PotSize,
		ShowdownType: event.ShowdownType,
		FinalBoard:   event.FinalBoard,
		Summary:      event.Summary,
	}

	msg, err := NewMessage("hand_end", data)
	if err != nil {
		tes.logger.Error("Failed to create hand end message", "error", err)
		return
	}

	tes.server.BroadcastToTable(tes.table.ID, msg)
}

func (gt *GameTable) getDealerSeat() int {
	if gt.engine != nil && gt.engine.GetTable() != nil {
		return gt.engine.GetTable().DealerPosition()
	}
	return 0
}

// GameService manages multiple poker tables
type GameService struct {
	tables       map[string]*GameTable // tableID -> GameTable
	server       *Server
	agentManager *NetworkAgentManager
	logger       *log.Logger
	mu           sync.RWMutex
}

// NewGameService creates a new game service
func NewGameService(server *Server, logger *log.Logger) *GameService {
	agentManager := NewNetworkAgentManager(server, logger)

	gs := &GameService{
		tables:       make(map[string]*GameTable),
		server:       server,
		agentManager: agentManager,
		logger:       logger.WithPrefix("game-service"),
	}

	// Set up connection handlers
	gs.setupConnectionHandlers()

	return gs
}

// setupConnectionHandlers configures the server to route messages to the game service
func (gs *GameService) setupConnectionHandlers() {
	// This is a simplified approach - in a real implementation, you'd want more sophisticated routing
	gs.logger.Info("Setting up connection handlers")
}

// CreateTable creates a new poker table
func (gs *GameService) CreateTable(name string, maxPlayers, smallBlind, bigBlind int) (*GameTable, error) {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	tableID := fmt.Sprintf("table_%d", time.Now().UnixNano())

	table := &GameTable{
		ID:            tableID,
		Name:          name,
		MaxPlayers:    maxPlayers,
		SmallBlind:    smallBlind,
		BigBlind:      bigBlind,
		players:       make(map[string]*game.Player),
		networkAgents: make(map[string]*NetworkAgent),
		botAgents:     make(map[string]game.Agent),
		status:        "waiting",
		logger:        gs.logger.WithPrefix("table").With("id", tableID),
	}

	// Create event subscriber for this table
	table.eventSub = &TableEventSubscriber{
		table:  table,
		server: gs.server,
		logger: table.logger.WithPrefix("events"),
	}

	gs.tables[tableID] = table
	gs.logger.Info("Created new table", "id", tableID, "name", name)

	return table, nil
}

// GetTable returns a table by ID
func (gs *GameService) GetTable(tableID string) *GameTable {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	return gs.tables[tableID]
}

// ListTables returns all available tables
func (gs *GameService) ListTables() []TableInfo {
	gs.mu.RLock()
	defer gs.mu.RUnlock()

	var tables []TableInfo
	for _, table := range gs.tables {
		tables = append(tables, TableInfo{
			ID:          table.ID,
			Name:        table.Name,
			PlayerCount: len(table.players),
			MaxPlayers:  table.MaxPlayers,
			Stakes:      fmt.Sprintf("%d/%d", table.SmallBlind, table.BigBlind),
			Status:      table.status,
		})
	}

	return tables
}

// JoinTable adds a player to a table
func (gs *GameService) JoinTable(tableID, playerName string, buyIn int) error {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	table := gs.tables[tableID]
	if table == nil {
		return fmt.Errorf("table not found: %s", tableID)
	}

	if len(table.players) >= table.MaxPlayers {
		return fmt.Errorf("table is full")
	}

	if _, exists := table.players[playerName]; exists {
		return fmt.Errorf("player already at table")
	}

	// Create game engine if this is the first player
	if table.engine == nil {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		gameTable := game.NewTable(rng, game.TableConfig{
			MaxSeats:   table.MaxPlayers,
			SmallBlind: table.SmallBlind,
			BigBlind:   table.BigBlind,
			Seed:       time.Now().UnixNano(),
		})
		defaultAgent := bot.NewChartBot(gs.logger) // Default bot for fallback
		table.engine = game.NewGameEngine(gameTable, defaultAgent, table.logger)

		// Subscribe to events
		table.engine.GetEventBus().Subscribe(table.eventSub)
	}

	// Add player to game table
	player := &game.Player{
		Name:       playerName,
		Chips:      buyIn,
		Type:       game.Human,             // Assume human for network players
		SeatNumber: len(table.players) + 1, // Simple seat assignment
	}

	table.engine.GetTable().AddPlayer(player)
	table.players[playerName] = player

	// Create network agent for this player
	agent := gs.agentManager.CreateAgent(playerName, tableID)
	table.networkAgents[playerName] = agent

	table.logger.Info("Player joined table", "player", playerName, "buyIn", buyIn, "players", len(table.players))

	// Start game if we have enough players
	if len(table.players) >= 2 && table.status == "waiting" {
		go gs.startTableGame(table)
	}

	return nil
}

// LeaveTable removes a player from a table
func (gs *GameService) LeaveTable(tableID, playerName string) error {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	table := gs.tables[tableID]
	if table == nil {
		return fmt.Errorf("table not found: %s", tableID)
	}

	if _, exists := table.players[playerName]; !exists {
		return fmt.Errorf("player not at table")
	}

	// Remove player from game - for now just remove from maps
	// TODO: Implement proper player removal from game table
	// if table.engine != nil {
	//     table.engine.GetTable().RemovePlayer(playerName)
	// }

	delete(table.players, playerName)
	delete(table.networkAgents, playerName)
	gs.agentManager.RemoveAgent(playerName)

	table.logger.Info("Player left table", "player", playerName, "remaining", len(table.players))

	return nil
}

// AddBotToTable adds an AI bot to a table
func (gs *GameService) AddBotToTable(tableID, botName string, buyIn int, difficulty string) error {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	table := gs.tables[tableID]
	if table == nil {
		return fmt.Errorf("table not found: %s", tableID)
	}

	if len(table.players) >= table.MaxPlayers {
		return fmt.Errorf("table is full")
	}

	// Create game engine if this is the first player
	if table.engine == nil {
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		gameTable := game.NewTable(rng, game.TableConfig{
			MaxSeats:   table.MaxPlayers,
			SmallBlind: table.SmallBlind,
			BigBlind:   table.BigBlind,
			Seed:       time.Now().UnixNano(),
		})
		defaultAgent := bot.NewChartBot(gs.logger)
		table.engine = game.NewGameEngine(gameTable, defaultAgent, table.logger)

		// Subscribe to events
		table.engine.GetEventBus().Subscribe(table.eventSub)
	}

	// Create bot player
	player := &game.Player{
		Name:       botName,
		Chips:      buyIn,
		Type:       game.AI,
		SeatNumber: len(table.players) + 1,
	}

	table.engine.GetTable().AddPlayer(player)
	table.players[botName] = player

	// Create bot agent based on difficulty
	var botAgent game.Agent
	switch difficulty {
	case "easy", "simple":
		botAgent = bot.NewChartBot(table.logger)
	default:
		botAgent = bot.NewChartBot(table.logger) // Default to chart bot
	}

	table.botAgents[botName] = botAgent

	table.logger.Info("Bot added to table", "bot", botName, "difficulty", difficulty, "buyIn", buyIn)

	// Start game if we have enough players
	if len(table.players) >= 2 && table.status == "waiting" {
		go gs.startTableGame(table)
	}

	return nil
}

// startTableGame starts the game loop for a table
func (gs *GameService) startTableGame(table *GameTable) {
	table.status = "active"
	table.logger.Info("Starting game", "players", len(table.players))

	// Create agents map for the game engine
	agents := make(map[string]game.Agent)

	// Add network agents for human players
	for playerName, agent := range table.networkAgents {
		agents[playerName] = agent
	}

	// Add bot agents for AI players
	for playerName, agent := range table.botAgents {
		agents[playerName] = agent
	}

	// Run the game loop
	for {
		// Check if we still have enough players
		if len(table.players) < 2 {
			table.logger.Info("Not enough players, pausing game")
			table.status = "waiting"
			return
		}

		// Start a new hand
		table.engine.StartNewHand()

		// Play the hand
		result, err := table.engine.PlayHand(agents)
		if err != nil {
			table.logger.Error("Error playing hand", "error", err)
			break
		}

		table.logger.Info("Hand completed", "winner", result.Winner.Name, "pot", result.PotSize)

		// Small delay between hands
		time.Sleep(2 * time.Second)
	}

	table.status = "finished"
}

// HandlePlayerDecision routes a player decision to the appropriate agent
func (gs *GameService) HandlePlayerDecision(playerName string, data PlayerDecisionData) error {
	return gs.agentManager.HandlePlayerDecision(playerName, data)
}
