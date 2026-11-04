package app

import (
	"errors"
	"fmt"
	"runtime"
	"strings"

	"vitals/internal/config"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

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
					a.message = err.Error()
					return
				}
				a.shortcutDraft = shortcut
				a.setRecording(false)
				a.message = "点击「保存快捷键」使新组合生效"
			})
		}
	}
}

func (a *application) applyShortcut() {
	if a.native == nil {
		return
	}
	shortcut, err := normalizeShortcut(a.shortcutDraft)
	if err != nil {
		a.message = err.Error()
		return
	}
	if shortcut == a.shortcutActive {
		a.setRecording(false)
		return
	}
	// Register the new binding before releasing the old one. A conflict leaves
	// the working shortcut intact.
	if err := mygo.GlobalShortcut.Register(shortcut, a.toggleWindow); err != nil {
		a.message = "此快捷键无法注册，请换一个组合"
		return
	}
	if a.shortcutActive != "" {
		mygo.GlobalShortcut.Unregister(a.shortcutActive)
	}
	a.shortcutActive, a.settings.Shortcut, a.shortcutDraft = shortcut, shortcut, shortcut
	a.recording = false
	a.message = "快捷键已更新"
	a.persistSettings()
}

func (a *application) setRecording(recording bool) {
	a.recording = recording
	if a.native == nil || a.shortcutActive == "" {
		return
	}
	if recording && !a.shortcutSuspended {
		mygo.GlobalShortcut.Unregister(a.shortcutActive)
		a.shortcutSuspended = true
	} else if !recording && a.shortcutSuspended {
		a.shortcutSuspended = false
		if err := mygo.GlobalShortcut.Register(a.shortcutActive, a.toggleWindow); err != nil {
			a.message = "原快捷键无法恢复，请重新设置"
			a.shortcutActive = ""
		}
	}
}
