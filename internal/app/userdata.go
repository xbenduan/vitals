package app

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

func configureUserData() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	appData, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	path := developmentUserDataPath(executable, appData)
	legacyName := "Vitals"
	if path == "" {
		path = filepath.Join(appData, appName)
	} else {
		legacyName += " Dev"
	}
	// Finish the one-time copy before starting the UI or any disk workers.
	if err := migrateUserData(filepath.Join(appData, legacyName), path); err != nil {
		return fmt.Errorf("迁移旧版数据失败，原数据已保留：%w", err)
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	mygo.App.SetPath(mygo.PathUserData, path)
	return nil
}

// migrateUserData publishes a complete copy only when the new profile is absent.
// Existing profiles and the legacy source are never changed.
func migrateUserData(legacy, current string) error {
	if _, err := os.Lstat(current); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if info, err := os.Stat(legacy); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	} else if !info.IsDir() {
		return fmt.Errorf("旧版数据路径不是目录：%s", legacy)
	}
	stage, err := os.MkdirTemp(filepath.Dir(current), ".clipnest-migration-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	// Exclude settings left behind by the former system-monitor application.
	for _, name := range []string{"clipboard-settings.json", "Clipboard"} {
		source := filepath.Join(legacy, name)
		if _, err := os.Lstat(source); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		if err := copyUserData(source, filepath.Join(stage, name)); err != nil {
			return err
		}
	}
	return os.Rename(stage, current)
}

func copyUserData(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("旧版数据包含非普通文件：%s", path)
		}
		from, err := os.Open(path)
		if err != nil {
			return err
		}
		defer from.Close()
		to, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(to, from)
		closeErr := to.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}
