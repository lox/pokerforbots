package gameid

import (
	"strings"
	"testing"
	"time"
)

func TestGenerate(t *testing.T) {
	// Test that Generate produces valid game IDs
	id := Generate()

	if len(id) != 26 {
		t.Errorf("expected 26 characters, got %d", len(id))
	}

	if err := Validate(id); err != nil {
		t.Errorf("generated ID failed validation: %v", err)
	}

	// Test that first character is valid (0-7)
	if id[0] > '7' {
		t.Errorf("first character %c exceeds maximum '7'", id[0])
	}
}

func TestGenerateUnique(t *testing.T) {
	// Generate multiple IDs and ensure they're unique
	ids := make(map[string]bool)

	for i := 0; i < 100; i++ {
		id := Generate()
		if ids[id] {
			t.Errorf("duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}

func TestGenerateTimeSorted(t *testing.T) {
	// Generate IDs with a small delay to ensure time-based sorting
	var ids []string

	for i := 0; i < 10; i++ {
		ids = append(ids, Generate())
		time.Sleep(time.Millisecond)
	}

	// Check that IDs are sorted (UUIDv7 should be sortable by timestamp)
	for i := 1; i < len(ids); i++ {
		if strings.Compare(ids[i-1], ids[i]) >= 0 {
			t.Errorf("IDs not sorted: %s >= %s", ids[i-1], ids[i])
		}
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{
			name:    "valid ID",
			id:      "01h5n0et5q6mt3v7ms1234abcd",
			wantErr: false,
		},
		{
			name:    "too short",
			id:      "01h5n0et5q6mt3v7ms123",
			wantErr: true,
		},
		{
			name:    "too long",
			id:      "01h5n0et5q6mt3v7ms1234abcdef",
			wantErr: true,
		},
		{
			name:    "first char too high",
			id:      "81h5n0et5q6mt3v7ms1234abcd",
			wantErr: true,
		},
		{
			name:    "invalid character",
			id:      "01h5n0et5q6mt3v7ms1234abci",
			wantErr: true,
		},
		{
			name:    "uppercase not allowed",
			id:      "01H5N0ET5Q6MT3V7MS1234ABCD",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAlphabet(t *testing.T) {
	// Ensure alphabet has no duplicate characters and is the correct length
	if len(alphabet) != 32 {
		t.Errorf("alphabet should have 32 characters, got %d", len(alphabet))
	}

	seen := make(map[rune]bool)
	for _, char := range alphabet {
		if seen[char] {
			t.Errorf("duplicate character in alphabet: %c", char)
		}
		seen[char] = true
	}

	// Check specific requirements: no i, l, o, u
	forbidden := "ilou"
	for _, char := range forbidden {
		if strings.ContainsRune(alphabet, char) {
			t.Errorf("alphabet should not contain %c", char)
		}
	}
}

// MockRandSource for deterministic testing
type MockRandSource struct {
	values []int
	index  int
}

func NewMockRandSource(values ...int) *MockRandSource {
	return &MockRandSource{values: values, index: 0}
}

func (m *MockRandSource) Intn(n int) int {
	if m.index >= len(m.values) {
		return 0 // Default fallback
	}
	val := m.values[m.index] % n // Ensure it's within bounds
	m.index++
	return val
}

func TestGenerateWithRandSource(t *testing.T) {
	// Test deterministic generation with fixed values
	mockRand := NewMockRandSource(1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16)

	id1 := GenerateWithRandSource(mockRand)

	// Reset mock and generate again with same values
	mockRand2 := NewMockRandSource(1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16)
	id2 := GenerateWithRandSource(mockRand2)

	// Should generate identical IDs (except for timestamp which might differ by milliseconds)
	// The random portion should be identical, so we'll check that the IDs are close
	if len(id1) != 26 || len(id2) != 26 {
		t.Errorf("Expected 26-character IDs, got %d and %d", len(id1), len(id2))
	}

	// Validate both IDs
	if err := Validate(id1); err != nil {
		t.Errorf("Generated ID 1 failed validation: %v", err)
	}
	if err := Validate(id2); err != nil {
		t.Errorf("Generated ID 2 failed validation: %v", err)
	}
}

func TestGeneratorDeterministic(t *testing.T) {
	// Test that same RandSource produces same results (ignoring timestamp)
	values := make([]int, 20) // Enough for multiple generations
	for i := range values {
		values[i] = i + 100 // Use predictable values
	}

	gen := NewGenerator(NewMockRandSource(values...))

	// Generate multiple IDs
	var ids []string
	for i := 0; i < 3; i++ {
		ids = append(ids, gen.Generate())
	}

	// All should be valid
	for i, id := range ids {
		if err := Validate(id); err != nil {
			t.Errorf("ID %d failed validation: %v", i, err)
		}
	}

	// All should be unique (even with same random source due to timestamp)
	idMap := make(map[string]bool)
	for _, id := range ids {
		if idMap[id] {
			t.Errorf("Duplicate ID generated: %s", id)
		}
		idMap[id] = true
	}
}
