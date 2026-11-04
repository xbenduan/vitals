package app

import (
	"bytes"
	"image/png"
	"path/filepath"
	"testing"

	"vitals/internal/clipboard"

	"github.com/egoist/mygo"
)

func TestDevelopmentUserDataPath(t *testing.T) {
	appData := "/Users/example/Library/Application Support"
	if got := developmentUserDataPath("/tmp/Vitals Dev.app/Contents/MacOS/Vitals Dev", appData); got != filepath.Join(appData, "Vitals Dev") {
		t.Fatalf("development data path = %q", got)
	}
	if got := developmentUserDataPath("/Applications/Vitals.app/Contents/MacOS/Vitals", appData); got != "" {
		t.Fatalf("production data path = %q, want empty", got)
	}
}

func TestMenuBarIcon(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(menuBarIcon()))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 36 || img.Bounds().Dy() != 36 {
		t.Fatalf("unexpected tray size: %v", img.Bounds())
	}
}

func TestWindowCloseAllowsApplicationQuit(t *testing.T) {
	app := newApplication()
	ordinaryClose := &mygo.CloseEvent{}
	app.handleWindowClose(ordinaryClose)
	if !ordinaryClose.DefaultPrevented() {
		t.Fatal("ordinary close must hide the window")
	}
	app.quitting = true
	quitClose := &mygo.CloseEvent{}
	app.handleWindowClose(quitClose)
	if quitClose.DefaultPrevented() {
		t.Fatal("quit must be allowed to close the window")
	}
}

func TestOpeningDetailsCancelsPendingPaste(t *testing.T) {
	app := newApplication()
	before := app.actionSequence
	app.clickEntry(clipboard.Entry{ID: "first", Kind: clipboard.KindText, Text: "hello"}, 0)
	clickSequence := app.actionSequence
	app.openEntry(clipboard.Entry{ID: "first", Kind: clipboard.KindText, Text: "hello"})
	if before >= clickSequence || app.actionSequence <= clickSequence || app.detail == nil || app.editDraft != "hello" {
		t.Fatal("double click must invalidate the first click and open the editor")
	}
	app.hideWindow()
	if app.actionSequence <= clickSequence+1 {
		t.Fatal("hiding the panel must invalidate pending paste work")
	}
}
