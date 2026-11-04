package app

import (
	"errors"
	"fmt"
	"runtime"
	"strings"

	"vitals/internal/config"

	"github.com/egoist/mygo/ui"
)

type shortcutRegistry interface {
	Register(string, func()) error
	Unregister(string)
}

func primaryModifier() ui.Modifiers {
	if runtime.GOOS == "darwin" {
		return ui.Cmd
	}
	return ui.Ctrl
}

func primaryShortcutLabel(key string) string {
	if runtime.GOOS == "darwin" {
		return "⌘ " + key
	}
	return "Ctrl+" + key
}

func modifierNames() (command, alt string) {
	if runtime.GOOS == "darwin" {
		return "Command", "Option"
	}
	return "Win", "Alt"
}

func shortcutKeys() map[ui.Key]string {
	keys := map[ui.Key]string{
		ui.KeySpace: "Space", ui.KeyEnter: "Enter", ui.KeyTab: "Tab",
		ui.KeyBackspace: "Backspace", ui.KeyDelete: "Delete",
		ui.KeyUp: "Up", ui.KeyDown: "Down", ui.KeyLeft: "Left", ui.KeyRight: "Right",
		ui.KeyHome: "Home", ui.KeyEnd: "End", ui.KeyPageUp: "PageUp", ui.KeyPageDown: "PageDown",
		ui.KeyMinus: "-", ui.KeyEqual: "=", ui.KeyComma: ",", ui.KeyPeriod: ".",
		ui.KeySlash: "/", ui.KeySemicolon: ";", ui.KeyQuote: "'",
		ui.KeyBracketLeft: "[", ui.KeyBracketRight: "]", ui.KeyBackslash: "\\", ui.KeyBackquote: "`",
	}
	for key := ui.KeyA; key <= ui.KeyZ; key++ {
		keys[key] = string(rune('A' + key - ui.KeyA))
	}
	for key := ui.Key0; key <= ui.Key9; key++ {
		keys[key] = string(rune('0' + key - ui.Key0))
	}
	for key := ui.KeyF1; key <= ui.KeyF12; key++ {
		keys[key] = fmt.Sprintf("F%d", 1+key-ui.KeyF1)
	}
	return keys
}

func shortcutFromKey(mods ui.Modifiers, key string) (string, error) {
	command, alt := modifierNames()
	if mods&(ui.Cmd|ui.Ctrl|ui.Alt) == 0 {
		return "", fmt.Errorf("请至少包含 %s、Control 或 %s", command, alt)
	}
	var parts []string
	for _, modifier := range []struct {
		mask ui.Modifiers
		name string
	}{{ui.Cmd, command}, {ui.Ctrl, "Control"}, {ui.Alt, alt}, {ui.Shift, "Shift"}} {
		if mods&modifier.mask != 0 {
			parts = append(parts, modifier.name)
		}
	}
	return strings.Join(append(parts, key), "+"), nil
}

func normalizeShortcut(value string) (string, error) {
	parts := strings.Split(strings.TrimSpace(value), "+")
	if len(parts) < 2 {
		return "", errors.New("请设置一个带修饰键的快捷键，例如 " + config.DefaultShortcut)
	}
	var mods ui.Modifiers
	for _, part := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "command", "cmd", "super", "meta", "win":
			mods |= ui.Cmd
		case "control", "ctrl":
			mods |= ui.Ctrl
		case "option", "alt":
			mods |= ui.Alt
		case "shift":
			mods |= ui.Shift
		default:
			return "", fmt.Errorf("无法识别修饰键：%s", part)
		}
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	for _, key := range shortcutKeys() {
		if strings.EqualFold(last, key) {
			return shortcutFromKey(mods, key)
		}
	}
	return "", errors.New("无法识别按键，请点击「录制」直接按下快捷键")
}

func shortcutLabel(value string) string {
	if runtime.GOOS != "darwin" {
		return strings.NewReplacer(
			"Command", "Win", "Control", "Ctrl", "Option", "Alt",
			"Space", "空格",
		).Replace(value)
	}
	return strings.NewReplacer(
		"Command", "⌘", "Win", "⌘", "Control", "⌃", "Option", "⌥", "Alt", "⌥", "Shift", "⇧",
		"+", " ", "Space", "空格", "Enter", "↵",
	).Replace(value)
}

