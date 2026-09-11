package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ClaudeDesktopConfigPaths returns candidate Claude Desktop / 3P config paths
// (3P first, then consumer).
func ClaudeDesktopConfigPaths() []string {
	return append(ClaudeDesktop3PConfigPaths(), ClaudeDesktopConsumerConfigPaths()...)
}

// ClaudeDesktop3PConfigPaths returns Claude Desktop on 3P config paths.
func ClaudeDesktop3PConfigPaths() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		appdata := os.Getenv("APPDATA")
		if appdata == "" {
			appdata = filepath.Join(home, "AppData", "Roaming")
		}
		return []string{
			filepath.Join(appdata, "Claude-3p", "claude_desktop_config.json"),
		}
	default:
		return []string{
			filepath.Join(home, ".config", "Claude-3p", "claude_desktop_config.json"),
			filepath.Join(home, "Library", "Application Support", "Claude-3p", "claude_desktop_config.json"),
		}
	}
}

// ClaudeDesktopConsumerConfigPaths returns regular (non-3P) Claude Desktop paths.
func ClaudeDesktopConsumerConfigPaths() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		appdata := os.Getenv("APPDATA")
		if appdata == "" {
			appdata = filepath.Join(home, "AppData", "Roaming")
		}
		return []string{
			filepath.Join(appdata, "Claude", "claude_desktop_config.json"),
		}
	default:
		return []string{
			filepath.Join(home, ".config", "Claude", "claude_desktop_config.json"),
			filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json"),
		}
	}
}

// DiscoverClaudeDesktopConfig finds the first existing path (3P then consumer) or "".
func DiscoverClaudeDesktopConfig() string {
	return DiscoverClaudeDesktopConfigFor("3p")
}

// DiscoverClaudeDesktopConfigFor selects paths by target ("3p" or "consumer").
// If no existing file is found, returns the primary candidate path for that target
// (empty string only when home resolution yields no candidates).
func DiscoverClaudeDesktopConfigFor(target string) string {
	var paths []string
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "consumer", "stable", "regular", "main":
		paths = ClaudeDesktopConsumerConfigPaths()
	default:
		paths = ClaudeDesktop3PConfigPaths()
	}
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
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
