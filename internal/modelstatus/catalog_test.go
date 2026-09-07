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
