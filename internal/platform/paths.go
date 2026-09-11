package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const desktopConfigFile = "claude_desktop_config.json"

// desktopPathEnv is the OS/env snapshot used to build Desktop config
// candidates. Tests inject fake values so Windows/macOS layouts can be
// checked on Linux CI.
type desktopPathEnv struct {
	GOOS          string
	Home          string
	AppData       string
	LocalAppData  string
	XDGConfigHome string
}

func liveDesktopPathEnv() desktopPathEnv {
	home, _ := os.UserHomeDir()
	return desktopPathEnv{
		GOOS:          runtime.GOOS,
		Home:          home,
		AppData:       os.Getenv("APPDATA"),
		LocalAppData:  os.Getenv("LOCALAPPDATA"),
		XDGConfigHome: os.Getenv("XDG_CONFIG_HOME"),
	}
}

func (e desktopPathEnv) appData() string {
	if e.AppData != "" {
		return e.AppData
	}
	if e.Home != "" {
		return filepath.Join(e.Home, "AppData", "Roaming")
	}
	return ""
}

func (e desktopPathEnv) localAppData() string {
	if e.LocalAppData != "" {
		return e.LocalAppData
	}
	if e.Home != "" {
		return filepath.Join(e.Home, "AppData", "Local")
	}
	return ""
}

func (e desktopPathEnv) xdgConfig() string {
	if e.XDGConfigHome != "" {
		return e.XDGConfigHome
	}
	if e.Home != "" {
		return filepath.Join(e.Home, ".config")
	}
	return ""
}

func configFile(dir string) string {
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, desktopConfigFile)
}

