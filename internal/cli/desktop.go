package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/internal/platform"
)

func parseDesktopFlag(v string, stderr io.Writer) (config.Client, int) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "3p", "1":
		return config.Client{Desktop: "3p"}, ExitOK
	case "consumer", "2", "stable", "regular":
		return config.Client{Desktop: "consumer", AllowExperimental: true}, ExitOK
	default:
		fmt.Fprintf(stderr, "invalid --desktop %q (use 3p or consumer)\n", v)
		return config.Client{}, ExitUsage
	}
}

// pickDesktopClient chooses 3P vs consumer from what already exists on disk.
func pickDesktopClient(has3p, hasConsumer bool) (config.Client, string) {
	switch {
	case has3p && hasConsumer:
		return config.Client{Desktop: "3p"},
			"Auto-detected Claude Desktop 3P (regular Desktop also exists; pass --desktop consumer to use it)."
	case has3p:
		return config.Client{Desktop: "3p"}, "Auto-detected Claude Desktop 3P."
	case hasConsumer:
		return config.Client{Desktop: "consumer", AllowExperimental: true},
			"Auto-detected regular Claude Desktop — using experimental consumer apply. Prefer 3P if you can enable it. Override with --desktop 3p."
	default:
		return config.Client{Desktop: "3p"},
			"No Claude Desktop data dir yet — will use 3P (recommended) and create the official path."
	}
}

func resolveDesktopTarget(args []string, stdin io.Reader, stdout, stderr io.Writer) (config.Client, int) {
	if v, err := flagValue(args, "--desktop"); err == nil && strings.TrimSpace(v) != "" {
		c, code := parseDesktopFlag(v, stderr)
		if code == ExitOK {
			fmt.Fprintf(stdout, "Desktop target: %s (--desktop)\n", c.DesktopTarget())
		}
		return c, code
	}
	if hasFlag(args, "--ask-desktop") {
		return promptDesktopTarget(stdin, stdout, stderr)
	}
	has3p := len(platform.ExistingClaudeDesktopConfigPaths("3p")) > 0
	hasC := len(platform.ExistingClaudeDesktopConfigPaths("consumer")) > 0
	c, reason := pickDesktopClient(has3p, hasC)
	if reason != "" {
		fmt.Fprintln(stdout, reason)
	}
	return c, ExitOK
}

// desktopApplyPaths returns every existing layout for target, or the official
// primary path if none exist yet. An explicit clientPath wins.
func desktopApplyPaths(target, clientPath string) []string {
	if strings.TrimSpace(clientPath) != "" {
		return []string{clientPath}
	}
	existing := platform.ExistingClaudeDesktopConfigPaths(target)
	if len(existing) > 0 {
		return existing
	}
	p := platform.DiscoverClaudeDesktopConfigFor(target)
	if p == "" {
		data, _ := platform.GatewayDataDir()
		p = filepath.Join(data, "claude_desktop_config.json")
	}
	return []string{p}
}
