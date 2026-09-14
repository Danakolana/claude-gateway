package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mattn/go-isatty"

	"github.com/danakolana/claude-gateway/internal/clientintegration"
	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/internal/diagnose"
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
	return RunWithStreams(args, os.Stdin, stdout, stderr, resolver)
}

// RunWithStreams is like RunWith but allows injecting stdin (for prompts / tests).
func RunWithStreams(args []string, stdin io.Reader, stdout, stderr io.Writer, resolver secrets.Resolver) int {
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
		return runStart(startArgs, stdin, stdout, stderr, resolver)
	}
	switch args[0] {
	case "config":
		return runConfig(args[1:], stdout, stderr, resolver)
	case "profile":
		return runProfile(args[1:], stdout, stderr, resolver)
	case "proxy":
		return runProxy(args[1:], stdout, stderr, resolver)
	case "client":
		return runClient(args[1:], stdin, stdout, stderr, resolver)
	case "history":
		return runHistory(args[1:], stdout, stderr, resolver)
	case "doctor":
		return runDoctor(args[1:], stdout, stderr, resolver)
	case "provider":
		return runProvider(args[1:], stdout, stderr, resolver)
	case "models":
		return runModels(args[1:], stdout, stderr, resolver)
	case "version":
		fmt.Fprintln(stdout, diagnose.FormatVersion())
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

On first run, writes config.toml next to this binary (from the build-time
default), copies it to the OS user config path, applies Claude Desktop on 3P
settings, and starts the local Anthropic proxy (unless [proxy] mode = "direct").
Edit the sidecar config.toml; it is re-copied to the user path on every start.
Listen address comes from [proxy] listen in TOML (default 127.0.0.1:8080).

Config paths:
  Sidecar (edit this)  <dir-of-binary>/config.toml
  User copy            Windows  %APPDATA%\\claude-gateway\\config.toml
                       macOS/Linux  ~/.config/claude-gateway/config.toml
  Override             --config PATH or CLAUDE_GATEWAY_CONFIG

[proxy] mode:
  local   Desktop → local proxy → OpenRouter (default)
  direct  Desktop → OpenRouter Anthropic API (no local proxy; for comparison)

Optional flags on start:
  --config PATH       config file
  --listen HOST:PORT  override [proxy].listen
  --desktop 3p|consumer  skip auto-detect (default: auto from installed Desktop)
  --ask-desktop       force the old 3P vs consumer prompt
  --yes               skip "Desktop is closed?" confirm
  --no-apply          skip writing Desktop config
  --no-browser        do not open the local guide in a browser
  --inspect-prompts   capture Desktop system/tool harness for /debug/harness
  --fake              use in-memory provider (no API key)

On start the CLI auto-detects Claude Desktop 3P vs regular Desktop, writes
every existing data dir it finds, and binds the next free port if 8080 is
busy. It then prints READY / PARTIAL / NOT READY. If something is wrong,
it prints a copy-paste support prompt (secrets redacted).

Other commands:
  start                 same as bare ./claude-gateway
  config validate|explain
  profile list|create|select|delete
  proxy start|health
  client discover|diff|apply|restore
  history list|export|import
  models list|status
  provider health
  doctor [--json] [--prompt] [--offline]
  version

Env: OPENROUTER_API_KEY (or provider api_key_env), CLAUDE_GATEWAY_CONFIG,
     CLAUDE_GATEWAY_LISTEN, CLAUDE_GATEWAY_ACTIVE_PROFILE, OPENROUTER_BASE_URL`)
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
		return runStart(append([]string{"--no-apply"}, args[1:]...), os.Stdin, stdout, stderr, resolver)
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
func runStart(args []string, stdin io.Reader, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	printVersionBanner(stdout)
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
	if hasFlag(args, "--no-browser") || hasFlag(args, "--no-guide") {
		off := false
		cfg.Proxy.OpenGuide = &off
	}
	if hasFlag(args, "--inspect-prompts") {
		cfg.Proxy.InspectPrompts = true
	}
	if created := config.CreatedDefaultConfigPath(); created != "" {
		fmt.Fprintf(stdout, "First run: wrote default config next to this binary:\n  %s\n", created)
		if userPath, err := config.DefaultUserConfigPath(); err == nil {
			fmt.Fprintf(stdout, "It is copied to %s on every start.\n", userPath)
		}
		fmt.Fprintln(stdout, "Edit the file next to the binary; --config / CLAUDE_GATEWAY_CONFIG still override.")
	}
	if !useFake {
		if hint := missingAPIKeyMessage(cfg, resolver); hint != "" {
			fmt.Fprintln(stderr, hint)
			return ExitInvalidConfig
		}
	}
	if errs := config.Validate(cfg); len(errs) > 0 && !useFake {
		for _, e := range errs {
			fmt.Fprintln(stderr, e.Error())
		}
		return ExitInvalidConfig
	}
	live := fetchLiveCatalogBestEffort(cfg, resolver, stderr)
	printPriceSnapshot(stdout, cfg, live)

	yes := hasFlag(args, "--yes") || hasFlag(args, "-y")
	doApply := !noApply && cfg.Proxy.ShouldApplyDesktop()
	if doApply {
		choice, code := resolveDesktopTarget(args, stdin, stdout, stderr)
		if code != ExitOK {
			return code
		}
		cfg.Client = choice
	}

	prof := cfg.Profiles[cfg.ActiveProfile]
	prov := cfg.Providers[prof.Provider]
	catalogOK := live != nil

	if cfg.Proxy.IsDirect() {
		if useFake {
			fmt.Fprintln(stderr, "direct mode cannot use --fake (no local proxy)")
			return ExitUsage
		}
		gatewayURL := config.DirectGatewayBaseURL(cfg.Proxy, prov)
		fmt.Fprintf(stdout, "mode: direct → %s (OpenRouter Anthropic API, no local proxy)\n", gatewayURL)
		var applied []string
		applyErr := ""
		if doApply {
			var code int
			applied, code = applyDesktopConfig(cfg, gatewayURL, "", false, true, yes, live, stdin, stdout, stderr, resolver)
			if code != ExitOK {
				return code
			}
		} else {
			fmt.Fprintln(stdout, "desktop apply: skipped")
		}
		rep := diagnose.Collect(diagnose.Options{
			Config: cfg, ConfigPath: src, Resolver: resolver,
			GatewayURL: gatewayURL, DesktopTarget: cfg.Client.DesktopTarget(),
			AppliedPaths: applied, ApplyErr: applyErr,
			CatalogOK: catalogOK, Fake: false, Direct: true, ProbeProvider: !catalogOK,
		})
		if saved, err := diagnose.Persist(rep); err == nil {
			rep = saved
		}
		printStartupGuide(stdout, cfg, src)
		printSetupStatus(stdout, rep)
		fmt.Fprintln(stdout, "Direct mode: restart Claude Desktop / Apply Changes, then compare.")
		fmt.Fprintln(stdout, "Switch back with: [proxy] mode = \"local\" and re-run ./claude-gateway")
		printHandyCommands(stdout, gatewayURL, true)
		return ExitOK
	}

	addr := listenFlag
	if addr == "" {
		addr = cfg.Proxy.Addr()
	}
	fmt.Fprintf(stdout, "mode: local → http://%s (port may change if busy)\n", addr)

	return proxyListen(cfg, addr, useFake, live, src, doApply, yes, catalogOK, stdin, stdout, stderr, resolver)
}

func applyDesktopConfig(cfg *config.File, gatewayURL, clientPath string, dry, direct, yes bool, live map[string]modelstatus.LiveModel, stdin io.Reader, stdout, stderr io.Writer, resolver secrets.Resolver) ([]string, int) {
	prof := cfg.Profiles[cfg.ActiveProfile]
	prov := cfg.Providers[prof.Provider]
	key, err := resolver.Resolve(prov.APIKeyHandle())
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return nil, ExitInvalidConfig
	}

	if !dry {
		if code := confirmDesktopClosed(stdin, stdout, stderr, yes); code != ExitOK {
			return nil, code
		}
	}

	paths := desktopApplyPaths(cfg.Client.DesktopTarget(), clientPath)
	if !cfg.Client.IsConsumer() {
		auth := prov.AuthScheme
		if auth == "" {
			auth = "bearer"
		}
		picker := config.DesktopPickerEntries(cfg.Models)
		entries, skipped := desktopInferenceEntries(picker, direct)
		applyProviderLabelSuffixes(entries, prof.Provider)
		annotatePickerPrices(cfg, entries, picker, live)
		if direct && len(skipped) > 0 {
			fmt.Fprintf(stdout, "direct mode: skipped %d remapped model(s) (Desktop requires Anthropic-looking routes; use mode=local for these):\n", len(skipped))
			for _, s := range skipped {
				fmt.Fprintf(stdout, "  - %s\n", s)
			}
		}
		if len(entries) == 0 {
			fmt.Fprintln(stderr, "no Desktop-compatible models to apply")
			return nil, ExitInvalidConfig
		}
		cand := clientintegration.Render3PEntries(gatewayURL, key, auth, entries, false)
		var applied []string
		var lastErr error
		for _, p := range paths {
			backupDir := filepath.Join(filepath.Dir(p), "claude-gateway-backups")
			snap, err := clientintegration.Apply(p, cand, backupDir, cfg.ActiveProfile, diagnose.ToolVersion, dry)
			if err != nil {
				fmt.Fprintf(stderr, "desktop apply %s: %v\n", p, err)
				lastErr = err
				continue
			}
			applied = append(applied, p)
			provLabel := modelstatus.ProviderDisplayName(prof.Provider)
			if dry {
				fmt.Fprintf(stdout, "desktop apply: dry-run %s\n", p)
			} else {
				fmt.Fprintf(stdout, "desktop apply: %s (backup=%s)\n", p, snap.BackupPath)
				if provLabel != "" {
					fmt.Fprintf(stdout, "  models written: %d (labels include ~price hints from %s)\n", len(entries), provLabel)
				} else {
					fmt.Fprintf(stdout, "  models written: %d\n", len(entries))
				}
			}
		}
		if len(applied) == 0 {
			if lastErr != nil {
				fmt.Fprintf(stderr, "desktop apply: %v\n", lastErr)
			}
			return nil, ExitInternal
		}
		if !dry {
			fmt.Fprintf(stdout, "gateway base URL: %s\n", gatewayURL)
			fmt.Fprintln(stdout, "Reopen Claude Desktop (or Apply Changes) so Connection reloads.")
		}
		reportOtherProductLayouts(cfg.Client.DesktopTarget(), stdout)
		return applied, ExitOK
	}

	fmt.Fprintln(stderr, "WARN: EXPERIMENTAL consumer Desktop apply (env.ANTHROPIC_BASE_URL).")
	fmt.Fprintln(stderr, "WARN: Model picker stays Anthropic’s; custom DeepSeek labels need 3P.")
	fmt.Fprintln(stderr, "WARN: Remaps only apply when Desktop sends a routed model ID; may break across builds.")
	cand := clientintegration.RenderConsumerEnv(gatewayURL, key)
	var applied []string
	var lastErr error
	for _, p := range paths {
		backupDir := filepath.Join(filepath.Dir(p), "claude-gateway-backups")
		snap, err := clientintegration.ApplyConsumer(p, cand, backupDir, cfg.ActiveProfile, diagnose.ToolVersion, dry)
		if err != nil {
			fmt.Fprintf(stderr, "desktop apply %s: %v\n", p, err)
			lastErr = err
			continue
		}
		applied = append(applied, p)
		if dry {
			fmt.Fprintf(stdout, "desktop apply: dry-run %s\n", p)
		} else {
			fmt.Fprintf(stdout, "desktop apply (consumer/experimental): %s (backup=%s)\n", p, snap.BackupPath)
		}
	}
	if len(applied) == 0 {
		if lastErr != nil {
			fmt.Fprintf(stderr, "desktop apply: %v\n", lastErr)
		}
		return nil, ExitInternal
	}
	if !dry {
		fmt.Fprintf(stdout, "ANTHROPIC_BASE_URL=%s\n", gatewayURL)
		fmt.Fprintln(stdout, "Reopen Claude Desktop so env overrides reload.")
	}
	reportOtherProductLayouts(cfg.Client.DesktopTarget(), stdout)
	return applied, ExitOK
}

// fetchLiveCatalogBestEffort pulls OpenRouter /models prices with a short timeout.
// Failures are non-fatal: callers fall back to config.toml prices when present.
func fetchLiveCatalogBestEffort(cfg *config.File, resolver secrets.Resolver, stderr io.Writer) map[string]modelstatus.LiveModel {
	prof, ok := cfg.Profiles[cfg.ActiveProfile]
	if !ok {
		return nil
	}
	prov, ok := cfg.Providers[prof.Provider]
	if !ok || strings.TrimSpace(prov.BaseURL) == "" {
		return nil
	}
	key, err := resolver.Resolve(prov.APIKeyHandle())
	if err != nil {
		fmt.Fprintf(stderr, "price fetch skipped: %v\n", err)
		return nil
	}
	fmt.Fprintln(stderr, dim(stderr, "Fetching prices…"))
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	live, err := modelstatus.FetchCatalog(ctx, prov.BaseURL, key, nil)
	if err != nil {
		fmt.Fprintf(stderr, "Price fetch failed — using config.toml values: %v\n", err)
		return nil
	}
	return live
}

func printPriceSnapshot(w io.Writer, cfg *config.File, live map[string]modelstatus.LiveModel) {
	fromLive := live != nil
	if live == nil {
		live = map[string]modelstatus.LiveModel{}
	}
	rows := modelstatus.BuildStatusRows(cfg.Models, live, true)
	var desktop []modelstatus.StatusRow
	for _, r := range rows {
		if r.DesktopID != "" {
			desktop = append(desktop, r)
		}
	}
	if len(desktop) == 0 {
		return
	}
	fmt.Fprint(w, modelstatus.FormatCompactSnapshot(desktop, time.Now().UTC(), fromLive))
	fmt.Fprintln(w, "")
}

// annotatePickerPrices appends ~$/MTok + cheap|mid|pricey to Desktop labelOverride.
func annotatePickerPrices(cfg *config.File, entries []clientintegration.InferenceModelEntry, picker []config.DesktopPickerEntry, live map[string]modelstatus.LiveModel) {
	byDesktop := make(map[string]config.DesktopPickerEntry, len(picker))
	byModel := make(map[string]config.DesktopPickerEntry, len(picker))
	for _, p := range picker {
		byDesktop[p.DesktopID] = p
		byModel[p.ModelID] = p
	}
	type priced struct {
		idx int
		in  float64
	}
	prices := make([]priced, len(entries))
	for i := range entries {
		pe, ok := byDesktop[entries[i].Name]
		if !ok {
			pe, ok = byModel[entries[i].Name]
		}
		in, out := 0.0, 0.0
		if ok && live != nil {
			if lm, found := live[pe.ModelID]; found {
				in, out = lm.InputPerMTok, lm.OutputPerMTok
			}
		}
		if in == 0 && out == 0 && ok {
			if m, found := cfg.Models[pe.Key]; found {
				in, out = m.InputPrice, m.OutputPrice
			}
		}
		if in == 0 && out == 0 && ok {
			in, out = pe.InputPrice, pe.OutputPrice
		}
		prices[i] = priced{idx: i, in: in}
		if in == 0 && out == 0 {
			continue
		}
		base := entries[i].LabelOverride
		if base == "" {
			base = pe.DesktopLabel
		}
		entries[i].LabelOverride = modelstatus.AnnotateDesktopLabel(base, in, out)
	}
	// Cheapest input first so Desktop shortcuts 1–9 land on budget models.
	sort.SliceStable(prices, func(i, j int) bool {
		pi, pj := prices[i].in, prices[j].in
		if pi <= 0 && pj > 0 {
			return false
		}
		if pj <= 0 && pi > 0 {
			return true
		}
		if pi != pj {
			return pi < pj
		}
		return prices[i].idx < prices[j].idx
	})
	ordered := make([]clientintegration.InferenceModelEntry, len(entries))
	for i, p := range prices {
		ordered[i] = entries[p.idx]
	}
	copy(entries, ordered)
}

// applyProviderLabelSuffixes rewrites picker labels to "Name (OpenRouter|9router)"
// from the active provider, stripping legacy (gateway)/(OpenRouter) suffixes.
func applyProviderLabelSuffixes(entries []clientintegration.InferenceModelEntry, providerKey string) {
	for i := range entries {
		base := entries[i].LabelOverride
		if base == "" {
			base = entries[i].Name
		}
		entries[i].LabelOverride = modelstatus.WithProviderLabelSuffix(base, providerKey)
	}
}

func reportOtherProductLayouts(writtenTarget string, stdout io.Writer) {
	other := "consumer"
	if strings.EqualFold(writtenTarget, "consumer") {
		other = "3p"
	}
	for _, p := range platform.ExistingClaudeDesktopConfigPaths(other) {
		fmt.Fprintf(stdout, "note: %s Desktop data dir exists (not written): %s\n", other, p)
	}
}

// confirmDesktopClosed warns that Claude Desktop must be quit before writing
// config. --yes or a best-effort "not running" check skips the prompt.
// Interactive: requires y/yes unless skipped. Non-TTY: warn and continue.
// desktopRunning is the process probe used by confirmDesktopClosed.
// Tests replace it so prompts stay deterministic.
var desktopRunning = platform.ClaudeDesktopLikelyRunning

func confirmDesktopClosed(stdin io.Reader, stdout, stderr io.Writer, yes bool) int {
	msg := "Quit Claude Desktop completely before applying settings (config reloads on next launch)."
	if yes {
		return ExitOK
	}
	if !desktopRunning() {
		fmt.Fprintln(stderr, "Claude Desktop does not appear to be running — applying now.")
		return ExitOK
	}
	if !readerIsInteractive(stdin) {
		fmt.Fprintln(stderr, "WARN: Claude Desktop looks like it is running. "+msg)
		return ExitOK
	}
	br := bufio.NewReader(stdin)
	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, bold(stdout, "Before applying"))
	fmt.Fprintln(stdout, "  "+msg)
	fmt.Fprint(stdout, "Claude Desktop is closed? [y/N]: ")
	confirm, err := readLineBuf(br)
	if err != nil {
		fmt.Fprintf(stderr, "prompt: %v\n", err)
		return ExitInternal
	}
	c := strings.ToLower(strings.TrimSpace(confirm))
	if c != "y" && c != "yes" {
		fmt.Fprintln(stderr, "Cancelled. Close Claude Desktop, then re-run (or pass --yes / --no-apply).")
		return ExitUsage
	}
	return ExitOK
}

func missingAPIKeyMessage(cfg *config.File, resolver secrets.Resolver) string {
	if cfg == nil {
		return ""
	}
	prof, ok := cfg.Profiles[cfg.ActiveProfile]
	if !ok {
		return ""
	}
	prov, ok := cfg.Providers[prof.Provider]
	if !ok {
		return ""
	}
	h := prov.APIKeyHandle()
	envName := strings.TrimSpace(h.Ref)
	if envName == "" {
		envName = "OPENROUTER_API_KEY"
	}
	if resolver == nil {
		resolver = secrets.EnvResolver{}
	}
	if _, err := resolver.Resolve(h); err == nil {
		return ""
	}
	return fmt.Sprintf(`ERROR: %s is not set (or empty).

Get a key at https://openrouter.ai/keys then export it in this same terminal:

  export %s=sk-or-...

Windows PowerShell:

  $env:%s = "sk-or-..."

Then re-run ./claude-gateway`, envName, envName, envName)
}

func printVersionBanner(w io.Writer) {
	ver := diagnose.FormatVersion()
	body := []string{
		bold(w, ver),
		dim(w, runtime.GOOS+"/"+runtime.GOARCH),
	}
	fmt.Fprintln(w, "")
	drawBox(w, " claude-gateway ", body)
	fmt.Fprintln(w, "")
}

func printStartupGuide(w io.Writer, cfg *config.File, src string) {
	picker := config.DesktopPickerEntries(cfg.Models)
	mode := "local"
	if cfg.Proxy.IsDirect() {
		mode = "direct"
	}
	prof := cfg.ActiveProfile
	provName := ""
	if p, ok := cfg.Profiles[prof]; ok {
		provName = p.Provider
	}
	body := []string{
		fmt.Sprintf("●  %s", profileSelectedMessage(prof)),
		fmt.Sprintf("●  %s · %s", mode, cfg.Proxy.Addr()),
	}
	if provName != "" {
		body = append(body, fmt.Sprintf("●  %s · %d Desktop models", provName, len(picker)))
	} else {
		body = append(body, fmt.Sprintf("●  %d Desktop models", len(picker)))
	}
	body = append(body, fmt.Sprintf("●  %s", src))
	fmt.Fprintln(w, "")
	drawBox(w, " Current settings ", body)
	fmt.Fprintln(w, "")
}

// profileSelectedMessage is the user-facing active-profile line.
func profileSelectedMessage(profile string) string {
	label := strings.TrimSpace(profile)
	if label == "" {
		label = "?"
	}
	return fmt.Sprintf("profile %q is selected", label)
}

func printHandyCommands(w io.Writer, gatewayURL string, direct bool) {
	body := []string{}
	if direct {
		body = append(body,
			"●  Gateway  "+gatewayURL,
			"●  Next     restart Claude Desktop, then chat",
		)
	} else {
		body = append(body,
			"●  Guide    "+gatewayURL+"/",
			"●  Proxy    "+gatewayURL,
			"●  Health   "+gatewayURL+"/health",
			"●  Usage    "+gatewayURL+"/debug/usage",
			"●  Next     restart Claude Desktop, then chat",
		)
	}
	body = append(body,
		"─",
		dim(w, "models status · models list · Ctrl+C to quit"),
	)
	fmt.Fprintln(w, "")
	drawBox(w, " Ready ", body)
	fmt.Fprintln(w, "")
}

// promptDesktopTarget asks which Claude Desktop product to configure.
// Interactive stdin → user chooses; non-interactive → defaults to 3P.
func promptDesktopTarget(stdin io.Reader, stdout, stderr io.Writer) (config.Client, int) {
	if !readerIsInteractive(stdin) {
		fmt.Fprintln(stderr, "No TTY — using Claude Desktop 3P (recommended).")
		return config.Client{Desktop: "3p"}, ExitOK
	}
	br := bufio.NewReader(stdin)
	fmt.Fprintln(stdout, "")
	fmt.Fprintln(stdout, bold(stdout, "Where should we install and apply these settings?"))
	fmt.Fprintln(stdout, "  1) 3P "+dim(stdout, "(developer mode, recommended)")+" — custom model names in the picker")
	fmt.Fprintln(stdout, "  2) Consumer "+dim(stdout, "(experimental)")+" — redirects regular Desktop only")
	fmt.Fprint(stdout, "Choice [1/2] (default 1): ")
	line, err := readLineBuf(br)
	if err != nil {
		fmt.Fprintf(stderr, "prompt: %v\n", err)
		return config.Client{}, ExitInternal
	}
	switch strings.TrimSpace(line) {
	case "", "1", "3p", "3P":
		fmt.Fprintln(stdout, "→ 3P (developer mode)")
		return config.Client{Desktop: "3p"}, ExitOK
	case "2", "consumer", "c", "C":
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, bold(stdout, "Consumer mode limits"))
		fmt.Fprintln(stdout, "  • no custom model names in the picker")
		fmt.Fprintln(stdout, "  • may break after Desktop updates")
		fmt.Fprint(stdout, "Continue? [y/N]: ")
		confirm, err := readLineBuf(br)
		if err != nil {
			fmt.Fprintf(stderr, "prompt: %v\n", err)
			return config.Client{}, ExitInternal
		}
		c := strings.ToLower(strings.TrimSpace(confirm))
		if c != "y" && c != "yes" {
			fmt.Fprintln(stderr, "Cancelled. Choose 3P, or pass --no-apply.")
			return config.Client{}, ExitUsage
		}
		fmt.Fprintln(stdout, "→ Consumer (experimental)")
		return config.Client{Desktop: "consumer", AllowExperimental: true}, ExitOK
	default:
		fmt.Fprintf(stderr, "invalid choice %q (use 1 or 2)\n", line)
		return config.Client{}, ExitUsage
	}
}

func readLineBuf(br *bufio.Reader) (string, error) {
	line, err := br.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func readerIsInteractive(r io.Reader) bool {
	type interactiveMarker interface{ Interactive() bool }
	if m, ok := r.(interactiveMarker); ok {
		return m.Interactive()
	}
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// forcedInteractive marks a reader as a TTY for tests / scripted prompts.
type forcedInteractive struct{ io.Reader }

func (forcedInteractive) Interactive() bool { return true }

// desktopInferenceEntries builds Desktop inferenceModels.
// Local mode uses desktop_id (Anthropic-looking) and the proxy remaps to model_id.
// Direct mode has no remapper: Desktop strips non-Anthropic names, so only models
// whose upstream model_id is already an Anthropic-looking OpenRouter ID are kept.
func desktopInferenceEntries(picker []config.DesktopPickerEntry, direct bool) (entries []clientintegration.InferenceModelEntry, skipped []string) {
	entries = make([]clientintegration.InferenceModelEntry, 0, len(picker))
	for _, e := range picker {
		name := e.DesktopID
		if direct {
			if config.LooksLikeAnthropicModelRoute(e.ModelID) {
				name = e.ModelID
			} else {
				label := e.DesktopLabel
				if label == "" {
					label = e.DesktopID
				}
				skipped = append(skipped, fmt.Sprintf("%s → %s", label, e.ModelID))
				continue
			}
		}
		entries = append(entries, clientintegration.InferenceModelEntry{
			Name: name, LabelOverride: e.DesktopLabel,
			AnthropicFamilyTier: e.DesktopTier, IsFamilyDefault: e.IsDefault,
		})
	}
	return entries, skipped
}

func proxyListen(cfg *config.File, addr string, useFake bool, live map[string]modelstatus.LiveModel, configSrc string, doApply, yes, catalogOK bool, stdin io.Reader, stdout, stderr io.Writer, resolver secrets.Resolver) int {
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
		Registry:    routing.NewRegistry(cfg.Models),
		Adapter:     ad,
		Provider:    prof.Provider,
		ProviderCfg: cfg.Providers[prof.Provider],
		Routing:     prof.Routing,
		Breaker:     routing.NewBreaker(prof.Routing.CircuitFailures, time.Duration(prof.Routing.CircuitCooldownSeconds)*time.Second),
		Pins:        routing.NewPinStore(),
	}
	prices := modelstatus.NewCatalogCache(live)
	probe := modelstatus.NewProbe()
	trip := newSpendTrip(cfg.Proxy.SpendAlertUSD, strings.TrimSpace(cfg.Proxy.SpendAlertURL), stderr)
	catalogURL, catalogKey := "", ""
	if !useFake {
		p := cfg.Providers[prof.Provider]
		catalogURL = p.BaseURL
		if k, err := resolver.Resolve(p.APIKeyHandle()); err == nil {
			catalogKey = k
		}
	}
	dataDir, _ := platform.GatewayDataDir()
	histPath := filepath.Join(dataDir, "history.db")
	if cfg.History.LocalDatabase != "" {
		histPath = expandHome(cfg.History.LocalDatabase)
	}
	store := openHistoryBestEffort(histPath, stderr)
	if store != nil {
		defer store.Close()
	}

	inspect := cfg.Proxy.InspectPrompts
	usage := proxy.NewUsageStore()
	turns := newTurnTotals(stderr)
	srvCfg := proxy.Config{
		Addr:    addr,
		Harness: proxy.NewHarnessStore(inspect),
		Usage:   usage,
		OnRequest: func(req api.Request, resp api.Response) {
			status := "completed"
			if resp.Error != nil || resp.FinishReason == api.FinishCancelled || resp.FinishReason == api.FinishError {
				status = "incomplete"
			}
			target := req.TargetModel
			if target == "" {
				target = resp.Model
			}
			inP, outP, hasP := 0.0, 0.0, false
			if liveSnap, _, _ := prices.Snapshot(); liveSnap != nil {
				if lm, ok := liveSnap[target]; ok {
					inP, outP, hasP = lm.InputPerMTok, lm.OutputPerMTok, true
				}
			}
			if !hasP {
				inP, outP, hasP = modelstatus.PricesForModelID(cfg.Models, target)
			}
			snap := proxy.SnapshotFrom(req, resp, inP, outP, hasP, contextLimitFor(cfg.Models, target))
			usage.Record(snap)
			fmt.Fprintln(stderr, modelstatus.FormatUsageLine(req.SourceModel, target, resp.Usage, inP, outP, hasP))
			est := 0.0
			hasCost := false
			if snap.EstUSD != nil {
				est = *snap.EstUSD
				hasCost = true
			}
			turns.add(resp.Usage, est, hasCost, resp.FinishReason)
			if status != "completed" {
				probe.RecordModel(target, "last request incomplete")
			} else {
				probe.RecordModel(target, "")
			}
			syncHealthView(usage, probe)
			trip.maybe(usage.View().Session.EstUSD, usage.View().Session.Requests)
			if store == nil {
				return
			}
			st := store
			redact := cfg.History.RedactSecrets
			profile := cfg.ActiveProfile
			msgs := []map[string]any{
				{"role": "user", "content": req.Messages, "correlation_id": req.ID},
				{"role": "assistant", "content": resp.Content, "correlation_id": req.ID},
			}
			extra := map[string]any{
				"provider": eng.Provider, "source_model": req.SourceModel, "target_model": target,
				"outcome": status, "usage": resp.Usage,
			}
			go persistProxyHistory(stderr, st, redact, req.ID, status, profile, msgs, extra)
		},
	}
	srv := proxy.New(srvCfg, eng)
	bound, err := srv.Start()
	if err != nil {
		fmt.Fprintf(stderr, "start failed: %v\n", err)
		return ExitInternal
	}
	if requestedHost, requestedPort, e1 := splitHostPortSafe(addr); e1 == nil {
		if boundHost, boundPort, e2 := splitHostPortSafe(bound); e2 == nil && requestedPort != "0" && boundPort != requestedPort {
			fmt.Fprintf(stderr, "WARN: %s was busy; proxy bound %s:%s and Desktop will be pointed there\n", addr, boundHost, boundPort)
			_ = requestedHost
		}
	}
	base := "http://" + bound
	guideURL := base + "/"

	var applied []string
	applyErr := ""
	if doApply {
		var code int
		applied, code = applyDesktopConfig(cfg, base, "", false, false, yes, live, stdin, stdout, stderr, resolver)
		if !continueAfterDesktopApply(code, stderr) {
			shCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = srv.Shutdown(shCtx)
			cancel()
			return code
		}
		if code != ExitOK {
			applyErr = fmt.Sprintf("desktop apply failed (exit %d); proxy still running", code)
		}
	} else {
		fmt.Fprintln(stdout, "desktop apply: skipped")
	}

	fmt.Fprintf(stdout, "proxy listening on %s\n", base)
	fmt.Fprintf(stdout, "guide:  %s\n", guideURL)
	fmt.Fprintf(stdout, "Anthropic Messages API: POST %s/v1/messages\n", base)
	fmt.Fprintf(stdout, "health: %s/health\n", base)
	fmt.Fprintf(stdout, "usage:  %s/debug/usage\n", base)
	fmt.Fprintf(stdout, "status: %s/debug/status\n", base)
	if inspect {
		fmt.Fprintf(stdout, "harness inspect: %s/debug/harness\n", base)
	}
	if store == nil {
		fmt.Fprintln(stdout, "history: unavailable (sidecar; chat still works)")
	} else {
		fmt.Fprintf(stdout, "history: %s\n", histPath)
	}

	listenNote := ""
	if _, reqPort, e1 := splitHostPortSafe(addr); e1 == nil {
		if _, boundPort, e2 := splitHostPortSafe(bound); e2 == nil && reqPort != "0" && boundPort != reqPort {
			listenNote = fmt.Sprintf("preferred %s busy, bound %s", addr, bound)
		}
	}
	tried := []string{}
	if listenNote != "" {
		tried = append(tried, listenNote)
	}
	if len(applied) > 0 {
		tried = append(tried, "wrote Desktop config: "+strings.Join(applied, ", "))
	}
	rep := diagnose.Collect(diagnose.Options{
		Config: cfg, ConfigPath: configSrc, Resolver: resolver,
		GatewayURL: base, DesktopTarget: cfg.Client.DesktopTarget(),
		AppliedPaths: applied, ApplyErr: applyErr, ListenNote: listenNote,
		CatalogOK: catalogOK, Fake: useFake, ProbeProvider: !useFake && !catalogOK,
		Tried: tried,
	})
	if saved, perr := diagnose.Persist(rep); perr == nil {
		rep = saved
	}
	printStartupGuide(stdout, cfg, configSrc)
	printSetupStatus(stdout, rep)
	srv.SetStatus(rep)
	printHandyCommands(stdout, base, false)

	sideCtx, sideStop := context.WithCancel(context.Background())
	defer sideStop()
	startCatalogRefresh(sideCtx, prices, usage, catalogURL, catalogKey, stderr)
	startHealthProbe(sideCtx, ad, probe, usage, stderr)
	startDriftWatch(sideCtx, usage, stderr, cfg)

	if cfg.Proxy.ShouldOpenGuide() {
		if err := platform.OpenBrowser(guideURL); err != nil {
			fmt.Fprintf(stderr, "could not open browser: %v\n", err)
			fmt.Fprintf(stderr, "open manually: %s\n", guideURL)
		} else {
			fmt.Fprintf(stdout, "opened guide in browser\n")
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shCtx)
	fmt.Fprintln(stdout, "proxy stopped")
	return ExitOK
}

func runClient(args []string, stdin io.Reader, stdout, stderr io.Writer, resolver secrets.Resolver) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: client <discover|diff|apply|restore>")
		return ExitUsage
	}
	switch args[0] {
	case "discover":
		choice, code := resolveDesktopTarget(args[1:], stdin, stdout, stderr)
		if code != ExitOK {
			return code
		}
		target := choice.DesktopTarget()
		p := platform.DiscoverClaudeDesktopConfigFor(target)
		fmt.Fprintf(stdout, "target=%s\n", target)
		cands := platform.ClaudeDesktop3PConfigPaths()
		if choice.IsConsumer() {
			cands = platform.ClaudeDesktopConsumerConfigPaths()
		}
		if p == "" {
			fmt.Fprintln(stdout, "no Claude Desktop config path; candidates (newest layout first):")
			for _, c := range cands {
				fmt.Fprintln(stdout, " -", c)
			}
			return ExitOK
		}
		fmt.Fprintln(stdout, p)
		existing := platform.ExistingClaudeDesktopConfigPaths(target)
		if len(existing) > 1 {
			fmt.Fprintln(stdout, "other existing layouts:")
			for _, e := range existing {
				if e != p {
					fmt.Fprintln(stdout, " -", e)
				}
			}
		}
		if len(cands) > 1 {
			fmt.Fprintln(stdout, "candidates (newest layout first):")
			for _, c := range cands {
				mark := ""
				if c == p {
					mark = " (selected)"
				}
				fmt.Fprintln(stdout, " -", c+mark)
			}
		}
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
		choice, code := resolveDesktopTarget(args[1:], stdin, stdout, stderr)
		if code != ExitOK {
			return code
		}
		cfg.Client = choice
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
			if cfg.Client.IsConsumer() {
				fmt.Fprintln(stdout, clientintegration.RedactedConsumerDiff(clientintegration.RenderConsumerEnv(proxyURL, key)))
				return ExitOK
			}
			auth := prov.AuthScheme
			if auth == "" {
				auth = "bearer"
			}
			picker := config.DesktopPickerEntries(cfg.Models)
			entries, _ := desktopInferenceEntries(picker, direct)
			live := fetchLiveCatalogBestEffort(cfg, resolver, stderr)
			applyProviderLabelSuffixes(entries, prof.Provider)
			annotatePickerPrices(cfg, entries, picker, live)
			cand := clientintegration.Render3PEntries(proxyURL, key, auth, entries, false)
			fmt.Fprintln(stdout, clientintegration.RedactedDiff(cand))
			return ExitOK
		}
		dry := hasFlag(args[1:], "--dry-run")
		yes := hasFlag(args[1:], "--yes") || hasFlag(args[1:], "-y")
		live := fetchLiveCatalogBestEffort(cfg, resolver, stderr)
		printPriceSnapshot(stdout, cfg, live)
		_, code = applyDesktopConfig(cfg, proxyURL, clientPath, dry, direct, yes, live, stdin, stdout, stderr, resolver)
		return code
	case "restore":
		clientPath, _ := flagValue(args[1:], "--client-config")
		backup, _ := flagValue(args[1:], "--backup")
		if clientPath == "" || backup == "" {
			fmt.Fprintln(stderr, "usage: client restore --client-config PATH --backup PATH")
			return ExitUsage
		}
		backupDir := filepath.Join(filepath.Dir(clientPath), "claude-gateway-backups")
		if err := clientintegration.Restore(clientPath, backup, backupDir, "", diagnose.ToolVersion); err != nil {
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
		fmt.Fprintln(stderr, "usage: history <list|export|import>")
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
	case "import":
		from, _ := flagValue(args[1:], "--from")
		if from == "" {
			fmt.Fprintln(stderr, "usage: history import --from DIR [--replace] [--config PATH]")
			return ExitUsage
		}
		res, err := store.ImportJSONL(from, hasFlag(args[1:], "--replace"))
		if err != nil {
			fmt.Fprintf(stderr, "%v\n", err)
			return ExitInternal
		}
		fmt.Fprintf(stdout, "imported %d conversations, %d messages\n", res.ConversationsImported, res.MessagesImported)
		if res.ConversationsSkipped > 0 || res.MessagesSkipped > 0 {
			fmt.Fprintf(stdout, "skipped %d conversations, %d messages (already present; pass --replace to overwrite)\n",
				res.ConversationsSkipped, res.MessagesSkipped)
		}
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

func splitHostPortSafe(addr string) (host, port string, err error) {
	return net.SplitHostPort(addr)
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
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return p
		}
		return filepath.Join(home, p[2:])
	}
	return p
}
