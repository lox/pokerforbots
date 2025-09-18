package server

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
)

const serverReadyTimeout = 2 * time.Second

func startServerForTest(t *testing.T) (host string) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	addr := listener.Addr().String()

	rng := rand.New(rand.NewSource(42))
	server := NewServer(testLogger(), rng)

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("server error on %s: %v", addr, err)
		}
	}()

	waitForServer(t, addr)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("server shutdown error on %s: %v", addr, err)
		}
	})

	return addr
}

func waitForServer(t *testing.T, addr string) {
	t.Helper()

	healthURL := fmt.Sprintf("http://%s/health", addr)
	deadline := time.Now().Add(serverReadyTimeout)
	client := http.Client{Timeout: 100 * time.Millisecond}

	for time.Now().Before(deadline) {
		resp, err := client.Get(healthURL)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("server not ready at %s", healthURL)
}

// TestCompleteHandScenarios tests various complete hand scenarios
func TestCompleteHandScenarios(t *testing.T) {
	t.Parallel()
	t.Run("EveryoneFolds", func(t *testing.T) {
		testEveryoneFolds(t)
	})

	t.Run("ShowdownWithMultiplePlayers", func(t *testing.T) {
		testShowdownWithMultiplePlayers(t)
	})

	t.Run("AllInCascade", func(t *testing.T) {
		testAllInCascade(t)
	})

	t.Run("HeadsUpPlay", func(t *testing.T) {
		testHeadsUpPlay(t)
	})
}

func testEveryoneFolds(t *testing.T) {
	host := startServerForTest(t)

	// Connect 3 bots
	bots := make([]*websocket.Conn, 3)
	for i := 0; i < 3; i++ {
		u := url.URL{Scheme: "ws", Host: host, Path: "/ws"}
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			t.Fatalf("Bot %d failed to connect: %v", i, err)
		}
		bots[i] = conn
		connRef := conn
		t.Cleanup(func() {
			_ = connRef.Close()
		})

		// Send connect message
		connectMsg := &protocol.Connect{
			Type: protocol.TypeConnect,
			Name: fmt.Sprintf("FoldBot%d", i),
			Role: string(BotRolePlayer),
		}
		data, _ := protocol.Marshal(connectMsg)
		if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			t.Fatalf("Bot %d failed to send connect message: %v", i, err)
		}
	}

	// Wait for hand to start
	time.Sleep(200 * time.Millisecond)

	// Track hand completion
	handCompleted := make(chan bool, 3)

	// Each bot folds when asked to act
	for i, conn := range bots {
		go func(botNum int, c *websocket.Conn) {
			for {
				_, data, err := c.ReadMessage()
				if err != nil {
					return
				}

				// Check for action request
				var actionReq protocol.ActionRequest
				if err := protocol.Unmarshal(data, &actionReq); err == nil && actionReq.Type == "action_request" {
					// Fold immediately
					action := &protocol.Action{
						Type:   "action",
						Action: "fold",
						Amount: 0,
					}
					respData, _ := protocol.Marshal(action)
					c.WriteMessage(websocket.BinaryMessage, respData)
				}

				// Check for hand result
				var handResult protocol.HandResult
				if err := protocol.Unmarshal(data, &handResult); err == nil && handResult.Type == "hand_result" {
					handCompleted <- true
					return
				}
			}
		}(i, conn)
	}

	// Wait for hand to complete
	select {
	case <-handCompleted:
		t.Log("Hand completed successfully with everyone folding")
	case <-time.After(5 * time.Second):
		t.Fatal("Hand did not complete within timeout")
	}
}

