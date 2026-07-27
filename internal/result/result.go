// Package result builds and submits scrape results back to Mellowtel.
//
// The payload shape mirrors the Electron SDK's saveCrawl: a JSON body POSTed
// with Content-Type text/plain, where content/markDown are included only when
// requested and are sent uncompressed.
package result

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// Payload is the result document sent to the save endpoint.
type Payload struct {
	RecordID           string          `json:"recordID"`
	FastLane           bool            `json:"fastLane"`
	URL                string          `json:"url"`
	HTMLTransformer    string          `json:"htmlTransformer"`
	OrgID              string          `json:"orgId"`
	SaveText           bool            `json:"saveText"`
	NodeIdentifier     string          `json:"node_identifier"`
	BatchExecution     bool            `json:"BATCH_execution"`
	BatchID            string          `json:"batch_id"`
	FinalURL           string          `json:"final_url"`
	WebsiteUnreachable bool            `json:"website_unreachable"`
	StatusCode         int             `json:"statusCode"`
	RequestMessageInfo json.RawMessage `json:"requestMessageInfo,omitempty"`
	SaveHTML           bool            `json:"saveHtml"`
	SaveMarkdown       bool            `json:"saveMarkdown"`
	CerealResult       string          `json:"cereal_result"`
	FileNameBytes      string          `json:"file_name_bytes"`

	// Content and MarkDown are only marshalled when non-nil.
	Content  *string `json:"content,omitempty"`
	MarkDown *string `json:"markDown,omitempty"`
}

// Retry policy. The endpoint intermittently answers 5xx ("Error saving content
// to S3") for payloads that are otherwise identical to ones it accepts, so a
// transient failure should not cost us the whole scrape. Attempts are cheap:
// the body is already marshalled and held in memory.
const (
	maxAttempts  = 3
	retryBackoff = 1500 * time.Millisecond
)

// Submitter POSTs result payloads.
type Submitter struct {
	http *http.Client
	log  zerolog.Logger
}

// submitTimeout bounds a single result POST. The endpoint normally answers in
// well under a second; a long ceiling here would tie up a worker whenever the
// server stalls on a payload it doesn't like.
const submitTimeout = 30 * time.Second

// New builds a Submitter.
func New(log zerolog.Logger) *Submitter {
	return &Submitter{
		http: &http.Client{Timeout: submitTimeout},
		log:  log.With().Str("component", "result").Logger(),
	}
}

// Submit POSTs the payload to endpoint, retrying transient failures. Returns
// the number of bytes sent for bandwidth accounting.
func (s *Submitter) Submit(ctx context.Context, endpoint string, p Payload) (int64, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return 0, fmt.Errorf("marshal result payload: %w", err)
	}
	sent := int64(len(body))

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		retryable, err := s.attempt(ctx, endpoint, body)
		if err == nil {
			if attempt > 1 {
				s.log.Info().
					Str("recordID", p.RecordID).
					Int("attempt", attempt).
					Msg("result accepted after retry")
			}
			return sent, nil
		}
		lastErr = err
		if !retryable || attempt == maxAttempts {
			break
		}
		s.log.Debug().
			Err(err).
			Str("recordID", p.RecordID).
			Int("attempt", attempt).
			Msg("transient submit failure; retrying")

		select {
		case <-ctx.Done():
			return sent, fmt.Errorf("submit cancelled: %w", ctx.Err())
		case <-time.After(time.Duration(attempt) * retryBackoff):
		}
	}
	return sent, fmt.Errorf("after %d attempts: %w", maxAttempts, lastErr)
}

// attempt performs one POST. It reports whether the failure is worth retrying.
func (s *Submitter) attempt(ctx context.Context, endpoint string, body []byte) (retryable bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, submitTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("build result request: %w", err)
	}
	// The SDK sends the JSON body as text/plain.
	req.Header.Set("Content-Type", "text/plain")

	resp, err := s.http.Do(req)
	if err != nil {
		// Network errors and timeouts are worth another go.
		return true, fmt.Errorf("post result: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body: on failure the endpoint explains why, and that
	// message is the difference between guessing and knowing.
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return false, nil
	case resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests:
		// Server-side or throttling: retry.
		return true, fmt.Errorf("status %d: %s (sent %d bytes)",
			resp.StatusCode, truncate(strings.TrimSpace(string(respBody)), 200), len(body))
	default:
		// 4xx means the payload itself is unacceptable; retrying won't help.
		return false, fmt.Errorf("status %d: %s (sent %d bytes)",
			resp.StatusCode, truncate(strings.TrimSpace(string(respBody)), 200), len(body))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// StringPtr is a helper for populating the optional Content/MarkDown fields.
func StringPtr(s string) *string { return &s }
