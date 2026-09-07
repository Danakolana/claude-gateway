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

func TestApplySyncsConfigLibrary(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude_desktop_config.json")
	libID := "11111111-2222-4333-8444-555555555555"
	libDir := filepath.Join(dir, "configLibrary")
	if err := os.MkdirAll(libDir, 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(libDir, "_meta.json"), []byte(`{
  "appliedId": "`+libID+`",
  "entries": [{"id": "`+libID+`", "name": "Default"}]
}`), 0o600)
	_ = os.WriteFile(filepath.Join(libDir, libID+".json"), []byte(`{
  "inferenceGatewayBaseUrl": "http://127.0.0.1:8080",
  "modelDiscoveryEnabled": true,
  "inferenceProvider": "gateway",
  "inferenceCredentialKind": "static"
}`), 0o600)
	_ = os.WriteFile(path, []byte(`{"deploymentMode":"3p","preferences":{}}`), 0o600)

	g := clientintegration.Render3PEntries(
		"https://openrouter.ai/api", "sekrit", "bearer",
		[]clientintegration.InferenceModelEntry{
			{Name: "deepseek/deepseek-v4-flash-0731", LabelOverride: "DeepSeek", AnthropicFamilyTier: "haiku", IsFamilyDefault: true},
		},
		false,
	)
	if _, err := clientintegration.Apply(path, g, filepath.Join(dir, "bak"), "p", "0.1.0", false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(libDir, libID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var lib map[string]any
	if err := json.Unmarshal(raw, &lib); err != nil {
		t.Fatal(err)
	}
	if lib["inferenceGatewayBaseUrl"] != "https://openrouter.ai/api" {
		t.Fatalf("url=%v", lib["inferenceGatewayBaseUrl"])
	}
	if lib["modelDiscoveryEnabled"] != false {
		t.Fatalf("discovery=%v", lib["modelDiscoveryEnabled"])
	}
	models, _ := lib["inferenceModels"].([]any)
	if len(models) != 1 {
		t.Fatalf("models=%v", lib["inferenceModels"])
	}
	m0, _ := models[0].(map[string]any)
	if m0["name"] != "deepseek/deepseek-v4-flash-0731" {
		t.Fatalf("model=%v", m0)
	}
	metaRaw, _ := os.ReadFile(filepath.Join(libDir, "_meta.json"))
	var meta map[string]any
	_ = json.Unmarshal(metaRaw, &meta)
	if meta["appliedId"] != libID {
		t.Fatalf("meta=%v", meta)
	}
}