func testShowdownWithMultiplePlayers(t *testing.T) {
	host := startServerForTest(t)

	// Connect 4 bots
	bots := make([]*websocket.Conn, 4)
	for i := 0; i < 4; i++ {
		u := url.URL{Scheme: "ws", Host: host, Path: "/ws"}
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			t.Fatalf("Bot %d failed to connect: %v", i, err)
		}
		bots[i] = conn
		connRef := conn
		t.Cleanup(func() {
			_ = connRef.Close()
		})

		// Send connect message
		connectMsg := &protocol.Connect{
			Type: protocol.TypeConnect,
			Name: fmt.Sprintf("ShowdownBot%d", i),
			Role: string(BotRolePlayer),
		}
		data, _ := protocol.Marshal(connectMsg)
		if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			t.Fatalf("Bot %d failed to send connect message: %v", i, err)
		}
	}

	// Wait for hand to start
	time.Sleep(200 * time.Millisecond)

	// Track streets and hand completion
	streetsSeen := make(map[string]bool)
	handCompleted := make(chan bool, 4)
	var streetMutex sync.Mutex

	// Each bot checks/calls to showdown
	for i, conn := range bots {
		go func(botNum int, c *websocket.Conn) {
			for {
				_, data, err := c.ReadMessage()
				if err != nil {
					return
				}

				// Track street changes
				var streetChange protocol.StreetChange
				if err := protocol.Unmarshal(data, &streetChange); err == nil && streetChange.Type == "street_change" {
					streetMutex.Lock()
					streetsSeen[streetChange.Street] = true
					streetMutex.Unlock()
				}

				// Check for action request
				var actionReq protocol.ActionRequest
				if err := protocol.Unmarshal(data, &actionReq); err == nil && actionReq.Type == "action_request" {
					// Check if possible, otherwise call
					action := "call"
					for _, validAction := range actionReq.ValidActions {
						if validAction == "check" {
							action = "check"
							break
						}
					}

					actionMsg := &protocol.Action{
						Type:   "action",
						Action: action,
						Amount: 0,
					}
					respData, _ := protocol.Marshal(actionMsg)
					c.WriteMessage(websocket.BinaryMessage, respData)
				}

				// Check for hand result
				var handResult protocol.HandResult
				if err := protocol.Unmarshal(data, &handResult); err == nil && handResult.Type == "hand_result" {
					// Verify we saw all streets
					streetMutex.Lock()
					if !streetsSeen["flop"] || !streetsSeen["turn"] || !streetsSeen["river"] {
						t.Error("Did not see all streets before showdown")
					}
					streetMutex.Unlock()

					// Verify we have winners
					if len(handResult.Winners) == 0 {
						t.Error("No winners in hand result")
					}

					handCompleted <- true
					return
				}
			}
		}(i, conn)
	}

	// Wait for hand to complete
	select {
	case <-handCompleted:
		t.Log("Hand completed successfully with showdown")
	case <-time.After(5 * time.Second):
		t.Fatal("Hand did not complete within timeout")
	}
}

func testAllInCascade(t *testing.T) {
	host := startServerForTest(t)

	// Connect 3 bots
	bots := make([]*websocket.Conn, 3)
	for i := 0; i < 3; i++ {
		u := url.URL{Scheme: "ws", Host: host, Path: "/ws"}
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			t.Fatalf("Bot %d failed to connect: %v", i, err)
		}
		bots[i] = conn
		connRef := conn
		t.Cleanup(func() {
			_ = connRef.Close()
		})

		// Send connect message
		connectMsg := &protocol.Connect{
			Type: protocol.TypeConnect,
			Name: fmt.Sprintf("AllInBot%d", i),
			Role: string(BotRolePlayer),
		}
		data, _ := protocol.Marshal(connectMsg)
		if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			t.Fatalf("Bot %d failed to send connect message: %v", i, err)
		}
	}

	// Wait for hand to start
	time.Sleep(200 * time.Millisecond)

	handCompleted := make(chan bool, 3)

	// Each bot goes all-in when possible
	for i, conn := range bots {
		go func(botNum int, c *websocket.Conn) {
			for {
				_, data, err := c.ReadMessage()
				if err != nil {
					return
				}

				// Check for action request
				var actionReq protocol.ActionRequest
				if err := protocol.Unmarshal(data, &actionReq); err == nil && actionReq.Type == "action_request" {
					// Go all-in if possible, otherwise call
					action := "call"
					for _, validAction := range actionReq.ValidActions {
						if validAction == "allin" {
							action = "allin"
							break
						} else if validAction == "raise" && botNum == 0 {
							// First bot raises to trigger all-ins
							action = "raise"
							break
						}
					}

					amount := 0
					if action == "raise" {
						amount = actionReq.Pot * 3 // Big raise
					}

					actionMsg := &protocol.Action{
						Type:   "action",
						Action: action,
						Amount: amount,
					}
					respData, _ := protocol.Marshal(actionMsg)
					c.WriteMessage(websocket.BinaryMessage, respData)
				}

				// Check for hand result
				var handResult protocol.HandResult
				if err := protocol.Unmarshal(data, &handResult); err == nil && handResult.Type == "hand_result" {
					// Note: Side pots info not available in HandResult message
					// Would need GameUpdate to track pot information
					handCompleted <- true
					return
				}
			}
		}(i, conn)
	}

	// Wait for hand to complete
	select {
	case <-handCompleted:
		t.Log("Hand completed successfully with all-ins")
		// Note: Side pots might not always occur depending on chip distribution
	case <-time.After(5 * time.Second):
		t.Fatal("Hand did not complete within timeout")
	}
}

