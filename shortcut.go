package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

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
	if mods&(ui.Cmd|ui.Ctrl|ui.Alt) == 0 {
		return "", errors.New("请至少包含 Command、Control 或 Option")
	}
	var parts []string
	for _, modifier := range []struct {
		mask ui.Modifiers
		name string
	}{{ui.Cmd, "Command"}, {ui.Ctrl, "Control"}, {ui.Alt, "Option"}, {ui.Shift, "Shift"}} {
		if mods&modifier.mask != 0 {
			parts = append(parts, modifier.name)
		}
	}
	return strings.Join(append(parts, key), "+"), nil
}

func normalizeShortcut(value string) (string, error) {
	parts := strings.Split(strings.TrimSpace(value), "+")
	if len(parts) < 2 {
		return "", errors.New("请设置一个带修饰键的快捷键，例如 Command+Shift+V")
	}
	var mods ui.Modifiers
	for _, part := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "command", "cmd", "super":
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
	return strings.NewReplacer(
		"Command", "⌘", "Control", "⌃", "Option", "⌥", "Shift", "⇧",
		"+", " ", "Space", "空格", "Enter", "↵",
	).Replace(value)
}

func lowerExtension(path string) string { return strings.ToLower(filepath.Ext(path)) }

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
