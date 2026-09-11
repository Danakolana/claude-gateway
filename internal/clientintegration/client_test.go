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
	_ = os.WriteFile(path, []byte(`{"deploymentMode":"3p","preferences":{"sidebarMode":"chat"},"coworkUserFilesPath":"/tmp/x","enterpriseConfig":{"keepExtra":true}}`), 0o600)
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
	if ent["keepExtra"] != true {
		t.Fatalf("expected extra enterpriseConfig keys preserved: %v", ent)
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
  "updatedAt": "2026-01-01T00:00:00Z",
  "entries": [{"id": "`+libID+`", "name": "Default", "kind": "local"}]
}`), 0o600)
	_ = os.WriteFile(filepath.Join(libDir, libID+".json"), []byte(`{
  "inferenceGatewayBaseUrl": "http://127.0.0.1:8080",
  "modelDiscoveryEnabled": true,
  "inferenceProvider": "gateway",
  "inferenceCredentialKind": "static",
  "modelPrefer1mContext": true,
  "schemaVersion": "2",
  "managedMcpServers": [{"name": "keep-me"}]
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
	if lib["managedMcpServers"] == nil {
		t.Fatal("expected existing MCP servers to survive configLibrary merge")
	}
	if lib["schemaVersion"] != "2" {
		t.Fatalf("expected extra profile keys preserved, got %v", lib["schemaVersion"])
	}
	if lib["modelPrefer1mContext"] != true {
		t.Fatalf("user modelPrefer1mContext should be kept, got %v", lib["modelPrefer1mContext"])
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
	if meta["updatedAt"] != "2026-01-01T00:00:00Z" {
		t.Fatalf("meta extra fields dropped: %v", meta)
	}
	entries, _ := meta["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("entries=%v", meta["entries"])
	}
	e0, _ := entries[0].(map[string]any)
	if e0["kind"] != "local" {
		t.Fatalf("entry extra fields dropped: %v", e0)
	}
}

func TestApplyConsumerMergesEnvWithoutWipingPreferences(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude_desktop_config.json")
	_ = os.WriteFile(path, []byte(`{
  "preferences": {"sidebarMode": "chat"},
  "mcpServers": {"x": {}},
  "env": {"OTHER": "keep"}
}`), 0o600)
	cand := clientintegration.RenderConsumerEnv("http://127.0.0.1:8080", "sekrit")
	diff := clientintegration.RedactedConsumerDiff(cand)
	if strings.Contains(diff, "sekrit") {
		t.Fatal(diff)
	}
	if _, err := clientintegration.ApplyConsumer(path, cand, filepath.Join(dir, "bak"), "p", "0.1.0", false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	prefs, _ := root["preferences"].(map[string]any)
	if prefs["sidebarMode"] != "chat" {
		t.Fatalf("preferences wiped: %v", root)
	}
	if _, ok := root["mcpServers"]; !ok {
		t.Fatalf("mcp wiped: %v", root)
	}
	env, _ := root["env"].(map[string]any)
	if env["OTHER"] != "keep" {
		t.Fatalf("env other lost: %v", env)
	}
	if env["ANTHROPIC_BASE_URL"] != "http://127.0.0.1:8080" {
		t.Fatalf("base url: %v", env)
	}
	if env["ANTHROPIC_API_KEY"] != "sekrit" {
		t.Fatalf("key: %v", env)
	}
	// no configLibrary for consumer
	if _, err := os.Stat(filepath.Join(dir, "configLibrary")); !os.IsNotExist(err) {
		t.Fatalf("expected no configLibrary, err=%v", err)
	}
}

func TestDriftedAfterMutation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude_desktop_config.json")
	if err := os.WriteFile(path, []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	sum, err := clientintegration.FileChecksum(path)
	if err != nil {
		t.Fatal(err)
	}
	drifted, err := clientintegration.Drifted(path, sum)
	if err != nil || drifted {
		t.Fatalf("fresh file drifted=%v err=%v", drifted, err)
	}
	if err := os.WriteFile(path, []byte(`{"ok":false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	drifted, err = clientintegration.Drifted(path, sum)
	if err != nil || !drifted {
		t.Fatalf("expected drift, got %v %v", drifted, err)
	}
}

func TestDriftedMissingFileErrorsWithoutRewrite(t *testing.T) {
	_, err := clientintegration.Drifted(filepath.Join(t.TempDir(), "missing.json"), "abc")
	if err == nil {
		t.Fatal("missing file must error so the watcher WARNs and does not rewrite")
	}
}
