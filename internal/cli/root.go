package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/danakolana/claude-gateway/internal/clientintegration"
	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/internal/history"
	"github.com/danakolana/claude-gateway/internal/modelstatus"
	"github.com/danakolana/claude-gateway/internal/platform"
	"github.com/danakolana/claude-gateway/internal/provider"
	"github.com/danakolana/claude-gateway/internal/provider/custom"
	"github.com/danakolana/claude-gateway/internal/provider/fake"
	"github.com/danakolana/claude-gateway/internal/provider/nine"
	"github.com/danakolana/claude-gateway/internal/provider/openrouter"
	"github.com/danakolana/claude-gateway/internal/proxy"
	"github.com/danakolana/claude-gateway/internal/routing"
	"github.com/danakolana/claude-gateway/internal/secrets"
	"github.com/danakolana/claude-gateway/pkg/api"
)

const (
	ExitOK            = 0
	ExitUsage         = 2
	ExitInvalidConfig = 3
	ExitInternal      = 1
	ExitUnavailable   = 4
)

func Run(args []string) int {
	return RunWith(args, os.Stdout, os.Stderr, secrets.EnvResolver{})
}

func RunWith(args []string, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		printHelp(stdout)
		return ExitOK
	}
	// Default / first-run: load TOML, apply Desktop, start proxy.
	if len(args) == 0 || args[0] == "start" || strings.HasPrefix(args[0], "-") {
		startArgs := args
		if len(args) > 0 && args[0] == "start" {
			startArgs = args[1:]
		}
		return runStart(startArgs, stdout, stderr, resolver)
	}
	switch args[0] {
	case "config":
		return runConfig(args[1:], stdout, stderr, resolver)
	case "profile":
		return runProfile(args[1:], stdout, stderr, resolver)
	case "proxy":
		return runProxy(args[1:], stdout, stderr, resolver)
	case "client":
		return runClient(args[1:], stdout, stderr, resolver)
	case "history":
		return runHistory(args[1:], stdout, stderr, resolver)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr, resolver)
	case "provider":
		return runProvider(args[1:], stdout, stderr, resolver)
	case "models":
		return runModels(args[1:], stdout, stderr, resolver)
	case "version":
		fmt.Fprintln(stdout, "claude-gateway 0.1.0")
		return ExitOK
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		printHelp(stderr)
		return ExitUsage
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, `claude-gateway — Claude Desktop Gateway CLI

First run (recommended):
  export OPENROUTER_API_KEY=sk-or-...
  ./claude-gateway

Loads config.toml (or ./examples/config.toml), applies Claude Desktop on 3P
settings, and starts the local Anthropic proxy (unless [proxy] mode = "direct").
Listen address comes from [proxy] listen in TOML (default 127.0.0.1:8080).

[proxy] mode:
  local   Desktop → local proxy → OpenRouter (default)
  direct  Desktop → OpenRouter Anthropic API (no local proxy; for comparison)

Optional flags on start:
  --config PATH       config file
  --listen HOST:PORT  override [proxy].listen
  --no-apply          skip writing Desktop config
  --fake              use in-memory provider (no API key)

Other commands:
  start                 same as bare ./claude-gateway
  config validate|explain
  profile list|create|select|delete
  proxy start|health
  client discover|diff|apply|restore
  history list|export
  models list|status
  provider health
  doctor
  version

Env: OPENROUTER_API_KEY (or provider api_key_env), CLAUDE_GATEWAY_CONFIG,
     CLAUDE_GATEWAY_LISTEN, CLAUDE_GATEWAY_ACTIVE_PROFILE`)
}

func runConfig(args []string, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: config <validate|explain>")
		return ExitUsage
	}
	switch args[0] {
	case "validate":
		return configValidate(args[1:], stdout, stderr, resolver)
	case "explain":
		return configExplain(args[1:], stdout, stderr, resolver)
	default:
		fmt.Fprintf(stderr, "unknown config subcommand %q\n", args[0])
		return ExitUsage
	}
}

