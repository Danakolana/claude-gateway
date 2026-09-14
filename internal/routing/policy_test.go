package routing

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/internal/provider/openai"
	"github.com/danakolana/claude-gateway/pkg/api"
)

func TestPreferCheapest(t *testing.T) {
	reg := NewRegistry(map[string]config.Model{
		"a": {ModelID: "p/a", TierAlias: "fast", Enabled: true, Streaming: true, ToolCalls: true, InputPrice: 1.0, OutputPrice: 5.0},
		"b": {ModelID: "p/b", TierAlias: "fast", Enabled: true, Streaming: true, ToolCalls: true, InputPrice: 0.1, OutputPrice: 0.4},
	})
	// No explicit rule for "x"; it falls through to the "fast" tier.
	// prefer_cheapest should then pick the cheaper tier member.
	rc := config.Routing{
		PreferCheapest: true,
		FallbackTiers:  []string{"fast"},
	}
	d, err := reg.Resolve("x", "or", rc, api.Requirements{Tools: true}, 0)
	if err != nil || d.TargetModel != "p/b" {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestExplicitRuleWinsOverPreferCheapest(t *testing.T) {
	reg := NewRegistry(map[string]config.Model{
		"qwen":   {ModelID: "qwen/qwen3.8-flash", Enabled: true, Streaming: true, ToolCalls: true, InputPrice: 0.15, OutputPrice: 0.47},
		"mercury": {ModelID: "inception/mercury-2.5-preview", Enabled: true, Streaming: true, ToolCalls: true, InputPrice: 0.04, OutputPrice: 0.15},
	})
	// Explicit mapping says source "claude-haiku-4-7" → qwen. Mercury is cheaper,
	// but prefer_cheapest must NOT override an explicit rule.
	rc := config.Routing{
		PreferCheapest: true,
		Rules: []config.Rule{
			{Source: "claude-haiku-4-7", TargetModel: "qwen"},
			{Source: "claude-haiku-4-7", TargetModel: "mercury"},
		},
	}
	d, err := reg.Resolve("claude-haiku-4-7", "or", rc, api.Requirements{Tools: true}, 0)
	if err != nil || d.TargetModel != "qwen/qwen3.8-flash" {
		t.Fatalf("explicit rule should win over cheaper candidate: %+v %v", d, err)
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

func TestConversationPrefixCache(t *testing.T) {
	req := api.Request{
		System: "sys",
		Messages: []api.Message{
			{Role: api.RoleUser, Content: []api.ContentBlock{{Type: api.BlockText, Text: "one"}}},
			{Role: api.RoleAssistant, Content: []api.ContentBlock{{Type: api.BlockText, Text: "two"}}},
			{Role: api.RoleUser, Content: []api.ContentBlock{{Type: api.BlockText, Text: "three"}}},
		},
	}
	ApplyCostPolicies(&req, config.Model{Reasoning: false}, config.Routing{EnsurePromptCache: true}, config.Provider{})
	if len(req.Messages[1].Content[0].CacheControl) == 0 {
		t.Fatal("penultimate assistant block should be marked for prefix cache")
	}
	if len(req.Messages[2].Content[0].CacheControl) != 0 {
		t.Fatal("newest user turn must stay uncached")
	}
	if len(req.Messages[0].Content[0].CacheControl) != 0 {
		t.Fatal("only the last stable block should be marked")
	}
	req.TargetModel = "m"
	raw, err := openai.EncodeRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"cache_control"`) {
		t.Fatalf("encoded body missing cache_control: %s", raw)
	}
}

func TestConversationPrefixSkipsSingleTurn(t *testing.T) {
	req := api.Request{
		Messages: []api.Message{
			{Role: api.RoleUser, Content: []api.ContentBlock{{Type: api.BlockText, Text: "hi"}}},
		},
	}
	EnsurePromptCache(&req)
	if len(req.Messages[0].Content[0].CacheControl) != 0 {
		t.Fatal("first turn should not mark the only user message")
	}
}

func TestConversationPrefixSkipsIfMessageAlreadyCached(t *testing.T) {
	req := api.Request{
		Messages: []api.Message{
			{Role: api.RoleUser, Content: []api.ContentBlock{{Type: api.BlockText, Text: "one", CacheControl: map[string]any{"type": "ephemeral"}}}},
			{Role: api.RoleAssistant, Content: []api.ContentBlock{{Type: api.BlockText, Text: "two"}}},
			{Role: api.RoleUser, Content: []api.ContentBlock{{Type: api.BlockText, Text: "three"}}},
		},
	}
	EnsurePromptCache(&req)
	if req.Messages[1].Content[0].CacheControl != nil {
		t.Fatal("should not add a second history breakpoint when client already cached a message")
	}
}

func TestConversationPrefixMarksToolResult(t *testing.T) {
	req := api.Request{
		Messages: []api.Message{
			{Role: api.RoleUser, Content: []api.ContentBlock{{Type: api.BlockText, Text: "do"}}},
			{Role: api.RoleAssistant, Content: []api.ContentBlock{{Type: api.BlockToolUse, ToolUseID: "c1", ToolName: "bash"}}},
			{Role: api.RoleUser, Content: []api.ContentBlock{{Type: api.BlockToolResult, ToolUseID: "c1", ToolContent: "ok"}}},
			{Role: api.RoleUser, Content: []api.ContentBlock{{Type: api.BlockText, Text: "next"}}},
		},
	}
	EnsurePromptCache(&req)
	if len(req.Messages[2].Content[0].CacheControl) == 0 {
		t.Fatal("tool_result prefix block should be marked")
	}
	req.TargetModel = "m"
	raw, err := openai.EncodeRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range got["messages"].([]any) {
		mm := m.(map[string]any)
		if mm["role"] == "tool" && mm["cache_control"] != nil {
			found = true
		}
	}
	if !found {
		t.Fatalf("tool message missing cache_control: %s", raw)
	}
}

func TestMaxTokensCap(t *testing.T) {
	req := api.Request{MaxTokens: 64000, Thinking: &api.ThinkingConfig{Type: "enabled", BudgetTokens: 8000}}
	ApplyCostPolicies(&req, config.Model{Reasoning: true, ThinkingPolicy: "passthrough"}, config.Routing{MaxTokensCap: 4096}, config.Provider{})
	if req.MaxTokens != 4096 {
		t.Fatalf("max_tokens=%d", req.MaxTokens)
	}
	if req.Thinking.BudgetTokens != 4096 {
		t.Fatalf("thinking budget should clamp to max_tokens: %+v", req.Thinking)
	}
}

func TestMaxTokensCapModelOverridesRouting(t *testing.T) {
	req := api.Request{MaxTokens: 64000}
	ApplyCostPolicies(&req, config.Model{MaxTokensCap: 1024}, config.Routing{MaxTokensCap: 4096}, config.Provider{})
	if req.MaxTokens != 1024 {
		t.Fatalf("max_tokens=%d", req.MaxTokens)
	}
}

func TestMaxTokensCapDoesNotRaise(t *testing.T) {
	req := api.Request{MaxTokens: 128}
	ApplyCostPolicies(&req, config.Model{}, config.Routing{MaxTokensCap: 4096}, config.Provider{})
	if req.MaxTokens != 128 {
		t.Fatalf("should not raise client max_tokens: %d", req.MaxTokens)
	}
}

func TestStickyPinPrependsOrder(t *testing.T) {
	ttrue := true
	req := api.Request{}
	ApplyCostPolicies(&req, config.Model{}, config.Routing{}, config.Provider{
		Sort: "price", Sticky: &ttrue, ProviderOrder: []string{"Google"},
	})
	ApplyStickyPin(&req, "Together")
	order, _ := req.UpstreamProvider["order"].([]string)
	if len(order) < 2 || order[0] != "Together" || order[1] != "Google" {
		t.Fatalf("order=%v prefs=%v", order, req.UpstreamProvider)
	}
	if req.UpstreamProvider["sort"] != "price" {
		t.Fatalf("sort should remain: %v", req.UpstreamProvider)
	}
}

func TestStickyPinDedupes(t *testing.T) {
	req := api.Request{UpstreamProvider: map[string]any{"order": []string{"Together", "Google"}}}
	ApplyStickyPin(&req, "Together")
	order := req.UpstreamProvider["order"].([]string)
	if len(order) != 2 || order[0] != "Together" || order[1] != "Google" {
		t.Fatalf("%v", order)
	}
}

