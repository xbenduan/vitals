package config

import (
	"os"
	"path/filepath"
	"testing"

	"vitals/internal/translation"
)

func TestSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	want := Settings{Shortcut: "Command+Option+Space", HistoryLimit: 1000, Paused: true, OpenAtLogin: true}
	want.Translation = translation.Config{
		Provider: translation.OpenAI, BaseURL: "http://localhost:1234/v1",
		Model: "local-model", APIKey: "example-key", Prompt: "Translate code comments.",
	}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Shortcut != want.Shortcut || got.HistoryLimit != 1000 || !got.Paused || got.OpenAtLogin || got.Translation != want.Translation {
		t.Fatalf("unexpected round trip: %+v", got)
	}
}

func TestExistingPreferencesGainLocalTranslationDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"shortcut":"Command+Option+K","historyLimit":100,"paused":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.Shortcut != "Command+Option+K" || got.HistoryLimit != 100 || !got.Paused || got.Translation != translation.Default() {
		t.Fatalf("migration = %+v, %v", got, err)
	}
}

func TestMissingSettingsHaveDefaultShortcut(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || got.Shortcut != DefaultShortcut || got.HistoryLimit != 500 || got.Paused {
		t.Fatalf("defaults = %+v, %v", got, err)
	}
}
