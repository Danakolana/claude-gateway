package modelstatus_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danakolana/claude-gateway/internal/modelstatus"
	"github.com/danakolana/claude-gateway/pkg/api"
)

func TestCatalogCacheKeepsLastOnRefreshError(t *testing.T) {
	var fail atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "down", 500)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"p/a","pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`))
	}))
	defer srv.Close()

	c := modelstatus.NewCatalogCache(nil)
	c.Refresh(context.Background(), srv.URL, "k", srv.Client())
	live, _, err := c.Snapshot()
	if err != "" || live["p/a"].ID != "p/a" {
		t.Fatalf("%v %+v", err, live)
	}
	fail.Store(true)
	c.Refresh(context.Background(), srv.URL, "k", srv.Client())
	live2, _, err2 := c.Snapshot()
	if err2 == "" {
		t.Fatal("expected lastErr")
	}
	if live2["p/a"].InputPerMTok != live["p/a"].InputPerMTok {
		t.Fatalf("stale prices lost: %+v", live2)
	}
}

func TestContextNotesNearLimit(t *testing.T) {
	notes := modelstatus.ContextNotes(api.Request{}, api.Usage{InputTokens: 900}, 1000)
	if len(notes) != 1 {
		t.Fatalf("%v", notes)
	}
	quiet := modelstatus.ContextNotes(api.Request{}, api.Usage{InputTokens: 10}, 1000)
	if len(quiet) != 0 {
		t.Fatal(quiet)
	}
}

func TestProbeTimeoutIsDown(t *testing.T) {
	p := modelstatus.NewProbe()
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	<-ctx.Done()
	p.RecordProvider(false, ctx.Err().Error())
	ok, msg, _, _ := p.View()
	if ok || msg == "" {
		t.Fatalf("%v %q", ok, msg)
	}
}
