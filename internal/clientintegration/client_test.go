package clientintegration_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danakolana/claude-gateway/internal/clientintegration"
)

func TestApplyMergesWithoutWipingPreferences(t *testing.T) {
	g := clientintegration.Render3P("http://127.0.0.1:8080", "sekrit", "bearer", []string{"deepseek/deepseek-v4-flash-0731"})
	diff := clientintegration.RedactedDiff(g)
	if strings.Contains(diff, "sekrit") {
		t.Fatal(diff)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "claude_desktop_config.json")
	_ = os.WriteFile(path, []byte(`{"deploymentMode":"3p","preferences":{"sidebarMode":"chat"},"coworkUserFilesPath":"/tmp/x"}`), 0o600)
	_, err := clientintegration.Apply(path, g, filepath.Join(dir, "bak"), "p", "0.1.0", false)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "sekrit") == false {
		t.Fatal("expected key written (file is local)")
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	prefs, _ := root["preferences"].(map[string]any)
	if prefs["sidebarMode"] != "chat" {
		t.Fatalf("preferences wiped: %v", root)
	}
	if root["coworkUserFilesPath"] != "/tmp/x" {
		t.Fatalf("extra keys wiped: %v", root)
	}
	ent, _ := root["enterpriseConfig"].(map[string]any)
	if ent["inferenceGatewayBaseUrl"] != "http://127.0.0.1:8080" {
		t.Fatalf("%v", ent)
	}
	if ent["modelDiscoveryEnabled"] != false {
		t.Fatalf("expected discovery off: %v", ent["modelDiscoveryEnabled"])
	}
}
