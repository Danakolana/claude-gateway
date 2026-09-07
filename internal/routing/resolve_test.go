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
