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
	a.cancelModelDetection()
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
	anchor := a.tray.Bounds()
	if anchor.Width > 0 {
		area := mygo.Screen.DisplayNearestPoint(mygo.Point{X: anchor.X, Y: anchor.Y}).WorkArea
		bounds := a.win.Bounds()
		x := max(area.X, min(anchor.X+(anchor.Width-bounds.Width)/2, area.X+area.Width-bounds.Width))
		y := max(area.Y, min(anchor.Y+anchor.Height+6, area.Y+area.Height-bounds.Height))
		a.win.SetPosition(x, y)
	} else {
		a.win.Center()
	}
	if a.detail == nil && !a.settingsOpen {
		a.historyHandle.Focus()
	}
	a.win.Show()
	a.win.Focus()
}
