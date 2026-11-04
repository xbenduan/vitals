package app

import (
	"bytes"
	"context"
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
		t.Fatal("opening details must invalidate the pending paste and open the editor")
	}
	app.hideWindow()
	if app.actionSequence <= clickSequence+1 {
		t.Fatal("hiding the panel must invalidate pending paste work")
	}
}

func TestTranslationCancellationRejectsStaleResult(t *testing.T) {
	app := newApplication()
	entry := clipboard.Entry{ID: "one", Kind: clipboard.KindText, Text: "原文"}
	app.openEntry(entry)
	app.translationMode, app.translating = true, true
	ctx, cancel := context.WithCancel(context.Background())
	app.translationCancel = cancel
	sequence := app.translationSequence
	app.hideWindow()
	if ctx.Err() != context.Canceled || app.translating {
		t.Fatal("hiding must cancel the translation request")
	}
	app.finishTranslation(sequence, "one", "stale", "", nil)
	if app.editDraft != "原文" {
		t.Fatal("a canceled translation must not update its editor")
	}
	app.openEntry(entry)
	app.finishTranslation(sequence, "one", "stale", "", nil)
	if app.translationMode || app.editDraft != "原文" {
		t.Fatal("a stale request must not replace a new editor for the same entry")
	}
}

func TestSavingTranslationKeepsSource(t *testing.T) {
	store, err := clipboard.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Add(clipboard.Capture{Kind: clipboard.KindText, Text: "原文", Source: "TRAE"}, 500); err != nil {
		t.Fatal(err)
	}
	source := store.Snapshot()[0]
	if err := saveText(store, source, "Edited translation", 500, true); err != nil {
		t.Fatal(err)
	}
	entries := store.Snapshot()
	if len(entries) != 2 || entries[0].Text != "Edited translation" || entries[0].Source != "翻译" ||
		entries[1].ID != source.ID || entries[1].Text != source.Text || entries[1].Source != source.Source {
		t.Fatalf("translation must append a new entry: %+v", entries)
	}
}
