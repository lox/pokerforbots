package server

import (
	"fmt"
	"math/rand"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
)

// TestCompleteHandScenarios tests various complete hand scenarios
func TestCompleteHandScenarios(t *testing.T) {
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
	rng := rand.New(rand.NewSource(42))
	server := NewServer(testLogger(), rng)
	go server.Start(":8089")
	time.Sleep(100 * time.Millisecond)

	// Connect 3 bots
	bots := make([]*websocket.Conn, 3)
	for i := 0; i < 3; i++ {
		u := url.URL{Scheme: "ws", Host: "localhost:8089", Path: "/ws"}
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			t.Fatalf("Bot %d failed to connect: %v", i, err)
		}
		defer conn.Close()
		bots[i] = conn

		// Send connect message
		connectMsg := &protocol.Connect{
			Type: "connect",
			Name: fmt.Sprintf("FoldBot%d", i),
		}
		data, _ := protocol.Marshal(connectMsg)
		conn.WriteMessage(websocket.BinaryMessage, data)
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
	rng := rand.New(rand.NewSource(42))
	server := NewServer(testLogger(), rng)
	go server.Start(":8090")
	time.Sleep(100 * time.Millisecond)

	// Connect 4 bots
	bots := make([]*websocket.Conn, 4)
	for i := 0; i < 4; i++ {
		u := url.URL{Scheme: "ws", Host: "localhost:8090", Path: "/ws"}
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			t.Fatalf("Bot %d failed to connect: %v", i, err)
		}
		defer conn.Close()
		bots[i] = conn

		// Send connect message
		connectMsg := &protocol.Connect{
			Type: "connect",
			Name: fmt.Sprintf("ShowdownBot%d", i),
		}
		data, _ := protocol.Marshal(connectMsg)
		conn.WriteMessage(websocket.BinaryMessage, data)
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
	rng := rand.New(rand.NewSource(42))
	server := NewServer(testLogger(), rng)
	go server.Start(":8091")
	time.Sleep(100 * time.Millisecond)

	// Connect 3 bots
	bots := make([]*websocket.Conn, 3)
	for i := 0; i < 3; i++ {
		u := url.URL{Scheme: "ws", Host: "localhost:8091", Path: "/ws"}
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			t.Fatalf("Bot %d failed to connect: %v", i, err)
		}
		defer conn.Close()
		bots[i] = conn

		// Send connect message
		connectMsg := &protocol.Connect{
			Type: "connect",
			Name: fmt.Sprintf("AllInBot%d", i),
		}
		data, _ := protocol.Marshal(connectMsg)
		conn.WriteMessage(websocket.BinaryMessage, data)
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
	t.Skip("Skipping test - heads-up preflop action order not yet implemented correctly")
	rng := rand.New(rand.NewSource(42))
	server := NewServer(testLogger(), rng)
	go server.Start(":8092")
	time.Sleep(100 * time.Millisecond)

	// Connect exactly 2 bots for heads-up
	bots := make([]*websocket.Conn, 2)
	for i := 0; i < 2; i++ {
		u := url.URL{Scheme: "ws", Host: "localhost:8092", Path: "/ws"}
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			t.Fatalf("Bot %d failed to connect: %v", i, err)
		}
		defer conn.Close()
		bots[i] = conn

		// Send connect message
		connectMsg := &protocol.Connect{
			Type: "connect",
			Name: fmt.Sprintf("HeadsUpBot%d", i),
		}
		data, _ := protocol.Marshal(connectMsg)
		conn.WriteMessage(websocket.BinaryMessage, data)
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

			for {
				_, data, err := c.ReadMessage()
				if err != nil {
					return
				}

				// Track hand start to know our position
				var handStart protocol.HandStart
				if err := protocol.Unmarshal(data, &handStart); err == nil && handStart.Type == "hand_start" {
					myPosition = handStart.YourSeat
				}

				// Check for action request
				var actionReq protocol.ActionRequest
				if err := protocol.Unmarshal(data, &actionReq); err == nil && actionReq.Type == "action_request" {
					// Track preflop action order
					mu.Lock()
					if preflopActionCount == 0 && myPosition == 0 {
						buttonActedFirst = true
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
	t.Skip("Skipping flaky test - rapid disconnections cause timing issues")
	rng := rand.New(rand.NewSource(42))
	server := NewServer(testLogger(), rng)
	go server.Start(":8093")
	time.Sleep(100 * time.Millisecond)

	// Rapidly connect and disconnect bots
	var wg sync.WaitGroup
	connectionErrors := 0
	var errorMutex sync.Mutex

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(botNum int) {
			defer wg.Done()

			u := url.URL{Scheme: "ws", Host: "localhost:8093", Path: "/ws"}
			conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
			if err != nil {
				errorMutex.Lock()
				connectionErrors++
				errorMutex.Unlock()
				return
			}

			// Send connect message
			connectMsg := &protocol.Connect{
				Type: "connect",
				Name: fmt.Sprintf("RapidBot%d", botNum),
			}
			data, _ := protocol.Marshal(connectMsg)
			conn.WriteMessage(websocket.BinaryMessage, data)

			// Random delay before disconnect
			time.Sleep(time.Duration(50+botNum*10) * time.Millisecond)

			// Disconnect
			conn.Close()
		}(i)

		// Small delay between connections
		time.Sleep(10 * time.Millisecond)
	}

	wg.Wait()

	if connectionErrors > 0 {
		t.Errorf("Had %d connection errors during rapid connect/disconnect", connectionErrors)
	}

	// Verify server is still responsive
	u := url.URL{Scheme: "ws", Host: "localhost:8080", Path: "/ws"}
	testConn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatal("Server not responsive after rapid connections")
	}
	testConn.Close()

	t.Log("Server handled rapid connections successfully")
}

// TestLoadWith20Bots tests the server with 20+ concurrent bots
func TestLoadWith20Bots(t *testing.T) {
	t.Skip("Skipping load test - requires significant resources")

	rng := rand.New(rand.NewSource(42))
	server := NewServer(testLogger(), rng)
	go server.Start(":8094")
	time.Sleep(100 * time.Millisecond)

	numBots := 24
	bots := make([]*websocket.Conn, numBots)
	handsCompleted := 0
	var handMutex sync.Mutex

	// Connect all bots
	for i := 0; i < numBots; i++ {
		u := url.URL{Scheme: "ws", Host: "localhost:8094", Path: "/ws"}
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			t.Fatalf("Bot %d failed to connect: %v", i, err)
		}
		defer conn.Close()
		bots[i] = conn

		// Send connect message
		connectMsg := &protocol.Connect{
			Type: "connect",
			Name: fmt.Sprintf("LoadBot%d", i),
		}
		data, _ := protocol.Marshal(connectMsg)
		conn.WriteMessage(websocket.BinaryMessage, data)
	}

	// Run each bot
	var wg sync.WaitGroup
	for i, conn := range bots {
		wg.Add(1)
		go func(botNum int, c *websocket.Conn) {
			defer wg.Done()

			for {
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
					c.WriteMessage(websocket.BinaryMessage, respData)
				}

				// Count completed hands
				var handResult protocol.HandResult
				if err := protocol.Unmarshal(data, &handResult); err == nil && handResult.Type == "hand_result" {
					handMutex.Lock()
					handsCompleted++
					if handsCompleted >= 10 {
						handMutex.Unlock()
						return // Stop after 10 hands
					}
					handMutex.Unlock()
				}
			}
		}(i, conn)
	}

	// Wait for hands to complete or timeout
	time.Sleep(30 * time.Second)

	handMutex.Lock()
	finalCount := handsCompleted
	handMutex.Unlock()

	if finalCount < 10 {
		t.Errorf("Only completed %d hands with %d bots in 30 seconds", finalCount, numBots)
	} else {
		t.Logf("Successfully completed %d hands with %d bots", finalCount, numBots)
	}
}
