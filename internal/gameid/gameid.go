package gameid

import (
	"crypto/rand"
	"fmt"
	"time"
)

// Base32 alphabet used by TypeID (Crockford's base32)
const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// RandSource interface for dependency injection of randomness
type RandSource interface {
	Intn(n int) int
}

// Generator handles game ID generation with configurable randomness
type Generator struct {
	randSource RandSource
}

// NewGenerator creates a new generator with optional RandSource
func NewGenerator(randSource RandSource) *Generator {
	return &Generator{randSource: randSource}
}

// Generate creates a new game ID using UUIDv7 encoded as 26-character base32 string
func Generate() string {
	return NewGenerator(nil).Generate()
}

// GenerateWithRandSource creates a new game ID using the provided RandSource
func GenerateWithRandSource(randSource RandSource) string {
	return NewGenerator(randSource).Generate()
}

// Generate creates a new game ID using the generator's RandSource
func (g *Generator) Generate() string {
	uuid := g.generateUUIDv7()
	return encodeBase32(uuid)
}

// generateUUIDv7 creates a 128-bit UUIDv7
func (g *Generator) generateUUIDv7() [16]byte {
	var uuid [16]byte

	// UUIDv7 format:
	// 48-bit timestamp (milliseconds since Unix epoch)
	// 12-bit random data for sub-millisecond precision
	// 4-bit version (0111 for version 7)
	// 2-bit variant (10)
	// 62-bit random data

	now := time.Now().UnixMilli()

	// Set 48-bit timestamp in first 6 bytes
	uuid[0] = byte(now >> 40)
	uuid[1] = byte(now >> 32)
	uuid[2] = byte(now >> 24)
	uuid[3] = byte(now >> 16)
	uuid[4] = byte(now >> 8)
	uuid[5] = byte(now)

	// Fill remaining 10 bytes with random data
	if g.randSource != nil {
		// Use provided RandSource for deterministic testing
		for i := 6; i < 16; i++ {
			uuid[i] = byte(g.randSource.Intn(256))
		}
	} else {
		// Use crypto/rand for production
		if _, err := rand.Read(uuid[6:]); err != nil {
			panic("failed to generate random bytes: " + err.Error())
		}
	}

	// Set version (4 bits) to 7 (0111)
	uuid[6] = (uuid[6] & 0x0f) | 0x70

	// Set variant (2 bits) to 10
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return uuid
}

// encodeBase32 encodes a 128-bit UUID as a 26-character base32 string
func encodeBase32(data [16]byte) string {
	// Convert to big-endian 130-bit value (128 bits + 2 zero bits)
	// We'll work with the 128 bits directly and handle the encoding properly

	result := make([]byte, 26)

	// Convert 16 bytes to a big integer representation for easier bit manipulation
	// We'll encode in groups of 5 bits each
	for i := 0; i < 26; i++ {
		// Calculate which bits we need for this character
		bitOffset := i * 5
		byteIndex := bitOffset / 8
		bitIndex := bitOffset % 8

		var value uint8

		if byteIndex < 16 {
			// Get 5 bits starting at the current position
			if bitIndex <= 3 {
				// All 5 bits are in the same byte
				value = (data[byteIndex] >> (3 - bitIndex)) & 0x1f
			} else {
				// Bits span two bytes
				value = (data[byteIndex] << (bitIndex - 3)) & 0x1f
				if byteIndex+1 < 16 {
					value |= data[byteIndex+1] >> (11 - bitIndex)
				}
			}
		}

		result[i] = alphabet[value]
	}

	return string(result)
}

// Validate checks if a game ID is valid (26 characters, valid base32)
func Validate(id string) error {
	if len(id) != 26 {
		return fmt.Errorf("game ID must be exactly 26 characters, got %d", len(id))
	}

	// Check first character doesn't exceed 7 (to ensure it represents ≤ 128 bits)
	firstChar := id[0]
	if firstChar > '7' {
		return fmt.Errorf("game ID first character must be 0-7, got %c", firstChar)
	}

	// Validate all characters are in the base32 alphabet
	for i, char := range id {
		valid := false
		for _, validChar := range alphabet {
			if char == validChar {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid character %c at position %d", char, i)
		}
	}

	return nil
}
