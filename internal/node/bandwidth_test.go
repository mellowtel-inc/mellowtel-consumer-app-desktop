package node

import "testing"

func TestBandwidthCap(t *testing.T) {
	tr := newBandwidthTracker(1000)
	if tr.overCap() {
		t.Fatal("should not be over cap initially")
	}
	tr.add(600)
	if tr.overCap() {
		t.Fatal("600/1000 should not be over cap")
	}
	tr.add(500)
	if !tr.overCap() {
		t.Fatal("1100/1000 should be over cap")
	}
	if tr.usedToday() != 1100 {
		t.Errorf("usedToday = %d, want 1100", tr.usedToday())
	}
}

func TestBandwidthUnlimited(t *testing.T) {
	tr := newBandwidthTracker(0)
	tr.add(1 << 40)
	if tr.overCap() {
		t.Fatal("unlimited cap should never be over")
	}
}

func TestBandwidthDailyRollover(t *testing.T) {
	tr := newBandwidthTracker(1000)
	tr.add(900)
	// Simulate the clock advancing to the next day.
	tr.day = "2000-01-01"
	if tr.usedToday() != 0 {
		t.Errorf("usage should reset after rollover, got %d", tr.usedToday())
	}
}
