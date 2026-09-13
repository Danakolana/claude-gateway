package diagnose

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/danakolana/claude-gateway/internal/platform"
)

const (
	// Version is the semver release. Bump intentionally on meaningful changes:
	// major = breaking, minor = features, patch = fixes.
	Version = "0.1.1"
	// ToolVersion is the public version string used across CLI / diagnose /
	// Desktop backups (no "v" prefix). Kept as an alias of Version.
	ToolVersion = Version
)

// Optional build metadata. Release builds can inject these via -ldflags, e.g.
//
//	-X github.com/danakolana/claude-gateway/internal/diagnose.GitCommit=$(git rev-parse --short HEAD)
//	-X github.com/danakolana/claude-gateway/internal/diagnose.BuildDate=$(date -u +%Y-%m-%d)
var (
	GitCommit = ""
	BuildDate = ""
)

// FormatVersion returns a human-facing version string, e.g. "v0.1.0" or
// "v0.1.0 · abc1234 · 2026-09-13".
func FormatVersion() string {
	s := "v" + ToolVersion
	if c := strings.TrimSpace(GitCommit); c != "" {
		if len(c) > 7 {
			c = c[:7]
		}
		s += " · " + c
	}
	if d := strings.TrimSpace(BuildDate); d != "" {
		s += " · " + d
	}
	return s
}

const (
	StatusOK   = "OK"
	StatusWarn = "WARN"
	StatusFail = "FAIL"
)

const (
	VerdictReady    = "READY"
	VerdictPartial  = "PARTIAL"
	VerdictNotReady = "NOT_READY"
)

// Check is one line in a setup report. Detail and Fix must never contain secrets.
type Check struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// Report is a redacted snapshot of whether this machine is set up to chat.
type Report struct {
	GeneratedAt   time.Time `json:"generated_at"`
	Version       string    `json:"version"`
	GOOS          string    `json:"goos"`
	GOARCH        string    `json:"goarch"`
	Verdict       string    `json:"verdict"`
	Summary       string    `json:"summary"`
	ConfigPath    string    `json:"config_path,omitempty"`
	GatewayURL    string    `json:"gateway_url,omitempty"`
	DesktopTarget string    `json:"desktop_target,omitempty"`
	Checks        []Check   `json:"checks"`
	Tried         []string  `json:"tried,omitempty"`
	ReportFile    string    `json:"report_file,omitempty"`
	SupportPrompt string    `json:"support_prompt,omitempty"`
}

func (r Report) HasFail() bool {
	for _, c := range r.Checks {
		if c.Status == StatusFail {
			return true
		}
	}
	return false
}

func (r Report) HasWarn() bool {
	for _, c := range r.Checks {
		if c.Status == StatusWarn {
			return true
		}
	}
	return false
}

func finalize(r Report) Report {
	r.Checks = redactChecks(r.Checks)
	r.Tried = redactList(r.Tried)
	r.ConfigPath = Redact(r.ConfigPath)
	r.GatewayURL = Redact(r.GatewayURL)
	r.DesktopTarget = Redact(r.DesktopTarget)
	fails, warns := 0, 0
	for _, c := range r.Checks {
		switch c.Status {
		case StatusFail:
			fails++
		case StatusWarn:
			warns++
		}
	}
	switch {
	case fails > 0:
		r.Verdict = VerdictNotReady
		r.Summary = "Not fully working yet. Follow the fix next to each FAIL, or copy the support prompt and send it to whoever gave you this app."
	case warns > 0:
		r.Verdict = VerdictPartial
		r.Summary = "Core setup can run, but something needs attention. Chat may still work. See WARN lines below."
	default:
		r.Verdict = VerdictReady
		r.Summary = "Set up and working. Quit and reopen Claude Desktop if it was already open, then chat."
	}
	r.SupportPrompt = FormatPrompt(r)
	return r
}

func FormatHuman(r Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "doctor:\n")
	fmt.Fprintf(&b, " - verdict: %s\n", r.Verdict)
	fmt.Fprintf(&b, " - env: %s/%s gateway=%s\n", r.GOOS, r.GOARCH, FormatVersion())
	for _, c := range r.Checks {
		line := fmt.Sprintf(" - %s: %s (%s)", c.ID, c.Status, c.Detail)
		b.WriteString(line)
		b.WriteByte('\n')
		if c.Fix != "" && c.Status != StatusOK {
			fmt.Fprintf(&b, "     fix: %s\n", c.Fix)
		}
	}
	if r.ReportFile != "" {
		fmt.Fprintf(&b, " - report: %s\n", r.ReportFile)
	}
	return b.String()
}

