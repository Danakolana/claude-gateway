package clientintegration_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/danakolana/claude-gateway/internal/clientintegration"
)

func TestInspectReadsLibraryAndJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude_desktop_config.json")
	if err := os.WriteFile(path, []byte(`{
  "deploymentMode": "3p",
  "enterpriseConfig": {"inferenceGatewayBaseUrl": "http://127.0.0.1:8080"}
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	lib := filepath.Join(dir, "configLibrary")
	if err := os.MkdirAll(lib, 0o700); err != nil {
		t.Fatal(err)
	}
	id := "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
	_ = os.WriteFile(filepath.Join(lib, "_meta.json"), []byte(`{"appliedId":"`+id+`","entries":[{"id":"`+id+`","name":"g"}]}`), 0o600)
	_ = os.WriteFile(filepath.Join(lib, id+".json"), []byte(`{"inferenceGatewayBaseUrl":"http://127.0.0.1:8081"}`), 0o600)

	v := clientintegration.Inspect(path)
	if v.BaseURL != "http://127.0.0.1:8080" {
		t.Fatalf("json url=%s", v.BaseURL)
	}
	if v.LibraryURL != "http://127.0.0.1:8081" {
		t.Fatalf("lib url=%s", v.LibraryURL)
	}
	if v.EffectiveBaseURL() != "http://127.0.0.1:8081" {
		t.Fatalf("effective=%s", v.EffectiveBaseURL())
	}
	if !clientintegration.SameGatewayURL("http://127.0.0.1:8081/", "http://127.0.0.1:8081") {
		t.Fatal("slash mismatch")
	}
}

func TestInspectConsumerEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "claude_desktop_config.json")
	if err := os.WriteFile(path, []byte(`{"env":{"ANTHROPIC_BASE_URL":"http://127.0.0.1:9"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	v := clientintegration.Inspect(path)
	if v.ConsumerURL != "http://127.0.0.1:9" {
		t.Fatalf("%+v", v)
	}
	if v.EffectiveBaseURL() != "http://127.0.0.1:9" {
		t.Fatalf("effective=%s", v.EffectiveBaseURL())
	}
}
