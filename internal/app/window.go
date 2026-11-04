package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *application) handleWindowClose(event *mygo.CloseEvent) {
	if !a.quitting {
		event.PreventDefault()
		a.hideWindow()
	}
}

func (a *application) hideWindow() {
	a.cancelPending()
	a.cancelTranslation()
	a.cancelTranslationCheck()
	a.setRecording(false)
	if a.win != nil {
		a.win.Hide()
	}
}

func (a *application) toggleWindow() {
	if a.win.IsVisible() {
		a.hideWindow()
	} else {
		a.showWindow()
	}
}

func (a *application) showWindow() {
	a.resetForShow()
	if target := a.native.Frontmost(); target.PID != 0 {
		a.target = target
	}
	a.presentWindow()
}

// presentWindow raises the prepared page without resetting translation state
// or replacing the source app captured before selection acquisition.
func (a *application) presentWindow() {
	a.trusted = a.native.Trusted()
	a.positionWindow()
	a.win.Invalidate()
	a.win.Show()
	a.win.Focus()
}

func (a *application) resetForShow() {
	a.cancelPending()
	a.cancelTranslation()
	a.cancelTranslationCheck()
	a.setRecording(false)
	a.settingsOpen = false
	a.detail = nil
	a.editDraft = ""
	a.translationMode = false
	a.translationSource = ""
	a.translationError = ""
	a.previewID, a.previewImage, a.previewError = "", nil, ""
	a.confirmClear, a.confirmDelete, a.deleteID = false, false, ""
	a.query, a.tab, a.selected = "", 0, 0
	a.list.ScrollTo(0, ui.Start)
	a.searchHandle.CancelFocus()
	a.editorHandle.CancelFocus()
	a.sourceHandle.CancelFocus()
	// A hidden or not-yet-built list keeps this request until it appears.
	a.historyHandle.Focus()
}