func configValidate(args []string, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	path, _ := flagValue(args, "--config")
	cfg, src, err := config.Load(path, resolver)
	if err != nil {
		fmt.Fprintf(stderr, "load error: %v\n", err)
		return ExitInvalidConfig
	}
	errs := config.Validate(cfg)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(stderr, e.Error())
		}
		return ExitInvalidConfig
	}
	fmt.Fprintf(stdout, "OK: configuration valid (source=%s, profile=%s)\n", src, cfg.ActiveProfile)
	return ExitOK
}

func configExplain(args []string, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	path, _ := flagValue(args, "--config")
	cfg, src, err := config.Load(path, resolver)
	if err != nil {
		fmt.Fprintf(stderr, "load error: %v\n", err)
		return ExitInvalidConfig
	}
	eff, err := config.Resolve(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "resolve error: %v\n", err)
		return ExitInvalidConfig
	}
	fmt.Fprint(stdout, config.Explain(eff, src))
	return ExitOK
}

func runProfile(args []string, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: profile <list|create|select|delete>")
		return ExitUsage
	}
	path, _ := flagValue(args[1:], "--config")
	switch args[0] {
	case "list":
		cfg, _, err := config.Load(path, resolver)
		if err != nil {
			fmt.Fprintf(stderr, "load error: %v\n", err)
			return ExitInvalidConfig
		}
		for _, n := range config.ProfileNames(cfg) {
			mark := " "
			if n == cfg.ActiveProfile {
				mark = "*"
			}
			fmt.Fprintf(stdout, "%s %s\n", mark, n)
		}
		return ExitOK
	case "create":
		name, _ := flagValue(args[1:], "--name")
		if name == "" {
			fmt.Fprintln(stderr, "usage: profile create --name <name>")
			return ExitUsage
		}
		if err := config.CreateProfile(path, name); err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return ExitInvalidConfig
		}
		fmt.Fprintf(stdout, "created profile %q\n", name)
		return ExitOK
	case "select":
		name, _ := flagValue(args[1:], "--name")
		if err := config.SelectProfile(path, name); err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return ExitInvalidConfig
		}
		fmt.Fprintf(stdout, "selected profile %q\n", name)
		return ExitOK
	case "delete":
		name, _ := flagValue(args[1:], "--name")
		if err := config.DeleteProfile(path, name); err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return ExitInvalidConfig
		}
		fmt.Fprintf(stdout, "deleted profile %q\n", name)
		return ExitOK
	default:
		return ExitUsage
	}
}

func runProxy(args []string, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: proxy <start|health>")
		return ExitUsage
	}
	switch args[0] {
	case "start":
		return runStart(append([]string{"--no-apply"}, args[1:]...), stdout, stderr, resolver)
	case "health":
		addr, _ := flagValue(args[1:], "--addr")
		if addr == "" {
			addr, _ = flagValue(args[1:], "--listen")
		}
		if addr == "" {
			path, _ := flagValue(args[1:], "--config")
			if cfg, _, err := config.Load(path, resolver); err == nil {
				addr = cfg.Proxy.Addr()
			}
		}
		if addr == "" {
			addr = "127.0.0.1:8080"
		}
		res, err := http.Get("http://" + addr + "/health")
		if err != nil {
			fmt.Fprintf(stderr, "health failed: %v\n", err)
			return ExitUnavailable
		}
		defer res.Body.Close()
		fmt.Fprintf(stdout, "OK status=%d\n", res.StatusCode)
		return ExitOK
	default:
		return ExitUsage
	}
}

