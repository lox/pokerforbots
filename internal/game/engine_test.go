package game

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
)

// MockAgent is a simple test agent that follows a predetermined script
type MockAgent struct {
	actions []Decision
	index   int
}

func NewMockAgent(actions []Decision) *MockAgent {
	return &MockAgent{
		actions: actions,
		index:   0,
	}
}

func (m *MockAgent) MakeDecision(tableState TableState, validActions []ValidAction) Decision {
	if m.index >= len(m.actions) {
		// Default to fold if we run out of scripted actions
		return Decision{Action: Fold, Amount: 0, Reasoning: "script exhausted"}
	}

	decision := m.actions[m.index]
	m.index++
	return decision
}

// AlwaysFoldAgent always folds
type AlwaysFoldAgent struct{}

func (a *AlwaysFoldAgent) MakeDecision(tableState TableState, validActions []ValidAction) Decision {
	// Check if we can check instead of folding
	for _, action := range validActions {
		if action.Action == Check {
			return Decision{Action: Check, Amount: 0, Reasoning: "always fold (checking when possible)"}
		}
	}
	return Decision{Action: Fold, Amount: 0, Reasoning: "always fold"}
}

// EventCapturingAgent captures all events published to verify no duplicates
type EventCapturingAgent struct {
	*AlwaysFoldAgent
	events []GameEvent
}

func (e *EventCapturingAgent) CaptureEvent(event GameEvent) {
	e.events = append(e.events, event)
}

// testEventSubscriber captures events for testing
type testEventSubscriber struct {
	events *[]GameEvent
}

func (t *testEventSubscriber) OnEvent(event GameEvent) {
	*t.events = append(*t.events, event)
}

// AlwaysCallAgent always calls or checks
type AlwaysCallAgent struct{}

func (a *AlwaysCallAgent) MakeDecision(tableState TableState, validActions []ValidAction) Decision {
	// Try to call, otherwise check
	for _, action := range validActions {
		if action.Action == Call {
			return Decision{Action: Call, Amount: action.MinAmount, Reasoning: "always call"}
		}
	}
	for _, action := range validActions {
		if action.Action == Check {
			return Decision{Action: Check, Amount: 0, Reasoning: "always check"}
		}
	}
	// Fallback to fold if neither call nor check available
	return Decision{Action: Fold, Amount: 0, Reasoning: "forced fold"}
}

// TrackingAgent wraps another agent and tracks which rounds it sees
type TrackingAgent struct {
	wrapped Agent
	rounds  *[]BettingRound
}

func NewTrackingAgent(wrapped Agent, rounds *[]BettingRound) *TrackingAgent {
	return &TrackingAgent{
		wrapped: wrapped,
		rounds:  rounds,
	}
}

func (t *TrackingAgent) MakeDecision(tableState TableState, validActions []ValidAction) Decision {
	*t.rounds = append(*t.rounds, tableState.CurrentRound)
	return t.wrapped.MakeDecision(tableState, validActions)
}

func createTestTable() *Table {
	return NewTestTable() // Uses all defaults: seed 42, 6 seats, 10/20 blinds
}

func createTestPlayers() []*Player {
	return []*Player{
		NewPlayer(1, "Alice", AI, 1000),
		NewPlayer(2, "Bob", AI, 1000),
		NewPlayer(3, "Charlie", AI, 1000),
	}
}

func TestGameEngine_Creation(t *testing.T) {
	table := createTestTable()
	logger := log.New(io.Discard)

	engine := NewGameEngine(table, logger)

	if engine.table != table {
		t.Error("Engine table not set correctly")
	}
	if engine.logger != logger {
		t.Error("Engine logger not set correctly")
	}
}

