package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
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
	ctx        context.Context
	cfg        *config.Config
	manager    *node.Manager
	autostart  autostart.Manager
	tray       *tray.Tray
	account    *account.Client
	activity   *activity.Reporter
	sharingMu  sync.Mutex // serialize session validation, connect and sign-out
	signingOut atomic.Bool

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
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if a.account.HasSession() || a.manager.IsConnected() {
					_, _ = a.GetAuthState()
				}
			}
		}
	}()

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
	a.activity.Flush()
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
	a.sharingMu.Lock()
	defer a.sharingMu.Unlock()
	if a.signingOut.Load() {
		return fmt.Errorf("sign-out is in progress")
	}
	state, err := a.account.GetState()
	if err == nil && a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "auth:update", state)
	}
	if err != nil || !state.Authenticated || !state.EarningApproved || state.UserID == "" {
		reason := "Your account needs approval before sharing can start."
		if err != nil {
			reason = "Could not verify account approval. Sharing is paused."
		} else if !state.Authenticated {
			reason = "Sign in before starting sharing."
		}
		if err != nil {
			a.stopSharing(reason)
		} else {
			a.activity.ClearCredential()
			a.manager.BlockSharing("not_approved", reason)
		}
		return fmt.Errorf("%s", reason)
	}
	a.activity.SetAccount(state.UserID)
	a.activity.ClearCredential()
	deviceID := a.manager.Status().DeviceID
	registration, err := a.account.RegisterDevice(account.DeviceRegistration{
		DeviceID:        deviceID,
		AppVersion:      config.AppVersion,
		ProtocolVersion: a.cfg.ProtocolVersion,
		Platform:        runtime.GOOS,
		Integration:     a.cfg.Integration,
	})
	if err != nil {
		a.blockDeviceRegistration(err)
		return fmt.Errorf("register device: %w", err)
	}
	if a.signingOut.Load() {
		return fmt.Errorf("sign-out is in progress")
	}
	a.manager.SetDeviceToken(registration.DeviceToken)
	a.manager.SetOnActivity(func(activity node.AcceptedActivity) {
		activityID := fmt.Sprintf("%x", sha256.Sum256([]byte(deviceID+"\x00"+activity.RecordID)))
		if err := a.activity.EnqueueForAccount(state.UserID, account.ClientActivity{
			ActivityID: activityID, BytesUsed: activity.BytesUsed,
			OccurredAt: activity.OccurredAt.Format(time.RFC3339Nano),
		}); err != nil {
			log.Warn().Err(err).Str("activity_id", activityID).Msg("could not persist provisional activity")
		}
	})
	if err := a.manager.Connect(); err != nil {
		a.activity.ClearCredential()
		return err
	}
	if a.manager.Status().Approved {
		a.activity.SetCredential(deviceID, registration.DeviceToken)
	}
	return nil
}

func (a *App) stopSharing(reason string) {
	a.activity.ClearCredential()
	a.manager.PauseWithReason(reason)
}

// A registration conflict is a known device block, not an inconclusive
// approval check. Keep the original binding intact until it is resolved.
func (a *App) blockDeviceRegistration(err error) {
	var response *account.HTTPError
	if errors.As(err, &response) {
		log.Warn().Int("http_status", response.StatusCode).Msg("device registration failed")
		if response.StatusCode == http.StatusConflict {
			a.activity.ClearCredential()
			switch response.Code {
			case "device_changed":
				a.stopSharing("The device link changed during verification. Check approval again.")
			case "device_revoked":
				a.manager.BlockSharing("device_conflict", "This device has been revoked. Contact support to restore sharing.")
			case "device_linked":
				a.manager.BlockSharing("device_conflict", "This device is linked to another account. Sign in to its linked account or contact support.")
			default:
				a.manager.BlockSharing("device_conflict", "This device is revoked or linked to another account. Sharing is unavailable.")
			}
			return
		}
		if response.StatusCode == http.StatusForbidden {
			a.activity.ClearCredential()
			a.manager.BlockSharing("not_approved", "Your account is not approved to earn. Sharing is unavailable until approval.")
			return
		}
	} else {
		log.Warn().Msg("device registration could not be completed")
	}
	a.stopSharing("Could not register this device. Check your connection and try again.")
}

