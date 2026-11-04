package app

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"clipnest/internal/clipboard"
	"clipnest/internal/config"
)

func TestMigrateUserDataPreservesHistorySettingsAndImages(t *testing.T) {
	root := t.TempDir()
	legacy, current := filepath.Join(root, "Vitals"), filepath.Join(root, appName)
	fixtures := map[string]string{
		"clipboard-settings.json":    `{"shortcut":"Control+Alt+K","translation":{"user":"test-user","token":"test-token"}}`,
		"Clipboard/history.json":     `[{"id":"favorite","kind":"text","text":"保留的收藏","favorite":true},{"id":"image","kind":"image","image":"saved.png"}]`,
		"Clipboard/images/saved.png": "saved image bytes",
	}
	for name, content := range fixtures {
		path := filepath.Join(legacy, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(legacy, "settings.json"), []byte("old system monitor"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := migrateUserData(legacy, current); err != nil {
		t.Fatal(err)
	}
	for name, content := range fixtures {
		for _, dir := range []string{legacy, current} {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil || string(data) != content {
				t.Fatalf("%s/%s: data=%q, err=%v", dir, name, data, err)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(current, "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("system-monitor settings must not migrate: %v", err)
	}
	settings, err := config.Load(filepath.Join(current, "clipboard-settings.json"))
	if err != nil || settings.Shortcut != "Control+Alt+K" || settings.Translation.User != "test-user" || settings.Translation.Token != "test-token" {
		t.Fatalf("migrated settings cannot be loaded: %v", err)
	}
	store, err := clipboard.Open(filepath.Join(current, "Clipboard"))
	if err != nil {
		t.Fatal(err)
	}
	entries := store.Snapshot()
	if len(entries) != 2 || !entries[0].Favorite || entries[0].Text != "保留的收藏" || entries[1].Image != "saved.png" {
		t.Fatalf("migrated history = %+v", entries)
	}
	if runtime.GOOS != "windows" {
		for name := range fixtures {
			info, err := os.Stat(filepath.Join(current, name))
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("migrated %s must remain private: %v", name, err)
			}
		}
	}
}

func TestMigrateUserDataKeepsExistingProfile(t *testing.T) {
	for _, content := range []string{"", "new settings"} {
		t.Run(content, func(t *testing.T) {
			root := t.TempDir()
			legacy, current := filepath.Join(root, "Vitals"), filepath.Join(root, appName)
			if err := os.MkdirAll(current, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(legacy, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(legacy, "clipboard-settings.json"), []byte("old settings"), 0o600); err != nil {
				t.Fatal(err)
			}
			if content != "" {
				if err := os.WriteFile(filepath.Join(current, "clipboard-settings.json"), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := migrateUserData(legacy, current); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(current, "clipboard-settings.json"))
			if content == "" {
				if !os.IsNotExist(err) {
					t.Fatal("an existing empty profile must not import old settings")
				}
			} else if err != nil || string(data) != content {
				t.Fatalf("existing settings changed: %q, %v", data, err)
			}
		})
	}
}

func TestMigrateUserDataFailureCanRetry(t *testing.T) {
	root := t.TempDir()
	legacy, current := filepath.Join(root, "Vitals"), filepath.Join(root, appName)
	if err := os.MkdirAll(filepath.Join(legacy, "Clipboard"), 0o700); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(legacy, "clipboard-settings.json")
	if err := os.WriteFile(settingsPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	historyPath := filepath.Join(legacy, "Clipboard", "history.json")
	if err := os.Symlink(settingsPath, historyPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := migrateUserData(legacy, current); err == nil {
		t.Fatal("unsupported source must fail the migration")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != "Vitals" {
		t.Fatalf("failed migration must leave only the legacy directory: %v, %v", entries, err)
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil || string(data) != "{}" {
		t.Fatal("failed migration changed the original settings")
	}
	if err := os.Remove(historyPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(historyPath, []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := migrateUserData(legacy, current); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if _, err := clipboard.Open(filepath.Join(current, "Clipboard")); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateUserDataWithoutLegacyProfile(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, appName)
	if err := migrateUserData(filepath.Join(root, "Vitals"), current); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(current); !os.IsNotExist(err) {
		t.Fatalf("absent legacy profile must leave setup to the caller: %v", err)
	}
}
