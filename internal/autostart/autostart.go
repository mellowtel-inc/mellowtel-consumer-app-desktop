// Package autostart enables or disables launching the app when the user logs
// in. Each platform has its own implementation in a build-tagged file.
package autostart

// AppName is the label used for the autostart entry.
const AppName = "Mellowtel"

// Manager controls the login-item / autostart entry for the current executable.
type Manager interface {
	// Enable registers the app to launch on login.
	Enable() error
	// Disable removes the autostart entry.
	Disable() error
	// IsEnabled reports whether autostart is currently registered.
	IsEnabled() (bool, error)
}

// New returns the platform Manager for the current executable path.
func New(execPath string) Manager {
	return newManager(execPath)
}
