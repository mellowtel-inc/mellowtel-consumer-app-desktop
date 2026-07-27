// Package config loads and persists application configuration and user
// settings. Endpoints and protocol constants live here so nothing is
// hardcoded elsewhere in the codebase.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Endpoints holds every remote URL the app talks to. Centralised so tests can
// point them at local stubs and so there are no hardcoded URLs scattered
// through the code.
type Endpoints struct {
	// WebSocketURL is the Mellowtel node registration socket.
	WebSocketURL string `json:"websocketURL"`
	// ApprovalURL is polled periodically to confirm the node may keep running.
	ApprovalURL string `json:"approvalURL"`
	// DefaultResultEndpoint is where scrape results are POSTed when a job does
	// not carry its own save_html_endpoint.
	DefaultResultEndpoint string `json:"defaultResultEndpoint"`
}

// BandwidthCapMode enumerates the daily traffic ceiling options.
type BandwidthCapMode string

const (
	CapUnlimited BandwidthCapMode = "unlimited"
	Cap5GB       BandwidthCapMode = "5gb"
	Cap1GB       BandwidthCapMode = "1gb"
)

// BytesPerDay returns the daily byte ceiling for the cap mode, or 0 for
// unlimited.
func (m BandwidthCapMode) BytesPerDay() int64 {
	switch m {
	case Cap5GB:
		return 5 * 1024 * 1024 * 1024
	case Cap1GB:
		return 1 * 1024 * 1024 * 1024
	default:
		return 0
	}
}

// SharingIntensity is how hard the node works, expressed in user-friendly terms
// rather than a raw job count. The UI shows Low/Medium/Max; the concurrency
// numbers behind them are an implementation detail deliberately kept hidden.
type SharingIntensity string

const (
	IntensityLow    SharingIntensity = "low"
	IntensityMedium SharingIntensity = "medium"
	IntensityMax    SharingIntensity = "max"
)

// Workers returns how many jobs may run concurrently at this intensity.
func (s SharingIntensity) Workers() int {
	switch s {
	case IntensityLow:
		return 2
	case IntensityMax:
		return 8
	default: // medium
		return 4
	}
}

// Description is the helper copy shown under the intensity selector.
func (s SharingIntensity) Description() string {
	switch s {
	case IntensityLow:
		return "Light — minimal impact, ideal while you're working."
	case IntensityMax:
		return "Maximum — earn the most, best when you're away."
	default:
		return "Balanced — a steady amount of sharing without slowing you down."
	}
}

// Settings holds user-adjustable preferences surfaced in the settings screen.
type Settings struct {
	AutoConnect       bool             `json:"autoConnect"`
	LaunchOnStartup   bool             `json:"launchOnStartup"`
	CloseToTray       bool             `json:"closeToTray"`
	Notifications     bool             `json:"notifications"`
	SharingIntensity  SharingIntensity `json:"sharingIntensity"`
	BandwidthCap      BandwidthCapMode `json:"bandwidthCap"`
	PauseScheduleFrom string           `json:"pauseScheduleFrom"` // "HH:MM" 24h, empty = disabled
	PauseScheduleTo   string           `json:"pauseScheduleTo"`   // "HH:MM" 24h, empty = disabled
}

// Config is the full persisted configuration document.
type Config struct {
	// Integration is the configuration key embedded in the device identifier
	// (mllwtl_<integration>_<rand10>). It identifies this client to Mellowtel.
	Integration string `json:"integration"`
	// Platform label reported to the registration socket, e.g. "desktop-linux".
	// The OS suffix is appended at connection time.
	PlatformPrefix string `json:"platformPrefix"`
	// ProtocolVersion is the version string reported to the socket / approval
	// endpoint. Kept separate from the user-facing app version.
	ProtocolVersion string    `json:"protocolVersion"`
	Endpoints       Endpoints `json:"endpoints"`
	Settings        Settings  `json:"settings"`

	// StripFramingHeaders enables CDP Fetch interception to remove
	// X-Frame-Options/CSP response headers. Off by default: pages are rendered
	// as top-level navigations where those headers don't apply, and enabling
	// interception pauses every response (hanging navigation if a continue
	// fails). Only turn on if rendering inside a frame.
	StripFramingHeaders bool `json:"stripFramingHeaders"`

	// path is where this config was loaded from / will be saved to.
	path string
	mu   sync.Mutex
}

