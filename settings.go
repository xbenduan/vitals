package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultShortcut = "Command+Shift+V"

type appSettings struct {
	Shortcut     string `json:"shortcut"`
	HistoryLimit int    `json:"historyLimit"`
	Paused       bool   `json:"paused"`
	OpenAtLogin  bool   `json:"-"`
}

func defaultSettings() appSettings {
	return appSettings{Shortcut: defaultShortcut, HistoryLimit: 500}
}

func (s *appSettings) normalize() {
	if strings.TrimSpace(s.Shortcut) == "" {
		s.Shortcut = defaultShortcut
	}
	if s.HistoryLimit != 100 && s.HistoryLimit != 500 && s.HistoryLimit != 1000 {
		s.HistoryLimit = 500
	}
}

func loadSettings(path string) (appSettings, error) {
	settings := defaultSettings()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return settings, fmt.Errorf("读取设置：%w", err)
	}
	settings.normalize()
	return settings, nil
}

func saveSettings(path string, settings appSettings) error {
	settings.normalize()
	return writeJSON(path, settings)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, data)
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".clipboard-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err = temp.Write(data); err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temp.Name(), path)
}