// runStart is the default entrypoint: optional Desktop apply + proxy listen
// (or apply-only in [proxy] mode = "direct").
func runStart(args []string, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	path, _ := flagValue(args, "--config")
	listenFlag, _ := flagValue(args, "--listen")
	if listenFlag == "" {
		listenFlag, _ = flagValue(args, "--addr") // backward compatible
	}
	useFake := hasFlag(args, "--fake")
	noApply := hasFlag(args, "--no-apply")

	cfg, src, err := config.Load(path, resolver)
	if err != nil {
		fmt.Fprintf(stderr, "load error: %v\n", err)
		return ExitInvalidConfig
	}
	if errs := config.Validate(cfg); len(errs) > 0 && !useFake {
		for _, e := range errs {
			fmt.Fprintln(stderr, e.Error())
		}
		return ExitInvalidConfig
	}
	fmt.Fprintf(stdout, "config: %s (profile=%s)\n", src, cfg.ActiveProfile)

	prof := cfg.Profiles[cfg.ActiveProfile]
	prov := cfg.Providers[prof.Provider]

	if cfg.Proxy.IsDirect() {
		if useFake {
			fmt.Fprintln(stderr, "direct mode cannot use --fake (no local proxy)")
			return ExitUsage
		}
		gatewayURL := config.DirectGatewayBaseURL(cfg.Proxy, prov)
		fmt.Fprintf(stdout, "mode: direct → %s (OpenRouter Anthropic API, no local proxy)\n", gatewayURL)
		if !noApply && cfg.Proxy.ShouldApplyDesktop() {
			if code := applyDesktopConfig(cfg, gatewayURL, "", false, true, stdout, stderr, resolver); code != ExitOK {
				return code
			}
		} else {
			fmt.Fprintln(stdout, "desktop apply: skipped")
		}
		fmt.Fprintln(stdout, "Direct mode ready. Restart Claude Desktop / Apply Changes, then compare.")
		fmt.Fprintln(stdout, "Switch back with: [proxy] mode = \"local\" and re-run ./claude-gateway")
		return ExitOK
	}

	addr := listenFlag
	if addr == "" {
		addr = cfg.Proxy.Addr()
	}
	proxyURL := "http://" + addr
	fmt.Fprintf(stdout, "mode: local → %s\n", proxyURL)

	if !noApply && cfg.Proxy.ShouldApplyDesktop() {
		if code := applyDesktopConfig(cfg, proxyURL, "", false, false, stdout, stderr, resolver); code != ExitOK {
			return code
		}
	} else {
		fmt.Fprintln(stdout, "desktop apply: skipped")
	}

	return proxyListen(cfg, addr, useFake, stdout, stderr, resolver)
}

func applyDesktopConfig(cfg *config.File, gatewayURL, clientPath string, dry, direct bool, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	prof := cfg.Profiles[cfg.ActiveProfile]
	prov := cfg.Providers[prof.Provider]
	key, err := resolver.Resolve(prov.APIKeyHandle())
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return ExitInvalidConfig
	}
	auth := prov.AuthScheme
	if auth == "" {
		auth = "bearer"
	}
	picker := config.DesktopPickerEntries(cfg.Models)
	entries := make([]clientintegration.InferenceModelEntry, 0, len(picker))
	for _, e := range picker {
		name := e.DesktopID
		if direct {
			// OpenRouter Anthropic skin routes by real model_id.
			name = e.ModelID
		}
		entries = append(entries, clientintegration.InferenceModelEntry{
			Name: name, LabelOverride: e.DesktopLabel,
			AnthropicFamilyTier: e.DesktopTier, IsFamilyDefault: e.IsDefault,
		})
	}
	cand := clientintegration.Render3PEntries(gatewayURL, key, auth, entries, direct)
	if clientPath == "" {
		clientPath = platform.DiscoverClaudeDesktopConfig()
		if clientPath == "" {
			data, _ := platform.GatewayDataDir()
			clientPath = filepath.Join(data, "claude_desktop_config.json")
			fmt.Fprintf(stderr, "WARN: no existing Desktop config; will write %s\n", clientPath)
		}
	}
	backupDir := filepath.Join(filepath.Dir(clientPath), "claude-gateway-backups")
	snap, err := clientintegration.Apply(clientPath, cand, backupDir, cfg.ActiveProfile, "0.1.0", dry)
	if err != nil {
		fmt.Fprintf(stderr, "desktop apply: %v\n", err)
		return ExitInternal
	}
	if dry {
		fmt.Fprintln(stdout, "desktop apply: dry-run (no writes)")
	} else {
		fmt.Fprintf(stdout, "desktop apply: %s (backup=%s)\n", clientPath, snap.BackupPath)
		fmt.Fprintf(stdout, "gateway base URL: %s\n", gatewayURL)
		fmt.Fprintln(stdout, "Open Claude Desktop → click Apply Changes if prompted.")
	}
	return ExitOK
}

