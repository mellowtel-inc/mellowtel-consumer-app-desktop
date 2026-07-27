// Package tray provides the system tray icon and menu. It is best-effort:
// failures to initialise the tray (e.g. missing appindicator libraries on
// Linux) are logged but never crash the app.
package tray

import (
	"sync"

	"fyne.io/systray"
	"github.com/rs/zerolog"
)

// Callbacks are the actions the tray menu triggers.
type Callbacks struct {
	OnShow   func()
	OnToggle func()
	OnQuit   func()
}

// Tray manages the system tray icon and menu.
type Tray struct {
	log zerolog.Logger
	cb  Callbacks

	mu        sync.Mutex
	ready     bool
	connected bool

	mShow   *systray.MenuItem
	mToggle *systray.MenuItem
	mQuit   *systray.MenuItem
}

// New builds a Tray.
func New(log zerolog.Logger, cb Callbacks) *Tray {
	return &Tray{log: log.With().Str("component", "tray").Logger(), cb: cb}
}

// Start launches the tray. systray.Run blocks, so it runs in its own goroutine.
// On macOS the tray must own the main thread; there this may be a no-op and the
// app still functions via its window.
func (t *Tray) Start() {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t.log.Warn().Interface("panic", r).Msg("tray unavailable")
			}
		}()
		systray.Run(t.onReady, t.onExit)
	}()
}

func (t *Tray) onReady() {
	systray.SetTitle("Mellowtel")
	systray.SetTooltip("Mellowtel — Paused")
	systray.SetIcon(iconPaused)

	t.mShow = systray.AddMenuItem("Show", "Show the Mellowtel window")
	t.mToggle = systray.AddMenuItem("Connect", "Start or pause sharing")
	systray.AddSeparator()
	t.mQuit = systray.AddMenuItem("Quit", "Quit Mellowtel")

	t.mu.Lock()
	t.ready = true
	connected := t.connected
	t.mu.Unlock()
	t.applyState(connected)

	go t.loop()
}

func (t *Tray) loop() {
	for {
		select {
		case <-t.mShow.ClickedCh:
			if t.cb.OnShow != nil {
				t.cb.OnShow()
			}
		case <-t.mToggle.ClickedCh:
			if t.cb.OnToggle != nil {
				t.cb.OnToggle()
			}
		case <-t.mQuit.ClickedCh:
			if t.cb.OnQuit != nil {
				t.cb.OnQuit()
			}
			return
		}
	}
}

func (t *Tray) onExit() {
	t.log.Debug().Msg("tray exited")
}

// SetConnected updates the tray icon/label to reflect connection state. Safe to
// call before the tray is ready.
func (t *Tray) SetConnected(connected bool) {
	t.mu.Lock()
	t.connected = connected
	ready := t.ready
	t.mu.Unlock()
	if ready {
		t.applyState(connected)
	}
}

func (t *Tray) applyState(connected bool) {
	if connected {
		systray.SetIcon(iconConnected)
		systray.SetTooltip("Mellowtel — Connected")
		if t.mToggle != nil {
			t.mToggle.SetTitle("Pause")
		}
	} else {
		systray.SetIcon(iconPaused)
		systray.SetTooltip("Mellowtel — Paused")
		if t.mToggle != nil {
			t.mToggle.SetTitle("Connect")
		}
	}
}

// Quit tears down the tray.
func (t *Tray) Quit() {
	t.mu.Lock()
	ready := t.ready
	t.mu.Unlock()
	if ready {
		systray.Quit()
	}
}
