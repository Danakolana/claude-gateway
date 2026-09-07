package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danakolana/claude-gateway/internal/cli"
	"github.com/danakolana/claude-gateway/internal/secrets"
)

func TestHelp(t *testing.T) {
	var out bytes.Buffer
	code := cli.RunWith(nil, &out, &out, secrets.EnvResolver{})
	if code != 0 || !strings.Contains(out.String(), "claude-gateway") {
		t.Fatalf("code=%d out=%s", code, out.String())
	}
}

func TestConfigValidateExplain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `
version = 1
active_profile = "cheap"
[providers.openrouter]
base_url = "https://openrouter.ai/api/v1"
api_key_env = "OPENROUTER_API_KEY"
[models.fast]
model_id = "x"
tier_alias = "fast"
enabled = true
[profiles.cheap]
provider = "openrouter"
[[profiles.cheap.routing.rules]]
source = "premium"
target_model = "fast"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENROUTER_API_KEY", "test-key-not-for-logs")
	var out, errb bytes.Buffer
	code := cli.RunWith([]string{"config", "validate", "--config", path}, &out, &errb, secrets.EnvResolver{})
	if code != 0 {
		t.Fatalf("validate failed: %s %s", out.String(), errb.String())
	}
	out.Reset()
	errb.Reset()
	code = cli.RunWith([]string{"config", "explain", "--config", path}, &out, &errb, secrets.EnvResolver{})
	if code != 0 {
		t.Fatalf("explain failed: %s", errb.String())
	}
	s := out.String()
	if strings.Contains(s, "test-key-not-for-logs") {
		t.Fatal("secret leaked in explain")
	}
}
