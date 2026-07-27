// Package node ties every subsystem together: it owns the WebSocket client,
// approval poller, browser engine and job executor, and exposes a small
// connect/disconnect/status surface to the UI layer.
package node

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"mellowtel-consumer/internal/approval"
	"mellowtel-consumer/internal/browser"
	"mellowtel-consumer/internal/config"
	"mellowtel-consumer/internal/executor"
	"mellowtel-consumer/internal/fetch"
	"mellowtel-consumer/internal/job"
	"mellowtel-consumer/internal/markdown"
	"mellowtel-consumer/internal/notify"
	"mellowtel-consumer/internal/result"
	"mellowtel-consumer/internal/stats"
	"mellowtel-consumer/internal/wsclient"
)

const (
	// jobQueueSize bounds the backlog of pending jobs. Kept deliberately small:
	// a deep queue just means jobs sit around going stale, and the server is
	// liable to reassign or expire work we hold too long. Better to decline
	// immediately (it goes to another node) than to deliver a rejected result.
	jobQueueSize = 8

	// maxJobAge is how old a job may be at pickup before we skip it entirely.
	// Rendering a job the server has already written off wastes bandwidth and
	// a worker slot for a submission that will be refused.
	maxJobAge = 45 * time.Second
)

// Status is a snapshot of node state for the UI.
type Status struct {
	Connection  string `json:"connection"` // disconnected|connecting|connected
	Detail      string `json:"detail"`     // human-readable status line
	Paused      bool   `json:"paused"`
	Approved    bool   `json:"approved"`
	ChromeFound bool   `json:"chromeFound"`
	DeviceID    string `json:"deviceId"`
	Version     string `json:"version"`

	// Session counters (reset each run).
	JobsCompleted  int   `json:"jobsCompleted"`
	JobsFailed     int   `json:"jobsFailed"`
	BytesUsedToday int64 `json:"bytesUsedToday"`

	// Lifetime totals (persisted across restarts) — what the UI headlines.
	TotalJobsCompleted int64   `json:"totalJobsCompleted"`
	TotalJobsFailed    int64   `json:"totalJobsFailed"`
	SuccessRate        float64 `json:"successRate"`

	// Earnings. Zero until the ledger backend exists — never fabricated.
	TotalEarnedUSD   float64 `json:"totalEarnedUsd"`
	SessionEarnedUSD float64 `json:"sessionEarnedUsd"`
	EarningsReady    bool    `json:"earningsReady"`
}

// Manager orchestrates the node lifecycle.
type Manager struct {
	log       zerolog.Logger
	cfg       *config.Config
	configDir string
	deviceID  string

	fetcher   *fetch.Client
	md        *markdown.Converter
	submitter *result.Submitter
	engine    *browser.Engine

	chromePath  string
	chromeFound bool

	onStatus func(Status)

	stats    *stats.Store
	notifier *notify.Notifier

	mu        sync.Mutex
	status    Status
	runCtx    context.Context
	runCancel context.CancelFunc
	jobQueue  chan *job.Request
	bw        *bandwidthTracker
	wg        sync.WaitGroup
	connected bool

	// workerGen lets the pool be resized live: workers exit once their
	// generation is superseded, and a fresh set is spawned at the new size.
	workerGen int
	exec      *executor.Executor
}

// NewManager builds a Manager. It performs Chrome detection up front so the UI
// can prompt the user if Chrome is missing.
func NewManager(log zerolog.Logger, cfg *config.Config, configDir, deviceID string) *Manager {
	chromePath, found := browser.FindChrome()
	nodeLog := log.With().Str("component", "node").Logger()

	store, err := stats.Load(configDir)
	if err != nil {
		nodeLog.Warn().Err(err).Msg("could not load lifetime stats; starting from zero")
		store, _ = stats.Load(os.TempDir())
	}

	m := &Manager{
		log:         nodeLog,
		cfg:         cfg,
		configDir:   configDir,
		deviceID:    deviceID,
		fetcher:     fetch.New(),
		md:          markdown.New(),
		submitter:   result.New(nodeLog),
		chromePath:  chromePath,
		chromeFound: found,
		bw:          newBandwidthTracker(cfg.Settings.BandwidthCap.BytesPerDay()),
		stats:       store,
		notifier:    notify.New(nodeLog, cfg.Settings.Notifications),
	}
	m.status = Status{
		Connection:  string(wsclient.StateDisconnected),
		Detail:      "Paused",
		Paused:      true,
		ChromeFound: found,
		DeviceID:    deviceID,
		Version:     config.AppVersion,
	}
	m.applyTotals(&m.status)
	return m
}

