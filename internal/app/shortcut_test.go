package app

import (
	"errors"
	"strings"
	"testing"

	"clipnest/internal/config"
	"clipnest/internal/translation"

	"github.com/egoist/mygo/ui"
)

type testShortcutRegistry struct {
	bindings map[string]func()
	occupied map[string]bool
}

func newTestShortcutRegistry() *testShortcutRegistry {
	return &testShortcutRegistry{bindings: make(map[string]func()), occupied: make(map[string]bool)}
}

func (r *testShortcutRegistry) Register(shortcut string, callback func()) error {
	if r.occupied[shortcut] || r.bindings[shortcut] != nil {
		return errors.New("shortcut occupied")
	}
	r.bindings[shortcut] = callback
	return nil
}

func (r *testShortcutRegistry) Unregister(shortcut string) {
	delete(r.bindings, shortcut)
}

func TestShortcutValidation(t *testing.T) {
	command, alt := modifierNames()
	for input, want := range map[string]string{
		"cmd+shift+v":    command + "+Shift+V",
		"alt+ctrl+space": "Control+" + alt + "+Space",
		"Command+F12":    command + "+F12",
		" Command + , ":  command + "+,",
		"win+alt+k":      command + "+" + alt + "+K",
		"Meta+F2":        command + "+F2",
		"ctrl+shift+v":   "Control+Shift+V",
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

func TestShortcutConflictKeepsBindingsAndTranslationConfig(t *testing.T) {
	app := previewApplication()
	registry := app.shortcuts.(*testShortcutRegistry)
	originalConfig := app.settings.Translation
	app.translationDraft = translation.Config{User: "new-user", Token: "new-token", TargetLanguage: translation.TargetEnglish}
	for _, draft := range []string{strings.ToLower(app.settings.Shortcut), "Control+Shift+T", "Shift+T"} {
		app.translationShortcutDraft = draft
		registry.occupied["Control+Shift+T"] = true
		app.saveTranslationSettings()
		if app.settings.Translation != originalConfig || app.settings.TranslationShortcut != config.DefaultTranslationShortcut {
			t.Fatal("a shortcut conflict or invalid input must not partially save the translation configuration")
		}
		if len(registry.bindings) != 2 || registry.bindings[config.DefaultTranslationShortcut] == nil || registry.bindings[config.DefaultShortcut] == nil {
			t.Fatal("both original shortcuts must remain registered after a failed save")
		}
	}
	app.shortcutDraft = app.settings.TranslationShortcut
	app.applyShortcut()
	if app.settings.Shortcut != config.DefaultShortcut || !strings.Contains(app.shortcutStatus, "划词翻译") {
		t.Fatal("history shortcut must reject the saved translation combination")
	}
	delete(registry.occupied, "Control+Shift+T")
	app.translationShortcutDraft = "ctrl+shift+t"
	app.saveTranslationSettings()
	if app.settings.TranslationShortcut != "Control+Shift+T" {
		t.Fatalf("saved shortcut = %s, status = %s", app.settings.TranslationShortcut, app.translationStatus)
	}
	if app.settings.Translation != app.translationDraft || registry.bindings[config.DefaultTranslationShortcut] != nil || len(registry.bindings) != 2 {
		t.Fatal("successful save must replace only the translation shortcut and commit the configuration")
	}
}

func TestStartupKeepsExistingHistoryShortcutOnCollision(t *testing.T) {
	app := newApplication()
	app.shortcuts = newTestShortcutRegistry()
	app.settings.Shortcut = strings.ToLower(config.DefaultTranslationShortcut)
	app.registerShortcuts()
	if app.shortcutActive != config.DefaultTranslationShortcut || app.settings.Shortcut != strings.ToLower(config.DefaultTranslationShortcut) {
		t.Fatal("migration must not overwrite a custom history shortcut")
	}
	if app.translationShortcutActive != "" || app.translationShortcutError == "" {
		t.Fatal("the translation collision must be reported")
	}
	app.translationShortcutDraft = "Control+Shift+T"
	app.saveTranslationSettings()
	if app.translationShortcutActive != "Control+Shift+T" || app.translationShortcutError != "" {
		t.Fatal("saving a different translation shortcut must recover from a startup collision")
	}
}

func TestTranslationShortcutRecordingAndDefaultAreDrafts(t *testing.T) {
	app := previewApplication()
	registry := app.shortcuts.(*testShortcutRegistry)
	app.openSettings()
	app.settingsTab = 1
	tester := ui.NewTester(app.view, panelWidth, panelHeight)
	if err := tester.Click("录制"); err != nil {
		t.Fatal(err)
	}
	if len(registry.bindings) != 0 {
		t.Fatal("recording must suspend both global shortcuts")
	}
	tester.Key(ui.Ctrl|ui.Shift, ui.KeyT)
	if app.recording || app.translationShortcutDraft != "Control+Shift+T" || app.shortcutDraft != config.DefaultShortcut || len(registry.bindings) != 2 {
		t.Fatal("recording must update only the translation draft and restore both bindings")
	}
	if app.settings.TranslationShortcut != config.DefaultTranslationShortcut {
		t.Fatal("recording must not save the shortcut")
	}
	if err := tester.Click("保存翻译配置"); err != nil {
		t.Fatal(err)
	}
	if app.settings.TranslationShortcut != "Control+Shift+T" || registry.bindings["Control+Shift+T"] == nil {
		t.Fatalf("shortcut must save with empty API credentials: %s", app.translationStatus)
	}
	_ = tester.Click("恢复默认")
	if app.translationShortcutDraft != config.DefaultTranslationShortcut || app.settings.TranslationShortcut != "Control+Shift+T" || registry.bindings["Control+Shift+T"] == nil {
		t.Fatal("restoring defaults must change only the draft")
	}
	_ = tester.Click("返回历史")
	_ = tester.Click("设置")
	_ = tester.Click("翻译")
	if app.translationShortcutDraft != "Control+Shift+T" {
		t.Fatal("reopening settings must discard the unsaved shortcut draft")
	}
	_ = tester.Click("恢复默认")
	_ = tester.Click("保存翻译配置")
	if app.settings.TranslationShortcut != config.DefaultTranslationShortcut || registry.bindings["Control+Shift+T"] != nil {
		t.Fatal("saving the restored default must release the custom shortcut")
	}
}

func TestLeavingShortcutRecordingRestoresBothBindings(t *testing.T) {
	for _, exit := range []string{"escape", "tab", "back", "hide"} {
		t.Run(exit, func(t *testing.T) {
			app := previewApplication()
			app.openSettings()
			app.settingsTab = 1
			tester := ui.NewTester(app.view, panelWidth, panelHeight)
			_ = tester.Click("录制")
			switch exit {
			case "escape":
				tester.Key(0, ui.KeyEscape)
			case "tab":
				_ = tester.Click("通用")
			case "back":
				_ = tester.Click("返回历史")
			case "hide":
				app.hideWindow()
			}
			tester.Frame()
			tester.Frame()
			registry := app.shortcuts.(*testShortcutRegistry)
			if app.recording || app.shortcutSuspended || app.translationShortcutSuspended || len(registry.bindings) != 2 {
				t.Fatal("leaving recording must restore both bindings")
			}
		})
	}
}

func TestShortcutRestorationFailureCanBeRetried(t *testing.T) {
	app := previewApplication()
	registry := app.shortcuts.(*testShortcutRegistry)
	app.toggleRecording(true)
	registry.occupied[config.DefaultTranslationShortcut] = true
	app.setRecording(false)
	if app.translationShortcutActive != "" || app.translationShortcutError == "" || registry.bindings[config.DefaultShortcut] == nil {
		t.Fatal("restoration failure must be visible and keep the other shortcut usable")
	}
	delete(registry.occupied, config.DefaultTranslationShortcut)
	app.saveTranslationSettings()
	if app.translationShortcutActive != config.DefaultTranslationShortcut || app.translationShortcutError != "" {
		t.Fatal("save must retry a previously failed registration")
	}
}
