package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/danakolana/claude-gateway/internal/platform"
)

// DefaultTOML is the shipped default configuration (kept in sync with
// /config.toml at the repo root). Embedded so a downloaded binary works
// without cloning the repo.
//
//go:embed default.toml
var DefaultTOML []byte

// DefaultUserConfigPath returns the OS-appropriate user config path:
//   - Windows: %APPDATA%\claude-gateway\config.toml
//   - macOS/Linux: ~/.config/claude-gateway/config.toml
func DefaultUserConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return defaultUserConfigPath(runtime.GOOS, home, os.Getenv("APPDATA")), nil
}

func defaultUserConfigPath(goos, home, appdata string) string {
	if goos == "windows" {
		base := appdata
		if base == "" {
			base = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(base, "claude-gateway", "config.toml")
	}
	return filepath.Join(home, ".config", "claude-gateway", "config.toml")
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
	if err := platform.WriteFileAtomic(path, DefaultTOML, 0o600); err != nil {
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
	return platform.WriteFileAtomic(path, DefaultTOML, 0o600)
}