// applyTotals refreshes the lifetime fields on a Status from the stats store.
func (m *Manager) applyTotals(s *Status) {
	t := m.stats.Totals()
	s.TotalJobsCompleted = t.JobsCompleted
	s.TotalJobsFailed = t.JobsFailed
	s.SuccessRate = t.SuccessRate()
	s.TotalEarnedUSD = t.EarnedUSD()
	// Earnings stay zero until the ledger backend is wired up.
	s.EarningsReady = false
}

// SetOnStatus registers a callback invoked on every status change.
func (m *Manager) SetOnStatus(fn func(Status)) {
	m.mu.Lock()
	m.onStatus = fn
	m.mu.Unlock()
}

// ChromeFound reports whether a Chrome installation was detected.
func (m *Manager) ChromeFound() bool { return m.chromeFound }

// Status returns the current status snapshot.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.status
	s.BytesUsedToday = m.bw.usedToday()
	m.applyTotals(&s)
	return s
}

// emit publishes a status update while holding no locks the callback might need.
func (m *Manager) emit(mutate func(*Status)) {
	m.mu.Lock()
	mutate(&m.status)
	m.status.BytesUsedToday = m.bw.usedToday()
	m.applyTotals(&m.status)
	snapshot := m.status
	cb := m.onStatus
	m.mu.Unlock()
	if cb != nil {
		cb(snapshot)
	}
}

// Connect starts the node: browser engine, worker pool, WebSocket and approval
// polling. It is idempotent.
func (m *Manager) Connect() error {
	m.mu.Lock()
	if m.connected {
		m.mu.Unlock()
		return nil
	}
	m.connected = true
	ctx, cancel := context.WithCancel(context.Background())
	m.runCtx = ctx
	m.runCancel = cancel
	m.jobQueue = make(chan *job.Request, jobQueueSize)
	m.bw.setCap(m.cfg.Settings.BandwidthCap.BytesPerDay())
	m.mu.Unlock()

	m.log.Info().Msg("connecting node")

	// Start the browser engine if Chrome is available. Failure is non-fatal:
	// jobs fall back to the simple fetch path.
	var engine *browser.Engine
	if m.chromeFound {
		userDataDir := filepath.Join(m.configDir, "chrome-profile")
		engine = browser.NewEngine(m.log, m.chromePath, userDataDir)
		engine.SetStripHeaders(m.cfg.StripFramingHeaders)
		if err := engine.Start(); err != nil {
			m.log.Warn().Err(err).Msg("failed to start browser engine; simple jobs only")
			engine = nil
		}
	}
	m.mu.Lock()
	m.engine = engine
	m.mu.Unlock()

	exec := m.buildExecutor(engine)
	m.mu.Lock()
	m.exec = exec
	m.mu.Unlock()

	// Worker pool, sized by the user's sharing intensity.
	m.spawnWorkers(ctx, m.cfg.Settings.SharingIntensity.Workers())

	// WebSocket client.
	ws := wsclient.New(wsclient.Config{
		Log:            m.log,
		BaseURL:        m.cfg.Endpoints.WebSocketURL,
		DeviceID:       m.deviceID,
		Version:        m.cfg.ProtocolVersion,
		PlatformPrefix: m.cfg.PlatformPrefix,
		SpeedDownload:  500,
		OnMessage:      m.onMessage,
		OnState:        m.onWSState,
		OnDisconnectCommand: func() {
			m.log.Info().Msg("pausing due to server disconnect command")
			go m.Disconnect()
		},
	})
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ws.Run(ctx)
	}()

	// Approval polling.
	checker := approval.New(m.log, m.cfg.Endpoints.ApprovalURL, approval.Params{
		DeviceID:      m.deviceID,
		Version:       m.cfg.ProtocolVersion,
		Platform:      m.cfg.PlatformPrefix,
		SpeedDownload: 500,
	})
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		checker.Run(ctx, m.onApproval)
	}()

	m.emit(func(s *Status) { s.Paused = false })
	return nil
}

