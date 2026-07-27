//go:build darwin

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
)

const plistLabel = "tel.mellow.consumer"

type darwinManager struct {
	execPath string
}

func newManager(execPath string) Manager {
	return &darwinManager{execPath: execPath}
}

func (m *darwinManager) plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Library", "LaunchAgents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, plistLabel+".plist"), nil
}

func (m *darwinManager) Enable() error {
	path, err := m.plistPath()
	if err != nil {
		return err
	}
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array><string>%s</string></array>
  <key>RunAtLoad</key><true/>
</dict>
</plist>
`, plistLabel, m.execPath)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write launch agent: %w", err)
	}
	return nil
}

func (m *darwinManager) Disable() error {
	path, err := m.plistPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove launch agent: %w", err)
	}
	return nil
}

func (m *darwinManager) IsEnabled() (bool, error) {
	path, err := m.plistPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
