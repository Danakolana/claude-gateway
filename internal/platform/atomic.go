package platform

import (
	"os"
	"path/filepath"
	"runtime"
)

// ReplaceFile renames tmp onto dest. On Windows, the destination must not
// exist for os.Rename to succeed, so we remove dest first when present.
func ReplaceFile(tmp, dest string) error {
	if runtime.GOOS == "windows" {
		if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return os.Rename(tmp, dest)
}

// WriteFileAtomic writes data to path via a same-directory temp file, then
// ReplaceFile. Safe for concurrent readers on Unix; on Windows it replaces
// by delete+rename (brief window without the file).
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := ReplaceFile(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
