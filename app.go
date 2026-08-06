package main

import (
	"context"
	"io"
	"os/exec"
	"runtime"

	"github.com/rs/zerolog/log"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"mellowtel-consumer/internal/autostart"
	"mellowtel-consumer/internal/browser"
	"mellowtel-consumer/internal/config"
	"mellowtel-consumer/internal/node"
	"mellowtel-consumer/internal/tray"
)

// App is the Wails-bound application surface exposed to the frontend.
type App struct {
	ctx       context.Context
	cfg       *config.Config
	manager   *node.Manager
	autostart autostart.Manager
	tray      *tray.Tray

	configDir string
	logPath   string
	logCloser io.Closer
	execPath  string
}

// startup is invoked by Wails once the runtime is ready. All wiring that needs
// the runtime context (event emission, auto-connect) happens here.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Push status changes to both the frontend and the tray icon.
	a.manager.SetOnStatus(func(s node.Status) {
		wailsruntime.EventsEmit(a.ctx, "status:update", s)
		if a.tray != nil {
			a.tray.SetConnected(s.Connection == "connected")
		}
	})

	if !a.manager.ChromeFound() {
		log.Warn().Msg("chrome not detected at startup")
		wailsruntime.EventsEmit(a.ctx, "chrome:missing", browser.DownloadURL)
	}

	if a.cfg.Settings.AutoConnect {
		log.Info().Msg("auto-connect enabled; connecting")
		if err := a.manager.Connect(); err != nil {
			log.Error().Err(err).Msg("auto-connect failed")
		}
	}
}

// shutdown is invoked by Wails on quit. It stops the node and closes logs.
func (a *App) shutdown(ctx context.Context) {
	log.Info().Msg("shutting down")
	a.manager.Disconnect()
	if a.tray != nil {
		a.tray.Quit()
	}
	if a.logCloser != nil {
		a.logCloser.Close()
	}
}

// beforeClose implements close-to-tray: returning true prevents the window from
// closing so the app keeps running in the tray.
func (a *App) beforeClose(ctx context.Context) bool {
	if a.cfg.Settings.CloseToTray {
		wailsruntime.WindowHide(ctx)
		return true // prevent close
	}
	return false
}

// ---- Bound methods callable from the frontend ----

// Connect starts sharing.
func (a *App) Connect() error {
	log.Info().Msg("frontend requested connect")
	return a.manager.Connect()
}

// Disconnect pauses sharing.
func (a *App) Disconnect() {
	log.Info().Msg("frontend requested disconnect")
	a.manager.Disconnect()
}

// Toggle flips connection state and returns the resulting connected flag.
func (a *App) Toggle() bool {
	if a.manager.IsConnected() {
		a.manager.DisconnectAsync()
		return false
	}
	if err := a.manager.Connect(); err != nil {
		log.Error().Err(err).Msg("connect failed")
		return false
	}
	return true
}

// GetStatus returns the current node status.
func (a *App) GetStatus() node.Status {
	return a.manager.Status()
}

// GetDeviceID returns the persistent node identifier.
func (a *App) GetDeviceID() string {
	return a.manager.Status().DeviceID
}

// GetVersion returns the app version.
func (a *App) GetVersion() string {
	return config.AppVersion
}

// IsChromeInstalled reports whether Chrome was detected.
func (a *App) IsChromeInstalled() bool {
	return a.manager.ChromeFound()
}

// ChromeDownloadURL returns the URL shown when Chrome is missing.
func (a *App) ChromeDownloadURL() string {
	return browser.DownloadURL
}

// GetSettings returns the current user settings.
func (a *App) GetSettings() config.Settings {
	return a.cfg.Settings
}

// SaveSettings persists settings and applies live changes.
func (a *App) SaveSettings(s config.Settings) error {
	prev := a.cfg.Settings
	if err := a.cfg.UpdateSettings(s); err != nil {
		return err
	}
	a.manager.ApplySettings(s)

	// Reconcile the OS autostart entry if the toggle changed.
	if s.LaunchOnStartup != prev.LaunchOnStartup {
		var err error
		if s.LaunchOnStartup {
			err = a.autostart.Enable()
		} else {
			err = a.autostart.Disable()
		}
		if err != nil {
			log.Error().Err(err).Msg("failed to update autostart entry")
		}
	}
	return nil
}

// OpenLogsFolder opens the log file location in the OS file manager.
func (a *App) OpenLogsFolder() {
	openPath(a.configDir)
}

// GetLogPath returns the absolute log file path.
func (a *App) GetLogPath() string {
	return a.logPath
}

// OpenURL opens a URL in the user's default browser.
func (a *App) OpenURL(url string) {
	wailsruntime.BrowserOpenURL(a.ctx, url)
}

// ShowWindow brings the main window to the foreground (used by the tray).
func (a *App) ShowWindow() {
	if a.ctx != nil {
		wailsruntime.WindowShow(a.ctx)
	}
}

// openPath opens a file-system path in the platform file manager.
func openPath(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	if err := cmd.Start(); err != nil {
		log.Warn().Err(err).Str("path", path).Msg("failed to open path")
	}
}
