package app

import (
	"vitals/internal/clipboard"
	"vitals/internal/config"

	"github.com/egoist/mygo"
)

func (a *application) persistSettings() {
	settings, path := a.settings, a.settingsPath
	if path == "" {
		return
	}
	a.submit(func(*clipboard.Store) {
		if err := config.Save(path, settings); err != nil {
			a.win.Update(func() { a.message = "设置未能保存：" + err.Error() })
		}
	})
}

func (a *application) toggleListening() {
	a.cancelPending()
	a.settings.Paused = !a.settings.Paused
	if a.native != nil {
		// Resuming starts with the next copy, not content copied while paused.
		a.lastChange = a.native.ChangeCount()
	}
	a.persistSettings()
}

func (a *application) setOpenAtLogin() {
	if err := mygo.App.SetOpenAtLogin(a.settings.OpenAtLogin); err != nil {
		a.settings.OpenAtLogin = !a.settings.OpenAtLogin
		a.message = "登录启动设置失败：" + err.Error()
	}
}