func TestGameEngine_PlayHandWithFolds(t *testing.T) {
	// Create table with players using new helper
	_, engine := NewTestGameEngine(
		WithPlayers("Alice", "Bob", "Charlie"),
	)

	// Set up agents - first player calls, others fold
	agents := map[string]Agent{
		"Alice":   NewMockAgent([]Decision{{Action: Call, Amount: 20, Reasoning: "call bb"}}),
		"Bob":     &AlwaysFoldAgent{},
		"Charlie": &AlwaysFoldAgent{},
	}

	for playerName, agent := range agents {
		engine.AddAgent(playerName, agent)
	}

	// Start a new hand
	engine.StartNewHand()

	// Play the hand
	result, err := engine.PlayHand()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify result
	if result == nil {
		t.Fatal("Expected hand result, got nil")
	}

	if result.ShowdownType != "fold" {
		t.Errorf("Expected showdown type 'fold', got '%s'", result.ShowdownType)
	}

	if result.Winner == nil {
		t.Error("Expected a winner")
	}

	if len(result.Actions) == 0 {
		t.Error("Expected some actions to be recorded")
	}
}

func TestGameEngine_PlayHandToShowdown(t *testing.T) {
	table := createTestTable()
	players := createTestPlayers()

	// Add players to table
	for _, player := range players {
		table.AddPlayer(player)
	}

	logger := log.New(io.Discard)
	engine := NewGameEngine(table, logger)

	// Add agents to engine
	for _, player := range players {
		engine.AddAgent(player.Name, &AlwaysCallAgent{})
	}

	// Start a new hand
	engine.StartNewHand()

	// Play the hand
	result, err := engine.PlayHand() // Use default agent for all players
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify result
	if result == nil {
		t.Fatal("Expected hand result, got nil")
	}

	if result.ShowdownType != "showdown" {
		t.Errorf("Expected showdown type 'showdown', got '%s'", result.ShowdownType)
	}

	if result.Winner == nil {
		t.Error("Expected a winner")
	}

	if table.currentRound != Showdown {
		t.Errorf("Expected game to end in showdown, got %s", table.currentRound)
	}
}

func TestGameEngine_HandProgression(t *testing.T) {
	table := createTestTable()
	players := createTestPlayers()

	// Add players to table
	for _, player := range players {
		table.AddPlayer(player)
	}

	// Track the betting rounds we see
	var roundsSeen []BettingRound

	logger := log.New(io.Discard)
	engine := NewGameEngine(table, logger)

	// Add agents to engine
	for _, player := range players {
		// Create agent that tracks rounds and always calls
		engine.AddAgent(player.Name, NewTrackingAgent(&AlwaysCallAgent{}, &roundsSeen))
	}

	// Start a new hand
	engine.StartNewHand()

	// Play the hand
	result, err := engine.PlayHand()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify we went through all betting rounds
	if result == nil {
		t.Fatal("Expected hand result, got nil")
	}

	// Should see actions in PreFlop, Flop, Turn, and River
	hasPreFlop := false
	hasFlop := false
	hasTurn := false
	hasRiver := false

	for _, round := range roundsSeen {
		switch round {
		case PreFlop:
			hasPreFlop = true
		case Flop:
			hasFlop = true
		case Turn:
			hasTurn = true
		case River:
			hasRiver = true
		}
	}

	if !hasPreFlop {
		t.Error("Expected to see PreFlop actions")
	}
	if !hasFlop {
		t.Error("Expected to see Flop actions")
	}
	if !hasTurn {
		t.Error("Expected to see Turn actions")
	}
	if !hasRiver {
		t.Error("Expected to see River actions")
	}
}

