package protocol

import (
	"testing"
)

func TestConnectMessage(t *testing.T) {
	original := Connect{
		Type: TypeConnect,
		Name: "TestBot",
	}

	// Marshal
	data, err := original.MarshalMsg(nil)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal
	var decoded Connect
	_, err = decoded.UnmarshalMsg(data)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify
	if decoded.Type != original.Type {
		t.Errorf("Type mismatch: got %s, want %s", decoded.Type, original.Type)
	}
	if decoded.Name != original.Name {
		t.Errorf("Name mismatch: got %s, want %s", decoded.Name, original.Name)
	}
}

func TestActionMessage(t *testing.T) {
	original := Action{
		Type:   TypeAction,
		Action: "raise",
		Amount: 100,
	}

	// Marshal
	data, err := original.MarshalMsg(nil)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal
	var decoded Action
	_, err = decoded.UnmarshalMsg(data)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify
	if decoded.Action != original.Action {
		t.Errorf("Action mismatch: got %s, want %s", decoded.Action, original.Action)
	}
	if decoded.Amount != original.Amount {
		t.Errorf("Amount mismatch: got %d, want %d", decoded.Amount, original.Amount)
	}
}

func TestHandStartMessage(t *testing.T) {
	original := HandStart{
		Type:      TypeHandStart,
		HandID:    "hand-12345",
		HoleCards: []string{"As", "Kh"},
		YourSeat:  2,
		Button:    0,
		Players: []Player{
			{Seat: 0, Name: "Bot1", Chips: 1000},
			{Seat: 2, Name: "Bot2", Chips: 1000},
			{Seat: 4, Name: "Bot3", Chips: 1000},
		},
		SmallBlind: 5,
		BigBlind:   10,
	}

	// Marshal
	data, err := original.MarshalMsg(nil)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal
	var decoded HandStart
	_, err = decoded.UnmarshalMsg(data)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify
	if decoded.HandID != original.HandID {
		t.Errorf("HandID mismatch: got %s, want %s", decoded.HandID, original.HandID)
	}
	if len(decoded.HoleCards) != len(original.HoleCards) {
		t.Errorf("HoleCards length mismatch: got %d, want %d", len(decoded.HoleCards), len(original.HoleCards))
	}
	if len(decoded.Players) != len(original.Players) {
		t.Errorf("Players length mismatch: got %d, want %d", len(decoded.Players), len(original.Players))
	}
}

func TestActionRequestMessage(t *testing.T) {
	original := ActionRequest{
		Type:          TypeActionRequest,
		HandID:        "hand-456",
		TimeRemaining: 100,
		ValidActions:  []string{"fold", "call", "raise"},
		ToCall:        20,
		MinRaise:      40,
		Pot:           35,
	}

	// Marshal
	data, err := original.MarshalMsg(nil)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal
	var decoded ActionRequest
	_, err = decoded.UnmarshalMsg(data)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify
	if decoded.TimeRemaining != original.TimeRemaining {
		t.Errorf("TimeRemaining mismatch: got %d, want %d", decoded.TimeRemaining, original.TimeRemaining)
	}
	if decoded.ToCall != original.ToCall {
		t.Errorf("ToCall mismatch: got %d, want %d", decoded.ToCall, original.ToCall)
	}
}

func TestHandResultMessage(t *testing.T) {
	original := HandResult{
		Type:   TypeHandResult,
		HandID: "hand-789",
		Winners: []Winner{
			{
				Name:   "Bot2",
				Amount: 200,
			},
		},
		Board: []string{"Ah", "Kd", "7c", "2s", "9h"},
	}

	// Marshal
	data, err := original.MarshalMsg(nil)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal
	var decoded HandResult
	_, err = decoded.UnmarshalMsg(data)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify
	if len(decoded.Winners) != len(original.Winners) {
		t.Errorf("Winners length mismatch: got %d, want %d", len(decoded.Winners), len(original.Winners))
	}
	if len(decoded.Board) != len(original.Board) {
		t.Errorf("Board length mismatch: got %d, want %d", len(decoded.Board), len(original.Board))
	}
}

