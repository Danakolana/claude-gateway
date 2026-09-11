package modelstatus_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/internal/modelstatus"
	"github.com/danakolana/claude-gateway/pkg/api"
)

func TestFetchCatalogAndTable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{
			"id":"z-ai/glm-5.3-flash","name":"GLM","context_length":1048576,
			"pricing":{"prompt":"0.000000075","completion":"0.00000025"},
			"benchmarks":{"artificial_analysis":{"coding_index":71.5,"agentic_index":51.5,"intelligence_index":46.2},
			"design_arena":[{"arena":"models","category":"codecategories","rank":12,"elo":1200}]}
		}]}`))
	}))
	defer srv.Close()

	live, err := modelstatus.FetchCatalog(context.Background(), srv.URL, "k", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	m := live["z-ai/glm-5.3-flash"]
	if m.InputPerMTok < 0.07 || m.InputPerMTok > 0.08 {
		t.Fatalf("input=%v", m.InputPerMTok)
	}
	if !m.HasCodingIndex || m.CodingIndex != 71.5 {
		t.Fatalf("%+v", m)
	}
	if !m.HasCodeArena || m.CodeArenaRank != 12 {
		t.Fatalf("%+v", m)
	}

	rows := modelstatus.BuildStatusRows(map[string]config.Model{
		"glm_flash": {
			ModelID: "z-ai/glm-5.3-flash", Enabled: true,
			DesktopID: "claude-haiku-4-1", DesktopLabel: "GLM",
		},
	}, live, true)
	table := modelstatus.FormatTable(rows, time.Unix(0, 0).UTC())
	if !strings.Contains(table, "glm_flash") || !strings.Contains(table, "71.5") {
		t.Fatal(table)
	}
}

func TestPriceBandAndAnnotateLabel(t *testing.T) {
	if modelstatus.PriceBand(0.14, 0.28) != "cheap" {
		t.Fatal("expected cheap")
	}
	if modelstatus.PriceBand(2, 10) != "mid" {
		t.Fatal("expected mid")
	}
	if modelstatus.PriceBand(5, 25) != "pricey" {
		t.Fatal("expected pricey")
	}
	got := modelstatus.AnnotateDesktopLabel("DeepSeek V4 Flash (gateway)", 0.14, 0.28)
	if !strings.Contains(got, "~$0.14/$0.28") || !strings.Contains(got, "cheap") {
		t.Fatal(got)
	}
	// idempotent strip
	got2 := modelstatus.AnnotateDesktopLabel(got, 0.20, 0.40)
	if strings.Count(got2, " · ~$") != 1 {
		t.Fatal(got2)
	}
	snap := modelstatus.FormatCompactSnapshot([]modelstatus.StatusRow{{
		Key: "x", DesktopID: "claude-haiku-4", DesktopLabel: "DeepSeek",
		ModelID: "deepseek/x", Found: true,
		Live: modelstatus.LiveModel{InputPerMTok: 0.14, OutputPerMTok: 0.28},
	}}, time.Unix(0, 0).UTC(), true)
	if !strings.Contains(snap, "approximate") || !strings.Contains(snap, "cheap") {
		t.Fatal(snap)
	}
}

func TestFormatUsageLine(t *testing.T) {
	u := api.Usage{
		InputTokens: 1000, OutputTokens: 500, ReasoningTokens: 200,
		CachedTokens: 100, HasProviderCost: true, ProviderCostUSD: 0.00123,
	}
	line := modelstatus.FormatUsageLine("claude-haiku-4", "deepseek/x", u, 0.14, 0.28, true)
	if !strings.Contains(line, "prompt") || !strings.Contains(line, "reasoning 200") {
		t.Fatal(line)
	}
	if !strings.Contains(line, "OpenRouter reported") || !strings.Contains(line, "0.001230") {
		t.Fatal(line)
	}
	est := modelstatus.EstimateCostUSD(api.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000}, 1, 2)
	if est != 3 {
		t.Fatalf("est=%v", est)
	}
}
