// Package notify sends desktop notifications for milestones and status changes.
// Delivery is best-effort: a notification failure must never affect earning.
package notify

import (
	"sync"

	"github.com/gen2brain/beeep"
	"github.com/rs/zerolog"
)

// Notifier sends desktop notifications when enabled by the user.
type Notifier struct {
	log zerolog.Logger

	mu      sync.Mutex
	enabled bool
}

// New builds a Notifier.
func New(log zerolog.Logger, enabled bool) *Notifier {
	return &Notifier{log: log.With().Str("component", "notify").Logger(), enabled: enabled}
}

// SetEnabled turns notifications on or off at runtime.
func (n *Notifier) SetEnabled(v bool) {
	n.mu.Lock()
	n.enabled = v
	n.mu.Unlock()
}

// Send posts a notification if enabled.
func (n *Notifier) Send(title, message string) {
	n.mu.Lock()
	enabled := n.enabled
	n.mu.Unlock()
	if !enabled {
		return
	}
	go func() {
		defer func() {
			// Some desktop environments panic inside the notification stack;
			// never let that reach the job pipeline.
			if r := recover(); r != nil {
				n.log.Debug().Interface("panic", r).Msg("notification failed")
			}
		}()
		if err := beeep.Notify(title, message, ""); err != nil {
			n.log.Debug().Err(err).Msg("notification failed")
		}
	}()
}

// Milestone returns a friendly message when the completed-job count hits a
// round number worth celebrating, or "" when it does not.
func Milestone(total int64) string {
	switch {
	case total == 10:
		return "You've completed your first 10 jobs."
	case total == 100:
		return "100 jobs completed. Nice work."
	case total == 1000:
		return "1,000 jobs completed!"
	case total > 0 && total%5000 == 0:
		return "Another 5,000 jobs completed."
	}
	return ""
}