func FormatPrompt(r Report) string {
	var b strings.Builder
	b.WriteString("You are debugging claude-gateway (https://github.com/Danakolana/claude-gateway) for a colleague who only has the binary and the README install steps.\n")
	b.WriteString("Do not ask them to reinstall blindly. Use this report. Give the smallest fix (exact commands, env vars, paths). If the bug is in claude-gateway itself, say what to change in the Go code.\n\n")
	fmt.Fprintf(&b, "## Verdict\n%s — %s\n\n", r.Verdict, r.Summary)
	fmt.Fprintf(&b, "## Environment\n- OS: %s/%s\n- Gateway version: %s\n- Time (UTC): %s\n",
		r.GOOS, r.GOARCH, FormatVersion(), r.GeneratedAt.UTC().Format(time.RFC3339))
	if r.ConfigPath != "" {
		fmt.Fprintf(&b, "- Config: %s\n", r.ConfigPath)
	}
	if r.GatewayURL != "" {
		fmt.Fprintf(&b, "- Gateway URL: %s\n", r.GatewayURL)
	}
	if r.DesktopTarget != "" {
		fmt.Fprintf(&b, "- Desktop target: %s\n", r.DesktopTarget)
	}
	b.WriteString("\n## Checks\n")
	for _, c := range r.Checks {
		fmt.Fprintf(&b, "- %s (%s): %s [%s]", c.ID, c.Title, c.Status, c.Detail)
		if c.Fix != "" && c.Status != StatusOK {
			fmt.Fprintf(&b, " | fix: %s", c.Fix)
		}
		b.WriteByte('\n')
	}
	if len(r.Tried) > 0 {
		b.WriteString("\n## What we already tried\n")
		for _, t := range r.Tried {
			fmt.Fprintf(&b, "- %s\n", t)
		}
	}
	b.WriteString("\n## Please\n")
	b.WriteString("1. One-sentence root cause.\n")
	b.WriteString("2. Exact commands or file edits to fix it on this OS.\n")
	b.WriteString("3. If a code change in claude-gateway is required, describe the patch.\n")
	return Redact(b.String())
}

func Persist(r Report) (Report, error) {
	dir, err := platform.GatewayDataDir()
	if err != nil {
		return r, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return r, err
	}
	txtPath := filepath.Join(dir, "last-doctor.txt")
	body := FormatHuman(r) + "\n----- support prompt -----\n" + r.SupportPrompt + "\n"
	if err := platform.WriteFileAtomic(txtPath, []byte(body), 0o600); err != nil {
		return r, err
	}
	r.ReportFile = txtPath
	js, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return r, nil
	}
	_ = platform.WriteFileAtomic(filepath.Join(dir, "last-doctor.json"), js, 0o600)
	return r, nil
}

var (
	bearerRE = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._\-+=/]{8,}`)
	keyRE    = regexp.MustCompile(`(?i)\b(sk-[a-z0-9_\-]{8,}|or-v1-[a-z0-9_\-]{8,}|sk-or-[a-z0-9_\-]{8,}|sk-ant-[a-z0-9_\-]{8,})\b`)
	assignRE = regexp.MustCompile(`(?i)(api[_-]?key|authorization|token|secret|password)(["']?\s*[:=]\s*["']?)[^\s"',}]+`)
)

// Redact strips secret-shaped tokens from diagnostic text.
func Redact(s string) string {
	if s == "" {
		return s
	}
	s = bearerRE.ReplaceAllString(s, "${1}<redacted>")
	s = keyRE.ReplaceAllString(s, "<redacted-key>")
	s = assignRE.ReplaceAllString(s, "${1}<redacted>")
	return s
}

func redactChecks(in []Check) []Check {
	out := make([]Check, len(in))
	for i, c := range in {
		c.Detail = Redact(c.Detail)
		c.Fix = Redact(c.Fix)
		c.Title = Redact(c.Title)
		out[i] = c
	}
	return out
}

func redactList(in []string) []string {
	if len(in) == 0 {
		return in
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = Redact(s)
	}
	return out
}
