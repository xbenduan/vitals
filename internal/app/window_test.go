package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"clipnest/internal/clipboard"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestReopenReturnsToFirstHistoryEntry(t *testing.T) {
	for _, page := range []string{"scrolled list", "settings", "editor", "search", "favorites", "delete confirmation"} {
		t.Run(page, func(t *testing.T) {
			app := newApplication()
			app.loading = false
			for i := range 120 {
				app.entries = append(app.entries, clipboard.Entry{
					ID: fmt.Sprint(i), Kind: clipboard.KindText, Text: fmt.Sprintf("History entry %03d", i),
					CopiedAt: time.Now().Add(-time.Duration(i) * time.Minute), Favorite: i == 99,
				})
			}
			var historyFocused bool
			tester := ui.NewTester(func(c *ui.Context) {
				app.view(c)
				historyFocused = app.historyHandle.Focused(c)
			}, panelWidth, panelHeight)

			app.selected = 99
			app.list.ScrollTo(99, ui.Start)
			tester.Frame()
			if first, _ := app.list.Visible(); first < 90 {
				t.Fatalf("test did not scroll deeply enough: first=%d", first)
			}
			switch page {
			case "settings":
				app.openSettings()
				tester.Frame()
				_ = tester.Click("全局快捷键")
			case "editor":
				app.openEntry(app.entries[99])
			case "search":
				app.query = "099"
				app.searchHandle.Focus()
			case "favorites":
				app.tab = 3
			case "delete confirmation":
				app.confirmDelete, app.deleteID = true, "99"
			}
			tester.Frame()
			app.hideWindow()
			tester.SetFocused(false)
			app.resetForShow()
			tester.SetFocused(true)
			tester.Frame()
			tester.Frame()

			if app.settingsOpen || app.detail != nil || app.confirmDelete || app.confirmClear ||
				app.query != "" || app.tab != 0 || app.selected != 0 {
				t.Fatal("reopen must return to the first entry in all history")
			}
			if first, _ := app.list.Visible(); first != 0 {
				t.Fatalf("reopen kept the old scroll position: first=%d", first)
			}
			if !historyFocused {
				t.Fatal("the history list must receive keyboard focus")
			}
			tester.Key(0, ui.KeyDown)
			if entry, ok := app.selectedEntry(); !ok || entry.ID != "1" {
				t.Fatal("keyboard navigation must start from the first entry")
			}
			tester.Key(primaryModifier(), ui.KeyE)
			if app.detail == nil || app.detail.ID != "1" {
				t.Fatal("editing must use the newly selected entry")
			}
			app.resetForShow()
			tester.Frame()
			tester.Key(primaryModifier(), ui.KeyE)
			if app.detail == nil || app.detail.ID != "0" {
				t.Fatal("every reopen must reset the selection, not only the first")
			}
		})
	}
}

func TestReopenFocusWaitsForHistoryLoading(t *testing.T) {
	app := newApplication()
	var historyFocused bool
	tester := ui.NewTester(func(c *ui.Context) {
		app.view(c)
		historyFocused = app.historyHandle.Focused(c)
	}, panelWidth, panelHeight)
	app.resetForShow()
	tester.Frame()
	app.loading = false
	tester.Frame()
	if _, ok := app.selectedEntry(); ok {
		t.Fatal("empty history must not select a nonexistent entry")
	}
	app.entries = []clipboard.Entry{{ID: "first", Kind: clipboard.KindText, Text: "First loaded entry"}}
	tester.Frame()
	tester.Frame()
	if !historyFocused || app.selected != 0 {
		t.Fatal("the first loaded record must receive the pending history focus")
	}
}

func TestReopenRejectsOldEditorCallbacks(t *testing.T) {
	app := previewApplication()
	app.win = &mygo.Window{}
	app.openEntry(app.entries[0])
	oldDetail := app.detail
	app.translationMode, app.translating = true, true
	ctx, cancel := context.WithCancel(context.Background())
	app.translationCancel = cancel
	translationSequence := app.translationSequence
	app.checkingTranslation = true
	checkCtx, checkCancel := context.WithCancel(context.Background())
	app.translationCheckCancel = checkCancel
	checkSequence, cfg := app.translationCheckSequence, app.translationDraft
	app.saving = true

	app.resetForShow()
	app.finishTranslation(translationSequence, oldDetail.ID, "Late translation", nil)
	app.finishTranslationCheck(checkSequence, cfg, nil)
	app.openEntry(app.entries[1])
	newDetail := app.detail
	app.finishEdit(oldDetail, "Saved text", true)

	if ctx.Err() != context.Canceled || checkCtx.Err() != context.Canceled ||
		app.translating || app.checkingTranslation || app.saving {
		t.Fatal("old translation and save work must settle without reviving the previous page")
	}
	if app.detail != newDetail || app.editDraft != app.entries[1].Text || len(app.jobs) != 0 || app.pasting {
		t.Fatal("an old save callback must not close the new editor or paste")
	}
}
