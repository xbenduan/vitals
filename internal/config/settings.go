// Package config loads and saves clipboard application preferences.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"vitals/internal/atomicfile"
	"vitals/internal/translation"
)

// Settings holds user preferences; OpenAtLogin is read from the OS, not persisted.
type Settings struct {
	Shortcut     string             `json:"shortcut"`
	HistoryLimit int                `json:"historyLimit"`
	Paused       bool               `json:"paused"`
	OpenAtLogin  bool               `json:"-"`
	Translation  translation.Config `json:"translation"`
}

// Default returns the initial clipboard preferences.
func Default() Settings {
	return Settings{Shortcut: DefaultShortcut, HistoryLimit: 500, Translation: translation.Default()}
}

func (s *Settings) normalize() {
	s.Translation = s.Translation.WithDefaults()
	if strings.TrimSpace(s.Shortcut) == "" {
		s.Shortcut = DefaultShortcut
	}
	if s.HistoryLimit != 100 && s.HistoryLimit != 500 && s.HistoryLimit != 1000 {
		s.HistoryLimit = 500
	}
}

// Load reads preferences, using defaults for a missing file or unsupported values.
func Load(path string) (Settings, error) {
	settings := Default()
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

// Save normalizes preferences and atomically replaces the settings file.
func Save(path string, settings Settings) error {
	settings.normalize()
	return atomicfile.WriteJSON(path, settings)
}