func uniquePaths(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, p := range in {
		if p == "" {
			continue
		}
		key := p
		if runtime.GOOS == "windows" {
			key = strings.ToLower(p)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out
}

func globDirs(pattern string) []string {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, m := range matches {
		if st, err := os.Stat(m); err == nil && st.IsDir() {
			dirs = append(dirs, m)
		}
	}
	return dirs
}

// msixClaudePackageDirs returns Windows MSIX package roots (Claude_*).
// Official Store/MSIX installs virtualize Roaming writes under
// %LOCALAPPDATA%\Packages\Claude_<publisher>\LocalCache\...
func msixClaudePackageDirs(localAppData string) []string {
	if localAppData == "" {
		return nil
	}
	return globDirs(filepath.Join(localAppData, "Packages", "Claude_*"))
}

func msixProductDirs(localAppData, product string) []string {
	var dirs []string
	for _, pkg := range msixClaudePackageDirs(localAppData) {
		localDir := filepath.Join(pkg, "LocalCache", "Local", product)
		roamingDir := filepath.Join(pkg, "LocalCache", "Roaming", product)
		// Local is only used after Anthropic's Roaming→Local migration
		// inside the package. Prefer it when the directory already exists.
		if st, err := os.Stat(localDir); err == nil && st.IsDir() {
			dirs = append(dirs, localDir)
		}
		dirs = append(dirs, roamingDir)
	}
	return dirs
}

func linuxConfigRoots(e desktopPathEnv) []string {
	var roots []string
	if x := e.xdgConfig(); x != "" {
		roots = append(roots, x)
	}
	if e.Home != "" {
		def := filepath.Join(e.Home, ".config")
		if len(roots) == 0 || filepath.Clean(roots[0]) != filepath.Clean(def) {
			roots = append(roots, def)
		}
	}
	return uniquePaths(roots)
}

func linuxProductDirs(e desktopPathEnv, product string) []string {
	var dirs []string
	for _, root := range linuxConfigRoots(e) {
		primary := filepath.Join(root, product)
		dirs = append(dirs, primary)
		// Unofficial Linux ports sometimes use named 3P profiles
		// (~/.config/Claude-<name>-3p/). Skip the primary dir if glob hits it.
		if product == "Claude-3p" {
			for _, extra := range globDirs(filepath.Join(root, "Claude-*-3p")) {
				if filepath.Clean(extra) != filepath.Clean(primary) {
					dirs = append(dirs, extra)
				}
			}
		}
	}
	if e.Home != "" {
		for _, extra := range globDirs(filepath.Join(e.Home, ".var", "app", "*", "config", product)) {
			dirs = append(dirs, extra)
		}
	}
	return dirs
}

func darwinProductDir(e desktopPathEnv, product string) string {
	if e.Home == "" {
		return ""
	}
	return filepath.Join(e.Home, "Library", "Application Support", product)
}

func windowsNativeProductDirs(e desktopPathEnv, product string) []string {
	var dirs []string
	if local := e.localAppData(); local != "" {
		dirs = append(dirs, filepath.Join(local, product))
	}
	if roaming := e.appData(); roaming != "" {
		legacy := filepath.Join(roaming, product)
		if len(dirs) == 0 || filepath.Clean(dirs[0]) != filepath.Clean(legacy) {
			dirs = append(dirs, legacy)
		}
	}
	return dirs
}

func productConfigPaths(e desktopPathEnv, product string) []string {
	var dirs []string
	switch e.GOOS {
	case "windows":
		// MSIX first: packaged apps ignore native %APPDATA% / %LOCALAPPDATA%\Claude-3p.
		dirs = append(dirs, msixProductDirs(e.localAppData(), product)...)
		dirs = append(dirs, windowsNativeProductDirs(e, product)...)
	case "darwin":
		if d := darwinProductDir(e, product); d != "" {
			dirs = append(dirs, d)
		}
		// Rare leftover from Linux-style tools / XDG on macOS.
		dirs = append(dirs, linuxProductDirs(e, product)...)
	default:
		dirs = append(dirs, linuxProductDirs(e, product)...)
		if d := darwinProductDir(e, product); d != "" {
			dirs = append(dirs, d)
		}
	}
	var paths []string
	for _, dir := range dirs {
		if p := configFile(dir); p != "" {
			paths = append(paths, p)
		}
	}
	return uniquePaths(paths)
}

// ClaudeDesktopConfigPaths returns candidate Claude Desktop / 3P config paths
// (3P first, then consumer).
func ClaudeDesktopConfigPaths() []string {
	return uniquePaths(append(ClaudeDesktop3PConfigPaths(), ClaudeDesktopConsumerConfigPaths()...))
}

// ClaudeDesktop3PConfigPaths returns Claude Desktop on 3P config paths,
// ranked newest-layout first:
//
//   - Windows MSIX: %LOCALAPPDATA%\Packages\Claude_*\LocalCache\{Local,Roaming}\Claude-3p\
//   - Windows current: %LOCALAPPDATA%\Claude-3p\  (post-Roaming migration)
//   - Windows legacy: %APPDATA%\Claude-3p\
//   - macOS: ~/Library/Application Support/Claude-3p\
//   - Linux: $XDG_CONFIG_HOME/Claude-3p\ or ~/.config/Claude-3p\
func ClaudeDesktop3PConfigPaths() []string {
	return productConfigPaths(liveDesktopPathEnv(), "Claude-3p")
}

// ClaudeDesktopConsumerConfigPaths returns regular (non-3P) Claude Desktop paths.
// Ranked the same way as 3P so MSIX/XDG layouts are found before legacy ones.
func ClaudeDesktopConsumerConfigPaths() []string {
	return productConfigPaths(liveDesktopPathEnv(), "Claude")
}

// DiscoverClaudeDesktopConfig finds the first existing path (3P then consumer) or "".
func DiscoverClaudeDesktopConfig() string {
	return DiscoverClaudeDesktopConfigFor("3p")
}

// desktopConfigExists reports whether Desktop already has state at this
// config file path: the JSON file, a configLibrary profile, or the data dir.
func desktopConfigExists(p string) bool {
	if p == "" {
		return false
	}
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return true
	}
	dir := filepath.Dir(p)
	if st, err := os.Stat(filepath.Join(dir, "configLibrary", "_meta.json")); err == nil && !st.IsDir() {
		return true
	}
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		return true
	}
	return false
}

// ExistingClaudeDesktopConfigPaths returns candidates that already have
// Desktop state, in preference order (newest layout first).
func ExistingClaudeDesktopConfigPaths(target string) []string {
	var found []string
	for _, p := range desktopPathsFor(target) {
		if desktopConfigExists(p) {
			found = append(found, p)
		}
	}
	return found
}

func desktopPathsFor(target string) []string {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "consumer", "stable", "regular", "main":
		return ClaudeDesktopConsumerConfigPaths()
	default:
		return ClaudeDesktop3PConfigPaths()
	}
}

// DiscoverClaudeDesktopConfigFor selects paths by target ("3p" or "consumer").
// Prefers an existing Desktop data dir / configLibrary / JSON file. If none
// exist, returns the primary candidate for the current OS (empty string only
// when home resolution yields no candidates).
func DiscoverClaudeDesktopConfigFor(target string) string {
	paths := desktopPathsFor(target)
	for _, p := range paths {
		if desktopConfigExists(p) {
			return p
		}
	}
	if len(paths) > 0 {
		return paths[0]
	}
	return ""
}

// GatewayDataDir returns the local data directory.
func GatewayDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(base, "claude-gateway"), nil
	}
	return filepath.Join(home, ".local", "share", "claude-gateway"), nil
}
