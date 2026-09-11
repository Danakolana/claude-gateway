package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/danakolana/claude-gateway/internal/clientintegration"
	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/internal/history"
	"github.com/danakolana/claude-gateway/internal/modelstatus"
	"github.com/danakolana/claude-gateway/internal/platform"
	"github.com/danakolana/claude-gateway/internal/provider"
	"github.com/danakolana/claude-gateway/internal/proxy"
)

const catalogRefreshEvery = 15 * time.Minute
const healthProbeEvery = 60 * time.Second
const driftWatchEvery = 30 * time.Second

// continueAfterDesktopApply reports whether local proxy start should proceed
// after a failed Desktop apply. User cancel (ExitUsage) is not fail-open.
func continueAfterDesktopApply(code int, stderr io.Writer) bool {
	if code == ExitOK {
		return true
	}
	if code == ExitUsage {
		return false
	}
	fmt.Fprintln(stderr, "WARN: desktop apply failed; local proxy will still start. Re-run: claude-gateway client apply")
	return true
}

// openHistoryBestEffort opens SQLite history. Failures are WARN-only so the
// local proxy can still serve /v1/messages (ADR-014).
func openHistoryBestEffort(path string, stderr io.Writer) *history.Store {
	store, err := history.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "WARN: history unavailable (%v); proxy continues without local history\n", err)
		return nil
	}
	return store
}

func contextLimitFor(models map[string]config.Model, id string) int {
	for _, m := range models {
		if m.ModelID == id || m.DesktopID == id {
			return m.ContextLimit
		}
	}
	return 0
}

func syncCatalogView(usage *proxy.UsageStore, c *modelstatus.CatalogCache) {
	if usage == nil || c == nil {
		return
	}
	live, at, err := c.Snapshot()
	info := proxy.CatalogInfo{Models: len(live), Error: err}
	if !at.IsZero() {
		info.FetchedAt = at.Format(time.RFC3339)
	}
	usage.SetCatalog(info)
	usage.SetLivePrices(live)
}

func syncHealthView(usage *proxy.UsageStore, p *modelstatus.Probe) {
	if usage == nil || p == nil {
		return
	}
	ok, msg, at, hints := p.View()
	info := proxy.HealthInfo{OK: ok, Message: msg, Models: hints}
	if !at.IsZero() {
		info.CheckedAt = at.Format(time.RFC3339)
	}
	usage.SetHealth(info)
}

func startCatalogRefresh(ctx context.Context, cache *modelstatus.CatalogCache, usage *proxy.UsageStore, baseURL, apiKey string, stderr io.Writer) {
	if cache == nil || baseURL == "" {
		return
	}
	syncCatalogView(usage, cache)
	go func() {
		defer func() { _ = recover() }()
		t := time.NewTicker(catalogRefreshEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
				cache.Refresh(cctx, baseURL, apiKey, nil)
				cancel()
				if err := cache.LastError(); err != "" {
					fmt.Fprintf(stderr, "WARN: price catalog refresh failed (%s); using last-known prices\n", err)
				}
				syncCatalogView(usage, cache)
			}
		}
	}()
}

func startHealthProbe(ctx context.Context, ad provider.Adapter, probe *modelstatus.Probe, usage *proxy.UsageStore, stderr io.Writer) {
	if ad == nil || probe == nil {
		return
	}
	run := func() {
		defer func() { _ = recover() }()
		cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		h := ad.Health(cctx)
		cancel()
		probe.RecordProvider(h.OK, h.Message)
		syncHealthView(usage, probe)
		if !h.OK {
			fmt.Fprintf(stderr, "WARN: provider health: %s (requests still attempted)\n", h.Message)
		}
	}
	run()
	go func() {
		defer func() { _ = recover() }()
		t := time.NewTicker(healthProbeEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	}()
}

// persistProxyHistory writes one conversation after the client already has a
// response. Redaction or store errors skip the write (ADR-014 / T150).
func persistProxyHistory(stderr io.Writer, store *history.Store, redact bool, id, status, profile string, msgs []map[string]any, extra map[string]any) {
	if store == nil {
		return
	}
	defer func() { _ = recover() }()
	if redact {
		out, err := history.RedactMessages(msgs)
		if err != nil {
			fmt.Fprintf(stderr, "WARN: history redaction failed (%v); skip write\n", err)
			return
		}
		msgs = out
	}
	_ = store.AppendConversation(context.Background(), id, status, msgs, extra)
	_ = store.Audit("proxy.request", profile, status, id)
}

func startDriftWatch(ctx context.Context, usage *proxy.UsageStore, stderr io.Writer, cfg *config.File) {
	target := "3p"
	if cfg != nil {
		target = cfg.Client.DesktopTarget()
	}
	path := platform.DiscoverClaudeDesktopConfigFor(target)
	if path == "" {
		return
	}
	sum, err := clientintegration.FileChecksum(path)
	if err != nil {
		fmt.Fprintf(stderr, "WARN: drift watch skipped (%v)\n", err)
		return
	}
	go func() {
		defer func() { _ = recover() }()
		t := time.NewTicker(driftWatchEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				drifted, err := clientintegration.Drifted(path, sum)
				if err != nil {
					fmt.Fprintf(stderr, "WARN: desktop config unreadable (%v); not rewriting\n", err)
					if usage != nil {
						usage.SetDrift("desktop config unreadable; run client apply --dry-run")
					}
					continue
				}
				if drifted {
					msg := "desktop config drifted from last apply; run: claude-gateway client apply --dry-run"
					fmt.Fprintln(stderr, "WARN: "+msg)
					if usage != nil {
						usage.SetDrift(msg)
					}
				}
			}
		}
	}()
}
