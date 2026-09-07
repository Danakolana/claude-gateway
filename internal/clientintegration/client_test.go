package clientintegration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danakolana/claude-gateway/internal/clientintegration"
)

func TestApplyDryRunAndRedaction(t *testing.T) {
	g := clientintegration.Render3P("http://127.0.0.1:8080", "sekrit", "bearer", []string{"m"})
	diff := clientintegration.RedactedDiff(g)
	if strings.Contains(diff, "sekrit") {
		t.Fatal(diff)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "claude_desktop_config.json")
	_ = os.WriteFile(path, []byte(`{"deploymentMode":"3p"}`), 0o600)
	snap, err := clientintegration.Apply(path, g, filepath.Join(dir, "bak"), "p", "0.1.0", true)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "sekrit") {
		t.Fatal("dry-run wrote secrets")
	}
	_, err = clientintegration.Apply(path, g, filepath.Join(dir, "bak"), "p", "0.1.0", false)
	if err != nil {
		t.Fatal(err)
	}
	_ = snap
}
