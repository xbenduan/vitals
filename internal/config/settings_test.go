package config

import (
	"path/filepath"
	"testing"
)

func TestSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	want := Settings{Shortcut: "Command+Option+Space", HistoryLimit: 1000, Paused: true, OpenAtLogin: true}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Shortcut != want.Shortcut || got.HistoryLimit != 1000 || !got.Paused || got.OpenAtLogin {
		t.Fatalf("unexpected round trip: %+v", got)
	}
}

func TestMissingSettingsHaveDefaultShortcut(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || got.Shortcut != DefaultShortcut || got.HistoryLimit != 500 || got.Paused {
		t.Fatalf("defaults = %+v, %v", got, err)
	}
}
