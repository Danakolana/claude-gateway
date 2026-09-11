package routing

import (
	"encoding/json"
	"testing"

	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/internal/provider/openai"
	"github.com/danakolana/claude-gateway/pkg/api"
)

func TestPreferCheapest(t *testing.T) {
	reg := NewRegistry(map[string]config.Model{
		"a": {ModelID: "p/a", Enabled: true, Streaming: true, ToolCalls: true, InputPrice: 1.0, OutputPrice: 5.0},
		"b": {ModelID: "p/b", Enabled: true, Streaming: true, ToolCalls: true, InputPrice: 0.1, OutputPrice: 0.4},
	})
	rc := config.Routing{
		PreferCheapest: true,
		Rules: []config.Rule{
			{Source: "x", TargetModel: "a"},
			{Source: "x", TargetModel: "b"},
		},
	}
	d, err := reg.Resolve("x", "or", rc, api.Requirements{Tools: true}, 0)
	if err != nil || d.TargetModel != "p/b" {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestThinkingForceOffWhenNoReasoning(t *testing.T) {
	req := api.Request{Thinking: &api.ThinkingConfig{Type: "enabled", BudgetTokens: 8000}}
	ApplyCostPolicies(&req, config.Model{Reasoning: false}, config.Routing{}, config.Provider{})
	if req.Thinking == nil || req.Thinking.Type != "disabled" {
		t.Fatalf("%+v", req.Thinking)
	}
}

func TestThinkingCap(t *testing.T) {
	req := api.Request{Thinking: &api.ThinkingConfig{Type: "enabled", BudgetTokens: 9000}}
	ApplyCostPolicies(&req, config.Model{Reasoning: true, ThinkingPolicy: "cap", ThinkingBudgetMax: 512}, config.Routing{}, config.Provider{})
	if req.Thinking.BudgetTokens != 512 {
		t.Fatalf("%+v", req.Thinking)
	}
}

func TestEnsurePromptCacheInjects(t *testing.T) {
	req := api.Request{
		System: "sys",
		Tools:  []api.ToolDef{{Name: "t", InputSchema: map[string]any{"type": "object"}}},
	}
	ApplyCostPolicies(&req, config.Model{Reasoning: false}, config.Routing{EnsurePromptCache: true}, config.Provider{})
	if len(req.SystemBlocks) == 0 || len(req.SystemBlocks[0].CacheControl) == 0 {
		t.Fatalf("system blocks %+v", req.SystemBlocks)
	}
	if len(req.Tools[0].CacheControl) == 0 {
		t.Fatal("tool cache missing")
	}
	// Round-trip encode must keep cache_control
	req.TargetModel = "m"
	raw, err := openai.EncodeRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	msgs := got["messages"].([]any)
	sys := msgs[0].(map[string]any)
	content := sys["content"].([]any)
	part := content[0].(map[string]any)
	if part["cache_control"] == nil {
		t.Fatalf("encoded body missing cache_control: %s", raw)
	}
	tools := got["tools"].([]any)
	if tools[0].(map[string]any)["cache_control"] == nil {
		t.Fatalf("tool cache_control missing: %s", raw)
	}
}

func TestBuildUpstreamProvider(t *testing.T) {
	ttrue := true
	p := BuildUpstreamProvider(config.Provider{Sort: "price", RequireParameters: &ttrue, IgnoreProviders: []string{"X"}})
	if p["sort"] != "price" || p["require_parameters"] != true {
		t.Fatalf("%v", p)
	}
	req := api.Request{TargetModel: "m", UpstreamProvider: p}
	raw, err := openai.EncodeRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	_ = json.Unmarshal(raw, &got)
	if got["provider"] == nil {
		t.Fatalf("%s", raw)
	}
}

func TestEnsurePromptCacheSkipsIfPresent(t *testing.T) {
	req := api.Request{
		System:       "sys",
		CacheControl: map[string]any{"type": "ephemeral"},
	}
	EnsurePromptCache(&req)
	if req.System != "sys" || len(req.SystemBlocks) != 0 {
		t.Fatalf("should not rewrite when cache already present: %+v", req)
	}
}