func proxyListen(cfg *config.File, addr string, useFake bool, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	prof := cfg.Profiles[cfg.ActiveProfile]
	var ad provider.Adapter
	if useFake {
		ad = &fake.Adapter{}
	} else {
		prov := cfg.Providers[prof.Provider]
		key, err := resolver.Resolve(prov.APIKeyHandle())
		if err != nil {
			fmt.Fprintf(stderr, "secret: %v\n", err)
			return ExitInvalidConfig
		}
		tlsVerify := true
		if prov.TLSVerify != nil {
			tlsVerify = *prov.TLSVerify
			if !tlsVerify {
				fmt.Fprintln(stderr, "WARN: tls_verify=false for provider", prof.Provider)
			}
		}
		timeout := time.Duration(prov.TimeoutSeconds) * time.Second
		switch {
		case strings.Contains(strings.ToLower(prof.Provider), "openrouter"):
			ad = openrouter.New(prov.BaseURL, key, prov.Headers, tlsVerify, timeout)
		case strings.Contains(strings.ToLower(prof.Provider), "nine"):
			ad = nine.New(prov.BaseURL, key, tlsVerify, timeout)
		default:
			ad = custom.New(prov.BaseURL, key, prov.AuthScheme, prov.Headers, tlsVerify, timeout)
		}
	}
	eng := &routing.Engine{
		Registry: routing.NewRegistry(cfg.Models),
		Adapter:  ad,
		Provider: prof.Provider,
		Routing:  prof.Routing,
	}
	dataDir, _ := platform.GatewayDataDir()
	histPath := filepath.Join(dataDir, "history.db")
	if cfg.History.LocalDatabase != "" {
		histPath = expandHome(cfg.History.LocalDatabase)
	}
	store, err := history.Open(histPath)
	if err != nil {
		fmt.Fprintf(stderr, "history: %v\n", err)
		return ExitInternal
	}
	defer store.Close()

	srvCfg := proxy.Config{Addr: addr, OnRequest: func(req api.Request, resp api.Response) {
		status := "completed"
		if resp.Error != nil || resp.FinishReason == api.FinishCancelled || resp.FinishReason == api.FinishError {
			status = "incomplete"
		}
		_ = store.AppendConversation(context.Background(), req.ID, status, []map[string]any{
			{"role": "user", "content": req.Messages, "correlation_id": req.ID},
			{"role": "assistant", "content": resp.Content, "correlation_id": req.ID},
		}, map[string]any{
			"provider": eng.Provider, "source_model": req.SourceModel, "target_model": resp.Model,
			"outcome": status, "usage": resp.Usage,
		})
		_ = store.Audit("proxy.request", cfg.ActiveProfile, status, req.ID)
	}}
	srv := proxy.New(srvCfg, eng)
	bound, err := srv.Start()
	if err != nil {
		fmt.Fprintf(stderr, "start failed: %v\n", err)
		return ExitInternal
	}
	fmt.Fprintf(stdout, "proxy listening on http://%s\n", bound)
	fmt.Fprintf(stdout, "Anthropic Messages API: POST http://%s/v1/messages\n", bound)
	fmt.Fprintf(stdout, "health: http://%s/health\n", bound)
	fmt.Fprintf(stdout, "history: %s\n", histPath)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shCtx)
	fmt.Fprintln(stdout, "proxy stopped")
	return ExitOK
}

