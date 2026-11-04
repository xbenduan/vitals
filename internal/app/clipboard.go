package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"vitals/internal/clipboard"
	"vitals/internal/platform/macos"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
)

func (a *application) monitorLoop() {
	ticker := time.NewTicker(350 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-a.done:
			return
		case <-ticker.C:
			a.win.Update(a.pollClipboard)
		}
	}
}

func (a *application) pollClipboard() {
	if a.quitting {
		return
	}
	front := a.native.Frontmost()
	if !a.win.IsVisible() && !a.pasting && front.PID != 0 {
		a.target = front
	}
	count := a.native.ChangeCount()
	if a.settings.Paused || a.storageFailed || a.loading {
		a.lastChange = count
		return
	}
	if count == a.lastChange || len(a.jobs) == cap(a.jobs) {
		return
	}
	capture := clipboard.Capture{Source: front.Name, At: time.Now()}
	formats := mygo.Clipboard.Formats()
	// Files take precedence over an accompanying text URL or image preview.
	if slices.Contains(formats, transfer.FileList) || slices.Contains(formats, transfer.URIList) {
		files, err := mygo.Clipboard.ReadFiles()
		if err == nil && len(files) > 0 {
			capture.Kind, capture.Files = clipboard.KindFiles, files
		}
	}
	if capture.Kind == "" && slices.Contains(formats, transfer.PNG) {
		if data := mygo.Clipboard.ReadImage(); len(data) > 0 {
			capture.Kind, capture.PNG = clipboard.KindImage, data
		}
	}
	if capture.Kind == "" && slices.Contains(formats, transfer.Text) {
		if text := mygo.Clipboard.ReadText(); text != "" {
			capture.Kind, capture.Text = clipboard.KindText, text
		}
	}
	// A copy arriving during the read is retried in the next poll.
	if a.native.ChangeCount() != count {
		return
	}
	a.lastChange = count
	if capture.Kind == "" {
		return
	}
	limit := a.settings.HistoryLimit
	a.submit(func(store *clipboard.Store) {
		if store == nil {
			return
		}
		err := store.Add(capture, limit)
		entries := store.Snapshot()
		a.win.Update(func() {
			if err != nil {
				a.message = err.Error()
				return
			}
			a.setEntries(entries)
		})
	})
}

func (a *application) cancelPending() {
	a.actionSequence++
	a.pasting = false
}

func (a *application) clickEntry(entry clipboard.Entry, index int) {
	a.selected = index
	a.pasteEntry(entry)
}

func (a *application) openEntry(entry clipboard.Entry) {
	a.cancelPending()
	a.cancelTranslation()
	a.translationMode = false
	a.translationError = ""
	a.detail = &entry
	a.editDraft = entry.Text
	a.message = ""
	if entry.Kind == clipboard.KindText {
		a.editorHandle.Focus()
	}
}

func (a *application) saveEdit(paste bool) {
	if a.detail == nil || a.detail.Kind != clipboard.KindText || a.saving || a.translating || a.translationError != "" {
		return
	}
	entry, draft, limit := *a.detail, a.editDraft, a.settings.HistoryLimit
	translated := a.translationMode
	a.saving = true
	a.mutate(func(store *clipboard.Store) error {
		return saveText(store, entry, draft, limit, translated)
	}, func() {
		a.saving = false
		a.detail = nil
		a.translationMode = false
		a.historyHandle.Focus()
		if paste {
			entry.Text = draft
			a.pasteEntry(entry)
		}
	})
}

func saveText(store *clipboard.Store, entry clipboard.Entry, draft string, limit int, translated bool) error {
	if translated {
		return store.Add(clipboard.Capture{Kind: clipboard.KindText, Text: draft, Source: "翻译"}, limit)
	}
	return store.Edit(entry.ID, draft, limit)
}

func (a *application) pasteEntry(entry clipboard.Entry) {
	a.cancelPending()
	if a.win == nil {
		return
	}
	a.pasting = true
	seq, target := a.actionSequence, a.target
	if !a.submit(func(store *clipboard.Store) {
		var data transfer.Data
		var err error
		switch entry.Kind {
		case clipboard.KindText:
			data = transfer.TextData(entry.Text)
		case clipboard.KindImage:
			if store == nil {
				err = errors.New("图片存储不可用")
			} else {
				var pngData []byte
				pngData, err = os.ReadFile(store.ImagePath(entry))
				if err == nil {
					data = transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, pngData)))
				}
			}
		case clipboard.KindFiles:
			for _, path := range entry.Files {
				if _, statErr := os.Stat(path); statErr != nil {
					err = fmt.Errorf("文件已移动或不可访问：%s", filepath.Base(path))
					break
				}
			}
			if err == nil {
				data, err = transfer.FileData(entry.Files...)
			}
		}
		a.win.Update(func() {
			if seq != a.actionSequence {
				return
			}
			if err == nil {
				err = mygo.Clipboard.Write(data)
			}
			if err != nil {
				a.pasting = false
				a.message = err.Error()
				return
			}
			a.lastChange = a.native.ChangeCount()
			a.trusted = a.native.Trusted()
			if !a.trusted {
				a.pasting = false
				a.openSettings()
				a.message = "内容已复制。开启「辅助功能」后即可自动粘贴"
				return
			}
			if target.PID == 0 {
				a.pasting = false
				a.message = "内容已复制。请先切换到目标应用，再打开剪贴板"
				return
			}
			a.win.Hide()
			if !a.native.Activate(target) {
				a.pasteFailed("目标应用已退出或无法激活，内容已复制")
				return
			}
			a.awaitPaste(target, seq, time.Now().Add(2*time.Second))
		})
	}) {
		a.pasting = false
	}
}

func (a *application) awaitPaste(target macos.Target, seq uint64, deadline time.Time) {
	time.AfterFunc(45*time.Millisecond, func() {
		a.win.Update(func() {
			if a.quitting || seq != a.actionSequence {
				return
			}
			if a.native.Frontmost().PID == target.PID && a.native.ModifiersReleased() {
				err := a.native.Paste(target)
				a.pasting = false
				if err != nil {
					a.pasteFailed(err.Error())
				}
				return
			}
			if time.Now().After(deadline) {
				a.pasteFailed("未能恢复目标应用焦点，内容已复制，可使用 ⌘V 粘贴")
				return
			}
			a.awaitPaste(target, seq, deadline)
		})
	})
}

func (a *application) pasteFailed(message string) {
	a.pasting = false
	a.message = message
	a.showWindow()
}
