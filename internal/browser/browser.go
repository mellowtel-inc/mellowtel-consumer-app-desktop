// Package browser drives the user's installed Chrome via CDP (chromedp) to
// render JavaScript-heavy pages. It launches a dedicated headless instance
// using an isolated user-data-dir so the user's real profile is untouched, and
// strips framing/CSP response headers via the CDP Fetch domain.
package browser

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/rs/zerolog"

	"mellowtel-consumer/internal/job"
)

// strippedHeaders are response headers removed so pages that would otherwise
// refuse to render (framing / CSP protections) load cleanly. Matched
// case-insensitively.
var strippedHeaders = map[string]bool{
	"x-frame-options":                   true,
	"content-security-policy":           true,
	"content-security-policy-report-only": true,
	"cross-origin-embedder-policy":      true,
	"cross-origin-opener-policy":        true,
	"cross-origin-resource-policy":      true,
}

// ErrNotFound is returned when no Chrome installation can be located.
var ErrNotFound = fmt.Errorf("chrome not found")

// Engine manages a long-lived headless Chrome allocator that tabs are created
// against per job.
type Engine struct {
	log         zerolog.Logger
	chromePath  string
	userDataDir string
	perJobTO    time.Duration // whole-render budget
	navTO       time.Duration // navigation-only budget; overrun salvages partial DOM

	// stripHeaders enables CDP Fetch interception to remove framing/CSP response
	// headers. OFF by default: pages are loaded as top-level navigations, where
	// X-Frame-Options and CSP frame-ancestors do not apply, so stripping buys
	// nothing — while interception pauses every matched response and will hang
	// navigation outright if any continue fails.
	stripHeaders bool

	mu            sync.Mutex
	allocCtx      context.Context
	allocStop     context.CancelFunc
	browserCtx    context.Context
	browserCancel context.CancelFunc
	started       bool
}

// RenderResult is the output of rendering a page.
type RenderResult struct {
	HTML       string
	FinalURL   string
	StatusCode int
	Bytes      int64
}

// NewEngine builds an Engine. chromePath may be empty, in which case Chrome is
// auto-detected on Start. userDataDir is the isolated profile directory.
func NewEngine(log zerolog.Logger, chromePath, userDataDir string) *Engine {
	return &Engine{
		log:         log.With().Str("component", "browser").Logger(),
		chromePath:  chromePath,
		userDataDir: userDataDir,
		perJobTO:    45 * time.Second,
		navTO:       25 * time.Second,
	}
}

// SetStripHeaders toggles CDP Fetch response interception (framing/CSP header
// removal). Leave disabled unless pages are rendered inside a frame; see the
// stripHeaders field for why.
func (e *Engine) SetStripHeaders(v bool) {
	e.mu.Lock()
	e.stripHeaders = v
	e.mu.Unlock()
}

// Start launches the headless Chrome allocator. Safe to call repeatedly.
func (e *Engine) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.started {
		return nil
	}

	path := e.chromePath
	if path == "" {
		found, ok := FindChrome()
		if !ok {
			return ErrNotFound
		}
		path = found
	}

	// Clear any stale singleton lock left by a previously crashed Chrome so the
	// launch below doesn't fail with "SingletonLock: File exists".
	e.clearSingletonLocks()

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(path),
		chromedp.UserDataDir(e.userDataDir),
		chromedp.Flag("headless", "new"),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("mute-audio", true),
	)

	allocCtx, allocStop := chromedp.NewExecAllocator(context.Background(), opts...)
	// Launch a single browser process. Every job opens a new tab against this
	// same process (see Render) — never a second browser, which would collide on
	// the profile's SingletonLock.
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	if err := chromedp.Run(browserCtx); err != nil {
		browserCancel()
		allocStop()
		return fmt.Errorf("launch chrome: %w", err)
	}

	e.allocCtx = allocCtx
	e.allocStop = allocStop
	e.browserCtx = browserCtx
	e.browserCancel = browserCancel
	e.started = true
	e.log.Info().Str("chrome", path).Str("profile", e.userDataDir).Msg("headless chrome started")
	return nil
}