func TestGameEngine_ActionRecording(t *testing.T) {
	table := createTestTable()
	players := createTestPlayers()

	// Add players to table
	for _, player := range players {
		table.AddPlayer(player)
	}

	// Create agents with specific actions
	agents := map[string]Agent{
		"Alice":   NewMockAgent([]Decision{{Action: Raise, Amount: 40, Reasoning: "raise to 40"}}),
		"Bob":     NewMockAgent([]Decision{{Action: Call, Amount: 40, Reasoning: "call the raise"}}),
		"Charlie": NewMockAgent([]Decision{{Action: Fold, Amount: 0, Reasoning: "fold to raise"}}),
	}

	logger := log.New(io.Discard)
	engine := NewGameEngine(table, logger)

	// Add agents to engine
	for _, player := range players {
		engine.AddAgent(player.Name, agents[player.Name])
	}

	// Start a new hand
	engine.StartNewHand()

	// Play the hand
	result, err := engine.PlayHand()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify actions were recorded
	if result == nil {
		t.Fatal("Expected hand result, got nil")
	}

	if len(result.Actions) == 0 {
		t.Fatal("Expected actions to be recorded")
	}

	// Find the raise action
	var raiseAction *HandAction
	for i := range result.Actions {
		if result.Actions[i].Action == Raise {
			raiseAction = &result.Actions[i]
			break
		}
	}

	if raiseAction == nil {
		t.Error("Expected to find a raise action")
	} else {
		if raiseAction.PlayerName != "Alice" {
			t.Errorf("Expected raise by Alice, got %s", raiseAction.PlayerName)
		}
		if raiseAction.Amount != 20 { // Amount actually bet (raise total - current bet)
			t.Errorf("Expected raise amount 20, got %d", raiseAction.Amount)
		}
		if raiseAction.Thinking != "raise to 40" {
			t.Errorf("Expected reasoning 'raise to 40', got '%s'", raiseAction.Thinking)
		}
	}
}

func TestGameEngine_AllInScenario(t *testing.T) {
	table := createTestTable()

	// Create players with different stack sizes
	players := []*Player{
		NewPlayer(1, "Alice", AI, 50), // Short stack
		NewPlayer(2, "Bob", AI, 1000), // Big stack
	}

	// Add players to table
	for _, player := range players {
		table.AddPlayer(player)
	}

	// In heads-up, big blind acts first pre-flop
	// Both players should have reasonable action sequences
	agents := map[string]Agent{
		"Alice": NewMockAgent([]Decision{
			{Action: AllIn, Amount: 0, Reasoning: "all-in"},     // If Alice acts first
			{Action: Call, Amount: 0, Reasoning: "call all-in"}, // If responding to Bob's action
		}),
		"Bob": NewMockAgent([]Decision{
			{Action: AllIn, Amount: 0, Reasoning: "all-in"},     // If Bob acts first
			{Action: Call, Amount: 0, Reasoning: "call all-in"}, // If responding to Alice's action
		}),
	}

	logger := log.New(io.Discard)
	engine := NewGameEngine(table, logger)

	// Add agents to engine
	for _, player := range players {
		engine.AddAgent(player.Name, agents[player.Name])
	}

	// Start a new hand
	engine.StartNewHand()

	// Play the hand
	result, err := engine.PlayHand()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify result
	if result == nil {
		t.Fatal("Expected hand result, got nil")
	}

	// Find Alice and verify she's all-in
	var alice *Player
	for _, player := range table.activePlayers {
		if player.Name == "Alice" {
			alice = player
			break
		}
	}

	if alice == nil {
		t.Fatal("Could not find Alice")
	}

	if !alice.IsAllIn {
		t.Error("Expected Alice to be all-in")
	}

	if alice.Chips != 0 {
		t.Errorf("Expected Alice to have 0 chips after all-in, got %d (started with 50, should have posted blinds and gone all-in)", alice.Chips)
	}
}

func TestGameEngine_NoAgentsUsesDefault(t *testing.T) {
	table := createTestTable()
	players := createTestPlayers()

	// Add players to table
	for _, player := range players {
		table.AddPlayer(player)
	}

	logger := log.New(io.Discard)
	engine := NewGameEngine(table, logger)

	// Add agents to engine
	for _, player := range players {
		engine.AddAgent(player.Name, &AlwaysFoldAgent{})
	}

	// Start a new hand
	engine.StartNewHand()

	// Play the hand with no specific agents (should use default)
	result, err := engine.PlayHand()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify result
	if result == nil {
		t.Fatal("Expected hand result, got nil")
	}

	// With always fold agent, hand should end quickly with folds
	if result.ShowdownType != "fold" {
		t.Errorf("Expected showdown type 'fold', got '%s'", result.ShowdownType)
	}
}

