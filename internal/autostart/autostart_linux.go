//go:build linux

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
)

type linuxManager struct {
	execPath string
}

func newManager(execPath string) Manager {
	return &linuxManager{execPath: execPath}
}

func (m *linuxManager) desktopPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "autostart")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "mellowtel.desktop"), nil
}

func (m *linuxManager) Enable() error {
	path, err := m.desktopPath()
	if err != nil {
		return err
	}
	content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=%s
Exec=%s
X-GNOME-Autostart-enabled=true
Terminal=false
`, AppName, m.execPath)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write autostart entry: %w", err)
	}
	return nil
}

func (m *linuxManager) Disable() error {
	path, err := m.desktopPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove autostart entry: %w", err)
	}
	return nil
}

func (m *linuxManager) IsEnabled() (bool, error) {
	path, err := m.desktopPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}
