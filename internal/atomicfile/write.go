// Package atomicfile writes local data using a synced temporary file and rename.
package atomicfile

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// WriteJSON encodes an indented JSON value and atomically replaces path.
func WriteJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return Write(path, data)
}

// Write replaces path with data after syncing a temporary file in the same directory.
// Missing parent directories are created with owner-only access.
func Write(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".clipboard-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err = temp.Write(data); err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temp.Name(), path)
}
