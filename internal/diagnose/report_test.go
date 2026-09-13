package diagnose_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/internal/diagnose"
	"github.com/danakolana/claude-gateway/internal/secrets"
)

func TestRedactStripsKeys(t *testing.T) {
	in := "Authorization: Bearer sk-or-v1-secretsecret export OPENROUTER_API_KEY=sk-ant-abc123456"
	out := diagnose.Redact(in)
	if strings.Contains(out, "secretsecret") || strings.Contains(out, "sk-or-") || strings.Contains(out, "sk-ant-abc") {
		t.Fatalf("leaked: %s", out)
	}
	if !strings.Contains(out, "<redacted>") && !strings.Contains(out, "<redacted-key>") {
		t.Fatalf("expected redaction markers: %s", out)
	}
}

func TestCollectOfflineValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `
version = 1
active_profile = "cheap"
[proxy]
listen = "127.0.0.1:18091"
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
	t.Setenv("OPENROUTER_API_KEY", "sk-or-v1-this-must-not-appear-in-report")
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rep := diagnose.Collect(diagnose.Options{
		Config: cfg, ConfigPath: path, Resolver: secrets.EnvResolver{},
		ProbeProvider: false, DesktopTarget: "3p",
	})
	if rep.GOOS != runtime.GOOS {
		t.Fatalf("goos=%s", rep.GOOS)
	}
	human := diagnose.FormatHuman(rep)
	prompt := rep.SupportPrompt
	if strings.Contains(human, "this-must-not-appear") || strings.Contains(prompt, "this-must-not-appear") {
		t.Fatal("secret leaked")
	}
	if !strings.Contains(human, "config: OK") {
		t.Fatalf("human=%s", human)
	}
	if !strings.Contains(prompt, "## Verdict") || !strings.Contains(prompt, "claude-gateway") {
		t.Fatalf("prompt=%s", prompt)
	}
	if rep.Verdict == diagnose.VerdictReady && strings.Contains(human, "FAIL") {
		t.Fatalf("ready with fail: %s", human)
	}
}

func TestCollectMissingKeyIsFail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `
version = 1
active_profile = "cheap"
[proxy]
listen = "127.0.0.1:18092"
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
	t.Setenv("OPENROUTER_API_KEY", "")
	_ = os.Unsetenv("OPENROUTER_API_KEY")
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rep := diagnose.Collect(diagnose.Options{
		Config: cfg, ConfigPath: path, Resolver: secrets.EnvResolver{},
		ProbeProvider: false,
	})
	if rep.Verdict != diagnose.VerdictNotReady {
		t.Fatalf("verdict=%s human=%s", rep.Verdict, diagnose.FormatHuman(rep))
	}
	found := false
	for _, c := range rep.Checks {
		if c.ID == "api_key" && c.Status == diagnose.StatusFail {
			found = true
			if c.Fix == "" {
				t.Fatal("expected fix")
			}
		}
	}
	if !found {
		t.Fatalf("missing api_key FAIL: %+v", rep.Checks)
	}
}

func TestFormatHumanHistoryWarnLine(t *testing.T) {
	rep := diagnose.Report{
		Verdict: diagnose.VerdictPartial,
		Checks: []diagnose.Check{{
			ID: "history", Status: diagnose.StatusWarn, Detail: "unavailable at /x; proxy can still run",
		}},
	}
	s := diagnose.FormatHuman(rep)
	if !strings.Contains(s, "history: WARN") {
		t.Fatalf("%s", s)
	}
}

func TestFormatVersion(t *testing.T) {
	prevC, prevD := diagnose.GitCommit, diagnose.BuildDate
	t.Cleanup(func() {
		diagnose.GitCommit, diagnose.BuildDate = prevC, prevD
	})
	diagnose.GitCommit, diagnose.BuildDate = "", ""
	if got := diagnose.FormatVersion(); got != "v0.1.1" {
		t.Fatalf("got %q", got)
	}
	diagnose.GitCommit = "abcdef123456"
	diagnose.BuildDate = "2026-09-13"
	got := diagnose.FormatVersion()
	if got != "v0.1.1 · abcdef1 · 2026-09-13" {
		t.Fatalf("got %q", got)
	}
}
