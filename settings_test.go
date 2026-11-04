package main

import (
	"path/filepath"
	"testing"
)

func TestSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	want := appSettings{Shortcut: "Command+Option+Space", HistoryLimit: 1000, Paused: true, OpenAtLogin: true}
	if err := saveSettings(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Shortcut != want.Shortcut || got.HistoryLimit != 1000 || !got.Paused || got.OpenAtLogin {
		t.Fatalf("unexpected round trip: %+v", got)
	}
}

func TestMissingSettingsHaveDefaultShortcut(t *testing.T) {
	got, err := loadSettings(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || got.Shortcut != defaultShortcut || got.HistoryLimit != 500 || got.Paused {
		t.Fatalf("defaults = %+v, %v", got, err)
	}
}

func TestShortcutValidation(t *testing.T) {
	for input, want := range map[string]string{
		"cmd+shift+v":    "Command+Shift+V",
		"alt+ctrl+space": "Control+Option+Space",
		"Command+F12":    "Command+F12",
		" Command + , ":  "Command+,",
	} {
		got, err := normalizeShortcut(input)
		if err != nil || got != want {
			t.Fatalf("%q => %q, %v; want %q", input, got, err, want)
		}
	}
	for _, invalid := range []string{"V", "Shift+V", "Command+", "Whatever+V", "Command+NoSuchKey"} {
		if _, err := normalizeShortcut(invalid); err == nil {
			t.Fatalf("accepted invalid shortcut %q", invalid)
		}
	}
}
