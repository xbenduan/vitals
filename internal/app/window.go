package app

import "github.com/egoist/mygo"

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
	a.cancelPending()
	if target := a.native.Frontmost(); target.PID != 0 {
		a.target = target
	}
	a.trusted = a.native.Trusted()
	a.positionWindow()
	if a.detail == nil && !a.settingsOpen {
		a.historyHandle.Focus()
	}
	a.win.Show()
	a.win.Focus()
}