func (a *application) recordShortcut(c *ui.Context) {
	c.OnShortcut(0, ui.KeyEscape, func() { a.setRecording(false) })
	for mods := ui.Modifiers(0); mods < 16; mods++ {
		for key, name := range shortcutKeys() {
			c.OnShortcut(mods, key, func() {
				shortcut, err := shortcutFromKey(mods, name)
				if err != nil {
					a.recordingStatus(err.Error())
					return
				}
				if a.recordingTranslation {
					a.translationShortcutDraft = shortcut
				} else {
					a.shortcutDraft = shortcut
				}
				a.setRecording(false)
				a.recordingStatus("已录制，保存后生效")
			})
		}
	}
}

func (a *application) applyShortcut() {
	if a.recording {
		return
	}
	shortcut, err := a.replaceShortcut(a.shortcutDraft, false)
	if err != nil {
		a.shortcutStatus = err.Error()
		return
	}
	a.settings.Shortcut, a.shortcutDraft = shortcut, shortcut
	a.shortcutError = ""
	a.shortcutStatus = "快捷键已保存"
	a.persistSettings()
}

func (a *application) registerShortcuts() {
	// Preserve the existing history binding when an older configuration already
	// uses the new translation default. Surface the collision without resetting it.
	shortcut, err := normalizeShortcut(a.settings.Shortcut)
	if err == nil {
		err = a.shortcuts.Register(shortcut, a.toggleWindow)
	}
	if err != nil {
		a.shortcutError = "打开剪贴板快捷键注册失败，请更换组合"
	} else {
		a.shortcutActive = shortcut
	}
	if _, err := a.replaceShortcut(a.settings.TranslationShortcut, true); err != nil {
		a.translationShortcutError = "划词翻译快捷键未启用：" + err.Error()
	}
	if a.shortcutError != "" || a.translationShortcutError != "" {
		a.message = "部分快捷键未启用，请在设置中更换组合"
	}
}

func (a *application) replaceShortcut(draft string, forTranslation bool) (string, error) {
	shortcut, err := normalizeShortcut(draft)
	if err != nil {
		return "", err
	}
	active, callback := &a.shortcutActive, a.toggleWindow
	otherSaved, otherActive, otherName := a.settings.TranslationShortcut, a.translationShortcutActive, "划词翻译"
	if forTranslation {
		active, callback = &a.translationShortcutActive, a.translateSelection
		otherSaved, otherActive, otherName = a.settings.Shortcut, a.shortcutActive, "打开剪贴板"
	}
	otherSaved, _ = normalizeShortcut(otherSaved)
	if shortcut == otherSaved || shortcut == otherActive {
		return "", fmt.Errorf("此组合已用于%s，请换一个组合", otherName)
	}
	if a.shortcuts == nil {
		return "", errors.New("快捷键服务尚未就绪")
	}
	if shortcut == *active {
		return shortcut, nil
	}
	// Register first so conflicts never discard the currently working binding.
	if err := a.shortcuts.Register(shortcut, callback); err != nil {
		return "", errors.New("此快捷键无法注册，请检查占用或换一个组合")
	}
	if *active != "" {
		a.shortcuts.Unregister(*active)
	}
	*active = shortcut
	return shortcut, nil
}

func (a *application) recordingStatus(status string) {
	if a.recordingTranslation {
		a.translationStatus = status
	} else {
		a.shortcutStatus = status
	}
}

func (a *application) toggleRecording(forTranslation bool) {
	a.cancelPending()
	if a.recording {
		a.setRecording(false)
		return
	}
	a.recordingTranslation = forTranslation
	a.recordingStatus("")
	a.setRecording(true)
}

func (a *application) setRecording(recording bool) {
	a.recording = recording
	if a.shortcuts == nil {
		return
	}
	for _, binding := range []struct {
		active    *string
		suspended *bool
		errorText *string
		callback  func()
		name      string
	}{
		{&a.shortcutActive, &a.shortcutSuspended, &a.shortcutError, a.toggleWindow, "打开剪贴板"},
		{&a.translationShortcutActive, &a.translationShortcutSuspended, &a.translationShortcutError, a.translateSelection, "划词翻译"},
	} {
		if recording && *binding.active != "" && !*binding.suspended {
			a.shortcuts.Unregister(*binding.active)
			*binding.suspended = true
		} else if !recording && *binding.suspended {
			*binding.suspended = false
			if err := a.shortcuts.Register(*binding.active, binding.callback); err != nil {
				*binding.errorText = binding.name + "快捷键无法恢复，请重新保存或更换组合"
				*binding.active = ""
			} else {
				*binding.errorText = ""
			}
		}
	}
}
