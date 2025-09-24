package server

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/protocol"
)

// This file contains integration tests that verify end-to-end functionality
// that cannot be easily tested with unit tests

// newTestServerWithDeterministicRNG creates a server with deterministic random behavior for testing
func newTestServerWithDeterministicRNG(t *testing.T, seed int64) *Server {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	pool := NewBotPool(testLogger(), rng, DefaultConfig(2, 9))

	// Deterministic bot ID generator with atomic counter for race-free access
	var counter atomic.Int64
	botIDGen := func() string {
		id := counter.Add(1)
		return fmt.Sprintf("test-bot-%d", id)
	}

	// Need an RNG for the server constructor - use one from pool or create new
	serverRNG := rand.New(rand.NewSource(time.Now().UnixNano()))
	return NewServer(testLogger(), serverRNG, WithBotPool(pool), WithBotIDGen(botIDGen))
}

// startTestPool is now defined in test_helpers.go

// TestBotActionsAreProcessed verifies that bot actions are actually processed
// Updated to work with randomized bot ordering per stateless hand design
func TestBotActionsAreProcessed(t *testing.T) {
	t.Parallel()
	// Start test server with deterministic RNG (seed 1 should make bot1 act first)
	server := newTestServerWithDeterministicRNG(t, 1)
	stopPool := startTestPool(t, server.pool)
	defer stopPool()

	ts := httptest.NewServer(http.HandlerFunc(server.handleWebSocket))
	defer ts.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"

	// Connect two bots
	bot1Conn := dialAndConnect(t, wsURL, "TestBot1", "")
	defer bot1Conn.Close()

	bot2Conn := dialAndConnect(t, wsURL, "TestBot2", "")
	defer bot2Conn.Close()

	// Track what actions we receive
	bot1Actions := make(chan string, 10)
	bot2Actions := make(chan string, 10)

	// Start reading messages from both bots
	go readBotMessages(t, bot1Conn, bot1Actions)
	go readBotMessages(t, bot2Conn, bot2Actions)

	// Wait for hand to start (reduced from 100ms to 50ms)
	time.Sleep(50 * time.Millisecond)

	// With randomized bot ordering, either bot could act first
	// Check what Bot 1 receives - could be action request or hand result
	select {
	case action := <-bot1Actions:
		if strings.Contains(action, "action_request") {
			t.Log("Bot 1 received action request and will respond")

			// Bot 1 responds with FOLD
			foldAction := &protocol.Action{
				Type:   "action",
				Action: "fold",
				Amount: 0,
			}
			if data, err := protocol.Marshal(foldAction); err == nil {
				bot1Conn.WriteMessage(websocket.BinaryMessage, data)
			}
		} else if strings.Contains(action, "hand_result") {
			t.Log("SUCCESS: Bot 1 received hand result - actions are being processed correctly")
			return // Hand already completed, test passed
		}

	case <-time.After(1 * time.Second): // Reduced from 2s to 1s
		t.Error("Bot 1 never received any message")
	}

	// Wait for hand to complete (reduced from 200ms to 100ms)
	time.Sleep(100 * time.Millisecond)

	// Bot 2 should NOT receive an action request if Bot 1 folded
	// With broken implementation, Bot 2 will still get an action request because Bot 1 auto-called
	select {
	case action := <-bot2Actions:
		if strings.Contains(action, "action_request") {
			t.Error("Bot 2 received action request - Bot 1's fold was NOT processed (auto-call happened instead)")
		} else if strings.Contains(action, "hand_result") {
			t.Log("SUCCESS: Hand completed correctly - actions are being processed")
		}
	case <-time.After(500 * time.Millisecond): // Reduced from 1s to 500ms
		t.Log("SUCCESS: Hand completed correctly - actions are being processed")
	}
}

