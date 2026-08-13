package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"time"

	"github.com/rs/zerolog/log"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"mellowtel-consumer/internal/account"
	"mellowtel-consumer/internal/activity"
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
	account   *account.Client
	activity  *activity.Reporter

	configDir string
	logPath   string
	logCloser io.Closer
	execPath  string
}

// startup is invoked by Wails once the runtime is ready. All wiring that needs
// the runtime context (event emission, auto-connect) happens here.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.activity.Start(ctx)

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

	if a.cfg.Settings.AutoConnect && a.account.HasSession() {
		log.Info().Msg("auto-connect enabled; connecting")
		if err := a.connectRegisteredDevice(); err != nil {
			log.Error().Err(err).Msg("auto-connect failed")
		}
	}
}

// shutdown is invoked by Wails on quit. It stops the node and closes logs.
func (a *App) shutdown(ctx context.Context) {
	log.Info().Msg("shutting down")
	// Begin cleanup without waiting for every in-flight network worker. A slow
	// request must never keep the desktop process alive after the user quits.
	a.manager.DisconnectAsync()
	if a.tray != nil {
		a.tray.Quit()
	}
	if a.logCloser != nil {
		a.logCloser.Close()
	}
}

// Quit closes Earnbear completely, even when close-to-tray is enabled.
func (a *App) Quit() {
	if a.ctx != nil {
		wailsruntime.Quit(a.ctx)
	}
}

// ---- Bound methods callable from the frontend ----

// Connect starts sharing.
func (a *App) Connect() error {
	if !a.account.HasSession() {
		return fmt.Errorf("sign in before starting sharing")
	}
	log.Info().Msg("frontend requested connect")
	return a.connectRegisteredDevice()
}

func (a *App) connectRegisteredDevice() error {
	deviceID := a.manager.Status().DeviceID
	registration, err := a.account.RegisterDevice(account.DeviceRegistration{
		DeviceID:        deviceID,
		AppVersion:      config.AppVersion,
		ProtocolVersion: a.cfg.ProtocolVersion,
		Platform:        runtime.GOOS,
		Integration:     a.cfg.Integration,
	})
	if err != nil {
		return fmt.Errorf("register device: %w", err)
	}
	a.manager.SetDeviceToken(registration.DeviceToken)
	a.activity.SetCredential(deviceID, registration.DeviceToken)
	a.manager.SetOnActivity(func(activity node.AcceptedActivity) {
		activityID := fmt.Sprintf("%x", sha256.Sum256([]byte(deviceID+"\x00"+activity.RecordID)))
		if err := a.activity.Enqueue(account.ClientActivity{
			ActivityID: activityID, BytesUsed: activity.BytesUsed,
			OccurredAt: activity.OccurredAt.Format(time.RFC3339Nano),
		}); err != nil {
			log.Warn().Err(err).Str("activity_id", activityID).Msg("could not persist provisional activity")
		}
	})
	return a.manager.Connect()
}

// Disconnect pauses sharing.
func (a *App) Disconnect() {
	log.Info().Msg("frontend requested disconnect")
	a.manager.Disconnect()
}

// Toggle flips connection state and returns the resulting connected flag.
func (a *App) Toggle() bool {
	if !a.account.HasSession() {
		return false
	}
	if a.manager.IsConnected() {
		a.manager.DisconnectAsync()
		return false
	}
	// A previous disconnect may still be draining browser and worker resources.
	// Reconnect in the background so the UI bridge returns immediately instead
	// of leaving the power button disabled while that cleanup completes.
	go func() {
		if err := a.connectRegisteredDevice(); err != nil {
			log.Error().Err(err).Msg("connect failed")
		}
	}()
	return true
}

// GetAuthState restores and validates the user's Earnbear account session.
func (a *App) GetAuthState() (account.State, error) {
	return a.account.GetState()
}

// SignIn authenticates through Earnbear's server-side Cognito integration.
func (a *App) SignIn(email, password string) (account.State, error) {
	return a.account.SignIn(email, password)
}

// SignUp creates an account and reports whether email confirmation is needed.
func (a *App) SignUp(email, password, affiliateCode string) (account.SignUpResult, error) {
	return a.account.SignUp(email, password, affiliateCode)
}

// ConfirmSignUp verifies the six-digit code sent to the user's email.
func (a *App) ConfirmSignUp(email, code string) error {
	return a.account.ConfirmSignUp(email, code)
}

// ResendSignUpCode sends a fresh email verification code.
func (a *App) ResendSignUpCode(email string) error {
	return a.account.ResendSignUpCode(email)
}

// SignOut clears the local session and stops bandwidth sharing immediately.
func (a *App) SignOut() error {
	a.manager.Disconnect()
	a.activity.ClearCredential()
	return a.account.SignOut()
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