func TestGameEngine_GetTable(t *testing.T) {
	table := createTestTable()
	logger := log.New(io.Discard)
	engine := NewGameEngine(table, logger)

	if engine.GetTable() != table {
		t.Error("GetTable() should return the same table instance")
	}
}

func TestGameEngine_StartNewHand(t *testing.T) {
	table := createTestTable()
	players := createTestPlayers()

	// Add players to table
	for _, player := range players {
		table.AddPlayer(player)
	}

	logger := log.New(io.Discard)
	engine := NewGameEngine(table, logger)

	// Add agents to engine
	for _, player := range players {
		engine.AddAgent(player.Name, &AlwaysFoldAgent{})
	}

	// Start a new hand
	engine.StartNewHand()

	// Verify hand was started
	if table.state != InProgress {
		t.Errorf("Expected table state InProgress, got %s", table.state)
	}

	if table.handID == "" {
		t.Error("Expected hand ID to be set")
	}

	if table.currentRound != PreFlop {
		t.Errorf("Expected current round PreFlop, got %s", table.currentRound)
	}
}

// Test the new TableState architecture
func TestTableState_CreateAndValidActions(t *testing.T) {
	table := createTestTable()
	players := createTestPlayers()

	// Add players to table
	for _, player := range players {
		table.AddPlayer(player)
	}

	// Start a new hand
	table.StartNewHand()

	currentPlayer := table.GetCurrentPlayer()
	if currentPlayer == nil {
		t.Fatal("No current player")
	}

	// Create table state
	tableState := table.CreateTableState(currentPlayer)

	// Verify table state
	if tableState.CurrentRound != PreFlop {
		t.Errorf("Expected PreFlop, got %s", tableState.CurrentRound)
	}

	if len(tableState.Players) != 3 {
		t.Errorf("Expected 3 players, got %d", len(tableState.Players))
	}

	// Verify acting player has hole cards, others don't
	actingPlayer := tableState.Players[tableState.ActingPlayerIdx]
	if len(actingPlayer.HoleCards) == 0 {
		t.Error("Acting player should have hole cards")
	}

	for i, player := range tableState.Players {
		if i != tableState.ActingPlayerIdx && len(player.HoleCards) > 0 {
			t.Errorf("Non-acting player %s should not have hole cards", player.Name)
		}
	}

	// Test valid actions
	validActions := table.GetValidActions()
	if len(validActions) == 0 {
		t.Error("Should have valid actions")
	}

	// Should have call and fold available (facing big blind)
	hasCall := false
	hasFold := false
	for _, action := range validActions {
		if action.Action == Call {
			hasCall = true
		}
		if action.Action == Fold {
			hasFold = true
		}
	}

	if !hasCall {
		t.Error("Should have call action available")
	}
	if !hasFold {
		t.Error("Should have fold action available")
	}
}

// Test new agent interface
func TestNewAgentInterface(t *testing.T) {
	table := createTestTable()
	players := createTestPlayers()

	// Add players to table
	for _, player := range players {
		table.AddPlayer(player)
	}

	// Start a new hand
	table.StartNewHand()

	currentPlayer := table.GetCurrentPlayer()
	if currentPlayer == nil {
		t.Fatal("No current player")
	}

	// Create table state and get valid actions
	tableState := table.CreateTableState(currentPlayer)
	validActions := table.GetValidActions()

	// Test new agent interface
	agent := &AlwaysCallAgent{}
	decision := agent.MakeDecision(tableState, validActions)

	// Should return a call decision
	if decision.Action != Call {
		t.Errorf("Expected Call, got %s", decision.Action)
	}

	// Apply the decision
	reasoning, err := table.ApplyDecision(decision)
	if err != nil {
		t.Errorf("Failed to apply decision: %v", err)
	}

	if reasoning != decision.Reasoning {
		t.Errorf("Expected reasoning '%s', got '%s'", decision.Reasoning, reasoning)
	}
}

