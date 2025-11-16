package phh

import "time"

// HandHistory represents a single poker hand encoded in PHH format.
type HandHistory struct {
	Variant           string         `toml:"variant"`
	Table             string         `toml:"table,omitempty"`
	SeatCount         int            `toml:"seat_count,omitempty"`
	Seats             []int          `toml:"seats,omitempty"`
	Antes             []int          `toml:"antes"`
	BlindsOrStraddles []int          `toml:"blinds_or_straddles"`
	MinBet            int            `toml:"min_bet"`
	StartingStacks    []int          `toml:"starting_stacks"`
	FinishingStacks   []int          `toml:"finishing_stacks,omitempty"`
	Winnings          []int          `toml:"winnings,omitempty"`
	Actions           []string       `toml:"actions"`
	Players           []string       `toml:"players,omitempty"`
	HandID            string         `toml:"hand"`
	LegacyHandID      string         `toml:"hand_id,omitempty"`
	Time              string         `toml:"time,omitempty"`
	TimeZone          string         `toml:"time_zone,omitempty"`
	TimeZoneAbbrev    string         `toml:"time_zone_abbreviation,omitempty"`
	Day               int            `toml:"day,omitempty"`
	Month             int            `toml:"month,omitempty"`
	Year              int            `toml:"year,omitempty"`
	Metadata          map[string]any `toml:"metadata,omitempty"`

	Board     []string  `toml:"-"`
	Timestamp time.Time `toml:"-"`
}