func runClient(args []string, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: client <discover|diff|apply|restore>")
		return ExitUsage
	}
	switch args[0] {
	case "discover":
		p := platform.DiscoverClaudeDesktopConfig()
		if p == "" {
			fmt.Fprintln(stdout, "no Claude Desktop config found; candidates:")
			for _, c := range platform.ClaudeDesktopConfigPaths() {
				fmt.Fprintln(stdout, " -", c)
			}
			return ExitOK
		}
		fmt.Fprintln(stdout, p)
		return ExitOK
	case "diff", "apply":
		path, _ := flagValue(args[1:], "--config")
		clientPath, _ := flagValue(args[1:], "--client-config")
		proxyURL, _ := flagValue(args[1:], "--proxy-url")
		cfg, _, err := config.Load(path, resolver)
		if err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return ExitInvalidConfig
		}
		direct := cfg.Proxy.IsDirect()
		if proxyURL == "" {
			if direct {
				prof := cfg.Profiles[cfg.ActiveProfile]
				proxyURL = config.DirectGatewayBaseURL(cfg.Proxy, cfg.Providers[prof.Provider])
			} else {
				proxyURL = cfg.Proxy.BaseURL()
			}
		}
		if args[0] == "diff" {
			prof := cfg.Profiles[cfg.ActiveProfile]
			prov := cfg.Providers[prof.Provider]
			key, err := resolver.Resolve(prov.APIKeyHandle())
			if err != nil {
				fmt.Fprintf(stderr, "%v\n", err)
				return ExitInvalidConfig
			}
			auth := prov.AuthScheme
			if auth == "" {
				auth = "bearer"
			}
			picker := config.DesktopPickerEntries(cfg.Models)
			entries := make([]clientintegration.InferenceModelEntry, 0, len(picker))
			for _, e := range picker {
				name := e.DesktopID
				if direct {
					name = e.ModelID
				}
				entries = append(entries, clientintegration.InferenceModelEntry{
					Name: name, LabelOverride: e.DesktopLabel,
					AnthropicFamilyTier: e.DesktopTier, IsFamilyDefault: e.IsDefault,
				})
			}
			cand := clientintegration.Render3PEntries(proxyURL, key, auth, entries, direct)
			fmt.Fprintln(stdout, clientintegration.RedactedDiff(cand))
			return ExitOK
		}
		dry := hasFlag(args[1:], "--dry-run")
		return applyDesktopConfig(cfg, proxyURL, clientPath, dry, direct, stdout, stderr, resolver)
	case "restore":
		clientPath, _ := flagValue(args[1:], "--client-config")
		backup, _ := flagValue(args[1:], "--backup")
		if clientPath == "" || backup == "" {
			fmt.Fprintln(stderr, "usage: client restore --client-config PATH --backup PATH")
			return ExitUsage
		}
		backupDir := filepath.Join(filepath.Dir(clientPath), "claude-gateway-backups")
		if err := clientintegration.Restore(clientPath, backup, backupDir, "", "0.1.0"); err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return ExitInternal
		}
		fmt.Fprintln(stdout, "restored")
		return ExitOK
	default:
		return ExitUsage
	}
}

