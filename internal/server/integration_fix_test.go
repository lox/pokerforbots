package server

import (
	"fmt"
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
	// Start test server
	server := NewServer()
	go server.pool.Run()

	ts := httptest.NewServer(http.HandlerFunc(server.handleWebSocket))
	defer ts.Close()

	// Convert http:// to ws://
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"

	// Connect multiple bots concurrently to create race conditions
	numBots := 4
	numHandsPerBot := 5
	var wg sync.WaitGroup

	emptyActionsFound := make(chan string, 10) // Buffer for error messages

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
				Type: "connect",
				Name: botName,
			}
			if data, err := protocol.Marshal(connectMsg); err == nil {
				conn.WriteMessage(websocket.BinaryMessage, data)
			}

			// Process messages for several hands
			handCount := 0
			for handCount < numHandsPerBot {
				_, data, err := conn.ReadMessage()
				if err != nil {
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

				// Check for hand result to count completed hands
				var handResult protocol.HandResult
				if err := protocol.Unmarshal(data, &handResult); err == nil && handResult.Type == "hand_result" {
					handCount++
				}
			}
		}(i)
	}

	// Wait for all bots to finish or timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All bots finished successfully
		t.Log("All bots completed successfully")
	case errMsg := <-emptyActionsFound:
		t.Fatalf("RACE CONDITION BUG DETECTED: %s", errMsg)
	case <-time.After(30 * time.Second):
		t.Fatal("Test timed out - possible infinite loop or deadlock")
	}

	// Check if any empty actions were found during the test
	select {
	case errMsg := <-emptyActionsFound:
		t.Fatalf("RACE CONDITION BUG DETECTED: %s", errMsg)
	default:
		// No errors found - test passed
	}
}