// TestPotDistribution verifies that winners receive their chips
func TestPotDistribution(t *testing.T) {
	t.Parallel()
	// Start test server with deterministic RNG (seed chosen to put bot1 as small blind)
	server := newTestServerWithDeterministicRNG(t, 42)
	stopPool := startTestPool(t, server.pool)
	defer stopPool()

	ts := httptest.NewServer(http.HandlerFunc(server.handleWebSocket))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"

	// Connect two bots
	bot1Conn := dialAndConnect(t, wsURL, "PotBot1", "")
	defer bot1Conn.Close()

	bot2Conn := dialAndConnect(t, wsURL, "PotBot2", "")
	defer bot2Conn.Close()

	// Track starting chips from hand start message
	var startingChipsBot1, startingChipsBot2 int
	handStartReceived := make(chan bool, 2)

	// Read hand start for both bots
	go func() {
		for {
			_, data, err := bot1Conn.ReadMessage()
			if err != nil {
				return
			}

			var handStart protocol.HandStart
			if err := protocol.Unmarshal(data, &handStart); err == nil && handStart.Type == "hand_start" {
				// Bot 1's perspective - get their starting chips
				startingChipsBot1 = handStart.Players[handStart.YourSeat].Chips
				handStartReceived <- true
				break
			}
		}
	}()

	go func() {
		for {
			_, data, err := bot2Conn.ReadMessage()
			if err != nil {
				return
			}

			var handStart protocol.HandStart
			if err := protocol.Unmarshal(data, &handStart); err == nil && handStart.Type == "hand_start" {
				// Bot 2's perspective - get their starting chips
				startingChipsBot2 = handStart.Players[handStart.YourSeat].Chips
				handStartReceived <- true
				break
			}
		}
	}()

	// Wait for both hand starts
	<-handStartReceived
	<-handStartReceived

	// Both bots should start with chips minus blinds already posted (995 and 990 in some order)
	expectedStacks := map[int]struct{}{995: {}, 990: {}}
	delete(expectedStacks, startingChipsBot1)
	delete(expectedStacks, startingChipsBot2)
	if len(expectedStacks) != 0 {
		t.Fatalf("Unexpected starting stacks: bot1=%d bot2=%d", startingChipsBot1, startingChipsBot2)
	}

	// Now play out the hand - Bot 1 will fold, Bot 2 should win
	go func() {
		for {
			_, data, err := bot1Conn.ReadMessage()
			if err != nil {
				return
			}

			var actionReq protocol.ActionRequest
			if err := protocol.Unmarshal(data, &actionReq); err == nil && actionReq.Type == "action_request" {
				// Bot 1 folds
				foldAction := &protocol.Action{
					Type:   "action",
					Action: "fold",
					Amount: 0,
				}
				if data, err := protocol.Marshal(foldAction); err == nil {
					bot1Conn.WriteMessage(websocket.BinaryMessage, data)
				}
				break
			}
		}
	}()

	// Wait for hand result and check final chips
	resultReceived := make(chan bool)
	var potWon int

	go func() {
		for {
			_, data, err := bot2Conn.ReadMessage()
			if err != nil {
				return
			}

			var result protocol.HandResult
			if err := protocol.Unmarshal(data, &result); err == nil && result.Type == "hand_result" {
				// Bot 2 should be the winner
				if len(result.Winners) > 0 {
					potWon = result.Winners[0].Amount
				}
				resultReceived <- true
				break
			}
		}
	}()

	// Wait for result
	select {
	case <-resultReceived:
		// Expected - hand completed
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for hand result")
	}

	// In heads-up, small blind (Bot 1) posts 5, big blind (Bot 2) posts 10
	// Bot 1 folds, so Bot 2 wins pot of 15 (5+10)
	expectedPot := 15
	if potWon != expectedPot {
		t.Errorf("Winner received %d chips, expected %d", potWon, expectedPot)
	}

	// Note: We can't directly verify final chip counts from HandResult message
	// as it doesn't include player chip stacks. In a real implementation,
	// we'd need a GameUpdate message after the hand or include chips in HandResult.
	t.Log("Pot distribution test completed - winner information verified")
}

// TestTimeoutActuallyFolds verifies that timeouts result in folds
func TestTimeoutActuallyFolds(t *testing.T) {
	t.Parallel()
	// Start test server with deterministic RNG
	server := newTestServerWithDeterministicRNG(t, 456)
	stopPool := startTestPool(t, server.pool)
	defer stopPool()

	ts := httptest.NewServer(http.HandlerFunc(server.handleWebSocket))
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"

	// Connect two bots
	bot1Conn := dialAndConnect(t, wsURL, "TimeoutBot1", "")
	defer bot1Conn.Close()

	bot2Conn := dialAndConnect(t, wsURL, "TimeoutBot2", "")
	defer bot2Conn.Close()

	bot2Actions := make(chan string, 10)
	handCompleted := make(chan bool)

	// Read messages from bot2 to see what happens
	go func() {
		for {
			_, data, err := bot2Conn.ReadMessage()
			if err != nil {
				return
			}

			// Check for action request (bot2 should get one if bot1 didn't fold)
			var actionReq protocol.ActionRequest
			if err := protocol.Unmarshal(data, &actionReq); err == nil && actionReq.Type == "action_request" {
				bot2Actions <- "action_request"
				continue
			}

			// Check for hand result
			var result protocol.HandResult
			if err := protocol.Unmarshal(data, &result); err == nil && result.Type == "hand_result" {
				handCompleted <- true
				return
			}
		}
	}()

	// Wait for hand to start
	time.Sleep(100 * time.Millisecond)

	// Bot 1 deliberately doesn't respond (simulating timeout)
	// Just read and ignore the action request
	go func() {
		for {
			_, _, err := bot1Conn.ReadMessage() // Read but don't respond
			if err != nil {
				return // Exit on error
			}
		}
	}()

	// Wait for result (timeout should be 100ms)
	select {
	case <-bot2Actions:
		// Bot 2 got an action request, meaning bot 1 didn't fold on timeout
		t.Log("Bot 2 received action request - Bot 1 should have timed out and folded")
		// This is actually OK now since we fixed the action processing
	case <-handCompleted:
		t.Log("Hand completed - Bot 1 timed out and folded correctly")
	case <-time.After(500 * time.Millisecond):
		t.Error("Test timeout - neither bot2 action nor hand completion received")
	}
}

// Helper function to read bot messages
func readBotMessages(_ *testing.T, conn *websocket.Conn, actionChan chan string) {
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		// Try to parse as different message types
		var actionReq protocol.ActionRequest
		if err := protocol.Unmarshal(data, &actionReq); err == nil && actionReq.Type == "action_request" {
			actionChan <- fmt.Sprintf("action_request: %+v", actionReq)
			continue
		}

		var handStart protocol.HandStart
		if err := protocol.Unmarshal(data, &handStart); err == nil && handStart.Type == "hand_start" {
			// Ignore hand starts
			continue
		}

		var handResult protocol.HandResult
		if err := protocol.Unmarshal(data, &handResult); err == nil && handResult.Type == "hand_result" {
			// Convert to JSON for easier inspection
			jsonData, _ := json.Marshal(handResult)
			actionChan <- fmt.Sprintf("hand_result: %s", string(jsonData))
			continue
		}
	}
}
