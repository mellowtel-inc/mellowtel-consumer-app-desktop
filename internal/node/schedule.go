package node

import (
	"time"
)

// withinPauseWindow reports whether now falls inside the [from,to) daily pause
// window. from/to are "HH:MM" 24-hour strings; empty strings disable the
// schedule. Windows that wrap past midnight (from > to) are supported.
func withinPauseWindow(now time.Time, from, to string) bool {
	if from == "" || to == "" {
		return false
	}
	fromMin, ok1 := parseHHMM(from)
	toMin, ok2 := parseHHMM(to)
	if !ok1 || !ok2 || fromMin == toMin {
		return false
	}
	nowMin := now.Hour()*60 + now.Minute()

	if fromMin < toMin {
		return nowMin >= fromMin && nowMin < toMin
	}
	// Wrapping window, e.g. 22:00 -> 06:00.
	return nowMin >= fromMin || nowMin < toMin
}

// parseHHMM parses "HH:MM" into minutes-since-midnight.
func parseHHMM(s string) (int, bool) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}
