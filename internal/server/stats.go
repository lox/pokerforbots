package server

import "time"

// PlayerStats captures aggregate performance metrics for a single bot within a game.
type PlayerStats struct {
	BotID       string    `json:"bot_id"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role"`
	Hands       int       `json:"hands"`
	NetChips    int64     `json:"net_chips"`
	AvgPerHand  float64   `json:"avg_per_hand"`
	TotalWon    int64     `json:"total_won"`
	TotalLost   int64     `json:"total_lost"`
	LastDelta   int       `json:"last_delta"`
	LastUpdated time.Time `json:"last_updated"`
}

// GameStats provides an aggregated snapshot for a game instance.
type GameStats struct {
	ID               string        `json:"id"`
	SmallBlind       int           `json:"small_blind"`
	BigBlind         int           `json:"big_blind"`
	StartChips       int           `json:"start_chips"`
	TimeoutMs        int           `json:"timeout_ms"`
	MinPlayers       int           `json:"min_players"`
	MaxPlayers       int           `json:"max_players"`
	RequirePlayer    bool          `json:"require_player"`
	InfiniteBankroll bool          `json:"infinite_bankroll"`
	HandsCompleted   uint64        `json:"hands_completed"`
	HandLimit        uint64        `json:"hand_limit"`
	HandsRemaining   uint64        `json:"hands_remaining"`
	Timeouts         uint64        `json:"timeouts"`
	HandsPerSecond   float64       `json:"hands_per_second"`
	Seed             int64         `json:"seed"`
	Players          []PlayerStats `json:"players"`
}
