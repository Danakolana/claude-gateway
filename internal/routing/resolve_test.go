package routing

import (
	"testing"

	"github.com/danakolana/claude-gateway/internal/config"
	"github.com/danakolana/claude-gateway/pkg/api"
)

func TestExactAndCapabilityFilter(t *testing.T) {
	reg := NewRegistry(map[string]config.Model{
		"fast": {ModelID: "p/fast", TierAlias: "fast", Streaming: true, ToolCalls: true, Enabled: true, ContextLimit: 8000},
		"tiny": {ModelID: "p/tiny", TierAlias: "fast", Streaming: true, ToolCalls: false, Enabled: true, ContextLimit: 2048},
	})
	rc := config.Routing{Rules: []config.Rule{{Source: "premium", TargetModel: "tiny"}, {Source: "premium", TargetModel: "fast"}}}
	d, err := reg.Resolve("premium", "or", rc, api.Requirements{Tools: true}, 0)
	if err != nil || d.TargetModel != "p/fast" {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestContextOverflowReject(t *testing.T) {
	reg := NewRegistry(map[string]config.Model{
		"small": {ModelID: "p/s", Enabled: true, Streaming: true, ContextLimit: 4096},
	})
	rc := config.Routing{Rules: []config.Rule{{Source: "x", TargetModel: "small"}}}
	_, err := reg.Resolve("x", "or", rc, api.Requirements{}, 8000)
	if err == nil {
		t.Fatal("expected context overflow error")
	}
}

// A source without an explicit rule must still respect prefer_cheapest.
// Fallback/tier candidates must not be mistaken for explicit rules just
// because their candidate index happens to be below the rule count.
func TestFallbackRespectsPreferCheapestDespiteManyRules(t *testing.T) {
	for attempt := 0; attempt < 20; attempt++ {
		reg := NewRegistry(map[string]config.Model{
			"a": {ModelID: "p/a", TierAlias: "fast", Enabled: true, Streaming: true, ToolCalls: true, InputPrice: 5.0, OutputPrice: 20.0},
			"b": {ModelID: "p/b", TierAlias: "fast", Enabled: true, Streaming: true, ToolCalls: true, InputPrice: 0.1, OutputPrice: 0.4},
		})
		// Many rules, none matching source "unknown": mirror config.toml (29).
		rules := make([]config.Rule, 29)
		for i := range rules {
			rules[i] = config.Rule{Source: "claude-other", TargetModel: "a"}
		}
		rc := config.Routing{
			PreferCheapest: true,
			Rules:          rules,
			FallbackTiers:  []string{"fast"},
		}
		d, err := reg.Resolve("unknown", "or", rc, api.Requirements{Tools: true}, 0)
		if err != nil {
			t.Fatal(err)
		}
		// prefer_cheapest must always pick the cheaper tier member (b).
		if d.TargetModel != "p/b" {
			t.Fatalf("attempt %d: prefer_cheapest ignored, got %q (want p/b)", attempt, d.TargetModel)
		}
	}
}

// Explicit rule wins, but a later capable rule must win over a cheaper model.
func TestRuleOrderPreferredOverPrice(t *testing.T) {
	reg := NewRegistry(map[string]config.Model{
		"qwen":    {ModelID: "qwen/qwen3.8-flash", Enabled: true, Streaming: true, ToolCalls: true, InputPrice: 0.15, OutputPrice: 0.47},
		"mercury": {ModelID: "inception/mercury-2.5-preview", Enabled: true, Streaming: true, ToolCalls: true, InputPrice: 0.04, OutputPrice: 0.15},
	})
	rc := config.Routing{
		PreferCheapest: true,
		Rules: []config.Rule{
			{Source: "claude-haiku-4-7", TargetModel: "qwen"},
			{Source: "claude-haiku-4-7", TargetModel: "mercury"},
		},
	}
	d, err := reg.Resolve("claude-haiku-4-7", "or", rc, api.Requirements{Tools: true}, 0)
	if err != nil || d.TargetModel != "qwen/qwen3.8-flash" {
		t.Fatalf("first explicit rule should win: %+v %v", d, err)
	}
}