// Disconnect stops the node cleanly.
func (m *Manager) Disconnect() {
	m.mu.Lock()
	if !m.connected {
		m.mu.Unlock()
		return
	}
	m.connected = false
	cancel := m.runCancel
	engine := m.engine
	m.mu.Unlock()

	m.log.Info().Msg("disconnecting node")
	// Workers and goroutines exit on context cancellation. The job queue is
	// intentionally not closed (senders may still race); it is dropped and GC'd.
	if cancel != nil {
		cancel()
	}
	m.wg.Wait()
	if engine != nil {
		engine.Stop()
	}
	m.mu.Lock()
	m.engine = nil
	m.jobQueue = nil
	m.mu.Unlock()

	m.emit(func(s *Status) {
		s.Connection = string(wsclient.StateDisconnected)
		s.Paused = true
		s.Detail = "Paused"
	})
}

// IsConnected reports whether the node is currently running.
func (m *Manager) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connected
}

// spawnWorkers starts a fresh generation of workers, retiring any previous
// generation. Callers must not hold m.mu.
func (m *Manager) spawnWorkers(ctx context.Context, count int) {
	if count < 1 {
		count = 1
	}
	m.mu.Lock()
	m.workerGen++
	gen := m.workerGen
	exec := m.exec
	m.mu.Unlock()

	m.log.Info().Int("workers", count).Msg("sizing worker pool")
	for i := 0; i < count; i++ {
		m.wg.Add(1)
		go m.worker(ctx, exec, gen)
	}
}

// ApplySettings applies setting changes live, without needing a reconnect.
func (m *Manager) ApplySettings(s config.Settings) {
	m.bw.setCap(s.BandwidthCap.BytesPerDay())
	m.notifier.SetEnabled(s.Notifications)

	// Resize the worker pool if sharing intensity changed while connected.
	m.mu.Lock()
	connected := m.connected
	ctx := m.runCtx
	m.mu.Unlock()
	if connected && ctx != nil {
		m.spawnWorkers(ctx, s.SharingIntensity.Workers())
	}
}

func (m *Manager) buildExecutor(engine *browser.Engine) *executor.Executor {
	var renderer executor.Renderer
	if engine != nil {
		renderer = engine
	}
	return executor.New(executor.Config{
		Log:             m.log,
		Fetcher:         m.fetcher,
		Renderer:        renderer,
		Markdown:        m.md,
		Submitter:       m.submitter,
		NodeID:          m.deviceID,
		DefaultEndpoint: m.cfg.Endpoints.DefaultResultEndpoint,
	})
}

// onMessage parses an inbound frame and enqueues jobs.
func (m *Manager) onMessage(data []byte) {
	req, err := job.Parse(data)
	if err != nil {
		m.log.Warn().Err(err).Msg("failed to parse inbound message")
		return
	}
	if !req.IsJob() {
		m.log.Debug().Str("type_event", req.TypeEvent).Msg("non-job message ignored")
		return
	}

	if m.shouldSkip(req) {
		return
	}

	m.mu.Lock()
	queue := m.jobQueue
	m.mu.Unlock()
	if queue == nil {
		return
	}
	select {
	case queue <- req:
		m.log.Info().Str("recordID", req.RecordID).Str("url", req.URL).Msg("job queued")
	default:
		m.log.Warn().Str("recordID", req.RecordID).Msg("job queue full; dropping job")
	}
}