func testHeadsUpPlay(t *testing.T) {
	host := startServerForTest(t)

	// Connect exactly 2 bots for heads-up
	bots := make([]*websocket.Conn, 2)
	for i := 0; i < 2; i++ {
		u := url.URL{Scheme: "ws", Host: host, Path: "/ws"}
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			t.Fatalf("Bot %d failed to connect: %v", i, err)
		}
		bots[i] = conn
		connRef := conn
		t.Cleanup(func() {
			_ = connRef.Close()
		})

		// Send connect message
		connectMsg := &protocol.Connect{
			Type: protocol.TypeConnect,
			Name: fmt.Sprintf("HeadsUpBot%d", i),
			Role: string(BotRolePlayer),
		}
		data, _ := protocol.Marshal(connectMsg)
		if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			t.Fatalf("Bot %d failed to send connect message: %v", i, err)
		}
	}

	// Wait for hand to start
	time.Sleep(200 * time.Millisecond)

	handCompleted := make(chan bool, 2)
	var mu sync.Mutex
	buttonActedFirst := false
	preflopActionCount := 0

	// Track heads-up specific rules
	for i, conn := range bots {
		go func(botNum int, c *websocket.Conn) {
			myPosition := -1
			actedPreflop := false

			for {
				_, data, err := c.ReadMessage()
				if err != nil {
					return
				}

				// Track hand start to know our position
				var handStart protocol.HandStart
				if err := protocol.Unmarshal(data, &handStart); err == nil && handStart.Type == "hand_start" {
					myPosition = handStart.YourSeat
					mu.Lock()
					if !buttonActedFirst && myPosition == 0 && actedPreflop {
						buttonActedFirst = true
					}
					mu.Unlock()
				}

				// Check for action request
				var actionReq protocol.ActionRequest
				if err := protocol.Unmarshal(data, &actionReq); err == nil && actionReq.Type == "action_request" {
					// Track preflop action order
					mu.Lock()
					if preflopActionCount == 0 {
						actedPreflop = true
						if myPosition == 0 {
							buttonActedFirst = true
						}
					}
					preflopActionCount++
					mu.Unlock()

					// Check or call
					action := "call"
					for _, validAction := range actionReq.ValidActions {
						if validAction == "check" {
							action = "check"
							break
						}
					}

					actionMsg := &protocol.Action{
						Type:   "action",
						Action: action,
						Amount: 0,
					}
					respData, _ := protocol.Marshal(actionMsg)
					c.WriteMessage(websocket.BinaryMessage, respData)
				}

				// Check for hand result
				var handResult protocol.HandResult
				if err := protocol.Unmarshal(data, &handResult); err == nil && handResult.Type == "hand_result" {
					// Verify heads-up rules were followed
					mu.Lock()
					if !buttonActedFirst {
						t.Error("Button did not act first preflop in heads-up")
					}
					mu.Unlock()

					handCompleted <- true
					return
				}
			}
		}(i, conn)
	}

	// Wait for hand to complete
	select {
	case <-handCompleted:
		t.Log("Heads-up hand completed successfully")
	case <-time.After(5 * time.Second):
		t.Fatal("Hand did not complete within timeout")
	}
}

