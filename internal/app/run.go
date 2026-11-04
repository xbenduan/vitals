// Package app coordinates the MyGo clipboard UI, storage worker, and native services.
package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
)

// Run initializes the tray application and runs its event loop until quit.
// Call it once from the process entry point.
func Run() error {
	app := newApplication()
	mygo.App.SetName("Vitals")
	configureDevelopmentUserData()
	if !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	mygo.App.OnSecondInstance(func(_ []string, _ string) {
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

func configureDevelopmentUserData() {
	executable, executableErr := os.Executable()
	appData, appDataErr := os.UserConfigDir()
	if executableErr != nil || appDataErr != nil {
		return
	}
	if path := developmentUserDataPath(executable, appData); path != "" {
		if os.MkdirAll(path, 0o755) == nil {
			mygo.App.SetPath(mygo.PathUserData, path)
		}
	}
}

func developmentUserDataPath(executable, appData string) string {
	name := strings.TrimSuffix(filepath.Base(executable), ".exe")
	if !strings.HasSuffix(name, " Dev") {
		return ""
	}
	return filepath.Join(appData, "Vitals Dev")
}
