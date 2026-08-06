// Package tray provides the system tray icon and menu. It is best-effort:
// failures to initialise the tray (e.g. missing appindicator libraries on
// Linux) are logged but never crash the app.
package tray

import (
	"runtime"
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
	endLoop   func()

	mShow   *systray.MenuItem
	mToggle *systray.MenuItem
	mQuit   *systray.MenuItem
}

// New builds a Tray.
func New(log zerolog.Logger, cb Callbacks) *Tray {
	return &Tray{log: log.With().Str("component", "tray").Logger(), cb: cb}
}

// Start prepares the tray. On macOS Wails owns the Cocoa main loop, so systray
// is attached through its external-loop API before Wails takes over that loop.
func (t *Tray) Start() {
	if runtime.GOOS == "darwin" {
		start, end := systray.RunWithExternalLoop(t.onReady, t.onExit)
		t.mu.Lock()
		t.endLoop = end
		t.mu.Unlock()
		// Start is called from main before Wails.Run, which is the only safe
		// point to create AppKit status-bar objects on macOS.
		start()
		return
	}

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
	systray.SetTitle("Earnbear")
	systray.SetTooltip("Earnbear — Paused")
	systray.SetIcon(iconPaused)

	t.mShow = systray.AddMenuItem("Show", "Show the Earnbear window")
	t.mToggle = systray.AddMenuItem("Connect", "Start or pause sharing")
	systray.AddSeparator()
	t.mQuit = systray.AddMenuItem("Quit", "Quit Earnbear")

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
	t.mu.Lock()
	t.ready = false
	t.mu.Unlock()
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
		systray.SetTooltip("Earnbear — Connected")
		if t.mToggle != nil {
			t.mToggle.SetTitle("Pause")
		}
	} else {
		systray.SetIcon(iconPaused)
		systray.SetTooltip("Earnbear — Paused")
		if t.mToggle != nil {
			t.mToggle.SetTitle("Connect")
		}
	}
}

// Quit tears down the tray.
func (t *Tray) Quit() {
	t.mu.Lock()
	ready := t.ready
	end := t.endLoop
	t.mu.Unlock()
	if end != nil {
		end()
	} else if ready {
		systray.Quit()
	}
}