func BenchmarkMarshalAction(b *testing.B) {
	action := Action{
		Type:   TypeAction,
		Action: "raise",
		Amount: 100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, _ := action.MarshalMsg(nil)
		_ = data
	}
}

func BenchmarkUnmarshalActionCustom(b *testing.B) {
	action := Action{
		Type:   TypeAction,
		Action: "raise",
		Amount: 100,
	}
	data, _ := action.MarshalMsg(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var decoded Action
		_, _ = decoded.UnmarshalMsg(data)
	}
}

func BenchmarkMarshalActionRequest(b *testing.B) {
	req := ActionRequest{
		Type:          TypeActionRequest,
		HandID:        "bench-1",
		TimeRemaining: 100,
		ValidActions:  []string{"fold", "call", "raise"},
		ToCall:        20,
		MinRaise:      40,
		Pot:           35,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, _ := req.MarshalMsg(nil)
		_ = data
	}
}

func BenchmarkUnmarshalActionRequestCustom(b *testing.B) {
	req := ActionRequest{
		Type:          TypeActionRequest,
		HandID:        "bench-1",
		TimeRemaining: 100,
		ValidActions:  []string{"fold", "call", "raise"},
		ToCall:        20,
		MinRaise:      40,
		Pot:           35,
	}
	data, _ := req.MarshalMsg(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var decoded ActionRequest
		_, _ = decoded.UnmarshalMsg(data)
	}
}

// Test that messages are reasonably small
func TestMessageSizes(t *testing.T) {
	tests := []struct {
		name    string
		msg     interface{ MarshalMsg([]byte) ([]byte, error) }
		maxSize int
	}{
		{
			name: "Action",
			msg: &Action{
				Type:   TypeAction,
				Action: "raise",
				Amount: 100,
			},
			maxSize: 50,
		},
		{
			name: "ActionRequest",
			msg: &ActionRequest{
				Type:          TypeActionRequest,
				HandID:        "test-msg",
				TimeRemaining: 100,
				ValidActions:  []string{"fold", "call", "raise"},
				ToCall:        20,
				MinRaise:      40,
				Pot:           35,
			},
			maxSize: 200,
		},
		{
			name: "HandStart",
			msg: &HandStart{
				Type:      TypeHandStart,
				HandID:    "hand-test",
				HoleCards: []string{"As", "Kh"},
				YourSeat:  2,
				Button:    0,
				Players: []Player{
					{Seat: 0, Name: "Bot1", Chips: 1000},
					{Seat: 2, Name: "Bot2", Chips: 1000},
					{Seat: 4, Name: "Bot3", Chips: 1000},
				},
				SmallBlind: 5,
				BigBlind:   10,
			},
			maxSize: 300,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := tt.msg.MarshalMsg(nil)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}
			if len(data) > tt.maxSize {
				t.Errorf("%s message too large: %d bytes (max %d)", tt.name, len(data), tt.maxSize)
			}
		})
	}
}

// Test roundtrip with buffer reuse
func TestBufferReuse(t *testing.T) {
	var buf []byte

	// First message
	msg1 := Connect{Type: TypeConnect, Name: "Bot1"}
	buf, _ = msg1.MarshalMsg(buf[:0])

	var decoded1 Connect
	_, _ = decoded1.UnmarshalMsg(buf)
	if decoded1.Name != "Bot1" {
		t.Error("First decode failed")
	}

	// Reuse buffer for second message
	msg2 := Connect{Type: TypeConnect, Name: "Bot2"}
	buf, _ = msg2.MarshalMsg(buf[:0])

	var decoded2 Connect
	_, _ = decoded2.UnmarshalMsg(buf)
	if decoded2.Name != "Bot2" {
		t.Error("Second decode failed")
	}
}
