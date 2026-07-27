// Package approval polls the Mellowtel approval endpoint to confirm the node is
// still permitted to run. A node that loses approval is paused by the caller.
package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/rs/zerolog"
)

// CheckInterval is how often approval is re-verified.
const CheckInterval = 30 * time.Minute

// Params carries the query parameters the endpoint expects.
type Params struct {
	DeviceID      string
	Version       string
	Platform      string
	SpeedDownload int
}

// Checker calls the approval endpoint.
type Checker struct {
	log      zerolog.Logger
	endpoint string
	params   Params
	http     *http.Client
}

// New builds a Checker.
func New(log zerolog.Logger, endpoint string, params Params) *Checker {
	return &Checker{
		log:      log.With().Str("component", "approval").Logger(),
		endpoint: endpoint,
		params:   params,
		http:     &http.Client{Timeout: 20 * time.Second},
	}
}

// Check performs a single approval request. It returns whether the node is
// approved.
func (c *Checker) Check(ctx context.Context) (bool, error) {
	u, err := url.Parse(c.endpoint)
	if err != nil {
		return false, fmt.Errorf("parse approval url: %w", err)
	}
	q := u.Query()
	q.Set("device_id", c.params.DeviceID)
	q.Set("version", c.params.Version)
	q.Set("platform", c.params.Platform)
	if c.params.SpeedDownload > 0 {
		q.Set("speed_download", fmt.Sprintf("%d", c.params.SpeedDownload))
	}
	u.RawQuery = q.Encode()

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return false, fmt.Errorf("build approval request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("approval request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("approval endpoint status %d", resp.StatusCode)
	}

	var result struct {
		Approval bool `json:"approval"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return false, fmt.Errorf("parse approval response: %w", err)
	}
	return result.Approval, nil
}

// Run polls approval on CheckInterval, invoking onResult with each outcome,
// until ctx is cancelled. An initial check runs immediately.
func (c *Checker) Run(ctx context.Context, onResult func(approved bool, err error)) {
	report := func() {
		approved, err := c.Check(ctx)
		if err != nil {
			c.log.Warn().Err(err).Msg("approval check failed")
		} else {
			c.log.Info().Bool("approved", approved).Msg("approval checked")
		}
		if onResult != nil {
			onResult(approved, err)
		}
	}

	report()
	ticker := time.NewTicker(CheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			report()
		}
	}
}
