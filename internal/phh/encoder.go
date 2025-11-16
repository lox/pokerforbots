package phh

import (
	"fmt"
	"io"
	"strings"

	"github.com/BurntSushi/toml"
)

// Encode writes the hand history to the provided writer in PHH TOML format.
func Encode(w io.Writer, hand *HandHistory) error {
	if hand == nil {
		return fmt.Errorf("phh: hand history is nil")
	}

	enc := toml.NewEncoder(w)
	// Use tabs for arrays to match human expectations
	enc.Indent = "\t"
	return enc.Encode(hand)
}

// EncodeToBytes encodes and returns the result as bytes.
func EncodeToBytes(hand *HandHistory) ([]byte, error) {
	var buf strings.Builder
	if err := Encode(&buf, hand); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

// FormatAction converts the server action vocabulary to PHH action strings.
// It returns the formatted action along with a boolean indicating whether
// the action should be emitted (false for blind posts that are captured elsewhere).
func FormatAction(seat int, action string, totalBet int) (string, bool) {
	player := fmt.Sprintf("p%d", seat+1)
	switch action {
	case "fold", "timeout_fold":
		return fmt.Sprintf("%s f", player), true
	case "check", "call":
		return fmt.Sprintf("%s cc", player), true
	case "raise", "allin", "bet":
		if totalBet <= 0 {
			return "", false
		}
		return fmt.Sprintf("%s cbr %d", player, totalBet), true
	case "post_small_blind", "post_big_blind":
		return "", false
	default:
		return fmt.Sprintf("# %s %s %d", player, action, totalBet), true
	}
}
