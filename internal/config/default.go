package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/danakolana/claude-gateway/internal/platform"
)

// DefaultTOML is the shipped default configuration (kept in sync with
// /config.toml at the repo root). Embedded so a downloaded binary works
// without cloning the repo.
//
//go:embed default.toml
var DefaultTOML []byte

// sidecarDirFn resolves the directory that holds the editable sidecar
// config.toml (next to the binary). Overridable in tests.
var sidecarDirFn = defaultSidecarDir

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

// SidecarConfigPath is config.toml next to the running binary (the file
// users should edit). When the binary lives in an ephemeral dir (go run /
// go test), falls back to the process working directory.
func SidecarConfigPath() (string, error) {
	dir, err := sidecarDirFn()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

func defaultSidecarDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := filepath.Dir(exe)
	if isEphemeralDir(dir) {
		cwd, err := os.Getwd()
		if err == nil && cwd != "" {
			return cwd, nil
		}
	}
	return dir, nil
}

func isEphemeralDir(dir string) bool {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	if tmp, err := filepath.Abs(os.TempDir()); err == nil && tmp != "" {
		sep := string(os.PathSeparator)
		if abs == tmp || strings.HasPrefix(abs, tmp+sep) {
			return true
		}
	}
	lower := strings.ToLower(abs)
	sep := string(os.PathSeparator)
	return strings.Contains(lower, sep+"go-build") ||
		strings.Contains(lower, sep+"go-link-")
}

// EnsureSidecarAndSync makes sure the editable sidecar config exists, then
// copies it to the OS user config path on every call.
//
// Bootstrap order when the sidecar is missing:
//  1. Copy an existing ~/.config/.../config.toml into the sidecar (upgrade path)
//  2. Otherwise write the embedded build-time default
//
// created is true only when the sidecar file was newly written.
func EnsureSidecarAndSync() (sidecar string, created bool, err error) {
	sidecar, err = SidecarConfigPath()
	if err != nil {
		return "", false, err
	}
	userPath, err := DefaultUserConfigPath()
	if err != nil {
		return "", false, err
	}

	if st, err := os.Stat(sidecar); err == nil && !st.IsDir() {
		if err := syncFile(sidecar, userPath); err != nil {
			return "", false, err
		}
		return sidecar, false, nil
	} else if err != nil && !os.IsNotExist(err) {
		return "", false, err
	}

	var seed []byte
	if data, err := os.ReadFile(userPath); err == nil && len(data) > 0 {
		seed = data
	} else {
		seed = DefaultTOML
	}
	if err := platform.WriteFileAtomic(sidecar, seed, 0o600); err != nil {
		return "", false, err
	}
	if err := syncFile(sidecar, userPath); err != nil {
		return sidecar, true, err
	}
	return sidecar, true, nil
}

func syncFile(src, dst string) error {
	if src == "" || dst == "" {
		return fmt.Errorf("sync config: empty path")
	}
	if samePath(src, dst) {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read sidecar %s: %w", src, err)
	}
	return platform.WriteFileAtomic(dst, data, 0o600)
}

func samePath(a, b string) bool {
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(absA, absB)
	}
	return absA == absB
}

// EnsureUserConfig writes the embedded default config to the user path when
// missing. Existing files are left untouched. Prefer EnsureSidecarAndSync for
// normal CLI runs (sidecar is the editable source of truth).
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
// (used by tests / admin tools). Prefer EnsureSidecarAndSync for normal runs.
func WriteDefaultConfigAlways(path string) error {
	if path == "" {
		return fmt.Errorf("path required")
	}
	return platform.WriteFileAtomic(path, DefaultTOML, 0o600)
}
