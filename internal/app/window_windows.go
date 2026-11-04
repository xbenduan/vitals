package app

func (a *application) positionWindow() {
	// MyGo's WM_WINDOWPOSCHANGING handler blocks SetWindowPos for immovable
	// windows. Allow only this synchronous move; user dragging stays disabled.
	a.win.SetMovable(true)
	defer a.win.SetMovable(false)
	if err := a.native.PlaceWindow(a.win.NativeHandle(), panelWidth, panelHeight); err != nil {
		a.win.Center()
		a.message = "无法在鼠标附近展开：" + err.Error()
	}
}