// TestRapidConnectionDisconnection tests rapid connections and disconnections
func TestRapidConnectionDisconnection(t *testing.T) {
	t.Parallel()
	host := startServerForTest(t)

	// Rapidly connect and disconnect bots
	var wg sync.WaitGroup
	connectionErrors := 0
	var errorMutex sync.Mutex

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(botNum int) {
			defer wg.Done()

			u := url.URL{Scheme: "ws", Host: host, Path: "/ws"}
			conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
			if err != nil {
				errorMutex.Lock()
				connectionErrors++
				errorMutex.Unlock()
				return
			}
			defer conn.Close()

			// Send connect message
			connectMsg := &protocol.Connect{
				Type: protocol.TypeConnect,
				Name: fmt.Sprintf("RapidBot%d", botNum),
				Role: string(BotRolePlayer),
			}
			data, _ := protocol.Marshal(connectMsg)
			if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				errorMutex.Lock()
				connectionErrors++
				errorMutex.Unlock()
				return
			}

			// Random delay before disconnect
			// Use staggered waits to simulate churn without overwhelming the server.
			time.Sleep(time.Duration(50+botNum*10) * time.Millisecond)

			// Disconnect
		}(i)

		// Small delay between connection attempts to reduce contention
		time.Sleep(10 * time.Millisecond)
	}

	wg.Wait()

	if connectionErrors > 0 {
		t.Fatalf("Had %d connection errors during rapid connect/disconnect", connectionErrors)
	}

	// Verify server is still responsive
	u := url.URL{Scheme: "ws", Host: host, Path: "/ws"}
	testConn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("Server not responsive after rapid connections: %v", err)
	}
	defer testConn.Close()

	connectMsg := &protocol.Connect{
		Type: protocol.TypeConnect,
		Name: "RapidBotCheck",
		Role: string(BotRolePlayer),
	}
	data, _ := protocol.Marshal(connectMsg)
	if err := testConn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		t.Fatalf("Failed to complete post-test handshake: %v", err)
	}

	t.Log("Server handled rapid connections successfully")
}

// TestLoadWith20Bots tests the server with 20+ concurrent bots
func TestLoadWith20Bots(t *testing.T) {
	t.Parallel()
	host := startServerForTest(t)

	numBots := 24
	bots := make([]*websocket.Conn, numBots)
	handsCompleted := 0
	var handMutex sync.Mutex

	// Connect all bots
	for i := 0; i < numBots; i++ {
		u := url.URL{Scheme: "ws", Host: host, Path: "/ws"}
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			t.Fatalf("Bot %d failed to connect: %v", i, err)
		}
		bots[i] = conn
		connRef := conn
		t.Cleanup(func() {
			_ = connRef.Close()
		})

		// Send connect message
		connectMsg := &protocol.Connect{
			Type: protocol.TypeConnect,
			Name: fmt.Sprintf("LoadBot%d", i),
			Role: string(BotRolePlayer),
		}
		data, _ := protocol.Marshal(connectMsg)
		if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			t.Fatalf("Bot %d failed to send connect message: %v", i, err)
		}
	}

	// Run each bot
	var wg sync.WaitGroup
	done := make(chan struct{})
	var doneOnce sync.Once
	targetHands := 10

	for i, conn := range bots {
		wg.Add(1)
		go func(botNum int, c *websocket.Conn) {
			defer wg.Done()

			for {
				select {
				case <-done:
					return
				default:
				}

				_, data, err := c.ReadMessage()
				if err != nil {
					return
				}

				// Respond to action requests
				var actionReq protocol.ActionRequest
				if err := protocol.Unmarshal(data, &actionReq); err == nil && actionReq.Type == "action_request" {
					// Simple strategy: check/call
					action := "call"
					for _, validAction := range actionReq.ValidActions {
						if validAction == "check" {
							action = "check"
							break
						}
					}

					actionMsg := &protocol.Action{
						Type:   "action",
						Action: action,
						Amount: 0,
					}
					respData, _ := protocol.Marshal(actionMsg)
					if err := c.WriteMessage(websocket.BinaryMessage, respData); err != nil {
						return
					}
				}

				// Count completed hands
				var handResult protocol.HandResult
				if err := protocol.Unmarshal(data, &handResult); err == nil && handResult.Type == "hand_result" {
					handMutex.Lock()
					handsCompleted++
					reached := handsCompleted >= targetHands
					handMutex.Unlock()

					if reached {
						doneOnce.Do(func() { close(done) })
						return
					}
				}
			}
		}(i, conn)
	}

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		handMutex.Lock()
		finalCount := handsCompleted
		handMutex.Unlock()
		t.Fatalf("Timed out completing hands: only %d finished with %d bots", finalCount, numBots)
	}

	// Close connections proactively to ensure goroutines exit cleanly, then wait.
	for _, conn := range bots {
		if conn != nil {
			_ = conn.Close()
		}
	}

	wg.Wait()

	handMutex.Lock()
	finalCount := handsCompleted
	handMutex.Unlock()

	if finalCount < targetHands {
		t.Fatalf("Only completed %d hands with %d bots", finalCount, numBots)
	}

	t.Logf("Successfully completed %d hands with %d bots", finalCount, numBots)
}
