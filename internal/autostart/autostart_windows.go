//go:build windows

package autostart

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

type windowsManager struct {
	execPath string
}

func newManager(execPath string) Manager {
	return &windowsManager{execPath: execPath}
}

func (m *windowsManager) Enable() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open Run key: %w", err)
	}
	defer key.Close()
	if err := key.SetStringValue(AppName, fmt.Sprintf("%q", m.execPath)); err != nil {
		return fmt.Errorf("set Run value: %w", err)
	}
	return nil
}

func (m *windowsManager) Disable() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open Run key: %w", err)
	}
	defer key.Close()
	if err := key.DeleteValue(AppName); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("delete Run value: %w", err)
	}
	return nil
}

func (m *windowsManager) IsEnabled() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false, fmt.Errorf("open Run key: %w", err)
	}
	defer key.Close()
	_, _, err = key.GetStringValue(AppName)
	if err == registry.ErrNotExist {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
