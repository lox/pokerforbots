package server

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/lox/pokerforbots/internal/bot"
	"github.com/lox/pokerforbots/internal/game"
)

// ServerTable represents a poker table managed by the server
type ServerTable struct {
	*game.Table

	ID   string
	Name string
	// MaxPlayers    int
	// SmallBlind    int
	// BigBlind      int
	engine        *game.GameEngine
	players       map[string]*game.Player  // playerName -> Player
	networkAgents map[string]*NetworkAgent // playerName -> NetworkAgent for remote players
	botAgents     map[string]game.Agent    // playerName -> Bot agent for AI players
	status        string                   // "waiting", "active", "finished"
	logger        *log.Logger
	eventSub      *TableEventSubscriber
	waitingLogged bool // Track if we've logged the waiting message
	seed          int64
}

// TableEventSubscriber handles game events and forwards them to clients
type TableEventSubscriber struct {
	table  *ServerTable
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

			_ = tes.server.SendToPlayer(p.Name, playerMsg) // Ignore send errors
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

func (gt *ServerTable) getDealerSeat() int {
	if gt.engine != nil && gt.engine.GetTable() != nil {
		return gt.engine.GetTable().DealerPosition()
	}
	return 0
}

// GameService manages multiple poker tables
type GameService struct {
	tables       map[string]*ServerTable // tableID -> GameTable
	server       *Server
	agentManager *NetworkAgentManager
	logger       *log.Logger
	mu           sync.RWMutex
	tableCounter int // For shorter table IDs
	seed         int64
}

// NewGameService creates a new game service
func NewGameService(server *Server, logger *log.Logger, seed int64) *GameService {
	agentManager := NewNetworkAgentManager(server, logger)

	gs := &GameService{
		tables:       make(map[string]*ServerTable),
		server:       server,
		agentManager: agentManager,
		logger:       logger.WithPrefix("game-service"),
		seed:         seed,
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
func (gs *GameService) CreateTable(name string, maxPlayers, smallBlind, bigBlind int) (*ServerTable, error) {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	gs.tableCounter++

	tableID := fmt.Sprintf("table%d", gs.tableCounter)
	tableSeed := gs.seed + (int64(gs.tableCounter) - 1)
	rng := rand.New(rand.NewSource(tableSeed))
	logger := gs.logger.WithPrefix("table").With("id", tableID)

	table := game.NewTable(rng, game.TableConfig{
		MaxSeats:   maxPlayers,
		SmallBlind: smallBlind,
		BigBlind:   bigBlind,
		Seed:       tableSeed,
	})

	engine := game.NewGameEngine(table, logger)

	serverTable := &ServerTable{
		Table:         table,
		ID:            tableID,
		Name:          name,
		engine:        engine,
		players:       make(map[string]*game.Player),
		networkAgents: make(map[string]*NetworkAgent),
		botAgents:     make(map[string]game.Agent),
		status:        "waiting",
		logger:        logger,
		seed:          tableSeed,
	}

	// Create event subscriber for this table
	serverTable.eventSub = &TableEventSubscriber{
		table:  serverTable,
		server: gs.server,
		logger: logger.WithPrefix("events"),
	}

	// Subscribe to table events
	table.GetEventBus().Subscribe(serverTable.eventSub)

	gs.tables[tableID] = serverTable
	gs.logger.Info("Created new table", "id", tableID, "name", name)

	return serverTable, nil
}

// GetTable returns a table by ID
func (gs *GameService) GetTable(tableID string) *ServerTable {
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
			MaxPlayers:  table.MaxSeats(),
			Stakes:      fmt.Sprintf("%d/%d", table.SmallBlind(), table.BigBlind()),
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

	if len(table.players) >= table.MaxSeats() {
		return fmt.Errorf("table is full")
	}

	if _, exists := table.players[playerName]; exists {
		return fmt.Errorf("player already at table")
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
	table.engine.AddAgent(playerName, agent)

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

// startTableGame starts the game loop for a table
func (gs *GameService) startTableGame(table *ServerTable) {
	table.status = "active"
	table.logger.Info("Starting game", "players", len(table.players))

	// Register all agents with the game engine
	// Add network agents for human players
	for playerName, agent := range table.networkAgents {
		table.engine.AddAgent(playerName, agent)
	}

	// Add bot agents for AI players
	for playerName, agent := range table.botAgents {
		table.engine.AddAgent(playerName, agent)
	}

	// Run the game loop
	for {
		// Check if we still have enough players
		if len(table.players) < 2 {
			table.logger.Info("Not enough players, pausing game")
			table.status = "waiting"
			return
		}

		// Check if we have at least one remote player (not just server-local bots)
		hasRemotePlayer := len(table.networkAgents) > 0

		if !hasRemotePlayer {
			if !table.waitingLogged {
				table.logger.Info("No remote players connected, pausing game")
				table.waitingLogged = true
			}
			table.status = "waiting"

			// Wait for a remote player to join before continuing
			// This prevents endless server-local bot-vs-bot games
			time.Sleep(1 * time.Second)
			continue
		}

		// Reset waiting flag when we have remote players
		table.waitingLogged = false

		// Start a new hand
		table.engine.StartNewHand()

		// Play the hand
		result, err := table.engine.PlayHand()
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

// AddBots adds AI bots to the specified table
func (gs *GameService) AddBots(tableID string, count int) ([]string, error) {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	table, exists := gs.tables[tableID]
	if !exists {
		return nil, fmt.Errorf("table not found: %s", tableID)
	}

	var botNames []string
	for i := 0; i < count; i++ {
		// Check if table is full
		if len(table.players) >= table.MaxSeats() {
			break
		}

		// Generate unique bot name
		botName := fmt.Sprintf("Bot_%d", len(table.botAgents)+1)
		for table.players[botName] != nil {
			botName = fmt.Sprintf("Bot_%d", len(table.botAgents)+len(botNames)+i+1)
		}

		// Create bot with sophisticated AI
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))
		botConfig := bot.DefaultBotConfig()
		botConfig.Name = botName
		botAgent := bot.NewBotWithConfig(rng, gs.logger, botConfig)

		// Create game player for the bot
		botPlayer := &game.Player{
			Name:       botName,
			Chips:      200,                  // Match typical human buy-in
			Position:   game.UnknownPosition, // Will be assigned by engine
			SeatNumber: -1,                   // Will be assigned
			Type:       game.AI,
			IsActive:   true,
		}

		// Add to table
		table.players[botName] = botPlayer
		table.botAgents[botName] = botAgent

		// Add to game engine table and register agent
		table.engine.GetTable().AddPlayer(botPlayer)
		table.engine.AddAgent(botName, botAgent)

		botNames = append(botNames, botName)
		table.logger.Info("Added bot to table", "botName", botName, "tableId", tableID)
	}

	if len(botNames) == 0 {
		return nil, fmt.Errorf("could not add any bots (table may be full)")
	}

	// Start game if we have enough players and table is waiting
	// Only start if there's at least one human player (networkAgent)
	if len(table.players) >= 2 && table.status == "waiting" && len(table.networkAgents) > 0 {
		go gs.startTableGame(table)
	}

	return botNames, nil
}

// KickBot removes a bot from the specified table
func (gs *GameService) KickBot(tableID string, botName string) error {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	table, exists := gs.tables[tableID]
	if !exists {
		return fmt.Errorf("table not found: %s", tableID)
	}

	// Check if the player is actually a bot
	if _, isBot := table.botAgents[botName]; !isBot {
		return fmt.Errorf("player %s is not a bot or does not exist", botName)
	}

	// Mark bot as inactive first
	if botPlayer := table.players[botName]; botPlayer != nil {
		botPlayer.IsActive = false
	}

	// Remove from table maps
	delete(table.players, botName)
	delete(table.botAgents, botName)

	table.logger.Info("Kicked bot from table", "botName", botName, "tableId", tableID)
	return nil
}
