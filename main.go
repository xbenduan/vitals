package main

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
)

func main() {
	app := newApplication()
	mygo.App.SetName("Vitals")
	configureDevelopmentUserData()
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { app.quitting = true })
	mygo.App.WhenReady(app.ready)
	mygo.App.OnWindowAllClosed(func() {})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
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
