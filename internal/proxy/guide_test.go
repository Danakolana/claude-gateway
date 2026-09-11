package proxy_test

import (
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

func TestGuideAndHarnessEndpoints(t *testing.T) {
	eng := &routing.Engine{
		Registry: routing.NewRegistry(map[string]config.Model{
			"fast": {ModelID: "fake/fast", Enabled: true, Streaming: true, ToolCalls: true, ContextLimit: 32000, TierAlias: "fast"},
		}),
		Adapter:  &fake.Adapter{},
		Provider: "fake",
		Routing:  config.Routing{DefaultTier: "fast"},
	}
	store := proxy.NewHarnessStore(true)
	srv := proxy.New(proxy.Config{Addr: "127.0.0.1:0", Harness: store}, eng)
	addr, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Shutdown(t.Context()) }()
	base := "http://" + addr

	res, err := http.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 200 || !contains(string(body), "Claude Gateway") || !contains(string(body), "id=\"live\"") {
		t.Fatalf("guide status=%d body=%q", res.StatusCode, truncate(string(body), 200))
	}
	if !contains(string(body), "/debug/usage") || !contains(string(body), "usage unavailable") {
		t.Fatalf("guide missing live usage strings: %s", truncate(string(body), 400))
	}
	if !contains(string(body), "/debug/status") || !contains(string(body), "Copy report prompt") {
		t.Fatalf("guide missing setup status: %s", truncate(string(body), 400))
	}
	if !contains(string(body), "/debug/models") || !contains(string(body), "id=\"models-body\"") {
		t.Fatalf("guide missing models panel: %s", truncate(string(body), 400))
	}
	if !contains(string(body), "hamburger") || !contains(string(body), "/img/enable-developer-mode.png") {
		t.Fatalf("guide missing developer-mode help: %s", truncate(string(body), 400))
	}

	img, err := http.Get(base + "/img/enable-developer-mode.png")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, img.Body)
	_ = img.Body.Close()
	if img.StatusCode != 200 {
		t.Fatalf("screenshot status=%d", img.StatusCode)
	}

	mres, err := http.Get(base + "/debug/models")
	if err != nil {
		t.Fatal(err)
	}
	var modelsPayload struct {
		Rows []struct {
			Key     string `json:"key"`
			ModelID string `json:"model_id"`
		} `json:"rows"`
	}
	if err := json.NewDecoder(mres.Body).Decode(&modelsPayload); err != nil {
		t.Fatal(err)
	}
	_ = mres.Body.Close()
	if mres.StatusCode != 200 || len(modelsPayload.Rows) != 1 || modelsPayload.Rows[0].ModelID != "fake/fast" {
		t.Fatalf("debug models status=%d %+v", mres.StatusCode, modelsPayload)
	}

	store.Record(api.Request{
		ID: "c1", SourceModel: "claude-sonnet-4", System: "You are a harness test.",
		Tools: []api.ToolDef{{Name: "Bash"}}, Messages: []api.Message{{Role: "user"}},
	}, "fake/fast")

	res, err = http.Get(base + "/debug/harness")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var payload struct {
		Enabled bool `json:"enabled"`
		Items   []struct {
			SystemPreview string   `json:"system_preview"`
			ToolNames     []string `json:"tool_names"`
		} `json:"items"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Enabled || len(payload.Items) != 1 || payload.Items[0].SystemPreview == "" {
		t.Fatalf("%+v", payload)
	}
	if len(payload.Items[0].ToolNames) != 1 || payload.Items[0].ToolNames[0] != "Bash" {
		t.Fatalf("tools=%v", payload.Items[0].ToolNames)
	}
	time.Sleep(10 * time.Millisecond)
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || stringIndex(s, sub) >= 0)
}
func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
