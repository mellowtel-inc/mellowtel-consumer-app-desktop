package node

import (
	"sync"
	"time"
)

// bandwidthTracker accounts daily network usage and enforces a cap. Usage
// resets at local midnight.
type bandwidthTracker struct {
	mu       sync.Mutex
	day      string
	used     int64
	capBytes int64 // 0 = unlimited
	now      func() time.Time
}

func newBandwidthTracker(capBytes int64) *bandwidthTracker {
	t := &bandwidthTracker{capBytes: capBytes, now: time.Now}
	t.day = t.now().Format("2006-01-02")
	return t
}

func (t *bandwidthTracker) rollover() {
	today := t.now().Format("2006-01-02")
	if today != t.day {
		t.day = today
		t.used = 0
	}
}

func (t *bandwidthTracker) add(n int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rollover()
	t.used += n
}

func (t *bandwidthTracker) usedToday() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rollover()
	return t.used
}

// overCap reports whether the daily cap has been reached.
func (t *bandwidthTracker) overCap() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rollover()
	return t.capBytes > 0 && t.used >= t.capBytes
}

func (t *bandwidthTracker) setCap(capBytes int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.capBytes = capBytes
}
