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
