package handhistory

import "time"

type stubClock struct {
	current time.Time
}

func (s stubClock) Now() time.Time { return s.current }