// AppVersion is the user-facing version of the consumer app.
const AppVersion = "0.1.0-mvp"

// Defaults returns a Config populated with production defaults.
func Defaults() Config {
	return Config{
		Integration:     "consumer",
		PlatformPrefix:  "desktop",
		ProtocolVersion: "700.0.29",
		Endpoints: Endpoints{
			WebSocketURL:          "wss://ws.mellow.tel",
			ApprovalURL:           "https://api.mellow.tel/approval",
			DefaultResultEndpoint: "https://request.mellow.tel/",
		},
		Settings: Settings{
			AutoConnect:       false,
			LaunchOnStartup:   false,
			CloseToTray:       true,
			Notifications:     true,
			SharingIntensity:  IntensityMedium,
			BandwidthCap:      CapUnlimited,
			PauseScheduleFrom: "",
			PauseScheduleTo:   "",
		},
	}
}

// Dir returns the OS-appropriate application config directory, creating it if
// necessary.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	dir := filepath.Join(base, "Mellowtel")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create config dir %q: %w", dir, err)
	}
	return dir, nil
}

// Load reads config.json from the application config directory. If the file
// does not exist, defaults are written and returned.
func Load() (*Config, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "config.json")
	return LoadFrom(path)
}

// LoadFrom reads config from an explicit path, merging over defaults so new
// fields added in later versions get sane values.
func LoadFrom(path string) (*Config, error) {
	cfg := Defaults()
	cfg.path = path

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := cfg.Save(); err != nil {
			return nil, fmt.Errorf("write initial config: %w", err)
		}
		return &cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}
	cfg.normalize()
	return &cfg, nil
}

// normalize repairs any missing/invalid values after an unmarshal.
func (c *Config) normalize() {
	d := Defaults()
	if c.Integration == "" {
		c.Integration = d.Integration
	}
	if c.PlatformPrefix == "" {
		c.PlatformPrefix = d.PlatformPrefix
	}
	if c.ProtocolVersion == "" {
		c.ProtocolVersion = d.ProtocolVersion
	}
	if c.Endpoints.WebSocketURL == "" {
		c.Endpoints.WebSocketURL = d.Endpoints.WebSocketURL
	}
	if c.Endpoints.ApprovalURL == "" {
		c.Endpoints.ApprovalURL = d.Endpoints.ApprovalURL
	}
	if c.Endpoints.DefaultResultEndpoint == "" {
		c.Endpoints.DefaultResultEndpoint = d.Endpoints.DefaultResultEndpoint
	}
	if c.Settings.BandwidthCap == "" {
		c.Settings.BandwidthCap = d.Settings.BandwidthCap
	}
	switch c.Settings.SharingIntensity {
	case IntensityLow, IntensityMedium, IntensityMax:
		// valid
	default:
		c.Settings.SharingIntensity = d.Settings.SharingIntensity
	}
}

// Save writes the config back to disk atomically.
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.path == "" {
		dir, err := Dir()
		if err != nil {
			return err
		}
		c.path = filepath.Join(dir, "config.json")
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := os.Rename(tmp, c.path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

// Path returns the file the config is backed by.
func (c *Config) Path() string { return c.path }

// UpdateSettings replaces the settings block and persists.
func (c *Config) UpdateSettings(s Settings) error {
	c.mu.Lock()
	c.Settings = s
	c.mu.Unlock()
	c.normalize()
	return c.Save()
}
