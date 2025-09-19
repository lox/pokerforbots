package server

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
)

// TestNoEmptyValidActions is an integration test that verifies the fix for
// empty valid_actions arrays caused by buffer aliasing race conditions
func TestNoEmptyValidActions(t *testing.T) {
	t.Parallel()
	// Start test server
	rng := rand.New(rand.NewSource(42))
	server := NewServer(testLogger(), rng)
	stopPool := startTestPool(t, server.pool)
	defer stopPool()

	ts := httptest.NewServer(http.HandlerFunc(server.handleWebSocket))
	defer ts.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"

	// Connect multiple bots concurrently to create race conditions
	numBots := 4
	var wg sync.WaitGroup

	emptyActionsFound := make(chan string, 10) // Buffer for error messages
	stopBots := make(chan struct{})            // Signal to stop all bots

	for i := 0; i < numBots; i++ {
		wg.Add(1)
		go func(botID int) {
			defer wg.Done()

			botName := fmt.Sprintf("TestBot%d", botID)

			// Connect to server
			conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				t.Errorf("Bot %d connection failed: %v", botID, err)
				return
			}
			defer conn.Close()

			// Send connect message
			connectMsg := &protocol.Connect{
				Type: protocol.TypeConnect,
				Name: botName,
				Role: string(BotRolePlayer),
			}
			if data, err := protocol.Marshal(connectMsg); err == nil {
				conn.WriteMessage(websocket.BinaryMessage, data)
			}

			// Process messages until told to stop
			for {
				select {
				case <-stopBots:
					return
				default:
				}

				// Set a short read timeout so we can check for stop signal
				conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
				_, data, err := conn.ReadMessage()
				if err != nil {
					// Check if it's just a timeout (which is expected)
					if netErr, ok := err.(interface{ Timeout() bool }); ok && netErr.Timeout() {
						continue
					}
					t.Logf("Bot %d read error: %v", botID, err)
					return
				}

				// Try to parse as ActionRequest
				var actionReq protocol.ActionRequest
				if err := protocol.Unmarshal(data, &actionReq); err == nil && actionReq.Type == "action_request" {
					// Check for empty valid actions - this should NEVER happen after the fix
					if len(actionReq.ValidActions) == 0 {
						errMsg := fmt.Sprintf("Bot %d (%s) received ActionRequest with empty ValidActions in hand %s (pot=%d, to_call=%d)",
							botID, botName, actionReq.HandID, actionReq.Pot, actionReq.ToCall)
						select {
						case emptyActionsFound <- errMsg:
						default:
						}
						return
					}

					// Send a valid action back
					if len(actionReq.ValidActions) > 0 {
						action := &protocol.Action{
							Type:   "action",
							Action: actionReq.ValidActions[0], // Always take first valid action (fold/check/call)
							Amount: 0,
						}
						if data, err := protocol.Marshal(action); err == nil {
							conn.WriteMessage(websocket.BinaryMessage, data)
						}
					}
				}
			}
		}(i)
	}

	// Run the test for a fixed duration to allow for race conditions to manifest
	testDuration := 5 * time.Second

	select {
	case errMsg := <-emptyActionsFound:
		t.Fatalf("RACE CONDITION BUG DETECTED: %s", errMsg)
	case <-time.After(testDuration):
		// Test duration completed - stop all bots
		close(stopBots)
		t.Log("Test duration completed, stopping bots")
	}

	// Wait for all bots to finish with a reasonable timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All bots finished successfully
		t.Log("All bots stopped successfully")
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for bots to stop")
	}

	// Check if any empty actions were found during the test
	select {
	case errMsg := <-emptyActionsFound:
		t.Fatalf("RACE CONDITION BUG DETECTED: %s", errMsg)
	default:
		// No errors found - test passed
	}
}
