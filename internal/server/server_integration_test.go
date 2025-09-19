package server

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
)

// TestWebSocketIntegration tests full WebSocket functionality
func TestWebSocketIntegration(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(42))
	srv := NewServer(testLogger(), rng)
	srv.pool.minPlayers = 10
	srv.pool.config.RequirePlayer = false
	var poolWg sync.WaitGroup
	poolWg.Add(1)
	go func() {
		defer poolWg.Done()
		srv.pool.Run()
	}()
	t.Cleanup(func() {
		srv.pool.Stop()
		poolWg.Wait()
	})

	server := httptest.NewServer(http.HandlerFunc(srv.handleWebSocket))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	t.Run("single connection", func(t *testing.T) {
		ws := dialAndConnect(t, wsURL, "TestBot", "", string(BotRolePlayer))
		defer ws.Close()

		time.Sleep(100 * time.Millisecond)
		if srv.pool.BotCount() != 1 {
			t.Errorf("Expected 1 bot, got %d", srv.pool.BotCount())
		}

		ws.Close()
		time.Sleep(100 * time.Millisecond)
		if srv.pool.BotCount() != 0 {
			t.Errorf("Expected 0 bots after disconnect, got %d", srv.pool.BotCount())
		}
	})

	t.Run("multiple connections", func(t *testing.T) {
		var bots []*websocket.Conn
		for i := 0; i < 3; i++ {
			ws := dialAndConnect(t, wsURL, fmt.Sprintf("Bot%02d", i), "", string(BotRolePlayer))
			bots = append(bots, ws)
		}

		time.Sleep(100 * time.Millisecond)
		if srv.pool.BotCount() != 3 {
			t.Errorf("Expected 3 bots, got %d", srv.pool.BotCount())
		}

		for _, ws := range bots {
			ws.Close()
		}

		time.Sleep(100 * time.Millisecond)
		if srv.pool.BotCount() != 0 {
			t.Errorf("Expected 0 bots after disconnect, got %d", srv.pool.BotCount())
		}
	})
}

// TestBotActionsIntegration verifies bot actions are processed correctly
func TestBotActionsIntegration(t *testing.T) {
	t.Parallel()
	server := newTestServerWithDeterministicRNG(t, 1)
	stopPool := startTestPool(t, server.pool)
	defer stopPool()

	ts := httptest.NewServer(http.HandlerFunc(server.handleWebSocket))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"

	bot1Conn := dialAndConnect(t, wsURL, "TestBot1", "", string(BotRolePlayer))
	defer bot1Conn.Close()

	bot2Conn := dialAndConnect(t, wsURL, "TestBot2", "", string(BotRolePlayer))
	defer bot2Conn.Close()

	bot1Actions := make(chan string, 10)
	bot2Actions := make(chan string, 10)

	go readBotMessages(t, bot1Conn, bot1Actions)
	go readBotMessages(t, bot2Conn, bot2Actions)

	time.Sleep(100 * time.Millisecond)

	// Either bot could act first due to randomization
	actionProcessed := false
	select {
	case action := <-bot1Actions:
		if strings.Contains(action, "action_request") {
			foldAction := &protocol.Action{
				Type:   "action",
				Action: "fold",
				Amount: 0,
			}
			if data, err := protocol.Marshal(foldAction); err == nil {
				bot1Conn.WriteMessage(websocket.BinaryMessage, data)
				actionProcessed = true
			}
		}
	case <-time.After(500 * time.Millisecond):
		// Check bot2 if bot1 didn't get action request
		select {
		case action := <-bot2Actions:
			if strings.Contains(action, "action_request") {
				foldAction := &protocol.Action{
					Type:   "action",
					Action: "fold",
					Amount: 0,
				}
				if data, err := protocol.Marshal(foldAction); err == nil {
					bot2Conn.WriteMessage(websocket.BinaryMessage, data)
					actionProcessed = true
				}
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatal("No bot received action request")
		}
	}

	if !actionProcessed {
		t.Fatal("Failed to process bot action")
	}

	// Verify hand result was received
	select {
	case <-bot1Actions:
		// Success
	case <-bot2Actions:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("No hand result received")
	}
}

// TestEmptyValidActionsFix verifies fix for empty valid_actions arrays
func TestEmptyValidActionsFix(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(42))
	server := NewServer(testLogger(), rng)
	stopPool := startTestPool(t, server.pool)
	defer stopPool()

	ts := httptest.NewServer(http.HandlerFunc(server.handleWebSocket))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"

	numBots := 4
	var wg sync.WaitGroup
	emptyActionsFound := make(chan string, 10)
	stopBots := make(chan struct{})

	for i := 0; i < numBots; i++ {
		wg.Add(1)
		go func(botID int) {
			defer wg.Done()
			botName := fmt.Sprintf("TestBot%d", botID)

			conn := dialAndConnect(t, wsURL, botName, "", string(BotRolePlayer))
			defer conn.Close()

			for {
				select {
				case <-stopBots:
					return
				default:
					var msg map[string]any
					if err := conn.ReadJSON(&msg); err != nil {
						return
					}

					if msgType, ok := msg["type"].(string); ok && msgType == "action_request" {
						var actionReq protocol.ActionRequest
						if data, err := json.Marshal(msg); err == nil {
							if err := json.Unmarshal(data, &actionReq); err == nil {
								if len(actionReq.ValidActions) == 0 {
									emptyActionsFound <- fmt.Sprintf("Bot %s received empty ValidActions", botName)
									return
								}

								// Respond with fold
								foldAction := &protocol.Action{
									Type:   "action",
									Action: "fold",
									Amount: 0,
								}
								if data, err := protocol.Marshal(foldAction); err == nil {
									conn.WriteMessage(websocket.BinaryMessage, data)
								}
							}
						}
					}
				}
			}
		}(i)
	}

	// Run test for limited time
	testDone := time.After(2 * time.Second)
	select {
	case errMsg := <-emptyActionsFound:
		close(stopBots)
		wg.Wait()
		t.Fatalf("CRITICAL: %s", errMsg)
	case <-testDone:
		close(stopBots)
		wg.Wait()
		t.Log("SUCCESS: No empty valid_actions found")
	}
}

