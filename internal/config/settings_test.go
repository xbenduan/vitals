package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"vitals/internal/translation"
)

func TestSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	want := Settings{
		Shortcut: "Command+Option+Space", TranslationShortcut: "Control+Shift+T",
		HistoryLimit: 1000, Paused: true, OpenAtLogin: true,
	}
	want.Translation = translation.Config{
		User: "example-user", Token: "example-token", TargetLanguage: translation.TargetEnglish,
	}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Shortcut != want.Shortcut || got.TranslationShortcut != want.TranslationShortcut || got.HistoryLimit != 1000 || !got.Paused || got.OpenAtLogin || got.Translation != want.Translation {
		t.Fatal("preferences did not survive a round trip")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows uses the user's directory ACL rather than POSIX permission bits.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("credentials require an owner-only settings file: %v", err)
	}
}

func TestExistingPreferencesGainTranslationDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"shortcut":"Command+Option+K","historyLimit":100,"paused":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.Shortcut != "Command+Option+K" || got.TranslationShortcut != DefaultTranslationShortcut || got.HistoryLimit != 100 || !got.Paused || got.Translation != translation.Default() {
		t.Fatalf("migration = %+v, %v", got, err)
	}
}

func TestOldTranslationCredentialsAreNotReused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{
		"shortcut":"Command+Option+K","historyLimit":100,"paused":true,
		"translation":{"provider":"OpenAI compatible","baseURL":"http://localhost:1234/v1",
		"model":"local","apiKey":"old-secret","prompt":"custom","thinking":true}
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.Translation != translation.Default() || got.Shortcut != "Command+Option+K" || got.HistoryLimit != 100 || !got.Paused {
		t.Fatalf("translation migration failed: %v", err)
	}
	if err := Save(path, got); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"provider", "baseURL", "model", "apiKey", "old-secret", "prompt", "thinking"} {
		if strings.Contains(string(data), key) {
			t.Errorf("obsolete translation field %q survived saving", key)
		}
	}
}

func TestMissingSettingsHaveDefaultShortcut(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil || got.Shortcut != DefaultShortcut || got.TranslationShortcut != DefaultTranslationShortcut || got.HistoryLimit != 500 || got.Paused {
		t.Fatalf("defaults = %+v, %v", got, err)
	}
}

func TestBlankTranslationShortcutGainsDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"shortcut":"Control+Alt+K","translationShortcut":" "}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.Shortcut != "Control+Alt+K" || got.TranslationShortcut != DefaultTranslationShortcut {
		t.Fatalf("blank shortcut migration = %+v, %v", got, err)
	}
}
