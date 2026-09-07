package platform

import (
	"os"
	"path/filepath"
	"runtime"
)

// ClaudeDesktopConfigPaths returns candidate Claude Desktop / 3P config paths.
func ClaudeDesktopConfigPaths() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		appdata := os.Getenv("APPDATA")
		if appdata == "" {
			appdata = filepath.Join(home, "AppData", "Roaming")
		}
		return []string{
			filepath.Join(appdata, "Claude-3p", "claude_desktop_config.json"),
			filepath.Join(appdata, "Claude", "claude_desktop_config.json"),
		}
	default: // linux and others
		return []string{
			filepath.Join(home, ".config", "Claude-3p", "claude_desktop_config.json"),
			filepath.Join(home, ".config", "Claude", "claude_desktop_config.json"),
			filepath.Join(home, "Library", "Application Support", "Claude-3p", "claude_desktop_config.json"),
			filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json"),
		}
	}
}

// DiscoverClaudeDesktopConfig finds the first existing path or returns "".
func DiscoverClaudeDesktopConfig() string {
	for _, p := range ClaudeDesktopConfigPaths() {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
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