// AnalyticalAgent demonstrates the rich analysis capabilities of the new architecture
type AnalyticalAgent struct{}

func (a *AnalyticalAgent) MakeDecision(tableState TableState, validActions []ValidAction) Decision {
	actingPlayer := tableState.Players[tableState.ActingPlayerIdx]

	// Analyze betting action using hand history
	roundSummary := tableState.HandHistory.GetBettingRoundSummary(tableState.CurrentRound)

	// Position-aware decision making
	reasoning := "analytical decision: "

	// Check for aggressive action
	if roundSummary.NumRaises >= 2 {
		reasoning += "facing 3-bet+ (aggressive action), "

		// Fold from early position to heavy action
		if actingPlayer.Position == UnderTheGun || actingPlayer.Position == EarlyPosition {
			reasoning += "folding from early position"
			for _, action := range validActions {
				if action.Action == Fold {
					return Decision{Action: Fold, Amount: 0, Reasoning: reasoning}
				}
			}
		}
	}

	// Analyze bet sizing if available
	betSizing := tableState.HandHistory.GetBetSizingInfo(tableState.CurrentRound)
	if len(betSizing) > 0 {
		lastBet := betSizing[len(betSizing)-1]
		reasoning += fmt.Sprintf("last bet ratio %.2f, ", lastBet.Ratio)

		// Large bet on river = fold
		if tableState.CurrentRound == River && lastBet.Ratio > 0.8 {
			reasoning += "folding to large river bet"
			for _, action := range validActions {
				if action.Action == Fold {
					return Decision{Action: Fold, Amount: 0, Reasoning: reasoning}
				}
			}
		}
	}

	// Stack consideration
	stackToBBRatio := float64(actingPlayer.Chips) / float64(tableState.BigBlind)
	reasoning += fmt.Sprintf("stack %.1fBB, ", stackToBBRatio)

	// Short stack shove
	if stackToBBRatio < 10 && roundSummary.NumRaises == 0 {
		reasoning += "shoving short stack"
		for _, action := range validActions {
			if action.Action == AllIn {
				return Decision{Action: AllIn, Amount: action.MinAmount, Reasoning: reasoning}
			}
		}
	}

	// Default: call or check
	reasoning += "default action"
	for _, action := range validActions {
		if action.Action == Call {
			return Decision{Action: Call, Amount: action.MinAmount, Reasoning: reasoning}
		}
	}
	for _, action := range validActions {
		if action.Action == Check {
			return Decision{Action: Check, Amount: 0, Reasoning: reasoning}
		}
	}

	// Fallback fold
	return Decision{Action: Fold, Amount: 0, Reasoning: reasoning + " (forced fold)"}
}

