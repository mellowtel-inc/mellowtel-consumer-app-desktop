package wsclient

import (
	"encoding/json"
	"net/url"
	"time"
)

// backoff produces an exponentially increasing delay capped at maxDelay,
// resetting to the base only when reset() is called (i.e. after a good run).
type backoff struct {
	base    time.Duration
	max     time.Duration
	current time.Duration
}

func newBackoff() *backoff {
	return &backoff{base: 5 * time.Second, max: 60 * time.Second, current: 0}
}

func (b *backoff) next() time.Duration {
	if b.current == 0 {
		b.current = b.base
	} else {
		b.current *= 2
		if b.current > b.max {
			b.current = b.max
		}
	}
	return b.current
}

func (b *backoff) reset() { b.current = 0 }

// peekTypeEvent extracts the type_event field cheaply, returning "" if absent
// or unparseable.
func peekTypeEvent(data []byte) string {
	var envelope struct {
		TypeEvent string `json:"type_event"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return ""
	}
	return envelope.TypeEvent
}

// redact strips query parameters from a URL for logging (device_id is PII-ish).
func redact(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.RawQuery = ""
	return u.String()
}
