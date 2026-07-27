package node

import (
	"testing"
	"time"
)

func at(hhmm string) time.Time {
	t, _ := time.Parse("15:04", hhmm)
	return t
}

func TestWithinPauseWindow(t *testing.T) {
	tests := []struct {
		name       string
		now        string
		from, to   string
		want       bool
	}{
		{"disabled when empty", "12:00", "", "", false},
		{"inside normal window", "13:00", "12:00", "14:00", true},
		{"before normal window", "11:59", "12:00", "14:00", false},
		{"at end is excluded", "14:00", "12:00", "14:00", false},
		{"inside wrapping window (late)", "23:00", "22:00", "06:00", true},
		{"inside wrapping window (early)", "05:00", "22:00", "06:00", true},
		{"outside wrapping window", "12:00", "22:00", "06:00", false},
		{"equal bounds disabled", "12:00", "12:00", "12:00", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := withinPauseWindow(at(tt.now), tt.from, tt.to); got != tt.want {
				t.Errorf("withinPauseWindow(%s, %s, %s) = %v, want %v", tt.now, tt.from, tt.to, got, tt.want)
			}
		})
	}
}
