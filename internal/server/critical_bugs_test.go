package server

import (
	"fmt"
	"math/rand"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lox/pokerforbots/internal/protocol"
)

// TestButtonAssignedToFirstSeat ensures the dealer button is always given to seat 0 after shuffling.
func TestButtonAssignedToFirstSeat(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewSource(42))
	s := NewServer(testLogger(), rng)
	s.pool.SetMatchInterval(5 * time.Millisecond)

	s.mux.HandleFunc("/ws", s.handleWebSocket)
	s.mux.HandleFunc("/health", s.handleHealth)
	s.mux.HandleFunc("/stats", s.handleStats)

	testServer := httptest.NewServer(s.mux)
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

	wsURL := strings.Replace(testServer.URL, "http://", "ws://", 1) + "/ws"

	bots := make([]*websocket.Conn, 3)
	for i := 0; i < 3; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Bot %d failed to connect: %v", i, err)
		}
		bots[i] = conn

		connectMsg := &protocol.Connect{
			Type: protocol.TypeConnect,
			Name: fmt.Sprintf("ButtonTestBot%d", i),
			Role: string(BotRolePlayer),
		}
		data, _ := protocol.Marshal(connectMsg)
		if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			t.Fatalf("Bot %d failed to send connect: %v", i, err)
		}
	}

	buttonPositions := []int{}
	var buttonMutex sync.Mutex
	seenHands := make(map[string]struct{})
	targetHands := 5

	done := make(chan struct{})
	var doneOnce sync.Once

	var botWg sync.WaitGroup
	for _, conn := range bots {
		botWg.Add(1)
		go func(c *websocket.Conn) {
			defer botWg.Done()
			for {
				_, data, err := c.ReadMessage()
				if err != nil {
					return
				}

				var handStart protocol.HandStart
				if err := protocol.Unmarshal(data, &handStart); err == nil && handStart.Type == "hand_start" {
					buttonMutex.Lock()
					buttonPositions = append(buttonPositions, handStart.Button)
					buttonMutex.Unlock()
				}

				var actionReq protocol.ActionRequest
				if err := protocol.Unmarshal(data, &actionReq); err == nil && actionReq.Type == "action_request" {
					action := &protocol.Action{Type: "action", Action: "fold"}
					respData, _ := protocol.Marshal(action)
					_ = c.WriteMessage(websocket.BinaryMessage, respData)
					continue
				}

				var handResult protocol.HandResult
				if err := protocol.Unmarshal(data, &handResult); err == nil && handResult.Type == "hand_result" {
					buttonMutex.Lock()
					if _, seen := seenHands[handResult.HandID]; !seen {
						seenHands[handResult.HandID] = struct{}{}
						if len(seenHands) >= targetHands {
							buttonMutex.Unlock()
							doneOnce.Do(func() { close(done) })
							return
						}
					}
					buttonMutex.Unlock()
				}
			}
		}(conn)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for button rotation check")
	}

	for _, conn := range bots {
		_ = conn.Close()
	}
	botWg.Wait()

	buttonMutex.Lock()
	positions := append([]int(nil), buttonPositions...)
	buttonMutex.Unlock()

	t.Logf("button positions: %v", positions)

	if len(positions) < targetHands*len(bots) {
		t.Fatalf("Expected at least %d button updates, got %d", targetHands*len(bots), len(positions))
	}

	for _, pos := range positions {
		if pos != 0 {
			t.Fatalf("Expected button to always be seat 0, saw %d", pos)
		}
	}

	t.Logf("Button assigned to seat 0 for all %d observations", len(positions))
}
