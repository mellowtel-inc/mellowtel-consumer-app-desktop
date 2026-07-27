// Package fetch implements the non-browser execution path: a plain HTTP request
// used for jobs that set fetchInstead or otherwise need no JS rendering.
package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// defaultUserAgent mimics a recent Chrome so target sites behave normally.
const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// Result holds the outcome of a simple fetch.
type Result struct {
	Body       string
	StatusCode int
	FinalURL   string
	// Bytes is the number of response body bytes read (for bandwidth accounting).
	Bytes int64
}

// Options configures a fetch.
type Options struct {
	Method  string
	URL     string
	Payload string
	Headers map[string]string
	Timeout time.Duration
}

// Client is a reusable HTTP client for simple jobs.
type Client struct {
	http *http.Client
}

// New builds a Client with sane connection-pooling defaults.
func New() *Client {
	return &Client{
		http: &http.Client{
			// Per-request timeouts are applied via context; this is a safety net.
			Timeout: 90 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("stopped after 10 redirects")
				}
				return nil
			},
		},
	}
}

// Do performs the request described by opts.
func (c *Client) Do(ctx context.Context, opts Options) (*Result, error) {
	method := strings.ToUpper(opts.Method)
	switch method {
	case "", "GET", "GET_NORMAL":
		method = http.MethodGet
	case "POST":
		method = http.MethodPost
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var body io.Reader
	if method == http.MethodPost && opts.Payload != "" && opts.Payload != "no_payload" {
		body = strings.NewReader(opts.Payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, opts.URL, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	for k, v := range opts.Headers {
		req.Header.Set(k, v)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	// Cap the read to avoid unbounded memory use on huge responses (25 MB).
	const maxBody = 25 * 1024 * 1024
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	finalURL := opts.URL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}

	return &Result{
		Body:       string(data),
		StatusCode: resp.StatusCode,
		FinalURL:   finalURL,
		Bytes:      int64(len(data)),
	}, nil
}