// Test the analytical capabilities
func TestAnalyticalAgent_BettingAnalysis(t *testing.T) {
	table := createTestTable()
	players := createTestPlayers()

	// Add players to table
	for _, player := range players {
		table.AddPlayer(player)
	}

	// Create engine and set up event subscriptions
	logger := log.New(io.Discard)
	engine := NewGameEngine(table, logger)

	// Add agents to engine
	for _, player := range players {
		engine.AddAgent(player.Name, &AlwaysFoldAgent{})
	}

	// Start a new hand
	engine.StartNewHand()

	// Verify hand history exists and is subscribed
	if table.handHistory == nil {
		t.Fatal("Hand history not created")
	}

	// Create agents - we want the current player (first to act) to raise
	currentPlayerAtStart := table.GetCurrentPlayer()
	if currentPlayerAtStart == nil {
		t.Fatal("No current player")
	}

	agents := map[string]Agent{
		currentPlayerAtStart.Name: NewMockAgent([]Decision{
			{Action: Raise, Amount: 40, Reasoning: "test raise"},
		}),
	}

	// Add analytical agent for other players
	for _, player := range table.players {
		if player.Name != currentPlayerAtStart.Name {
			agents[player.Name] = &AnalyticalAgent{}
		}
	}

	// Run partial hand to generate one raise action
	currentPlayer := table.GetCurrentPlayer()
	if currentPlayer == nil {
		t.Fatal("No current player")
	}

	// First player makes a raise
	raiseAgent := agents[currentPlayer.Name]
	if raiseAgent == nil {
		t.Fatalf("No agent for current player %s", currentPlayer.Name)
	}
	tableState := table.CreateTableState(currentPlayer)
	validActions := table.GetValidActions()
	raiseDecision := raiseAgent.MakeDecision(tableState, validActions)

	// Apply the raise and publish event (like engine does)
	reasoning, err := table.ApplyDecision(raiseDecision)
	if err != nil {
		t.Fatalf("Failed to apply raise: %v", err)
	}

	// Publish the event (like engine does)
	event := NewPlayerActionEvent(currentPlayer, raiseDecision.Action, raiseDecision.Amount, table.currentRound, reasoning, table.pot)
	engine.GetEventBus().Publish(event)

	// Advance to next player
	table.AdvanceAction()
	currentPlayer = table.GetCurrentPlayer()
	if currentPlayer == nil {
		t.Fatal("No next player")
	}

	// Now test analytical agent
	agent := &AnalyticalAgent{}
	tableState = table.CreateTableState(currentPlayer)
	validActions = table.GetValidActions()
	decision := agent.MakeDecision(tableState, validActions)

	// Verify the decision includes analysis
	if decision.Reasoning == "" {
		t.Error("Expected reasoning from analytical agent")
	}

	if !strings.Contains(decision.Reasoning, "analytical decision") {
		t.Errorf("Expected analytical reasoning, got: %s", decision.Reasoning)
	}

	// Verify agent can access betting history
	roundSummary := tableState.HandHistory.GetBettingRoundSummary(tableState.CurrentRound)
	if roundSummary.NumRaises == 0 {
		t.Error("Expected to detect the raise in betting summary")
	}
}

func TestGameEngine_NoDuplicateStreetChangeEvents(t *testing.T) {
	// Create a table with event capturing capability using test helper
	_, engine := NewTestGameEngine(
		WithPlayers("Alice", "Bob"),
	)

	// Create event capturer
	capturedEvents := []GameEvent{}
	eventSubscriber := &testEventSubscriber{events: &capturedEvents}
	engine.table.GetEventBus().Subscribe(eventSubscriber)

	// Add call agents that will let hand go to showdown
	agents := map[string]Agent{
		"Alice": &AlwaysCallAgent{},
		"Bob":   &AlwaysCallAgent{},
	}

	for playerName, agent := range agents {
		engine.AddAgent(playerName, agent)
	}

	// Start a hand that will go through multiple streets
	engine.StartNewHand()
	_, err := engine.PlayHand()
	if err != nil {
		t.Fatalf("Failed to play hand: %v", err)
	}

	// Count street change events
	streetChangeCount := 0
	for _, event := range capturedEvents {
		if event.EventType() == EventTypeStreetChange {
			streetChangeCount++
		}
	}

	// We should have exactly 3 street change events: Flop, Turn, River
	// (Not 6 due to duplicates)
	if streetChangeCount != 3 {
		t.Errorf("Expected exactly 3 street change events (Flop, Turn, River), got %d", streetChangeCount)

		// Debug: log all street change events
		for i, event := range capturedEvents {
			if event.EventType() == EventTypeStreetChange {
				streetEvent := event.(StreetChangeEvent)
				t.Logf("Event %d: StreetChange to %s", i, streetEvent.Round)
			}
		}
	}
}
