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

func TestHelpAndConfig(t *testing.T) {
	var out bytes.Buffer
	if code := cli.RunWith(nil, &out, &out, secrets.EnvResolver{}); code != 0 {
		t.Fatal(code)
	}
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
streaming = true
tool_calls = true
[profiles.cheap]
provider = "openrouter"
[[profiles.cheap.routing.rules]]
source = "premium"
target_model = "fast"
`
	_ = os.WriteFile(path, []byte(body), 0o600)
	t.Setenv("OPENROUTER_API_KEY", "test-key-not-for-logs")
	out.Reset()
	var errb bytes.Buffer
	if code := cli.RunWith([]string{"config", "validate", "--config", path}, &out, &errb, secrets.EnvResolver{}); code != 0 {
		t.Fatalf("%s %s", out.String(), errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := cli.RunWith([]string{"config", "explain", "--config", path}, &out, &errb, secrets.EnvResolver{}); code != 0 {
		t.Fatal(errb.String())
	}
	if strings.Contains(out.String(), "test-key-not-for-logs") {
		t.Fatal("secret leaked")
	}
	out.Reset()
	if code := cli.RunWith([]string{"doctor", "--config", path}, &out, &errb, secrets.EnvResolver{}); code != 0 {
		t.Fatalf("doctor: %s %s", out.String(), errb.String())
	}
}
