package platform

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ClaudeDesktopLikelyRunning is best-effort. False negatives are OK
// (we then apply anyway). Must not match claude-gateway itself.
func ClaudeDesktopLikelyRunning() bool {
	switch runtime.GOOS {
	case "windows":
		return windowsImageRunning("Claude.exe")
	case "linux":
		if linuxCommEquals("Claude") || linuxCommEquals("claude") || linuxCommEquals("claude-desktop") {
			return true
		}
		return pgrepExact("Claude") || pgrepExact("claude")
	default:
		return pgrepExact("Claude") || pgrepExact("claude")
	}
}

func linuxCommEquals(name string) bool {
	matches, err := filepath.Glob("/proc/[0-9]*/comm")
	if err != nil {
		return false
	}
	for _, p := range matches {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(b)) == name {
			return true
		}
	}
	return false
}

func pgrepExact(name string) bool {
	if name == "" || strings.Contains(name, "gateway") {
		return false
	}
	cmd := exec.Command("pgrep", "-x", name)
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func windowsImageRunning(image string) bool {
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq "+image, "/NH").Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), strings.ToLower(image))
}
