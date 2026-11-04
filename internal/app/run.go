// Package app coordinates the MyGo clipboard UI, storage worker, and native services.
package app

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
)

const (
	appName  = "ClipNest"
	appTitle = "拾贴 · ClipNest"
)

// Run initializes the tray application and runs its event loop until quit.
// Call it once from the process entry point.
func Run() error {
	app := newApplication()
	mygo.App.SetName(appName)
	if !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	if err := configureUserData(); err != nil {
		return err
	}
	mygo.App.OnSecondInstance(func(_ []string, _ string) {
		if app.win != nil && !app.quitting {
			app.showWindow()
		}
	})
	mygo.App.OnActivate(func(_ bool) {
		if app.win != nil && !app.quitting {
			app.showWindow()
		}
	})
	if runtime.GOOS == "darwin" {
		mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	}
	mygo.App.WhenReady(app.ready)
	mygo.App.OnWindowAllClosed(func() {})
	return mygo.App.Run()
}

func developmentUserDataPath(executable, appData string) string {
	name := strings.TrimSuffix(filepath.Base(executable), ".exe")
	if !strings.HasSuffix(name, " Dev") {
		return ""
	}
	return filepath.Join(appData, appName+" Dev")
}
