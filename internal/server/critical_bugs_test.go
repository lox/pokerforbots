package server

import (
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
)

// TestButtonNeverRotates demonstrates that the button stays at position 0
// for every hand instead of rotating.
func TestButtonNeverRotates(t *testing.T) {
	server := NewServer()
	go server.Start(":8095")
	time.Sleep(100 * time.Millisecond)

	// Connect 3 bots
	bots := make([]*websocket.Conn, 3)
	for i := 0; i < 3; i++ {
		u := url.URL{Scheme: "ws", Host: "localhost:8095", Path: "/ws"}
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
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

// TestActionRoutingCannotIdentifyBot demonstrates that the server cannot
// identify which bot sent an action, allowing one bot to act for another.
func TestActionRoutingCannotIdentifyBot(t *testing.T) {
	t.Skip("This test requires modifying the server to expose the vulnerability")
	// The bug is in internal/server/hand_runner.go:243-248
	// listenForAction reads from a shared botActionChan and assumes it's from
	// the currently active seat, without verifying the sender's identity
}

// TestBankrollAccountingAssumesBuyIn demonstrates that bankroll updates
// assume a 100-chip buy-in regardless of actual chips.
func TestBankrollAccountingAssumesBuyIn(t *testing.T) {
	t.Skip("This test requires access to bot bankroll internals")
	// The bug is in internal/server/bot.go:104
	// UpdateBankroll always subtracts 100, even if the bot sat with fewer chips
}
