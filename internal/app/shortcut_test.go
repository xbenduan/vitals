package app

import "testing"

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