func runHistory(args []string, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: history <list|export>")
		return ExitUsage
	}
	path, _ := flagValue(args[1:], "--config")
	cfg, _, err := config.Load(path, resolver)
	dataDir, _ := platform.GatewayDataDir()
	histPath := filepath.Join(dataDir, "history.db")
	if err == nil && cfg.History.LocalDatabase != "" {
		histPath = expandHome(cfg.History.LocalDatabase)
	}
	store, err := history.Open(histPath)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return ExitInternal
	}
	defer store.Close()
	switch args[0] {
	case "list":
		ids, err := store.ListConversations(50)
		if err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return ExitInternal
		}
		for _, id := range ids {
			fmt.Fprintln(stdout, id)
		}
		return ExitOK
	case "export":
		outDir, _ := flagValue(args[1:], "--out")
		if outDir == "" {
			outDir = filepath.Join(dataDir, "export")
		}
		if err := store.ExportJSONL(outDir); err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return ExitInternal
		}
		fmt.Fprintln(stdout, "exported to", outDir)
		return ExitOK
	default:
		return ExitUsage
	}
}

func runProvider(args []string, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	if len(args) == 0 || args[0] != "health" {
		fmt.Fprintln(stderr, "usage: provider health [--config PATH] [--fake]")
		return ExitUsage
	}
	if hasFlag(args[1:], "--fake") {
		fmt.Fprintln(stdout, "fake: ok")
		return ExitOK
	}
	path, _ := flagValue(args[1:], "--config")
	cfg, _, err := config.Load(path, resolver)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return ExitInvalidConfig
	}
	prof := cfg.Profiles[cfg.ActiveProfile]
	prov := cfg.Providers[prof.Provider]
	key, err := resolver.Resolve(prov.APIKeyHandle())
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return ExitInvalidConfig
	}
	ad := custom.New(prov.BaseURL, key, prov.AuthScheme, prov.Headers, true, 15*time.Second)
	h := ad.Health(context.Background())
	if !h.OK {
		fmt.Fprintf(stderr, "unhealthy: %s\n", h.Message)
		return ExitUnavailable
	}
	fmt.Fprintln(stdout, "OK:", h.Message)
	return ExitOK
}

func runModels(args []string, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: models <list|status> [--config PATH] [--json] [--all] [--watch SECONDS]")
		return ExitUsage
	}
	switch args[0] {
	case "list":
		return modelsList(args[1:], stdout, stderr, resolver)
	case "status":
		return modelsStatus(args[1:], stdout, stderr, resolver)
	default:
		fmt.Fprintln(stderr, "usage: models <list|status> [--config PATH] [--json] [--all] [--watch SECONDS]")
		return ExitUsage
	}
}

func modelsList(args []string, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	path, _ := flagValue(args, "--config")
	asJSON := hasFlag(args, "--json")
	cfg, src, err := config.Load(path, resolver)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return ExitInvalidConfig
	}
	picker := config.DesktopPickerEntries(cfg.Models)
	if asJSON {
		_ = json.NewEncoder(stdout).Encode(map[string]any{
			"source": src, "models": picker,
		})
		return ExitOK
	}
	fmt.Fprintf(stdout, "config: %s\n", src)
	fmt.Fprintf(stdout, "%-28s %-40s %-8s %s\n", "DESKTOP_ID", "UPSTREAM_MODEL_ID", "TIER", "LABEL")
	fmt.Fprintln(stdout, strings.Repeat("-", 110))
	for _, e := range picker {
		def := ""
		if e.IsDefault {
			def = " *"
		}
		fmt.Fprintf(stdout, "%-28s %-40s %-8s %s%s\n", e.DesktopID, e.ModelID, e.DesktopTier, e.DesktopLabel, def)
	}
	if len(picker) == 0 {
		fmt.Fprintln(stdout, "(no enabled models with desktop_id)")
	} else {
		fmt.Fprintln(stdout, "\n* = family default for Desktop picker")
	}
	return ExitOK
}

