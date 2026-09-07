package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const validTOML = `
version = 1
active_profile = "cheap"

[providers.openrouter]
base_url = "https://openrouter.ai/api/v1"
protocol = "openai-compatible"
api_key_env = "OPENROUTER_API_KEY"
auth_scheme = "bearer"
timeout_seconds = 90

[models.fast]
model_id = "deepseek/deepseek-chat"
display_name = "DeepSeek Fast"
tier_alias = "fast"
streaming = true
tool_calls = true
enabled = true
context_limit = 64000

[profiles.cheap]
provider = "openrouter"
proxy_mode = "local"
history_mode = "local"

[[profiles.cheap.routing.rules]]
source = "premium"
target_model = "fast"
`

func TestParseAndValidateOK(t *testing.T) {
	path := writeTempConfig(t, validTOML)
	f, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if errs := Validate(f); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestProxyDefaults(t *testing.T) {
	var p Proxy
	if p.Addr() != "127.0.0.1:8080" || p.BaseURL() != "http://127.0.0.1:8080" || !p.ShouldApplyDesktop() {
		t.Fatalf("%+v", p)
	}
	off := false
	p.Listen = "127.0.0.1:9"
	p.ApplyDesktop = &off
	if p.Addr() != "127.0.0.1:9" || p.ShouldApplyDesktop() {
		t.Fatalf("%+v", p)
	}
	path := writeTempConfig(t, validTOML+`
[proxy]
listen = "127.0.0.1:9090"
apply_desktop = false
`)
	f, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.Proxy.Addr() != "127.0.0.1:9090" || f.Proxy.ShouldApplyDesktop() {
		t.Fatalf("%+v", f.Proxy)
	}
}

func TestSSRFBlocksMetadata(t *testing.T) {
	path := writeTempConfig(t, `
version = 1
active_profile = "p"
[providers.bad]
base_url = "http://169.254.169.254/latest"
api_key_env = "K"
[profiles.p]
provider = "bad"
`)
	f, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	errs := Validate(f)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Reason, "blocked") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected SSRF blocked error, got %v", errs)
	}
}

func TestLoopbackAllowedForLocalGateway(t *testing.T) {
	path := writeTempConfig(t, `
version = 1
active_profile = "p"
[providers.nine]
base_url = "http://127.0.0.1:20128/v1"
api_key_env = "NINEROUTER_API_KEY"
[profiles.p]
provider = "nine"
`)
	f, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range Validate(f) {
		if strings.Contains(e.Reason, "blocked") || strings.Contains(e.Reason, "private") {
			t.Fatalf("loopback should be allowed: %v", e)
		}
	}
}

func TestDiscoveryExplicitWins(t *testing.T) {
	a := writeTempConfig(t, validTOML)
	b := writeTempConfig(t, strings.Replace(validTOML, `active_profile = "cheap"`, `active_profile = "other"`, 1))
	t.Setenv("CLAUDE_GATEWAY_CONFIG", b)
	got, err := Discover(a)
	if err != nil {
		t.Fatal(err)
	}
	if got != a {
		t.Fatalf("explicit should win: got %s", got)
	}
}

func TestDiscoveryFallsBackToCwdExamples(t *testing.T) {
	t.Setenv("CLAUDE_GATEWAY_CONFIG", "")
	dir := t.TempDir()
	ex := filepath.Join(dir, "examples")
	if err := os.MkdirAll(ex, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ex, "config.toml")
	if err := os.WriteFile(path, []byte(validTOML), 0o600); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	got, err := Discover("")
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("got %s want %s", got, path)
	}
}

func TestExplainRedactsSecrets(t *testing.T) {
	path := writeTempConfig(t, validTOML)
	f, _, err := Load(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	eff, err := Resolve(f)
	if err != nil {
		t.Fatal(err)
	}
	out := Explain(eff, path)
	if strings.Contains(out, "sk-") || strings.Contains(strings.ToLower(out), "bearer ") {
		t.Fatalf("leaked secret material: %s", out)
	}
	if !strings.Contains(out, "<secret:env:OPENROUTER_API_KEY>") {
		t.Fatalf("expected redacted handle: %s", out)
	}
}

func TestProfileMutationsPreserveOthers(t *testing.T) {
	path := writeTempConfig(t, validTOML+`
[profiles.other]
provider = "openrouter"
proxy_mode = "local"
`)
	if err := CreateProfile(path, "third"); err != nil {
		t.Fatal(err)
	}
	if err := SelectProfile(path, "other"); err != nil {
		t.Fatal(err)
	}
	if err := DeleteProfile(path, "third"); err != nil {
		t.Fatal(err)
	}
	f, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Profiles["cheap"]; !ok {
		t.Fatal("cheap profile deleted unexpectedly")
	}
	if _, ok := f.Profiles["other"]; !ok {
		t.Fatal("other profile missing")
	}
	if _, ok := f.Profiles["third"]; ok {
		t.Fatal("third should be deleted")
	}
	if f.ActiveProfile != "other" {
		t.Fatalf("active=%q", f.ActiveProfile)
	}
}

func TestMissingTargetModel(t *testing.T) {
	body := strings.Replace(validTOML, `target_model = "fast"`, `target_model = "missing"`, 1)
	path := writeTempConfig(t, body)
	f, err := ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	errs := Validate(f)
	if len(errs) == 0 {
		t.Fatal("expected missing target error")
	}
}
