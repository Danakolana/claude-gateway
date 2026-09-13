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
	if code := cli.RunWith([]string{"--help"}, &out, &out, secrets.EnvResolver{}); code != 0 {
		t.Fatal(code)
	}
	if !strings.Contains(out.String(), "First run") {
		t.Fatalf("help missing first-run blurb: %s", out.String())
	}
	if !strings.Contains(out.String(), "history list|export|import") {
		t.Fatalf("help missing history import: %s", out.String())
	}
	out.Reset()
	if code := cli.RunWith([]string{"version"}, &out, &out, secrets.EnvResolver{}); code != 0 {
		t.Fatal(code)
	}
	if !strings.Contains(out.String(), "v0.1.1") {
		t.Fatalf("version missing: %s", out.String())
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `
version = 1
active_profile = "cheap"
[proxy]
listen = "127.0.0.1:18099"
apply_desktop = false
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
	if code := cli.RunWith([]string{"doctor", "--offline", "--config", path}, &out, &errb, secrets.EnvResolver{}); code != 0 {
		t.Fatalf("doctor: %s %s", out.String(), errb.String())
	}
}

func TestDoctorHistoryWarnDoesNotFail(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	block := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(block, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	hist := filepath.ToSlash(filepath.Join(block, "history.db"))
	path := filepath.Join(dir, "config.toml")
	body := `
version = 1
active_profile = "cheap"
[proxy]
listen = "127.0.0.1:18100"
apply_desktop = false
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
[history]
local_database = "` + hist + `"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENROUTER_API_KEY", "test-key-not-for-logs")
	var out, errb bytes.Buffer
	if code := cli.RunWith([]string{"doctor", "--offline", "--config", path}, &out, &errb, secrets.EnvResolver{}); code != 0 {
		t.Fatalf("doctor must fail-open on history: code=%d %s %s", code, out.String(), errb.String())
	}
	if !strings.Contains(out.String(), "history: WARN") {
		t.Fatalf("expected history WARN, got %s", out.String())
	}
}

func TestDoctorPromptRedactsSecrets(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `
version = 1
active_profile = "cheap"
[proxy]
listen = "127.0.0.1:18101"
apply_desktop = false
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
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENROUTER_API_KEY", "sk-or-v1-secret-must-not-leak-zzzz")
	var out, errb bytes.Buffer
	if code := cli.RunWith([]string{"doctor", "--offline", "--prompt", "--config", path}, &out, &errb, secrets.EnvResolver{}); code != 0 {
		t.Fatalf("code=%d %s %s", code, out.String(), errb.String())
	}
	s := out.String()
	if strings.Contains(s, "secret-must-not-leak") {
		t.Fatal("secret leaked in prompt")
	}
	if !strings.Contains(s, "claude-gateway support prompt") || !strings.Contains(s, "## Verdict") {
		t.Fatalf("prompt missing: %s", s)
	}
}

func TestStartMissingAPIKeyPrintsExportExample(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_ = os.Unsetenv("OPENROUTER_API_KEY")
	t.Setenv("OPENROUTER_API_KEY", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `
version = 1
active_profile = "cheap"
[proxy]
listen = "127.0.0.1:18102"
apply_desktop = false
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
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := cli.RunWith([]string{"start", "--config", path, "--no-apply"}, &out, &errb, secrets.EnvResolver{})
	if code == 0 {
		t.Fatalf("expected failure, got ok: %s", out.String())
	}
	msg := errb.String()
	if !strings.Contains(msg, "OPENROUTER_API_KEY is not set") {
		t.Fatalf("missing key warning: %s", msg)
	}
	if !strings.Contains(msg, "export OPENROUTER_API_KEY=sk-or-...") {
		t.Fatalf("missing export example: %s", msg)
	}
}
