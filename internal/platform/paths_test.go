package platform

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestClaudeDesktopPathsIncludeOSHints(t *testing.T) {
	p3 := ClaudeDesktop3PConfigPaths()
	if len(p3) == 0 {
		t.Fatal("no 3p paths")
	}
	pc := ClaudeDesktopConsumerConfigPaths()
	if len(pc) == 0 {
		t.Fatal("no consumer paths")
	}
	joined3 := strings.Join(p3, "\n")
	joinedC := strings.Join(pc, "\n")
	switch runtime.GOOS {
	case "windows":
		if !strings.Contains(joined3, `Claude-3p`) {
			t.Fatalf("windows 3p=%v", p3)
		}
		if !strings.Contains(joinedC, string(filepath.Separator)+"Claude"+string(filepath.Separator)) &&
			!strings.HasSuffix(filepath.Dir(pc[0]), "Claude") {
			t.Fatalf("windows consumer=%v", pc)
		}
	case "darwin":
		if !strings.Contains(joined3, "Application Support") {
			t.Fatalf("darwin should include Application Support: %v", p3)
		}
	}
	dir, err := GatewayDataDir()
	if err != nil || dir == "" {
		t.Fatalf("data dir: %v %q", err, dir)
	}
}