func modelsStatus(args []string, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	path, _ := flagValue(args, "--config")
	asJSON := hasFlag(args, "--json")
	all := hasFlag(args, "--all")
	watchStr, _ := flagValue(args, "--watch")
	watchSec := 0
	if watchStr != "" {
		n, err := strconv.Atoi(watchStr)
		if err != nil || n <= 0 {
			fmt.Fprintln(stderr, "--watch requires a positive seconds value")
			return ExitUsage
		}
		watchSec = n
	}

	cfg, _, err := config.Load(path, resolver)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return ExitInvalidConfig
	}
	prof := cfg.Profiles[cfg.ActiveProfile]
	prov := cfg.Providers[prof.Provider]
	key, err := resolver.Resolve(prov.APIKeyHandle())
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return ExitInvalidConfig
	}

	printOnce := func() int {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		live, err := modelstatus.FetchCatalog(ctx, prov.BaseURL, key, nil)
		if err != nil {
			fmt.Fprintf(stderr, "fetch catalog: %v\n", err)
			return ExitUnavailable
		}
		rows := modelstatus.BuildStatusRows(cfg.Models, live, !all)
		now := time.Now().UTC()
		if asJSON {
			_ = json.NewEncoder(stdout).Encode(map[string]any{
				"fetched_at": now.Format(time.RFC3339),
				"rows":       rows,
			})
			return ExitOK
		}
		fmt.Fprint(stdout, modelstatus.FormatTable(rows, now))
		return ExitOK
	}

	if watchSec == 0 {
		return printOnce()
	}
	fmt.Fprintf(stderr, "watching every %ds (Ctrl-C to stop)\n", watchSec)
	ticker := time.NewTicker(time.Duration(watchSec) * time.Second)
	defer ticker.Stop()
	if code := printOnce(); code != ExitOK {
		return code
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	for {
		select {
		case <-sigCh:
			fmt.Fprintln(stderr, "stopped")
			return ExitOK
		case <-ticker.C:
			fmt.Fprintln(stdout)
			if code := printOnce(); code != ExitOK {
				fmt.Fprintf(stderr, "refresh failed (will retry)\n")
			}
		}
	}
}

func runDoctor(args []string, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	path, _ := flagValue(args, "--config")
	fmt.Fprintln(stdout, "doctor:")
	cfg, src, err := config.Load(path, resolver)
	if err != nil {
		fmt.Fprintf(stdout, " - config: FAIL (%v)\n", err)
		return ExitInvalidConfig
	}
	errs := config.Validate(cfg)
	if len(errs) > 0 {
		fmt.Fprintf(stdout, " - config: FAIL (%d errors) source=%s\n", len(errs), src)
		for _, e := range errs {
			fmt.Fprintln(stdout, "   ", e.Error())
		}
		return ExitInvalidConfig
	}
	fmt.Fprintf(stdout, " - config: OK (%s profile=%s)\n", src, cfg.ActiveProfile)
	p := platform.DiscoverClaudeDesktopConfig()
	if p == "" {
		fmt.Fprintln(stdout, " - client: WARN (no Claude Desktop config found yet)")
	} else {
		fmt.Fprintln(stdout, " - client: OK", p)
	}
	dataDir, _ := platform.GatewayDataDir()
	histPath := filepath.Join(dataDir, "history.db")
	store, err := history.Open(histPath)
	if err != nil {
		fmt.Fprintf(stdout, " - history: FAIL (%v)\n", err)
		return ExitInternal
	}
	_ = store.Close()
	fmt.Fprintln(stdout, " - history: OK", histPath)
	fmt.Fprintln(stdout, " - adr-011: Accepted (Anthropic Messages inbound via Desktop on 3P)")
	return ExitOK
}

func flagValue(args []string, name string) (string, error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == name {
			if i+1 >= len(args) {
				return "", fmt.Errorf("missing value for %s", name)
			}
			return args[i+1], nil
		}
		if strings.HasPrefix(a, name+"=") {
			return strings.TrimPrefix(a, name+"="), nil
		}
	}
	return "", nil
}

func hasFlag(args []string, name string) bool {
	for _, a := range args {
		if a == name {
			return true
		}
	}
	return false
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}
