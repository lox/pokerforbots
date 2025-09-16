package server

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
)

// TestButtonNeverRotates demonstrates that the button stays at position 0
// for every hand instead of rotating.
func TestButtonNeverRotates(t *testing.T) {
	// Create test server
	s := NewServer()

	// Set up HTTP routes
	s.mux.HandleFunc("/ws", s.handleWebSocket)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/stats", s.handleStats)

	testServer := httptest.NewServer(s.mux)
	defer testServer.Close()

	// Start bot pool
	go s.pool.Run()

	// Extract host from test server URL
	wsURL := strings.Replace(testServer.URL, "http://", "ws://", 1) + "/ws"

	// Connect 3 bots
	bots := make([]*websocket.Conn, 3)
	for i := 0; i < 3; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Bot %d failed to connect: %v", i, err)
		}
		defer conn.Close()
		bots[i] = conn

		// Send connect message
		connectMsg := &protocol.Connect{
			Type: "connect",
			Name: fmt.Sprintf("ButtonTestBot%d", i),
		}
		data, _ := protocol.Marshal(connectMsg)
		conn.WriteMessage(websocket.BinaryMessage, data)
	}

	// Track button positions across hands
	buttonPositions := []int{}
	var buttonMutex sync.Mutex
	handsCompleted := 0

	// Each bot plays 3 hands and tracks button position
	for i, conn := range bots {
		go func(botNum int, c *websocket.Conn) {
			for {
				_, data, err := c.ReadMessage()
				if err != nil {
					return
				}

				// Track hand start to get button position
				var handStart protocol.HandStart
				if err := protocol.Unmarshal(data, &handStart); err == nil && handStart.Type == "hand_start" {
					buttonMutex.Lock()
					buttonPositions = append(buttonPositions, handStart.Button)
					t.Logf("Hand %d: Button at position %d", len(buttonPositions), handStart.Button)
					buttonMutex.Unlock()
				}

				// Respond to action requests (just fold to end quickly)
				var actionReq protocol.ActionRequest
				if err := protocol.Unmarshal(data, &actionReq); err == nil && actionReq.Type == "action_request" {
					action := &protocol.Action{
						Type:   "action",
						Action: "fold",
						Amount: 0,
					}
					respData, _ := protocol.Marshal(action)
					c.WriteMessage(websocket.BinaryMessage, respData)
				}

				// Count completed hands
				var handResult protocol.HandResult
				if err := protocol.Unmarshal(data, &handResult); err == nil && handResult.Type == "hand_result" {
					buttonMutex.Lock()
					handsCompleted++
					if handsCompleted >= 3 {
						buttonMutex.Unlock()
						return
					}
					buttonMutex.Unlock()
				}
			}
		}(i, conn)
	}

	// Wait for 3 hands to complete
	time.Sleep(2 * time.Second)

	// Check if button rotated
	buttonMutex.Lock()
	defer buttonMutex.Unlock()

	if len(buttonPositions) < 3 {
		t.Fatalf("Expected at least 3 hands, only got %d", len(buttonPositions))
	}

	// Check if all button positions are the same (bug)
	allSame := true
	for i := 1; i < len(buttonPositions); i++ {
		if buttonPositions[i] != buttonPositions[0] {
			allSame = false
			break
		}
	}

	if allSame {
		t.Errorf("BUG CONFIRMED: Button never rotates! It stayed at position %d for all %d hands",
			buttonPositions[0], len(buttonPositions))
		t.Logf("Button positions across hands: %v", buttonPositions)
	} else {
		t.Logf("Button rotated correctly across hands: %v", buttonPositions)
	}
}
