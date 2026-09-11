package diagnose

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/danakolana/claude-gateway/internal/clientintegration"
	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/internal/history"
	"github.com/danakolana/claude-gateway/internal/platform"
	"github.com/danakolana/claude-gateway/internal/provider/custom"
	"github.com/danakolana/claude-gateway/internal/secrets"
)

// Options controls which live probes Collect runs. Never put secret values here.
type Options struct {
	Config        *config.File
	ConfigPath    string
	ConfigErr     error
	Resolver      secrets.Resolver
	GatewayURL    string
	DesktopTarget string
	AppliedPaths  []string
	ApplyErr      string
	ListenNote    string
	CatalogOK     bool
	CatalogErr    string
	ProbeProvider bool
	Fake          bool
	Direct        bool
	Tried         []string
}

// Collect builds a redacted readiness report. Live provider probes are optional.
func Collect(opts Options) Report {
	r := Report{
		GeneratedAt:   time.Now().UTC(),
		Version:       ToolVersion,
		GOOS:          runtime.GOOS,
		GOARCH:        runtime.GOARCH,
		ConfigPath:    opts.ConfigPath,
		GatewayURL:    opts.GatewayURL,
		DesktopTarget: opts.DesktopTarget,
		Tried:         append([]string{}, opts.Tried...),
	}
	if opts.ConfigErr != nil {
		r.Checks = append(r.Checks, Check{
			ID: "config", Title: "Configuration", Status: StatusFail,
			Detail: opts.ConfigErr.Error(),
			Fix:    "Fix the TOML, or delete it and re-run so the default is written again. Path: --config or CLAUDE_GATEWAY_CONFIG.",
		})
		return finalize(r)
	}
	cfg := opts.Config
	if cfg == nil {
		r.Checks = append(r.Checks, Check{
			ID: "config", Title: "Configuration", Status: StatusFail,
			Detail: "no configuration loaded",
			Fix:    "Re-run ./claude-gateway so it writes the default config, or pass --config PATH.",
		})
		return finalize(r)
	}

	collectConfig(&r, cfg, opts.ConfigPath)
	collectAPIKey(&r, cfg, opts.Resolver, opts.Fake)
	collectProvider(&r, cfg, opts)
	collectDesktop(&r, cfg, opts)
	collectProxy(&r, opts)
	collectHistory(&r, cfg)
	return finalize(r)
}

func collectConfig(r *Report, cfg *config.File, src string) {
	errs := config.Validate(cfg)
	if len(errs) > 0 {
		var parts []string
		for _, e := range errs {
			parts = append(parts, e.Error())
		}
		r.Checks = append(r.Checks, Check{
			ID: "config", Title: "Configuration", Status: StatusFail,
			Detail: fmt.Sprintf("%d errors source=%s: %s", len(errs), src, strings.Join(parts, "; ")),
			Fix:    "Edit the config file (see path above) or replace it with the shipped default.",
		})
		return
	}
	r.Checks = append(r.Checks, Check{
		ID: "config", Title: "Configuration", Status: StatusOK,
		Detail: fmt.Sprintf("%s profile=%s", src, cfg.ActiveProfile),
	})
}

func collectAPIKey(r *Report, cfg *config.File, resolver secrets.Resolver, fake bool) {
	if fake {
		r.Checks = append(r.Checks, Check{
			ID: "api_key", Title: "API key", Status: StatusOK,
			Detail: "--fake in-memory provider (no key required)",
		})
		return
	}
	prof, ok := cfg.Profiles[cfg.ActiveProfile]
	if !ok {
		r.Checks = append(r.Checks, Check{
			ID: "api_key", Title: "API key", Status: StatusFail,
			Detail: "active profile missing",
			Fix:    "Set active_profile in config.toml to an existing [profiles.*] name.",
		})
		return
	}
	prov, ok := cfg.Providers[prof.Provider]
	if !ok {
		r.Checks = append(r.Checks, Check{
			ID: "api_key", Title: "API key", Status: StatusFail,
			Detail: fmt.Sprintf("provider %q not defined", prof.Provider),
			Fix:    "Add [providers." + prof.Provider + "] with api_key_env.",
		})
		return
	}
	h := prov.APIKeyHandle()
	envName := h.Ref
	if envName == "" {
		envName = "OPENROUTER_API_KEY"
	}
	if resolver == nil {
		resolver = secrets.EnvResolver{}
	}
	_, err := resolver.Resolve(h)
	if err != nil {
		r.Checks = append(r.Checks, Check{
			ID: "api_key", Title: "API key", Status: StatusFail,
			Detail: err.Error(),
			Fix:    fmt.Sprintf("Export %s (OpenRouter: https://openrouter.ai/keys) in the same terminal, then re-run.", envName),
		})
		return
	}
	r.Checks = append(r.Checks, Check{
		ID: "api_key", Title: "API key", Status: StatusOK,
		Detail: fmt.Sprintf("%s is set", envName),
	})
}