// CheckSharingApproval refreshes permission while leaving sharing paused.
func (a *App) CheckSharingApproval() node.Status {
	a.sharingMu.Lock()
	defer a.sharingMu.Unlock()
	if a.signingOut.Load() {
		a.manager.BlockSharing("not_approved", "Sign in before checking approval.")
		return a.manager.Status()
	}
	if a.manager.IsConnected() {
		return a.manager.Status()
	}
	state, err := a.account.GetState()
	if err != nil {
		a.stopSharing("Could not verify account approval. Sharing is unavailable.")
		return a.manager.Status()
	}
	if a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "auth:update", state)
	}
	if !state.Authenticated || !state.EarningApproved || state.UserID == "" {
		a.activity.ClearCredential()
		a.manager.BlockSharing("not_approved", "Your account is not approved to earn. Sharing is unavailable until approval.")
		return a.manager.Status()
	}
	a.activity.SetAccount(state.UserID)
	registration, err := a.account.RegisterDevice(account.DeviceRegistration{
		DeviceID: a.manager.Status().DeviceID, AppVersion: config.AppVersion,
		ProtocolVersion: a.cfg.ProtocolVersion, Platform: runtime.GOOS, Integration: a.cfg.Integration,
	})
	if err != nil {
		a.blockDeviceRegistration(err)
		return a.manager.Status()
	}
	if a.signingOut.Load() {
		return a.manager.Status()
	}
	a.manager.SetDeviceToken(registration.DeviceToken)
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	_ = a.manager.CheckApproval(ctx)
	return a.manager.Status()
}

// Disconnect pauses sharing.
func (a *App) Disconnect() {
	log.Info().Msg("frontend requested disconnect")
	a.manager.Disconnect()
	a.activity.Flush()
}

// Toggle flips connection state and returns the resulting connected flag.
func (a *App) Toggle() bool {
	if !a.account.HasSession() {
		return false
	}
	if a.manager.IsConnected() {
		a.manager.DisconnectAsync()
		a.activity.Flush()
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
	a.sharingMu.Lock()
	defer a.sharingMu.Unlock()
	state, err := a.account.GetState()
	if err != nil {
		a.stopSharing("Could not verify account approval. Sharing is paused.")
	} else if !state.Authenticated || !state.EarningApproved {
		a.activity.ClearCredential()
		a.manager.BlockSharing("not_approved", "Your account is not approved to earn. Sharing is unavailable until approval.")
	}
	if err == nil && state.Authenticated {
		a.activity.SetAccount(state.UserID)
	}
	if err == nil && a.ctx != nil {
		wailsruntime.EventsEmit(a.ctx, "auth:update", state)
	}
	return state, err
}

// SignIn authenticates through Earnbear's server-side Cognito integration.
func (a *App) SignIn(email, password string) (account.State, error) {
	a.sharingMu.Lock()
	defer a.sharingMu.Unlock()
	if a.signingOut.Load() {
		return account.State{}, fmt.Errorf("sign-out is in progress")
	}
	// Drain current work/uploads before replacing the session. The installation
	// credential survives logout; sharing permission never does.
	a.manager.Disconnect()
	a.manager.SetDeviceToken("")
	a.activity.SetAccount("")
	a.activity.ClearCredential()
	a.manager.BlockSharing("unknown", "Checking the signed-in account before sharing…")
	state, err := a.account.SignIn(email, password)
	if err != nil {
		return state, err
	}
	a.activity.SetAccount(state.UserID)
	if !state.EarningApproved {
		a.manager.BlockSharing("not_approved", "Your account is not approved to earn. Sharing is unavailable until approval.")
	}
	return state, nil
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

// JoinWaitlist creates or restores a waitlist place directly from desktop.
func (a *App) JoinWaitlist(email, referralCode, affiliateCode string) (account.WaitlistResult, error) {
	return a.account.JoinWaitlist(email, referralCode, affiliateCode)
}

// SaveWaitlistProfile completes the same onboarding profile used by the site.
func (a *App) SaveWaitlistProfile(email, onboardingToken, country string, devices []string, claimedX, claimedTikTok, claimedDiscord, onboardingCompleted bool) (account.WaitlistProfileResult, error) {
	return a.account.SaveWaitlistProfile(email, onboardingToken, country, devices, claimedX, claimedTikTok, claimedDiscord, onboardingCompleted)
}

// SignOut clears the local session and stops bandwidth sharing immediately.
func (a *App) SignOut() error {
	a.signingOut.Store(true)
	defer a.signingOut.Store(false)
	// Cancel active work even when session validation is holding the lock.
	a.manager.DisconnectAsync()
	a.activity.ClearCredential()
	a.sharingMu.Lock()
	defer a.sharingMu.Unlock()
	a.manager.Disconnect()
	a.manager.SetDeviceToken("")
	a.manager.BlockSharing("not_approved", "Sign in before starting sharing.")
	a.activity.SetAccount("")
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
