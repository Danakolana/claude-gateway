package platform

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// OpenBrowser opens url in the default browser. Best-effort; errors are returned
// but callers should treat them as non-fatal.
func OpenBrowser(url string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return fmt.Errorf("empty url")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