func collectProvider(r *Report, cfg *config.File, opts Options) {
	if opts.Fake {
		r.Checks = append(r.Checks, Check{
			ID: "provider", Title: "Provider", Status: StatusOK,
			Detail: "fake adapter",
		})
		return
	}
	if opts.CatalogOK {
		r.Checks = append(r.Checks, Check{
			ID: "provider", Title: "Provider", Status: StatusOK,
			Detail: "live model catalog fetched",
		})
		return
	}
	if opts.CatalogErr != "" && !opts.ProbeProvider {
		r.Checks = append(r.Checks, Check{
			ID: "provider", Title: "Provider", Status: StatusWarn,
			Detail: opts.CatalogErr,
			Fix:    "Check network / VPN / OPENROUTER_API_KEY. Chat may still work if the key is valid.",
		})
		return
	}
	if !opts.ProbeProvider {
		r.Checks = append(r.Checks, Check{
			ID: "provider", Title: "Provider", Status: StatusWarn,
			Detail: "live probe skipped (--offline)",
			Fix:    "Re-run without --offline to test the API key against the provider.",
		})
		return
	}
	prof, ok := cfg.Profiles[cfg.ActiveProfile]
	if !ok {
		return
	}
	prov, ok := cfg.Providers[prof.Provider]
	if !ok {
		return
	}
	if opts.Resolver == nil {
		opts.Resolver = secrets.EnvResolver{}
	}
	key, err := opts.Resolver.Resolve(prov.APIKeyHandle())
	if err != nil {
		r.Checks = append(r.Checks, Check{
			ID: "provider", Title: "Provider", Status: StatusFail,
			Detail: "cannot probe without API key",
			Fix:    "Set the provider API key env var, then re-run doctor.",
		})
		return
	}
	ad := custom.New(prov.BaseURL, key, prov.AuthScheme, prov.Headers, true, 4*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	h := ad.Health(ctx)
	msg := h.Message
	low := strings.ToLower(msg)
	switch {
	case h.OK:
		r.Checks = append(r.Checks, Check{
			ID: "provider", Title: "Provider", Status: StatusOK,
			Detail: fmt.Sprintf("%s %s", prof.Provider, msg),
		})
	case strings.Contains(low, "401") || strings.Contains(low, "403"):
		r.Checks = append(r.Checks, Check{
			ID: "provider", Title: "Provider", Status: StatusFail,
			Detail: fmt.Sprintf("%s rejected the API key (%s)", prof.Provider, msg),
			Fix:    "Create a new OpenRouter key and export it in this terminal. Do not paste the key into chat.",
		})
	default:
		r.Checks = append(r.Checks, Check{
			ID: "provider", Title: "Provider", Status: StatusWarn,
			Detail: fmt.Sprintf("%s unreachable: %s", prof.Provider, msg),
			Fix:    "Check internet, proxy/VPN, and providers.*.base_url. Chat may work later if the key is valid.",
		})
	}
}

func collectDesktop(r *Report, cfg *config.File, opts Options) {
	target := opts.DesktopTarget
	if target == "" {
		target = cfg.Client.DesktopTarget()
	}
	existing := platform.ExistingClaudeDesktopConfigPaths(target)
	cands := platform.ClaudeDesktop3PConfigPaths()
	if strings.EqualFold(target, "consumer") {
		cands = platform.ClaudeDesktopConsumerConfigPaths()
	}
	if opts.ApplyErr != "" {
		r.Checks = append(r.Checks, Check{
			ID: "desktop_apply", Title: "Desktop apply", Status: StatusFail,
			Detail: opts.ApplyErr,
			Fix:    "Quit Claude Desktop, then run: claude-gateway client apply --desktop " + target,
		})
	}

	expected := strings.TrimRight(strings.TrimSpace(opts.GatewayURL), "/")
	matched := 0
	var seen []string
	inspectPaths := uniqueStrings(append(append([]string{}, existing...), opts.AppliedPaths...))
	for _, p := range inspectPaths {
		st := clientintegration.Inspect(p)
		url := st.EffectiveBaseURL()
		line := p
		if url != "" {
			line += " url=" + url
		}
		seen = append(seen, line)
		if expected != "" && clientintegration.SameGatewayURL(url, expected) {
			matched++
		}
	}

	switch {
	case expected != "" && matched > 0:
		r.Checks = append(r.Checks, Check{
			ID: "client", Title: "Claude Desktop", Status: StatusOK,
			Detail: fmt.Sprintf("target=%s gateway URL written in %d location(s)", target, matched),
		})
	case len(opts.AppliedPaths) > 0 && expected != "":
		r.Checks = append(r.Checks, Check{
			ID: "client", Title: "Claude Desktop", Status: StatusWarn,
			Detail: fmt.Sprintf("wrote %s but the file does not show gateway %s yet", strings.Join(opts.AppliedPaths, ", "), expected),
			Fix:    "Quit Claude Desktop completely and re-run. If it still fails, copy the support prompt.",
		})
	case len(existing) > 0 && expected != "":
		r.Checks = append(r.Checks, Check{
			ID: "client", Title: "Claude Desktop", Status: StatusWarn,
			Detail: fmt.Sprintf("found Desktop at %s but gateway URL is not %s", strings.Join(seen, " | "), expected),
			Fix:    "Run: claude-gateway client apply   then quit and reopen Claude Desktop.",
		})
	case len(existing) > 0:
		r.Checks = append(r.Checks, Check{
			ID: "client", Title: "Claude Desktop", Status: StatusOK,
			Detail: fmt.Sprintf("found %s", existing[0]),
		})
		if len(existing) > 1 {
			r.Checks = append(r.Checks, Check{
				ID: "client_layouts", Title: "Extra Desktop layouts", Status: StatusWarn,
				Detail: fmt.Sprintf("%d extra data dirs: %s", len(existing)-1, strings.Join(existing[1:], ", ")),
				Fix:    "Start without --no-apply so every existing layout is written, or pass --desktop 3p|consumer.",
			})
		}
	default:
		primary := ""
		if len(cands) > 0 {
			primary = cands[0]
		}
		r.Checks = append(r.Checks, Check{
			ID: "client", Title: "Claude Desktop", Status: StatusWarn,
			Detail: fmt.Sprintf("no Claude Desktop data dir yet; official path is %s (%d candidates)", primary, len(cands)),
			Fix:    "Install Claude Desktop (3P / developer build if you can). Re-run this app, then open Desktop once so it creates its folder.",
		})
	}
}

func collectProxy(r *Report, opts Options) {
	if opts.Direct {
		r.Checks = append(r.Checks, Check{
			ID: "proxy", Title: "Local proxy", Status: StatusOK,
			Detail: "direct mode (Desktop talks to the provider; no local proxy)",
		})
		return
	}
	if opts.ListenNote != "" {
		r.Tried = append(r.Tried, opts.ListenNote)
	}
	url := strings.TrimRight(strings.TrimSpace(opts.GatewayURL), "/")
	if url == "" {
		r.Checks = append(r.Checks, Check{
			ID: "proxy", Title: "Local proxy", Status: StatusWarn,
			Detail: "proxy not running in this command",
			Fix:    "Start with: ./claude-gateway   and leave that terminal open.",
		})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var res *http.Response
	var err error
	for i := 0; i < 8; i++ {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, url+"/health", nil)
		if reqErr != nil {
			err = reqErr
			break
		}
		res, err = http.DefaultClient.Do(req)
		if err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		if strings.Contains(err.Error(), "invalid URL") || strings.Contains(err.Error(), "missing protocol") {
			r.Checks = append(r.Checks, Check{
				ID: "proxy", Title: "Local proxy", Status: StatusFail,
				Detail: err.Error(),
				Fix:    "Re-run ./claude-gateway. If the port is in use, it will pick the next free port automatically.",
			})
			return
		}
		r.Checks = append(r.Checks, Check{
			ID: "proxy", Title: "Local proxy", Status: StatusWarn,
			Detail: fmt.Sprintf("%s health: %v", url, err),
			Fix:    "Leave the gateway terminal open. Retry /health or copy the support prompt.",
		})
		return
	}
	_ = res.Body.Close()
	if res.StatusCode >= 400 {
		r.Checks = append(r.Checks, Check{
			ID: "proxy", Title: "Local proxy", Status: StatusFail,
			Detail: fmt.Sprintf("%s health status %d", url, res.StatusCode),
			Fix:    "Copy the support prompt; the local server started but /health is not OK.",
		})
		return
	}
	detail := url + "/health OK"
	if opts.ListenNote != "" {
		detail += "; " + opts.ListenNote
	}
	r.Checks = append(r.Checks, Check{
		ID: "proxy", Title: "Local proxy", Status: StatusOK,
		Detail: detail,
	})
}

func collectHistory(r *Report, cfg *config.File) {
	dataDir, _ := platform.GatewayDataDir()
	histPath := filepath.Join(dataDir, "history.db")
	if cfg.History.LocalDatabase != "" {
		histPath = expandHome(cfg.History.LocalDatabase)
	}
	store, err := history.Open(histPath)
	if err != nil {
		r.Checks = append(r.Checks, Check{
			ID: "history", Title: "Local history", Status: StatusWarn,
			Detail: fmt.Sprintf("unavailable at %s; proxy can still run", histPath),
			Fix:    "Chat still works. To persist history, fix permissions on that path.",
		})
		return
	}
	_ = store.Close()
	r.Checks = append(r.Checks, Check{
		ID: "history", Title: "Local history", Status: StatusOK,
		Detail: histPath,
	})
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~"+string(os.PathSeparator)) || p == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, p := range in {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}