// shouldSkip enforces the pause schedule and bandwidth cap.
func (m *Manager) shouldSkip(req *job.Request) bool {
	if withinPauseWindow(time.Now(), m.cfg.Settings.PauseScheduleFrom, m.cfg.Settings.PauseScheduleTo) {
		m.log.Info().Str("recordID", req.RecordID).Msg("skipping job: within pause schedule")
		m.emit(func(s *Status) { s.Detail = "Paused (schedule)" })
		return true
	}
	if m.bw.overCap() {
		m.log.Info().Str("recordID", req.RecordID).Msg("skipping job: bandwidth cap reached")
		m.emit(func(s *Status) { s.Detail = "Bandwidth cap reached" })
		return true
	}
	return false
}

// worker pulls jobs off the queue and executes them. It retires itself once its
// generation is superseded, which is how the pool resizes live.
func (m *Manager) worker(ctx context.Context, exec *executor.Executor, gen int) {
	defer m.wg.Done()
	m.mu.Lock()
	queue := m.jobQueue
	m.mu.Unlock()
	for {
		m.mu.Lock()
		stale := gen != m.workerGen
		m.mu.Unlock()
		if stale {
			return
		}

		select {
		case <-ctx.Done():
			return
		case req, ok := <-queue:
			if !ok {
				return
			}
			m.runJob(ctx, exec, req)
		}
	}
}

func (m *Manager) runJob(ctx context.Context, exec *executor.Executor, req *job.Request) {
	jobCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	// Age at pickup: how long this job waited in the queue before we started.
	// Stale jobs are liable to be reassigned or expired server-side.
	queueWait := time.Since(req.ReceivedAt)
	if queueWait > maxJobAge {
		// Not counted as a failure — we never attempted it.
		m.log.Warn().
			Str("recordID", req.RecordID).
			Dur("queue_wait", queueWait.Round(time.Millisecond)).
			Msg("skipping stale job; it waited too long to be worth submitting")
		return
	}

	outcome, err := exec.Execute(jobCtx, req)
	m.bw.add(outcome.BytesUsed)
	totalAge := time.Since(req.ReceivedAt)

	if err != nil {
		m.stats.RecordFailure(outcome.BytesUsed)
		m.emit(func(s *Status) {
			s.JobsFailed++
		})
		m.log.Error().Err(err).
			Str("recordID", req.RecordID).
			Dur("queue_wait", queueWait.Round(time.Millisecond)).
			Dur("job_age", totalAge.Round(time.Millisecond)).
			Msg("job failed")
		return
	}

	m.stats.RecordSuccess(outcome.BytesUsed)
	m.log.Debug().
		Str("recordID", req.RecordID).
		Dur("queue_wait", queueWait.Round(time.Millisecond)).
		Dur("job_age", totalAge.Round(time.Millisecond)).
		Msg("job accepted by server")
	m.emit(func(s *Status) {
		s.JobsCompleted++
		if !s.Paused {
			s.Detail = encouragement(s.JobsCompleted)
		}
	})

	if msg := notify.Milestone(m.stats.Totals().JobsCompleted); msg != "" {
		m.notifier.Send("Mellowtel", msg)
	}
}

// encouragement is the friendly status line shown while connected.
func encouragement(sessionJobs int) string {
	switch {
	case sessionJobs == 0:
		return "Connected — waiting for jobs."
	case sessionJobs < 10:
		return "You're up and earning."
	default:
		return "You're doing great — keep sharing."
	}
}

func (m *Manager) onWSState(state wsclient.State) {
	m.emit(func(s *Status) {
		s.Connection = string(state)
		switch state {
		case wsclient.StateConnected:
			s.Detail = encouragement(s.JobsCompleted)
		case wsclient.StateConnecting:
			s.Detail = "Connecting…"
		case wsclient.StateDisconnected:
			if m.connected {
				s.Detail = "Reconnecting…"
			}
		}
	})
}

func (m *Manager) onApproval(approved bool, err error) {
	m.emit(func(s *Status) { s.Approved = approved })
	if err != nil {
		// Network hiccups shouldn't pause the node; only an explicit denial does.
		return
	}
	if !approved {
		m.log.Warn().Msg("node not approved; disconnecting")
		m.emit(func(s *Status) { s.Detail = "Approval failed" })
		go m.Disconnect()
	}
}
