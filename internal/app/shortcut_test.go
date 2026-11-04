package app

import "testing"

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
