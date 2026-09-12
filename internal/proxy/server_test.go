package proxy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/internal/provider/fake"
	"github.com/danakolana/claude-gateway/internal/proxy"
	"github.com/danakolana/claude-gateway/internal/routing"
	"github.com/danakolana/claude-gateway/pkg/api"
)

func TestProxyMessagesAndStream(t *testing.T) {
	reg := routing.NewRegistry(map[string]config.Model{
		"fast": {ModelID: "fake/fast", Streaming: true, ToolCalls: true, Enabled: true, ContextLimit: 100000},
	})
	eng := &routing.Engine{
		Registry: reg, Adapter: &fake.Adapter{}, Provider: "fake",
		Routing: config.Routing{Rules: []config.Rule{{Source: "claude-sonnet", TargetModel: "fast"}}},
	}
	srv := proxy.New(proxy.Config{Addr: "127.0.0.1:0"}, eng)
	addr, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown(context.Background())

	body := `{"model":"claude-sonnet","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`
	res, err := http.Post("http://"+addr+"/v1/messages", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		t.Fatalf("%d %s", res.StatusCode, raw)
	}
	var msg map[string]any
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatal(err)
	}
	if msg["type"] != "message" {
		t.Fatalf("%v", msg)
	}
	if msg["model"] != "claude-sonnet" {
		t.Fatalf("expected client-visible model echo, got %v", msg["model"])
	}

	// streaming
	sbody := `{"model":"claude-sonnet","max_tokens":64,"stream":true,"messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/v1/messages", bytes.NewBufferString(sbody))
	req.Header.Set("Content-Type", "application/json")
	sres, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer sres.Body.Close()
	all, _ := io.ReadAll(sres.Body)
	if !bytes.Contains(all, []byte("message_start")) {
		t.Fatalf("stream=%s", all)
	}
	if !bytes.Contains(all, []byte("content_block_stop")) {
		t.Fatalf("expected content_block_stop in stream=%s", all)
	}

	// streaming tool use
	tbody := `{"model":"claude-sonnet","max_tokens":64,"stream":true,"tools":[{"name":"search","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"find x"}]}`
	treq, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/v1/messages", bytes.NewBufferString(tbody))
	treq.Header.Set("Content-Type", "application/json")
	tres, err := http.DefaultClient.Do(treq)
	if err != nil {
		t.Fatal(err)
	}
	defer tres.Body.Close()
	tall, _ := io.ReadAll(tres.Body)
	for _, want := range [][]byte{
		[]byte(`"type":"tool_use"`),
		[]byte(`"partial_json"`),
		[]byte(`"stop_reason":"tool_use"`),
		[]byte("content_block_stop"),
	} {
		if !bytes.Contains(tall, want) {
			t.Fatalf("missing %s in tool stream=%s", want, tall)
		}
	}
	if bytes.Contains(tall, []byte(`"type":"ping"`)) {
		t.Fatalf("tool deltas must not become pings: %s", tall)
	}

	hres, err := http.Get("http://" + addr + "/health")
	if err != nil {
		t.Fatal(err)
	}
	hres.Body.Close()

	mres, err := http.Get("http://" + addr + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer mres.Body.Close()
	mraw, _ := io.ReadAll(mres.Body)
	if !bytes.Contains(mraw, []byte("anthropic_family_tier")) {
		t.Fatalf("models discovery missing family tier: %s", mraw)
	}
	if !bytes.Contains(mraw, []byte(`"type":"model"`)) && !bytes.Contains(mraw, []byte(`"type": "model"`)) {
		t.Fatalf("models missing type=model: %s", mraw)
	}
	_ = time.Second
}

func TestProviderErrorNotHTTP200(t *testing.T) {
	reg := routing.NewRegistry(map[string]config.Model{
		"fast": {ModelID: "fake/fast", Streaming: true, ToolCalls: true, Enabled: true, ContextLimit: 100000},
	})
	eng := &routing.Engine{
		Registry: reg, Adapter: &fake.Adapter{FailAuth: true}, Provider: "fake",
		Routing: config.Routing{Rules: []config.Rule{{Source: "claude-sonnet", TargetModel: "fast"}}},
	}
	srv := proxy.New(proxy.Config{Addr: "127.0.0.1:0"}, eng)
	addr, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown(context.Background())

	body := `{"model":"claude-sonnet","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`
	res, err := http.Post("http://"+addr+"/v1/messages", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode == 200 {
		t.Fatalf("provider error must not be HTTP 200 (Desktop treats body as Message): %s", raw)
	}
	if res.Header.Get("X-Correlation-ID") == "" {
		t.Fatal("missing X-Correlation-ID")
	}
	var msg map[string]any
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatal(err)
	}
	if msg["type"] != "error" {
		t.Fatalf("%v", msg)
	}
}

func TestSidecarPanicAndUsageOmitsPrompt(t *testing.T) {
	reg := routing.NewRegistry(map[string]config.Model{
		"fast": {ModelID: "fake/fast", Streaming: true, ToolCalls: true, Enabled: true, ContextLimit: 100000},
	})
	eng := &routing.Engine{
		Registry: reg, Adapter: &fake.Adapter{}, Provider: "fake",
		Routing: config.Routing{Rules: []config.Rule{{Source: "claude-sonnet", TargetModel: "fast"}}},
	}
	usage := proxy.NewUsageStore()
	srv := proxy.New(proxy.Config{
		Addr:  "127.0.0.1:0",
		Usage: usage,
		OnRequest: func(req api.Request, resp api.Response) {
			usage.Record(proxy.SnapshotFrom(req, resp, 1, 2, true, 0))
			panic("sidecar boom")
		},
	}, eng)
	addr, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown(context.Background())

	secret := "secret-prompt-xyz-not-in-usage"
	body := `{"model":"claude-sonnet","max_tokens":64,"messages":[{"role":"user","content":"` + secret + `"}]}`
	res, err := http.Post("http://"+addr+"/v1/messages", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("sidecar panic must not fail chat: %d %s", res.StatusCode, raw)
	}

	ures, err := http.Get("http://" + addr + "/debug/usage")
	if err != nil {
		t.Fatal(err)
	}
	defer ures.Body.Close()
	ubody, _ := io.ReadAll(ures.Body)
	if ures.StatusCode != 200 {
		t.Fatalf("usage %d %s", ures.StatusCode, ubody)
	}
	if bytes.Contains(ubody, []byte(secret)) {
		t.Fatalf("usage leaked prompt: %s", ubody)
	}
	var view proxy.UsageView
	if err := json.Unmarshal(ubody, &view); err != nil {
		t.Fatal(err, string(ubody))
	}
	if !view.OK || view.Last == nil || view.Session.Requests != 1 {
		t.Fatalf("%+v", view)
	}
	if view.Last.SourceModel != "claude-sonnet" {
		t.Fatalf("source=%s", view.Last.SourceModel)
	}

	srv.SetStatus(map[string]any{"verdict": "READY", "summary": "test", "support_prompt": "hello"})
	sres, err := http.Get("http://" + addr + "/debug/status")
	if err != nil {
		t.Fatal(err)
	}
	defer sres.Body.Close()
	sbody, _ := io.ReadAll(sres.Body)
	if sres.StatusCode != 200 || !bytes.Contains(sbody, []byte(`"verdict":"READY"`)) && !bytes.Contains(sbody, []byte(`"verdict": "READY"`)) {
		t.Fatalf("status %d %s", sres.StatusCode, sbody)
	}
	if bytes.Contains(sbody, []byte(secret)) {
		t.Fatalf("status leaked prompt: %s", sbody)
	}
}

func TestHealthDownDoesNotBlockMessages(t *testing.T) {
	reg := routing.NewRegistry(map[string]config.Model{
		"fast": {ModelID: "fake/fast", Streaming: true, ToolCalls: true, Enabled: true, ContextLimit: 100000},
	})
	ad := &fake.Adapter{HealthFail: true}
	eng := &routing.Engine{
		Registry: reg, Adapter: ad, Provider: "fake",
		Routing: config.Routing{Rules: []config.Rule{{Source: "claude-sonnet", TargetModel: "fast"}}},
	}
	h := ad.Health(context.Background())
	if h.OK {
		t.Fatal("expected down")
	}
	srv := proxy.New(proxy.Config{Addr: "127.0.0.1:0"}, eng)
	addr, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown(context.Background())
	body := `{"model":"claude-sonnet","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`
	res, err := http.Post("http://"+addr+"/v1/messages", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("%d %s", res.StatusCode, raw)
	}
	if len(ad.Calls) == 0 {
		t.Fatal("health down must not skip the adapter")
	}
}

func TestHealthTimeoutDoesNotBlockMessages(t *testing.T) {
	reg := routing.NewRegistry(map[string]config.Model{
		"fast": {ModelID: "fake/fast", Streaming: true, ToolCalls: true, Enabled: true, ContextLimit: 100000},
	})
	ad := &fake.Adapter{HealthDelay: 2 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	h := ad.Health(ctx)
	if h.OK {
		t.Fatal("expected timeout to record down")
	}
	eng := &routing.Engine{
		Registry: reg, Adapter: ad, Provider: "fake",
		Routing: config.Routing{Rules: []config.Rule{{Source: "claude-sonnet", TargetModel: "fast"}}},
	}
	srv := proxy.New(proxy.Config{Addr: "127.0.0.1:0"}, eng)
	addr, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown(context.Background())
	body := `{"model":"claude-sonnet","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`
	res, err := http.Post("http://"+addr+"/v1/messages", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		raw, _ := io.ReadAll(res.Body)
		t.Fatalf("timeout health must not block chat: %d %s", res.StatusCode, raw)
	}
}
