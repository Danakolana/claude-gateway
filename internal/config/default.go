package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultTOML is the shipped default configuration (kept in sync with
// /config.toml at the repo root). Embedded so a downloaded binary works
// without cloning the repo.
//
//go:embed default.toml
var DefaultTOML []byte

// DefaultUserConfigPath returns ~/.config/claude-gateway/config.toml.
func DefaultUserConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "claude-gateway", "config.toml"), nil
}

// EnsureUserConfig writes the embedded default config to the user path when
// missing. Existing files are left untouched. created is true only when a
// new file was written.
func EnsureUserConfig() (path string, created bool, err error) {
	path, err = DefaultUserConfigPath()
	if err != nil {
		return "", false, err
	}
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		return path, false, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", false, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, DefaultTOML, 0o600); err != nil {
		return "", false, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", false, err
	}
	return path, true, nil
}

// WriteDefaultConfigAlways overwrites path with the embedded default
// (used by tests / admin tools). Prefer EnsureUserConfig for normal runs.
func WriteDefaultConfigAlways(path string) error {
	if path == "" {
		return fmt.Errorf("path required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, DefaultTOML, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
