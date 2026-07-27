// Package executor orchestrates a single scrape job end to end: it routes the
// job to the simple or browser path, converts output to the requested formats,
// and submits the result.
package executor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog"

	"mellowtel-consumer/internal/browser"
	"mellowtel-consumer/internal/fetch"
	"mellowtel-consumer/internal/job"
	"mellowtel-consumer/internal/markdown"
	"mellowtel-consumer/internal/result"
)

// Renderer is the browser rendering capability the executor depends on. It is
// satisfied by *browser.Engine and can be stubbed in tests.
type Renderer interface {
	Render(ctx context.Context, req *job.Request) (*browser.RenderResult, error)
}

// Executor runs jobs.
type Executor struct {
	log             zerolog.Logger
	fetcher         *fetch.Client
	renderer        Renderer
	md              *markdown.Converter
	submitter       *result.Submitter
	nodeID          string
	defaultEndpoint string
}

// Config configures a new Executor.
type Config struct {
	Log             zerolog.Logger
	Fetcher         *fetch.Client
	Renderer        Renderer
	Markdown        *markdown.Converter
	Submitter       *result.Submitter
	NodeID          string
	DefaultEndpoint string
}

// New builds an Executor.
func New(c Config) *Executor {
	return &Executor{
		log:             c.Log.With().Str("component", "executor").Logger(),
		fetcher:         c.Fetcher,
		renderer:        c.Renderer,
		md:              c.Markdown,
		submitter:       c.Submitter,
		nodeID:          c.NodeID,
		defaultEndpoint: c.DefaultEndpoint,
	}
}

// Outcome summarises what happened to a job.
type Outcome struct {
	RecordID   string
	Path       job.Path
	StatusCode int
	BytesUsed  int64 // total network bytes attributable to the job
}

// Execute runs a single job and submits its result.
func (e *Executor) Execute(ctx context.Context, req *job.Request) (Outcome, error) {
	path := job.Decide(req)
	log := e.log.With().Str("recordID", req.RecordID).Str("url", req.URL).Str("path", path.String()).Logger()
	log.Info().Msg("executing job")

	var (
		html       string
		finalURL   string
		statusCode int
		bytesUsed  int64
		unreachable bool
	)

	switch path {
	case job.PathSimple:
		r, err := e.runSimple(ctx, req)
		if err != nil {
			log.Warn().Err(err).Msg("simple fetch failed")
			unreachable = true
			statusCode = 0
			break
		}
		html, finalURL, statusCode, bytesUsed = r.Body, r.FinalURL, r.StatusCode, r.Bytes
	case job.PathBrowser:
		if e.renderer == nil {
			// No browser available: fall back to a simple fetch so the job
			// still produces something rather than being dropped.
			log.Warn().Msg("no browser engine; falling back to simple fetch")
			r, err := e.runSimple(ctx, req)
			if err != nil {
				unreachable = true
			} else {
				html, finalURL, statusCode, bytesUsed = r.Body, r.FinalURL, r.StatusCode, r.Bytes
			}
			break
		}
		r, err := e.renderer.Render(ctx, req)
		if err != nil {
			log.Warn().Err(err).Msg("browser render failed")
			unreachable = true
			statusCode = 0
			break
		}
		html, finalURL, statusCode, bytesUsed = r.HTML, r.FinalURL, r.StatusCode, r.Bytes
	}

	payload, sentBytes, err := e.buildAndSubmit(ctx, req, html, finalURL, statusCode, unreachable)
	bytesUsed += sentBytes
	if err != nil {
		return Outcome{RecordID: req.RecordID, Path: path, StatusCode: statusCode, BytesUsed: bytesUsed}, err
	}
	log.Info().Int("status", payload.StatusCode).Int64("bytes", bytesUsed).Msg("job completed")

	return Outcome{
		RecordID:   req.RecordID,
		Path:       path,
		StatusCode: statusCode,
		BytesUsed:  bytesUsed,
	}, nil
}

func (e *Executor) runSimple(ctx context.Context, req *job.Request) (*fetch.Result, error) {
	url := req.URL
	if req.MethodEndpoint != "" {
		url = req.MethodEndpoint
	}
	return e.fetcher.Do(ctx, fetch.Options{
		Method:  req.Method,
		URL:     url,
		Payload: req.MethodPayload,
		Headers: req.MethodHeaders,
	})
}

// buildAndSubmit assembles the result payload per the SDK shape and POSTs it.
func (e *Executor) buildAndSubmit(ctx context.Context, req *job.Request, html, finalURL string, statusCode int, unreachable bool) (result.Payload, int64, error) {
	endpoint := req.SaveHTMLEndpoint
	if endpoint == "" {
		endpoint = e.defaultEndpoint
	}
	if finalURL == "" {
		finalURL = req.URL
	}
	if statusCode == 0 && !unreachable {
		statusCode = 200
	}

	p := result.Payload{
		RecordID:           req.RecordID,
		FastLane:           req.FastLane,
		URL:                req.URL,
		HTMLTransformer:    req.HTMLTransformer,
		OrgID:              req.OrgID,
		SaveText:           req.SaveText,
		NodeIdentifier:     e.nodeID,
		FinalURL:           finalURL,
		WebsiteUnreachable: unreachable,
		StatusCode:         statusCode,
		SaveHTML:           req.SaveHTML,
		SaveMarkdown:       req.SaveMarkdown,
		CerealResult:       "",
		FileNameBytes:      "",
		RequestMessageInfo: rawMessage(req.Raw),
	}

	if req.SaveHTML {
		p.Content = result.StringPtr(html)
	}
	if req.SaveMarkdown {
		markdownText := ""
		if html != "" {
			converted, err := e.md.Convert(html)
			if err != nil {
				e.log.Warn().Err(err).Str("recordID", req.RecordID).Msg("markdown conversion failed")
			} else {
				markdownText = converted
			}
		}
		p.MarkDown = result.StringPtr(markdownText)
	}

	// Submitting must not inherit whatever is left of the render budget — a slow
	// page would otherwise starve the POST and we'd lose an already-scraped
	// result. Detach from the job deadline; Submit applies its own timeout.
	submitCtx := context.WithoutCancel(ctx)
	sent, err := e.submitter.Submit(submitCtx, endpoint, p)
	if err != nil {
		return p, sent, fmt.Errorf("submit result: %w", err)
	}
	return p, sent, nil
}

// rawMessage re-encodes the original job envelope for requestMessageInfo.
func rawMessage(raw map[string]json.RawMessage) json.RawMessage {
	if raw == nil {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	return b
}