// TestButtonAssignmentBugFix ensures dealer button is assigned correctly
func TestButtonAssignmentBugFix(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(42))
	s := NewServer(testLogger(), rng)

	testServer := httptest.NewServer(http.HandlerFunc(s.handleWebSocket))
	defer testServer.Close()

	var poolWg sync.WaitGroup
	poolWg.Add(1)
	go func() {
		defer poolWg.Done()
		s.pool.Run()
	}()
	defer func() {
		s.pool.Stop()
		poolWg.Wait()
	}()

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http")

	bots := make([]*websocket.Conn, 3)
	for i := 0; i < 3; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Bot %d failed to connect: %v", i, err)
		}
		bots[i] = conn

		connectMsg := &protocol.Connect{
			Type: "connect",
			Name: fmt.Sprintf("Player%d", i),
			Game: "",
			Role: string(BotRolePlayer),
		}

		if data, err := protocol.Marshal(connectMsg); err == nil {
			conn.WriteMessage(websocket.BinaryMessage, data)
		}
	}
	defer func() {
		for _, bot := range bots {
			bot.Close()
		}
	}()

	// Wait for hand to start and check button assignment
	buttonPosition := -1
	handStartReceived := make(chan int, 3)

	// Start goroutines to read from each bot
	for i, bot := range bots {
		go func(botIndex int, conn *websocket.Conn) {
			for {
				_, data, err := conn.ReadMessage()
				if err != nil {
					return
				}

				var handStart protocol.HandStart
				if err := protocol.Unmarshal(data, &handStart); err == nil && handStart.Type == "hand_start" {
					handStartReceived <- handStart.Button
					return
				}
			}
		}(i, bot)
	}

	// Wait for hand start message
	select {
	case buttonPos := <-handStartReceived:
		buttonPosition = buttonPos
		t.Logf("Button position: %d", buttonPos)
	case <-time.After(3 * time.Second):
		t.Log("No hand start received within timeout")
	}

	if buttonPosition != 0 {
		t.Errorf("Expected button at seat 0 after shuffling, got seat %d", buttonPosition)
	}
}

// TestHandLimitReached verifies hand limit functionality
func TestHandLimitReached(t *testing.T) {
	t.Parallel()
	logger := testLogger()
	rng := rand.New(rand.NewSource(42))
	handLimit := uint64(2)

	config := DefaultConfig(2, 4)
	config.HandLimit = handLimit
	pool := NewBotPool(logger, rng, config)
	pool.SetGameID("test-hand-limit")

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		pool.Run()
	}()
	defer func() {
		pool.Stop()
		wg.Wait()
	}()

	// Set hand counter to limit
	atomic.StoreUint64(&pool.handCounter, handLimit)

	// Create bots - should not trigger new hands
	bots := make([]*Bot, 0, 4)
	for i := 0; i < 4; i++ {
		bot := &Bot{
			ID:       fmt.Sprintf("test-bot-%d", i+1),
			send:     make(chan []byte, 256),
			done:     make(chan struct{}),
			pool:     pool,
			logger:   logger,
			bankroll: 1000,
			conn:     nil,
		}
		pool.Register(bot)
		bots = append(bots, bot)
	}

	time.Sleep(250 * time.Millisecond)

	finalHandCount := atomic.LoadUint64(&pool.handCounter)
	if finalHandCount != handLimit {
		t.Errorf("Hand limit not respected: expected %d, got %d", handLimit, finalHandCount)
	}

	// Check for game_completed message
	if !pool.HandLimitNotified() {
		time.Sleep(200 * time.Millisecond)
		if !pool.HandLimitNotified() {
			t.Fatal("Expected hand limit notification")
		}
	}

	// Verify game_completed message was sent
	messageReceived := false
	deadline := time.Now().Add(150 * time.Millisecond)
	for !messageReceived && time.Now().Before(deadline) {
		for _, bot := range bots {
			select {
			case msg := <-bot.send:
				var completed protocol.GameCompleted
				if err := protocol.Unmarshal(msg, &completed); err == nil {
					if completed.Type == protocol.TypeGameCompleted {
						messageReceived = true
						if completed.HandLimit != handLimit {
							t.Errorf("Expected hand limit %d, got %d", handLimit, completed.HandLimit)
						}
						if completed.Reason != "hand_limit_reached" {
							t.Errorf("Expected reason hand_limit_reached, got %s", completed.Reason)
						}
					}
				}
			default:
			}
		}
		if !messageReceived {
			time.Sleep(5 * time.Millisecond)
		}
	}

	if !messageReceived {
		t.Fatal("Expected game_completed message")
	}
}

// Helper functions - readBotMessages defined in integration_test.go