// Stop terminates the Chrome process and frees resources.
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.started {
		return
	}
	if e.browserCancel != nil {
		e.browserCancel()
	}
	if e.allocStop != nil {
		e.allocStop()
	}
	e.started = false
	e.log.Info().Msg("headless chrome stopped")
}

// clearSingletonLocks removes Chrome's per-profile singleton files. Chrome
// creates these to guarantee one process per user-data-dir; if a previous run
// crashed they can linger and block the next launch. Safe because this engine
// owns the isolated profile dir exclusively.
func (e *Engine) clearSingletonLocks() {
	for _, name := range []string{"SingletonLock", "SingletonSocket", "SingletonCookie"} {
		if err := os.Remove(filepath.Join(e.userDataDir, name)); err != nil && !os.IsNotExist(err) {
			e.log.Debug().Err(err).Str("file", name).Msg("could not remove stale singleton file")
		}
	}
}

// Render loads req.URL in a fresh tab, strips protective headers, waits, runs
// the job's actions, and returns the fully rendered outer HTML.
func (e *Engine) Render(ctx context.Context, req *job.Request) (*RenderResult, error) {
	e.mu.Lock()
	parent := e.browserCtx
	started := e.started
	stripHeaders := e.stripHeaders
	e.mu.Unlock()
	if !started || parent == nil {
		return nil, fmt.Errorf("browser engine not started")
	}

	// A new context off the browser context is a new TAB in the existing Chrome
	// process — not a second browser, which would fight over the profile lock.
	tabCtx, cancelTab := chromedp.NewContext(parent)
	defer cancelTab()

	timeout := e.perJobTO
	tabCtx, cancelTO := context.WithTimeout(tabCtx, timeout)
	defer cancelTO()

	// Materialise the tab's CDP session against tabCtx up front. chromedp binds
	// a target to whichever context first runs against it, so if the session
	// were created under the short-lived navigation context below, cancelling
	// that context would kill the target and every later action on this tab
	// would hang until the outer deadline. This also gives ListenTarget a real
	// target to attach to.
	if err := chromedp.Run(tabCtx); err != nil {
		return nil, fmt.Errorf("open tab for %q: %w", req.URL, err)
	}

	res := &RenderResult{FinalURL: req.URL, StatusCode: 200}
	var capMu sync.Mutex
	captured := false

	// Handle Fetch response-stage pauses: strip headers and continue.
	chromedp.ListenTarget(tabCtx, func(ev interface{}) {
		ev2, ok := ev.(*fetch.EventRequestPaused)
		if !ok {
			return
		}
		// Continue must run outside the event goroutine to avoid deadlock.
		go func() {
			c := chromedp.FromContext(tabCtx)
			if c == nil || c.Target == nil {
				return
			}
			exec := cdp.WithExecutor(tabCtx, c.Target)

			isResponseStage := ev2.ResponseStatusCode != 0 || len(ev2.ResponseHeaders) > 0
			if !isResponseStage {
				_ = fetch.ContinueRequest(ev2.RequestID).Do(exec)
				return
			}

			// Record the top-level document's status once.
			if ev2.ResourceType == network.ResourceTypeDocument {
				capMu.Lock()
				if !captured {
					captured = true
					res.StatusCode = int(ev2.ResponseStatusCode)
					res.FinalURL = ev2.Request.URL
				}
				capMu.Unlock()
			}

			headers := filterHeaders(ev2.ResponseHeaders)
			cont := fetch.ContinueResponse(ev2.RequestID)
			if len(headers) > 0 {
				cont = cont.WithResponseHeaders(headers)
			}
			if err := cont.Do(exec); err != nil {
				// A paused request that is never continued hangs the page
				// forever, so always fall back to an unmodified continue.
				e.log.Debug().Err(err).Str("url", ev2.Request.URL).Msg("continue response failed; retrying unmodified")
				if err2 := fetch.ContinueRequest(ev2.RequestID).Do(exec); err2 != nil {
					e.log.Debug().Err(err2).Str("url", ev2.Request.URL).Msg("fallback continue failed")
				}
			}
		}()
	})

	// --- Stage 1: navigation, under its own budget. ---
	//
	// Interception is scoped to the top-level Document only. Matching "*" would
	// pause every subresource (images, scripts, fonts, trackers) and round-trip
	// each back to Go, which is slow enough to blow the job timeout on
	// ad-heavy pages. Only the main document's framing/CSP headers matter here.
	navTasks := chromedp.Tasks{}
	if stripHeaders && !req.SkipHeaders {
		navTasks = append(navTasks, fetch.Enable().WithPatterns([]*fetch.RequestPattern{
			{
				URLPattern:   "*",
				ResourceType: network.ResourceTypeDocument,
				RequestStage: fetch.RequestStageResponse,
			},
		}))
	}
	navTasks = append(navTasks, chromedp.Navigate(req.URL))

	navCtx, cancelNav := context.WithTimeout(tabCtx, e.navTO)
	navErr := chromedp.Run(navCtx, navTasks...)
	cancelNav()
	if navErr != nil {
		// A navigation timeout is not fatal: many pages keep loading trackers
		// forever but already have usable DOM. Salvage whatever rendered.
		// Anything else (DNS failure, protocol error) is a genuine failure.
		if !errors.Is(navErr, context.DeadlineExceeded) {
			return nil, fmt.Errorf("render %q: %w", req.URL, navErr)
		}
		e.log.Debug().Str("url", req.URL).Msg("navigation timed out; salvaging partial DOM")
	}

	// --- Stage 2: post-load waits, actions and extraction. ---
	tasks := chromedp.Tasks{}

	// Wait after load.
	if req.WaitBeforeScraping > 0 {
		tasks = append(tasks, chromedp.Sleep(time.Duration(req.WaitBeforeScraping*float64(time.Second))))
	}
	// Optional explicit wait-for-element.
	if req.WaitForElement != "" && req.WaitForElement != "none" {
		tasks = append(tasks, waitForSelector(req.WaitForElement, req.WaitForElementTime))
	}
	// Job actions.
	tasks = append(tasks, buildActionTasks(req.Actions)...)
	// Optional image removal.
	if req.RemoveImages {
		tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.Evaluate(`document.querySelectorAll('img,picture,source').forEach(e=>e.remove())`, nil).Do(ctx)
		}))
	}

	var html string
	tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.OuterHTML("html", &html, chromedp.ByQuery).Do(ctx)
	}))
	tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
		var loc string
		if err := chromedp.Location(&loc).Do(ctx); err == nil && loc != "" {
			capMu.Lock()
			res.FinalURL = loc
			capMu.Unlock()
		}
		return nil
	}))

	if err := chromedp.Run(tabCtx, tasks...); err != nil {
		// If we already have HTML from a salvaged navigation, return it rather
		// than discarding a usable result.
		if html == "" {
			return nil, fmt.Errorf("render %q: %w", req.URL, err)
		}
		e.log.Debug().Err(err).Str("url", req.URL).Msg("post-load stage failed; returning partial HTML")
	}
	if html == "" {
		return nil, fmt.Errorf("render %q: no content extracted", req.URL)
	}

	res.HTML = html
	res.Bytes = int64(len(html))
	return res, nil
}

// filterHeaders removes protective headers from a response header set.
func filterHeaders(in []*fetch.HeaderEntry) []*fetch.HeaderEntry {
	out := make([]*fetch.HeaderEntry, 0, len(in))
	for _, h := range in {
		if strippedHeaders[strings.ToLower(h.Name)] {
			continue
		}
		out = append(out, h)
	}
	return out
}

// waitForSelector waits for a selector to become visible, capped by timeoutMs.
func waitForSelector(selector string, timeoutMs int) chromedp.Action {
	return chromedp.ActionFunc(func(ctx context.Context) error {
		if timeoutMs <= 0 {
			timeoutMs = 60000
		}
		wctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()
		if err := chromedp.WaitVisible(selector, chromedp.ByQuery).Do(wctx); err != nil {
			// Non-fatal: a missing element should not fail the whole job.
			return nil
		}
		return nil
	})
}
